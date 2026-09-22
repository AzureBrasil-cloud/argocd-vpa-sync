package domain

import (
	"fmt"
	"math/big"

	"k8s.io/apimachinery/pkg/api/resource"
)

// LimitSpec says how write-back sets one resource's limit alongside its new
// request. Exactly one field is set:
//   - HeadroomPercent: the new request becomes (100-p)% of the limit (see
//     LimitFromRequest), e.g. 20 -> limit = request / 0.8.
//   - Value: the limit is set to exactly this quantity, e.g. 512Mi.
type LimitSpec struct {
	HeadroomPercent *float64           `json:"headroomPercent,omitempty"`
	Value           *resource.Quantity `json:"value,omitempty"`
}

// Validate checks that exactly one mode is set and its value is acceptable
// for the resource (CPU or memory).
func (s LimitSpec) Validate(isCPU bool) error {
	switch {
	case s.HeadroomPercent != nil && s.Value != nil:
		return fmt.Errorf("set either headroomPercent or value, not both")
	case s.HeadroomPercent != nil:
		return ValidateLimitHeadroomPercent(*s.HeadroomPercent)
	case s.Value != nil:
		return ValidateLimitValue(*s.Value, isCPU)
	default:
		return fmt.Errorf("one of headroomPercent or value is required")
	}
}

// Limit returns the limit this spec asks for given the request actually
// written. An absolute Value below the request is an error: Kubernetes
// rejects a request greater than its limit.
func (s LimitSpec) Limit(request resource.Quantity, isCPU bool) (resource.Quantity, error) {
	if err := s.Validate(isCPU); err != nil {
		return resource.Quantity{}, err
	}
	if s.HeadroomPercent != nil {
		return LimitFromRequest(request, *s.HeadroomPercent, isCPU)
	}
	if s.Value.Cmp(request) < 0 {
		return resource.Quantity{}, fmt.Errorf("limit %s is below the request %s", s.Value.String(), request.String())
	}
	return s.Value.DeepCopy(), nil
}

// String is a canonical, order-stable form used for idempotency keys.
func (s LimitSpec) String() string {
	if s.HeadroomPercent != nil {
		return fmt.Sprintf("headroom=%v%%", *s.HeadroomPercent)
	}
	if s.Value != nil {
		return fmt.Sprintf("value=%d", s.Value.MilliValue())
	}
	return "<none>"
}

// ValidateLimitHeadroomPercent rejects a headroom outside [0, 100): at 100%
// the request would have to be 0% of the limit, i.e. an infinite limit.
func ValidateLimitHeadroomPercent(pct float64) error {
	if pct != pct || pct < 0 || pct >= 100 {
		return fmt.Errorf("limit headroom must be >= 0%% and < 100%%, got %v", pct)
	}
	return nil
}

// ValidateLimitValue rejects a non-positive limit and, for memory, a
// fractional byte count (e.g. "100m", which Kubernetes parses as 0.1 bytes).
func ValidateLimitValue(q resource.Quantity, isCPU bool) error {
	if q.Sign() <= 0 {
		return fmt.Errorf("limit must be greater than zero, got %s", q.String())
	}
	if !isCPU && q.MilliValue()%1000 != 0 {
		return fmt.Errorf("memory limit must be a whole number of bytes, got %s", q.String())
	}
	return nil
}

// LimitFromRequest returns the limit for which request is exactly
// (100-headroomPct)% -- i.e. request / (1 - headroomPct/100) -- rounded UP
// so the result is never below what the exact ratio asks for (and so never
// below request itself): to whole millicores for CPU, to whole bytes for
// memory. E.g. 305Mi at 20% -> 381.25Mi.
func LimitFromRequest(request resource.Quantity, headroomPct float64, isCPU bool) (resource.Quantity, error) {
	if err := ValidateLimitHeadroomPercent(headroomPct); err != nil {
		return resource.Quantity{}, err
	}

	// milli * 100 / (100 - pct), in exact rational arithmetic so an exact
	// ratio (e.g. 305Mi / 0.8) never picks up a float rounding error that
	// would push the ceiling one unit too high.
	denom := new(big.Rat).Sub(big.NewRat(100, 1), new(big.Rat).SetFloat64(headroomPct))
	limitMilli := new(big.Rat).SetInt64(request.MilliValue())
	limitMilli.Mul(limitMilli, big.NewRat(100, 1))
	limitMilli.Quo(limitMilli, denom)

	if isCPU {
		return *resource.NewMilliQuantity(ceilRat(limitMilli, 1), resource.DecimalSI), nil
	}
	return *resource.NewQuantity(ceilRat(limitMilli, 1000), resource.BinarySI), nil
}

// ceilRat returns ceil(r / unit) for a non-negative r.
func ceilRat(r *big.Rat, unit int64) int64 {
	scaled := new(big.Rat).Quo(r, big.NewRat(unit, 1))
	q, m := new(big.Int).QuoRem(scaled.Num(), scaled.Denom(), new(big.Int))
	if m.Sign() > 0 {
		q.Add(q, big.NewInt(1))
	}
	return q.Int64()
}
