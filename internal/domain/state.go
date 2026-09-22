package domain

import "time"

// OperationStatus is the lifecycle state of a write-back operation tracked
// in the StateStore.
type OperationStatus string

const (
	OperationPending  OperationStatus = "pending"
	OperationApplying OperationStatus = "applying"
	OperationApplied  OperationStatus = "applied"
	OperationFailed   OperationStatus = "failed"
	OperationConflict OperationStatus = "conflict"
)

// OperationState records everything the dashboard needs to show about one
// write-back attempt, keyed by its IdempotencyKey. It intentionally holds no
// credential material and no full file content -- only small operational
// metadata.
type OperationState struct {
	IdempotencyKey string          `json:"idempotencyKey"`
	VPANamespace   string          `json:"vpaNamespace"`
	VPAName        string          `json:"vpaName"`
	ContainerName  string          `json:"containerName"`
	Status         OperationStatus `json:"status"`

	RecommendationSummary ResourceAmount `json:"recommendationSummary"`

	Branch    string `json:"branch,omitempty"`
	CommitSHA string `json:"commitSha,omitempty"`
	PRURL     string `json:"prUrl,omitempty"`

	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`

	// ErrorMessage is truncated (see statestore/compaction.go) before being
	// stored, so it never grows the state document unbounded.
	ErrorMessage string `json:"errorMessage,omitempty"`
}

// PendingSelection records that a user has selected a recommendation for
// write-back but the operation has not yet been (or is still being) applied.
//
// Target/ApplyCPU/ApplyMemory/RecommendationSummary/Override*/CommitMessage
// (schema v2+) capture everything a later write-back attempt needs to build
// a gitwriteback.WriteBackRequest, so that processing a selection never has
// to re-resolve the binding or re-read live workload values -- what actually
// gets applied is exactly the eligibility verdict the dashboard showed the
// user at selection time, not whatever the live state happens to be by the
// time it's processed. A PendingSelection persisted before schema v2 has a
// zero-value Target (Target.RepoURL == "") and must be treated as
// unprocessable by anything that consumes it.
type PendingSelection struct {
	IdempotencyKey string    `json:"idempotencyKey"`
	VPANamespace   string    `json:"vpaNamespace"`
	VPAName        string    `json:"vpaName"`
	ContainerName  string    `json:"containerName"`
	SelectedAt     time.Time `json:"selectedAt"`
	SelectedBy     string    `json:"selectedBy,omitempty"`

	Target                WriteTarget     `json:"target"`
	ApplyCPU              bool            `json:"applyCPU,omitempty"`
	ApplyMemory           bool            `json:"applyMemory,omitempty"`
	RecommendationSummary ResourceAmount  `json:"recommendationSummary"`
	OverrideCPU           *ResourceAmount `json:"overrideCPU,omitempty"`
	OverrideMemory        *ResourceAmount `json:"overrideMemory,omitempty"`
	CommitMessage         string          `json:"commitMessage,omitempty"`
}

// StateDocument is the entire content of the argocd-vpa-updater-state
// Secret's "state.json" key.
type StateDocument struct {
	SchemaVersion     int                       `json:"schemaVersion"`
	UpdatedAt         time.Time                 `json:"updatedAt"`
	PendingSelections []PendingSelection        `json:"pendingSelections"`
	Operations        map[string]OperationState `json:"operations"`
}

// CurrentStateSchemaVersion is the schema version written by this build.
//
// v2 added PendingSelection.Target/ApplyCPU/ApplyMemory/RecommendationSummary
// (and the Override*/CommitMessage fields) so a write-back worker can build
// a request without re-resolving the binding. A v1 (or earlier, unversioned)
// PendingSelection decodes with a zero-value Target and must be treated as
// unprocessable by anything that consumes it (see PendingSelection's doc
// comment) -- there is no in-place rewrite of old documents on read, only
// this defensive check by consumers.
const CurrentStateSchemaVersion = 2

// NewEmptyStateDocument returns a valid, empty StateDocument at the current
// schema version.
func NewEmptyStateDocument() *StateDocument {
	return &StateDocument{
		SchemaVersion:     CurrentStateSchemaVersion,
		PendingSelections: []PendingSelection{},
		Operations:        map[string]OperationState{},
	}
}

// OperationKey identifies the (repo, branch, file, container) combination
// that must not have two concurrent pending operations at once, per the
// "impedir uma segunda operação enquanto existir uma operação pendente"
// rule. It is distinct from IdempotencyKey: IdempotencyKey changes whenever
// the recommended value changes, OperationKey does not.
type OperationKey struct {
	RepoURL       string
	Branch        string
	FilePath      string
	ContainerName string
}
