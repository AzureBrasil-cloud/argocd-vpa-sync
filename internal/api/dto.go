package api

import (
	"fmt"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/azurebrasil/argocd-vpa-updater/internal/domain"
)

// WorkloadDTO identifies the workload a recommendation targets.
type WorkloadDTO struct {
	Kind      string `json:"kind"`
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

// RecommendationDTO is one row of the dashboard's list view / the body of
// its detail view: a single container's VPA recommendation compared against
// the value currently requested by the live workload it targets (not Git
// -- see internal/workloadresources).
type RecommendationDTO struct {
	Cluster       string      `json:"cluster"`
	Namespace     string      `json:"namespace"`
	VPAName       string      `json:"vpaName"`
	Workload      WorkloadDTO `json:"workload"`
	ContainerName string      `json:"containerName"`
	UpdateMode    string      `json:"updateMode"`

	RecommendedCPU    string `json:"recommendedCpu,omitempty"`
	RecommendedMemory string `json:"recommendedMemory,omitempty"`
	LowerBoundCPU     string `json:"lowerBoundCpu,omitempty"`
	LowerBoundMemory  string `json:"lowerBoundMemory,omitempty"`
	UpperBoundCPU     string `json:"upperBoundCpu,omitempty"`
	UpperBoundMemory  string `json:"upperBoundMemory,omitempty"`

	CurrentCPU    string `json:"currentCpu,omitempty"`
	CurrentMemory string `json:"currentMemory,omitempty"`

	// CPURequestHeadroomPercent/MemoryRequestHeadroomPercent are the request
	// headroom last applied to this container (see domain.RequestHeadroom;
	// absent means none). TargetCPU/TargetMemory are the recommendation
	// plus that headroom -- what the live request is compared against, and
	// what the delta and limit fields below are computed from.
	CPURequestHeadroomPercent    *float64 `json:"cpuRequestHeadroomPercent,omitempty"`
	MemoryRequestHeadroomPercent *float64 `json:"memoryRequestHeadroomPercent,omitempty"`
	TargetCPU                    string   `json:"targetCpu,omitempty"`
	TargetMemory                 string   `json:"targetMemory,omitempty"`

	// BandLower*/BandUpper* are the VPA's lower/upper bounds with that same
	// headroom: a live request within them is not eligible (reason
	// within-vpa-bounds) however far the target has moved, so the dashboard
	// doesn't invite a new write-back on every recommendation wobble.
	BandLowerCPU    string `json:"bandLowerCpu,omitempty"`
	BandUpperCPU    string `json:"bandUpperCpu,omitempty"`
	BandLowerMemory string `json:"bandLowerMemory,omitempty"`
	BandUpperMemory string `json:"bandUpperMemory,omitempty"`

	// CurrentCPULimit/CurrentMemoryLimit are the live workload's limits
	// (empty when it declares none). CPULimitConfigured/
	// MemoryLimitConfigured say whether write-back has a limit key path to
	// keep in step with the request.
	CurrentCPULimit       string `json:"currentCpuLimit,omitempty"`
	CurrentMemoryLimit    string `json:"currentMemoryLimit,omitempty"`
	CPULimitConfigured    bool   `json:"cpuLimitConfigured"`
	MemoryLimitConfigured bool   `json:"memoryLimitConfigured"`

	// CPULimitRequired/MemoryLimitRequired say the recommendation exceeds
	// the live limit, so selecting that resource must also set a new limit
	// (Kubernetes rejects request > limit). Otherwise setting one is
	// optional and omitting it leaves the limit untouched.
	CPULimitRequired    bool `json:"cpuLimitRequired"`
	MemoryLimitRequired bool `json:"memoryLimitRequired"`

	// CPULimitExceeded/MemoryLimitExceeded say the recommendation exceeds
	// the live limit, whether or not write-back manages that limit. Exceeded
	// without a limit key path is a warning: write-back can't raise the
	// limit, so the new request would be rejected by Kubernetes.
	CPULimitExceeded    bool `json:"cpuLimitExceeded"`
	MemoryLimitExceeded bool `json:"memoryLimitExceeded"`

	DeltaCPUAbsoluteMilli    int64    `json:"deltaCpuAbsoluteMilli,omitempty"`
	DeltaCPUPercent          *float64 `json:"deltaCpuPercent,omitempty"`
	DeltaMemoryAbsoluteMilli int64    `json:"deltaMemoryAbsoluteMilli,omitempty"`
	DeltaMemoryPercent       *float64 `json:"deltaMemoryPercent,omitempty"`

	RecommendationAgeSeconds int64 `json:"recommendationAgeSeconds"`

	// Eligible/EligibilityReasons are the combined verdict (true if at least
	// one resource clears the bar) used for the list's single eligibility
	// badge. CPUConfigured/MemoryConfigured say whether the binding even
	// declares a key path for that resource (drives whether a selection
	// checkbox appears at all); CPUEligible/MemoryEligible are the
	// per-resource verdict (drives whether that checkbox is enabled).
	Eligible           bool     `json:"eligible"`
	EligibilityReasons []string `json:"eligibilityReasons,omitempty"`

	CPUConfigured         bool     `json:"cpuConfigured"`
	CPUEligible           bool     `json:"cpuEligible"`
	CPUEligibilityReasons []string `json:"cpuEligibilityReasons,omitempty"`

	MemoryConfigured         bool     `json:"memoryConfigured"`
	MemoryEligible           bool     `json:"memoryEligible"`
	MemoryEligibilityReasons []string `json:"memoryEligibilityReasons,omitempty"`

	// Status is one of: new, selected, applying, applied, skipped, failed.
	Status string `json:"status"`

	// CurrentValueError explains why CurrentCPU/CurrentMemory (and delta)
	// could not be computed -- e.g. a resolver or live workload read error
	// -- surfaced verbatim rather than silently omitted.
	CurrentValueError string `json:"currentValueError,omitempty"`

	// Warnings carries non-fatal issues, such as a repo-url mismatch against
	// the source Argo CD Application.
	Warnings []string `json:"warnings,omitempty"`

	// ValidationErrors is set instead of the above when the VPA's own
	// annotations failed validation (Valid=false); no target could even be
	// resolved.
	ValidationErrors []string `json:"validationErrors,omitempty"`

	// Operation carries branch/commit/error detail for the most recent
	// write-back attempt, once one exists (Status has moved past "new" or
	// "selected"). Nil when nothing has been applied/attempted yet.
	Operation *OperationDTO `json:"operation,omitempty"`
}

// OperationDTO is the write-back detail surfaced for one recommendation: the
// branch/commit a successful apply landed on, or the error a failed/conflict
// one recorded.
type OperationDTO struct {
	Branch       string    `json:"branch,omitempty"`
	CommitSHA    string    `json:"commitSha,omitempty"`
	PRURL        string    `json:"prUrl,omitempty"`
	ErrorMessage string    `json:"errorMessage,omitempty"`
	UpdatedAt    time.Time `json:"updatedAt"`
}

// ListRecommendationsResponse is the body of GET /api/v1/recommendations.
type ListRecommendationsResponse struct {
	Items []RecommendationDTO `json:"items"`
}

// SelectRequest is the body of POST .../select and POST .../bulk-select.
type SelectRequest struct {
	ApplyCPU    bool `json:"applyCPU"`
	ApplyMemory bool `json:"applyMemory"`

	// CPULimit / MemoryLimit, when set, also rewrite that resource's limit
	// (only if the resource itself is applied). Omitted leaves it untouched.
	CPULimit    *LimitSpecDTO `json:"cpuLimit,omitempty"`
	MemoryLimit *LimitSpecDTO `json:"memoryLimit,omitempty"`

	// CPURequestHeadroomPercent / MemoryRequestHeadroomPercent, when set,
	// write the request as recommendation / (1 - p/100), p in [0, 100), so
	// the recommendation is (100-p)% of it. Omitted (or 0) writes the bare
	// recommendation.
	CPURequestHeadroomPercent    *float64 `json:"cpuRequestHeadroomPercent,omitempty"`
	MemoryRequestHeadroomPercent *float64 `json:"memoryRequestHeadroomPercent,omitempty"`
}

// LimitSpecDTO is the wire form of domain.LimitSpec: exactly one of
// HeadroomPercent (the new request becomes (100-p)% of the limit, p in
// [0, 100)) or Value (an absolute Kubernetes quantity, e.g. "512Mi" or
// "500m").
type LimitSpecDTO struct {
	HeadroomPercent *float64 `json:"headroomPercent,omitempty"`
	Value           string   `json:"value,omitempty"`
}

func (d *LimitSpecDTO) toDomain() (*domain.LimitSpec, error) {
	if d == nil {
		return nil, nil
	}
	spec := &domain.LimitSpec{HeadroomPercent: d.HeadroomPercent}
	if d.Value != "" {
		q, err := resource.ParseQuantity(d.Value)
		if err != nil {
			return nil, fmt.Errorf("invalid quantity %q: %w", d.Value, err)
		}
		spec.Value = &q
	}
	return spec, nil
}

func (r SelectRequest) options() (SelectOptions, error) {
	cpu, err := r.CPULimit.toDomain()
	if err != nil {
		return SelectOptions{}, fmt.Errorf("%w: cpu limit: %v", ErrInvalidSelectRequest, err)
	}
	memory, err := r.MemoryLimit.toDomain()
	if err != nil {
		return SelectOptions{}, fmt.Errorf("%w: memory limit: %v", ErrInvalidSelectRequest, err)
	}
	return SelectOptions{
		ApplyCPU:              r.ApplyCPU,
		ApplyMemory:           r.ApplyMemory,
		CPULimit:              cpu,
		MemoryLimit:           memory,
		CPURequestHeadroom:    r.CPURequestHeadroomPercent,
		MemoryRequestHeadroom: r.MemoryRequestHeadroomPercent,
	}, nil
}

// SkippedSelectionDTO explains why one container was skipped by a bulk
// selection instead of aborting the whole batch.
type SkippedSelectionDTO struct {
	Namespace     string `json:"namespace"`
	VPAName       string `json:"vpaName"`
	ContainerName string `json:"containerName"`
	Reason        string `json:"reason"`
}

// BulkSelectResponse is the body of POST /api/v1/recommendations/bulk-select.
type BulkSelectResponse struct {
	Selected []domain.PendingSelection `json:"selected"`
	Skipped  []SkippedSelectionDTO     `json:"skipped"`
}
