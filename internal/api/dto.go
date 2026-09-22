package api

import (
	"time"

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
