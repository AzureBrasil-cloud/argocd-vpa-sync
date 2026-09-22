// Package vpaapi defines the minimal subset of the upstream
// VerticalPodAutoscaler API (autoscaling.k8s.io/v1, from
// kubernetes/autoscaler) that argocd-vpa-updater needs: enough of spec and
// status to read a recommendation and identify its target workload. It is
// hand-written rather than importing k8s.io/autoscaler/vertical-pod-autoscaler
// to avoid pulling that module's full (and fast-moving) dependency tree for
// a handful of read-only fields; the JSON/wire shape matches upstream, so
// this decodes real cluster objects unchanged.
package vpaapi

import (
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
)

// GroupVersion is group version used by the VerticalPodAutoscaler type.
var GroupVersion = schema.GroupVersion{Group: "autoscaling.k8s.io", Version: "v1"}

// SchemeBuilder and AddToScheme let callers register this type the same way
// as any other controller-runtime-managed API.
var (
	SchemeBuilder = runtime.NewSchemeBuilder(addKnownTypes)
	AddToScheme   = SchemeBuilder.AddToScheme
)

func addKnownTypes(scheme *runtime.Scheme) error {
	scheme.AddKnownTypes(GroupVersion,
		&VerticalPodAutoscaler{},
		&VerticalPodAutoscalerList{},
	)
	metav1.AddToGroupVersion(scheme, GroupVersion)
	return nil
}

// UpdateMode mirrors the upstream enum; argocd-vpa-updater only ever expects
// UpdateModeOff, but decodes the others for validation/warning purposes.
type UpdateMode string

const (
	UpdateModeOff      UpdateMode = "Off"
	UpdateModeInitial  UpdateMode = "Initial"
	UpdateModeRecreate UpdateMode = "Recreate"
	UpdateModeAuto     UpdateMode = "Auto"
)

// CrossVersionObjectReference identifies the workload a VPA targets.
type CrossVersionObjectReference struct {
	Kind       string `json:"kind"`
	Name       string `json:"name"`
	APIVersion string `json:"apiVersion,omitempty"`
}

type PodUpdatePolicy struct {
	UpdateMode *UpdateMode `json:"updateMode,omitempty"`
}

type VerticalPodAutoscalerSpec struct {
	TargetRef    *CrossVersionObjectReference `json:"targetRef,omitempty"`
	UpdatePolicy *PodUpdatePolicy             `json:"updatePolicy,omitempty"`
}

// RecommendedContainerResources is one entry of
// status.recommendation.containerRecommendations.
type RecommendedContainerResources struct {
	ContainerName  string              `json:"containerName,omitempty"`
	Target         corev1.ResourceList `json:"target,omitempty"`
	LowerBound     corev1.ResourceList `json:"lowerBound,omitempty"`
	UpperBound     corev1.ResourceList `json:"upperBound,omitempty"`
	UncappedTarget corev1.ResourceList `json:"uncappedTarget,omitempty"`
}

type RecommendedPodResources struct {
	ContainerRecommendations []RecommendedContainerResources `json:"containerRecommendations,omitempty"`
}

// VerticalPodAutoscalerConditionType mirrors the upstream enum; only
// RecommendationProvided is consumed today.
type VerticalPodAutoscalerConditionType string

const (
	RecommendationProvided VerticalPodAutoscalerConditionType = "RecommendationProvided"
)

type VerticalPodAutoscalerCondition struct {
	Type               VerticalPodAutoscalerConditionType `json:"type"`
	Status             corev1.ConditionStatus             `json:"status"`
	LastTransitionTime metav1.Time                        `json:"lastTransitionTime,omitempty"`
	Reason             string                             `json:"reason,omitempty"`
	Message            string                             `json:"message,omitempty"`
}

type VerticalPodAutoscalerStatus struct {
	Recommendation *RecommendedPodResources         `json:"recommendation,omitempty"`
	Conditions     []VerticalPodAutoscalerCondition `json:"conditions,omitempty"`
}

// +kubebuilder:object:root=true

// VerticalPodAutoscaler mirrors autoscaling.k8s.io/v1's VerticalPodAutoscaler,
// restricted to the fields argocd-vpa-updater reads.
type VerticalPodAutoscaler struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   VerticalPodAutoscalerSpec   `json:"spec,omitempty"`
	Status VerticalPodAutoscalerStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// VerticalPodAutoscalerList is a list of VerticalPodAutoscaler.
type VerticalPodAutoscalerList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`

	Items []VerticalPodAutoscaler `json:"items"`
}
