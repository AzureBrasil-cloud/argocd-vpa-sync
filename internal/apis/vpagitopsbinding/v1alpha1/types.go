package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// SourceType identifies the shape of the file a recommendation is written
// into. Mirrors domain.SourceType; duplicated here (rather than importing
// internal/domain) so this API package stays free of any dependency on the
// rest of the module, matching the discipline internal/vpaapi follows.
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

// Ready condition reasons, surfaced via status.conditions.
const (
	ConditionTypeReady = "Ready"

	ReasonValidationFailed = "ValidationFailed"
	ReasonVpaNotFound      = "VpaNotFound"
	ReasonReady            = "Ready"
)

// VpaReference names the VerticalPodAutoscaler this binding configures. It
// is always resolved in the binding's own namespace: VpaGitOpsBinding does
// not support cross-namespace VPA references.
type VpaReference struct {
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`
}

// ArgoCDApplicationReference identifies the source Argo CD Application used
// only to cross-check RepoURL (see resolver.ArgoCDApplicationRepoURLGetter).
// A mismatch is a warning, never an error, and never substitutes RepoURL.
type ArgoCDApplicationReference struct {
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// Namespace defaults to the controller's configured Argo CD namespace
	// when empty.
	// +optional
	Namespace string `json:"namespace,omitempty"`
}

// ContainerWriteBackConfig configures write-back for one container of the
// VPA's target workload.
type ContainerWriteBackConfig struct {
	// Name is the container name, matched against
	// status.recommendation.containerRecommendations[].containerName on the
	// referenced VPA.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	Name string `json:"name"`

	// +kubebuilder:validation:Required
	// +kubebuilder:validation:Enum=helm-values;kustomize-patch;yaml
	ManifestType SourceType `json:"manifestType"`

	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	ManifestPath string `json:"manifestPath"`

	// CPUKeyPath and MemoryKeyPath are each individually optional -- a
	// container may opt to write back only one resource -- but at least one
	// of the two must be set. That cross-field rule cannot be expressed as a
	// single CRD marker, so it is enforced defensively at reconcile time.
	// +optional
	CPUKeyPath string `json:"cpuKeyPath,omitempty"`
	// +optional
	MemoryKeyPath string `json:"memoryKeyPath,omitempty"`

	// CPULimitKeyPath and MemoryLimitKeyPath locate the resources.limits
	// value kept in step with the written request (the dashboard asks for a
	// headroom percentage or an absolute value). They are never inferred
	// from cpuKeyPath/memoryKeyPath: when omitted, write-back updates that
	// resource's request only and leaves its limit untouched. A limit key
	// that doesn't exist in the file is left absent, never created.
	// +optional
	CPULimitKeyPath string `json:"cpuLimitKeyPath,omitempty"`
	// +optional
	MemoryLimitKeyPath string `json:"memoryLimitKeyPath,omitempty"`

	// MinChangePercent overrides VpaGitOpsBindingSpec.MinChangePercent for
	// this container only.
	// +optional
	// +kubebuilder:validation:Minimum=0
	MinChangePercent *float64 `json:"minChangePercent,omitempty"`
}

// VpaGitOpsBindingSpec configures GitOps write-back for one
// VerticalPodAutoscaler's recommendations. The existence of a
// VpaGitOpsBinding is itself the opt-in signal for the VPA it references.
type VpaGitOpsBindingSpec struct {
	// +kubebuilder:validation:Required
	VpaRef VpaReference `json:"vpaRef"`

	// +optional
	ArgoCDApplicationRef *ArgoCDApplicationReference `json:"argoCDApplicationRef,omitempty"`

	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	RepoURL string `json:"repoURL"`

	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinLength=1
	RepoBranch string `json:"repoBranch"`

	// +optional
	// +kubebuilder:validation:Enum=commit;pull-request
	// +kubebuilder:default=commit
	WriteBackPolicy WriteBackPolicy `json:"writeBackPolicy,omitempty"`

	// MinChangePercent is the default eligibility threshold for every
	// container below, unless a container overrides it.
	// +optional
	// +kubebuilder:validation:Minimum=0
	// +kubebuilder:default=10
	MinChangePercent float64 `json:"minChangePercent,omitempty"`

	// Containers configures write-back for one or more containers of the
	// VPA's target workload.
	// +kubebuilder:validation:Required
	// +kubebuilder:validation:MinItems=1
	Containers []ContainerWriteBackConfig `json:"containers"`
}

// VpaGitOpsBindingStatus surfaces validation state natively, mirroring
// domain.NormalizedVPA.ValidationErrors for kubectl users.
type VpaGitOpsBindingStatus struct {
	// +optional
	// +patchStrategy=merge
	// +patchMergeKey=type
	Conditions []metav1.Condition `json:"conditions,omitempty" patchStrategy:"merge" patchMergeKey:"type"`

	// +optional
	ObservedGeneration int64 `json:"observedGeneration,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status
// +kubebuilder:resource:scope=Namespaced,shortName=vgb
// +kubebuilder:printcolumn:name="VPA",type=string,JSONPath=`.spec.vpaRef.name`
// +kubebuilder:printcolumn:name="Ready",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].status`
// +kubebuilder:printcolumn:name="Reason",type=string,JSONPath=`.status.conditions[?(@.type=="Ready")].reason`
// +kubebuilder:printcolumn:name="Age",type=date,JSONPath=`.metadata.creationTimestamp`

// VpaGitOpsBinding configures GitOps write-back for one
// VerticalPodAutoscaler's recommendations.
type VpaGitOpsBinding struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VpaGitOpsBindingSpec   `json:"spec"`
	Status VpaGitOpsBindingStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// VpaGitOpsBindingList is a list of VpaGitOpsBinding.
type VpaGitOpsBindingList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []VpaGitOpsBinding `json:"items"`
}

func init() {
	SchemeBuilder.Register(&VpaGitOpsBinding{}, &VpaGitOpsBindingList{})
}
