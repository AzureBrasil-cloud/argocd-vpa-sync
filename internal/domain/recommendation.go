// Package domain holds the core data model shared by every package in
// argocd-vpa-updater. It has no dependency on Kubernetes client libraries,
// git libraries, or HTTP -- only on k8s.io/apimachinery's resource.Quantity,
// which is the natural type for CPU/memory values.
package domain

import (
	"time"

	"k8s.io/apimachinery/pkg/api/resource"
)

// ResourceAmount holds a CPU and/or memory quantity. Either field may be nil
// when the source (VPA recommendation, Git manifest) does not declare that
// resource.
type ResourceAmount struct {
	CPU    *resource.Quantity `json:"cpu,omitempty"`
	Memory *resource.Quantity `json:"memory,omitempty"`
}

// WorkloadRef identifies the workload a VPA targets.
type WorkloadRef struct {
	Kind      string `json:"kind"` // Deployment | StatefulSet | CronJob
	Name      string `json:"name"`
	Namespace string `json:"namespace"`
}

// ContainerRecommendation is one entry of
// status.recommendation.containerRecommendations, normalized.
type ContainerRecommendation struct {
	ContainerName  string         `json:"containerName"`
	Target         ResourceAmount `json:"target"`
	LowerBound     ResourceAmount `json:"lowerBound"`
	UpperBound     ResourceAmount `json:"upperBound"`
	UncappedTarget ResourceAmount `json:"uncappedTarget"`
}

// NormalizedVPA is the controller's normalized, validated view of one
// opted-in VerticalPodAutoscaler object.
type NormalizedVPA struct {
	// Cluster is reserved for future multi-cluster support; today it is
	// always "in-cluster".
	Cluster   string `json:"cluster"`
	Namespace string `json:"namespace"`
	Name      string `json:"name"`

	// UpdateMode is copied from spec.updatePolicy.updateMode. Only "Off" is
	// expected; any other value produces a ValidationErrors entry, since
	// this controller assumes the VPA never mutates Pods itself.
	UpdateMode string `json:"updateMode"`

	Workload   WorkloadRef               `json:"workload"`
	Binding    GitOpsBindingSpec         `json:"binding"`
	Containers []ContainerRecommendation `json:"containers"`

	// RecommendationTime is status.conditions[type=RecommendationProvided].lastTransitionTime.
	RecommendationTime time.Time `json:"recommendationTime"`

	// Valid is false when annotation parsing or VPA status failed basic
	// sanity checks; ValidationErrors explains why. Invalid VPAs are still
	// surfaced (not silently dropped) so the dashboard can show why a VPA
	// isn't eligible for write-back.
	Valid            bool     `json:"valid"`
	ValidationErrors []string `json:"validationErrors,omitempty"`
}

// ContainerByName returns the recommendation for the named container, if
// present.
func (n NormalizedVPA) ContainerByName(name string) (ContainerRecommendation, bool) {
	for _, c := range n.Containers {
		if c.ContainerName == name {
			return c, true
		}
	}
	return ContainerRecommendation{}, false
}
