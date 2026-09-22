package domain

// SourceType identifies the shape of the file a recommendation is written
// into.
type SourceType string

const (
	SourceTypeHelmValues     SourceType = "helm-values"
	SourceTypeKustomizePatch SourceType = "kustomize-patch"
	SourceTypeYAML           SourceType = "yaml"
)

// WriteBackPolicy identifies whether a write-back should land as a direct
// commit or go through a pull/merge request.
type WriteBackPolicy string

const (
	WriteBackCommit      WriteBackPolicy = "commit"
	WriteBackPullRequest WriteBackPolicy = "pull-request"
)

// DefaultMinChangePercent is used when neither a binding nor a container
// declares its own MinChangePercent.
const DefaultMinChangePercent = 10.0

// ContainerWriteBackConfig configures write-back for one container of a
// VPA's target workload. This is the CRD-package-agnostic mirror of
// v1alpha1.ContainerWriteBackConfig (internal/apis/vpagitopsbinding/v1alpha1),
// built by internal/controller from a VpaGitOpsBinding CR.
type ContainerWriteBackConfig struct {
	ContainerName string
	ManifestType  SourceType
	ManifestPath  string
	CPUKeyPath    string
	MemoryKeyPath string

	// CPULimitKeyPath / MemoryLimitKeyPath locate the matching limit; empty
	// means write-back never touches that limit (they're never inferred).
	CPULimitKeyPath    string
	MemoryLimitKeyPath string

	MinChangePercent *float64
}

// GitOpsBindingSpec is the CRD-package-agnostic mirror of
// v1alpha1.VpaGitOpsBindingSpec: everything argocd-vpa-updater needs to know
// about how to write a VPA's recommendation back to Git. It is the
// replacement for the old annotation-derived VPAAnnotations -- same
// semantics, now built from a VpaGitOpsBinding CR instead of parsed from a
// map[string]string of annotations.
type GitOpsBindingSpec struct {
	ArgoCDApplication          string
	ArgoCDApplicationNamespace string
	RepoURL                    string
	RepoBranch                 string
	WriteBackPolicy            WriteBackPolicy
	MinChangePercent           float64
	Containers                 []ContainerWriteBackConfig
}

// ContainerByName returns the write-back config for the named container, if
// declared.
func (s GitOpsBindingSpec) ContainerByName(name string) (ContainerWriteBackConfig, bool) {
	for _, c := range s.Containers {
		if c.ContainerName == name {
			return c, true
		}
	}
	return ContainerWriteBackConfig{}, false
}

// EffectiveMinChangePercent resolves the eligibility threshold for a
// container: its own override if set, else the binding-level default, else
// DefaultMinChangePercent.
func (s GitOpsBindingSpec) EffectiveMinChangePercent(containerName string) float64 {
	if c, ok := s.ContainerByName(containerName); ok && c.MinChangePercent != nil {
		return *c.MinChangePercent
	}
	if s.MinChangePercent > 0 {
		return s.MinChangePercent
	}
	return DefaultMinChangePercent
}
