// Package eligibility computes the delta between a VPA recommendation and
// the value currently requested by the live workload it targets, and
// decides whether that delta is large enough, and safe enough, to be
// offered to the user as an eligible change.
package eligibility

import (
	"math"

	"k8s.io/apimachinery/pkg/api/resource"
)

// Delta describes the difference between a current (live workload) value
// and a recommended (VPA) value for a single resource (CPU or memory).
type Delta struct {
	HasCurrent     bool
	HasRecommended bool

	Current     *resource.Quantity
	Recommended *resource.Quantity

	// AbsoluteMilli is Recommended.MilliValue() - Current.MilliValue(). It is
	// zero when either side is absent.
	AbsoluteMilli int64

	// PercentChange is ((recommended-current)/current)*100. When Current is
	// zero but Recommended is not, PercentChange is +Inf (any increase from
	// zero is an infinite relative change) and callers should fall back to
	// treating the change as eligible on absolute-value grounds alone. When
	// both are zero, PercentChange is 0.
	PercentChange float64
}

// ComputeDelta computes the Delta between a current and a recommended
// quantity. Either pointer may be nil to represent "not declared".
func ComputeDelta(current, recommended *resource.Quantity) Delta {
	d := Delta{
		HasCurrent:     current != nil,
		HasRecommended: recommended != nil,
		Current:        current,
		Recommended:    recommended,
	}

	var curMilli, recMilli int64
	if current != nil {
		curMilli = current.MilliValue()
	}
	if recommended != nil {
		recMilli = recommended.MilliValue()
	}

	if !d.HasCurrent || !d.HasRecommended {
		return d
	}

	d.AbsoluteMilli = recMilli - curMilli

	switch {
	case curMilli == 0 && recMilli == 0:
		d.PercentChange = 0
	case curMilli == 0:
		d.PercentChange = math.Inf(1) * sign(recMilli)
	default:
		d.PercentChange = (float64(recMilli-curMilli) / math.Abs(float64(curMilli))) * 100
	}

	return d
}

func sign(v int64) float64 {
	if v < 0 {
		return -1
	}
	return 1
}

// IsZero reports whether the delta represents no change at all (both sides
// present and numerically equal).
func (d Delta) IsZero() bool {
	return d.HasCurrent && d.HasRecommended && d.AbsoluteMilli == 0
}

// MeetsThreshold reports whether the delta's magnitude is at or above
// minChangePercent. A delta missing either side never meets a threshold
// (there is nothing to compare). An infinite percent change (current==0,
// recommended!=0) always meets any finite threshold.
func (d Delta) MeetsThreshold(minChangePercent float64) bool {
	if !d.HasCurrent || !d.HasRecommended {
		return false
	}
	if math.IsInf(d.PercentChange, 0) {
		return d.AbsoluteMilli != 0
	}
	return math.Abs(d.PercentChange) >= minChangePercent
}

// Reason codes returned in Result.Reasons, kept as constants so API/dashboard
// code and tests can match on them without string-typo risk.
const (
	ReasonNoCurrentValue     = "no-current-value"
	ReasonNoRecommendation   = "no-recommendation"
	ReasonBelowThreshold     = "below-minimum-change-threshold"
	ReasonContainerExcluded  = "container-excluded"
	ReasonBelowMinAllowed    = "recommendation-below-min-allowed"
	ReasonAboveMaxAllowed    = "recommendation-above-max-allowed"
	ReasonNoResourceSelected = "no-resource-selected"
)

// Input bundles everything needed to decide eligibility for one container's
// CPU and/or memory recommendation.
type Input struct {
	CPU    Delta
	Memory Delta

	ApplyCPU    bool
	ApplyMemory bool

	MinChangePercent float64

	ContainerExcluded bool

	// MinAllowed/MaxAllowed mirror the VPA's own spec.resourcePolicy bounds,
	// when present; a recommendation outside them is not eligible because
	// the VPA itself would not consider it a valid recommendation.
	CPUMinAllowed, CPUMaxAllowed       *resource.Quantity
	MemoryMinAllowed, MemoryMaxAllowed *resource.Quantity
}

// ResourceEligibility is the per-resource breakdown of an eligibility
// decision -- e.g. what drives a checkbox in the dashboard being hidden
// (Requested false), disabled with a tooltip (Requested true, Eligible
// false), or checkable (Requested true, Eligible true).
type ResourceEligibility struct {
	// Requested mirrors Input.ApplyCPU/ApplyMemory: whether this resource
	// was even asked about. False means the other fields are meaningless
	// (the resource wasn't evaluated at all).
	Requested bool
	Eligible  bool
	Reasons   []string
}

// Result is the outcome of Evaluate. Eligible/Reasons are the combined
// verdict (true if at least one requested resource is eligible) used for
// the dashboard's single eligibility badge; CPU/Memory carry the same
// verdict broken out per resource, used to drive per-resource selection
// controls.
type Result struct {
	Eligible bool
	Reasons  []string

	CPU    ResourceEligibility
	Memory ResourceEligibility
}

// Evaluate decides whether at least one of the requested resources (CPU
// and/or memory, per ApplyCPU/ApplyMemory) is eligible for write-back.
// Reasons lists every reason a requested resource was rejected; it is empty
// when Eligible is true.
func Evaluate(in Input) Result {
	if in.ContainerExcluded {
		return Result{Eligible: false, Reasons: []string{ReasonContainerExcluded}}
	}
	if !in.ApplyCPU && !in.ApplyMemory {
		return Result{Eligible: false, Reasons: []string{ReasonNoResourceSelected}}
	}

	var reasons []string
	eligible := false
	result := Result{}

	if in.ApplyCPU {
		ok, r := evaluateOne(in.CPU, in.MinChangePercent, in.CPUMinAllowed, in.CPUMaxAllowed)
		result.CPU = ResourceEligibility{Requested: true, Eligible: ok, Reasons: r}
		if ok {
			eligible = true
		} else {
			reasons = append(reasons, r...)
		}
	}
	if in.ApplyMemory {
		ok, r := evaluateOne(in.Memory, in.MinChangePercent, in.MemoryMinAllowed, in.MemoryMaxAllowed)
		result.Memory = ResourceEligibility{Requested: true, Eligible: ok, Reasons: r}
		if ok {
			eligible = true
		} else {
			reasons = append(reasons, r...)
		}
	}

	result.Eligible = eligible
	if !eligible {
		result.Reasons = reasons
	}
	return result
}

func evaluateOne(d Delta, minChangePercent float64, minAllowed, maxAllowed *resource.Quantity) (bool, []string) {
	if !d.HasCurrent {
		return false, []string{ReasonNoCurrentValue}
	}
	if !d.HasRecommended {
		return false, []string{ReasonNoRecommendation}
	}
	if minAllowed != nil && d.Recommended.MilliValue() < minAllowed.MilliValue() {
		return false, []string{ReasonBelowMinAllowed}
	}
	if maxAllowed != nil && d.Recommended.MilliValue() > maxAllowed.MilliValue() {
		return false, []string{ReasonAboveMaxAllowed}
	}
	if !d.MeetsThreshold(minChangePercent) {
		return false, []string{ReasonBelowThreshold}
	}
	return true, nil
}
