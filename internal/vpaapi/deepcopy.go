package vpaapi

import (
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
)

// The methods below are hand-written equivalents of what
// controller-gen/deepcopy-gen would normally produce for these types. They
// exist only so VerticalPodAutoscaler(/List) satisfy runtime.Object, which
// controller-runtime's client and cache require.

func (in *CrossVersionObjectReference) DeepCopy() *CrossVersionObjectReference {
	if in == nil {
		return nil
	}
	out := *in
	return &out
}

func (in *PodUpdatePolicy) DeepCopy() *PodUpdatePolicy {
	if in == nil {
		return nil
	}
	out := new(PodUpdatePolicy)
	if in.UpdateMode != nil {
		m := *in.UpdateMode
		out.UpdateMode = &m
	}
	return out
}

func (in *VerticalPodAutoscalerSpec) DeepCopyInto(out *VerticalPodAutoscalerSpec) {
	*out = *in
	out.TargetRef = in.TargetRef.DeepCopy()
	out.UpdatePolicy = in.UpdatePolicy.DeepCopy()
}

func deepCopyResourceList(in corev1.ResourceList) corev1.ResourceList {
	if in == nil {
		return nil
	}
	out := make(corev1.ResourceList, len(in))
	for k, v := range in {
		out[k] = v.DeepCopy()
	}
	return out
}

func (in *RecommendedContainerResources) DeepCopy() RecommendedContainerResources {
	out := *in
	out.Target = deepCopyResourceList(in.Target)
	out.LowerBound = deepCopyResourceList(in.LowerBound)
	out.UpperBound = deepCopyResourceList(in.UpperBound)
	out.UncappedTarget = deepCopyResourceList(in.UncappedTarget)
	return out
}

func (in *RecommendedPodResources) DeepCopy() *RecommendedPodResources {
	if in == nil {
		return nil
	}
	out := &RecommendedPodResources{}
	if in.ContainerRecommendations != nil {
		out.ContainerRecommendations = make([]RecommendedContainerResources, len(in.ContainerRecommendations))
		for i := range in.ContainerRecommendations {
			out.ContainerRecommendations[i] = in.ContainerRecommendations[i].DeepCopy()
		}
	}
	return out
}

func (in *VerticalPodAutoscalerStatus) DeepCopyInto(out *VerticalPodAutoscalerStatus) {
	*out = *in
	out.Recommendation = in.Recommendation.DeepCopy()
	if in.Conditions != nil {
		out.Conditions = make([]VerticalPodAutoscalerCondition, len(in.Conditions))
		copy(out.Conditions, in.Conditions)
	}
}

// DeepCopyInto copies the receiver into out.
func (in *VerticalPodAutoscaler) DeepCopyInto(out *VerticalPodAutoscaler) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ObjectMeta.DeepCopyInto(&out.ObjectMeta)
	in.Spec.DeepCopyInto(&out.Spec)
	in.Status.DeepCopyInto(&out.Status)
}

// DeepCopy returns a deep copy of the receiver.
func (in *VerticalPodAutoscaler) DeepCopy() *VerticalPodAutoscaler {
	if in == nil {
		return nil
	}
	out := new(VerticalPodAutoscaler)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject implements runtime.Object.
func (in *VerticalPodAutoscaler) DeepCopyObject() runtime.Object {
	return in.DeepCopy()
}

// DeepCopyInto copies the receiver into out.
func (in *VerticalPodAutoscalerList) DeepCopyInto(out *VerticalPodAutoscalerList) {
	*out = *in
	out.TypeMeta = in.TypeMeta
	in.ListMeta.DeepCopyInto(&out.ListMeta)
	if in.Items != nil {
		out.Items = make([]VerticalPodAutoscaler, len(in.Items))
		for i := range in.Items {
			in.Items[i].DeepCopyInto(&out.Items[i])
		}
	}
}

// DeepCopy returns a deep copy of the receiver.
func (in *VerticalPodAutoscalerList) DeepCopy() *VerticalPodAutoscalerList {
	if in == nil {
		return nil
	}
	out := new(VerticalPodAutoscalerList)
	in.DeepCopyInto(out)
	return out
}

// DeepCopyObject implements runtime.Object.
func (in *VerticalPodAutoscalerList) DeepCopyObject() runtime.Object {
	return in.DeepCopy()
}
