package domain

import "k8s.io/apimachinery/pkg/api/resource"

// WriteTarget is the fully-resolved description of where and how a single
// container's recommendation should be written back to Git. It is produced
// by a GitOpsTargetResolver from a NormalizedVPA + container name, and
// consumed by a ManifestPatcher and a GitWriteBackService.
type WriteTarget struct {
	RepoURL         string
	Branch          string
	FilePath        string
	SourceType      SourceType
	CPUKeyPath      string
	MemoryKeyPath   string
	WriteBackPolicy WriteBackPolicy
	Workload        WorkloadRef
	ContainerName   string

	// Warnings holds non-fatal issues found while resolving this target
	// (e.g. the declared RepoURL does not match the source Argo CD
	// Application's spec.source.repoURL). They are surfaced to the
	// dashboard but never block resolution.
	Warnings []string
}

// PatchRequest is what a ManifestPatcher needs to produce a new version of a
// file's contents.
type PatchRequest struct {
	Target         WriteTarget
	Recommendation ContainerRecommendation

	// ApplyCPU / ApplyMemory let a caller apply only one of the two
	// resources even when the target declares key paths for both (the
	// dashboard lets a user pick CPU, memory, or both).
	ApplyCPU    bool
	ApplyMemory bool

	// Override, when non-nil, replaces the recommended value for the
	// resource it applies to (the dashboard allows editing the proposed
	// value before commit).
	OverrideCPU    *ResourceAmount
	OverrideMemory *ResourceAmount

	IdempotencyKey string
}

// EffectiveCPU returns the CPU value that should actually be written:
// OverrideCPU when the user edited the proposed value, otherwise the raw VPA
// recommendation.
func (r PatchRequest) EffectiveCPU() *resource.Quantity {
	if r.OverrideCPU != nil {
		return r.OverrideCPU.CPU
	}
	return r.Recommendation.Target.CPU
}

// EffectiveMemory is the memory equivalent of EffectiveCPU.
func (r PatchRequest) EffectiveMemory() *resource.Quantity {
	if r.OverrideMemory != nil {
		return r.OverrideMemory.Memory
	}
	return r.Recommendation.Target.Memory
}

// PatchResult describes the outcome of a ManifestPatcher.Patch call.
type PatchResult struct {
	// Changed is false when the file's current values already match the
	// requested values -- the patcher performed no-op work and the caller
	// should not create a commit.
	Changed bool

	// NewContentSHA256 is the hex-encoded SHA-256 of the returned file
	// content, used for idempotency/audit purposes.
	NewContentSHA256 string

	// Diff is a unified diff between the original and patched content, used
	// for the dashboard preview and the commit message body.
	Diff string
}
