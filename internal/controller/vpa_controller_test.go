package controller

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	gitopsv1alpha1 "github.com/azurebrasil/argocd-vpa-updater/internal/apis/vpagitopsbinding/v1alpha1"
	"github.com/azurebrasil/argocd-vpa-updater/internal/vpaapi"
	"github.com/azurebrasil/argocd-vpa-updater/internal/vparecommendation"
)

func newScheme(t *testing.T) *runtime.Scheme {
	t.Helper()
	scheme := runtime.NewScheme()
	if err := vpaapi.AddToScheme(scheme); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := gitopsv1alpha1.AddToScheme(scheme); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	return scheme
}

func offMode() *vpaapi.UpdateMode {
	m := vpaapi.UpdateModeOff
	return &m
}

func resourceQty(s string) resource.Quantity { return resource.MustParse(s) }

func validBinding(name, namespace, vpaName string) *gitopsv1alpha1.VpaGitOpsBinding {
	return &gitopsv1alpha1.VpaGitOpsBinding{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: gitopsv1alpha1.VpaGitOpsBindingSpec{
			VpaRef:     gitopsv1alpha1.VpaReference{Name: vpaName},
			RepoURL:    "https://git.example.com/team/app.git",
			RepoBranch: "main",
			Containers: []gitopsv1alpha1.ContainerWriteBackConfig{
				{
					Name:          "app",
					ManifestType:  gitopsv1alpha1.SourceTypeYAML,
					ManifestPath:  "deploy/checkout-api.yaml",
					CPUKeyPath:    "spec.template.spec.containers[app].resources.requests.cpu",
					MemoryKeyPath: "spec.template.spec.containers[app].resources.requests.memory",
				},
			},
		},
	}
}

func vpaWithRecommendation(name, namespace string) *vpaapi.VerticalPodAutoscaler {
	return &vpaapi.VerticalPodAutoscaler{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: namespace},
		Spec: vpaapi.VerticalPodAutoscalerSpec{
			TargetRef:    &vpaapi.CrossVersionObjectReference{Kind: "Deployment", Name: "checkout-api"},
			UpdatePolicy: &vpaapi.PodUpdatePolicy{UpdateMode: offMode()},
		},
		Status: vpaapi.VerticalPodAutoscalerStatus{
			Recommendation: &vpaapi.RecommendedPodResources{
				ContainerRecommendations: []vpaapi.RecommendedContainerResources{
					{
						ContainerName: "app",
						Target: corev1.ResourceList{
							corev1.ResourceCPU:    resourceQty("250m"),
							corev1.ResourceMemory: resourceQty("512Mi"),
						},
					},
				},
			},
		},
	}
}

func runReconcile(t *testing.T, c client.Client, cache *vparecommendation.Cache, namespace, name string) {
	t.Helper()
	r := &VPAReconciler{Client: c, Cache: cache}
	_, err := r.Reconcile(context.Background(), ctrl.Request{NamespacedName: types.NamespacedName{Namespace: namespace, Name: name}})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestVPAReconciler_AddsBoundVPAToCache(t *testing.T) {
	binding := validBinding("checkout-api-sync", "payments", "checkout-api-vpa")
	vpa := vpaWithRecommendation("checkout-api-vpa", "payments")

	scheme := newScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&gitopsv1alpha1.VpaGitOpsBinding{}).WithObjects(binding, vpa).Build()
	cache := vparecommendation.NewCache()

	runReconcile(t, c, cache, "payments", "checkout-api-sync")

	got, ok := cache.Get("payments", "checkout-api-sync")
	if !ok {
		t.Fatalf("expected the binding's normalized VPA to be cached")
	}
	if !got.Valid {
		t.Fatalf("expected a fully-configured binding to be valid, got errors: %v", got.ValidationErrors)
	}
	if got.Namespace != "payments" || got.Name != "checkout-api-vpa" {
		t.Fatalf("expected NormalizedVPA identity to be the VPA's own namespace/name, got %s/%s", got.Namespace, got.Name)
	}
	if got.Workload.Kind != "Deployment" || got.Workload.Name != "checkout-api" {
		t.Fatalf("unexpected workload ref: %+v", got.Workload)
	}
	rec, found := got.ContainerByName("app")
	if !found {
		t.Fatalf("expected a recommendation for container app")
	}
	if rec.Target.CPU.String() != "250m" {
		t.Fatalf("expected cpu target 250m, got %v", rec.Target.CPU)
	}

	var refreshed gitopsv1alpha1.VpaGitOpsBinding
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: "payments", Name: "checkout-api-sync"}, &refreshed); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(refreshed.Status.Conditions) != 1 || refreshed.Status.Conditions[0].Status != metav1.ConditionTrue {
		t.Fatalf("expected a Ready=True condition, got %+v", refreshed.Status.Conditions)
	}
}

func TestVPAReconciler_EvictsOnBindingDeletion(t *testing.T) {
	binding := validBinding("checkout-api-sync", "payments", "checkout-api-vpa")
	vpa := vpaWithRecommendation("checkout-api-vpa", "payments")

	scheme := newScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&gitopsv1alpha1.VpaGitOpsBinding{}).WithObjects(binding, vpa).Build()
	cache := vparecommendation.NewCache()

	// First reconcile populates the cache under the binding's own key.
	runReconcile(t, c, cache, "payments", "checkout-api-sync")
	if _, ok := cache.Get("payments", "checkout-api-sync"); !ok {
		t.Fatalf("sanity check failed: expected the cache to be populated before deletion")
	}

	// Delete the binding, then reconcile the same key again (as the manager
	// would on the resulting delete event) -- the entry must be evicted.
	if err := c.Delete(context.Background(), binding); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	runReconcile(t, c, cache, "payments", "checkout-api-sync")

	if _, ok := cache.Get("payments", "checkout-api-sync"); ok {
		t.Fatalf("expected the cache entry to be evicted once the binding is deleted")
	}
}

func TestVPAReconciler_EvictsWhenVPANotFound(t *testing.T) {
	binding := validBinding("checkout-api-sync", "payments", "checkout-api-vpa") // VPA is never created
	scheme := newScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&gitopsv1alpha1.VpaGitOpsBinding{}).WithObjects(binding).Build()
	cache := vparecommendation.NewCache()

	runReconcile(t, c, cache, "payments", "checkout-api-sync")

	if _, ok := cache.Get("payments", "checkout-api-sync"); ok {
		t.Fatalf("expected no cache entry when the referenced VPA does not exist")
	}

	var refreshed gitopsv1alpha1.VpaGitOpsBinding
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: "payments", Name: "checkout-api-sync"}, &refreshed); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(refreshed.Status.Conditions) != 1 || refreshed.Status.Conditions[0].Status != metav1.ConditionFalse || refreshed.Status.Conditions[0].Reason != gitopsv1alpha1.ReasonVpaNotFound {
		t.Fatalf("expected a Ready=False/VpaNotFound condition, got %+v", refreshed.Status.Conditions)
	}
}

func TestVPAReconciler_FlagsNonOffUpdateModeAsInvalid(t *testing.T) {
	binding := validBinding("checkout-api-sync", "payments", "checkout-api-vpa")
	autoMode := vpaapi.UpdateModeAuto
	vpa := vpaWithRecommendation("checkout-api-vpa", "payments")
	vpa.Spec.UpdatePolicy.UpdateMode = &autoMode

	scheme := newScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&gitopsv1alpha1.VpaGitOpsBinding{}).WithObjects(binding, vpa).Build()
	cache := vparecommendation.NewCache()

	runReconcile(t, c, cache, "payments", "checkout-api-sync")

	got, ok := cache.Get("payments", "checkout-api-sync")
	if !ok {
		t.Fatalf("expected the VPA to still be cached (for diagnostic visibility) despite being invalid")
	}
	if got.Valid {
		t.Fatalf("expected updateMode Auto to be flagged invalid")
	}

	var refreshed gitopsv1alpha1.VpaGitOpsBinding
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: "payments", Name: "checkout-api-sync"}, &refreshed); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if refreshed.Status.Conditions[0].Status != metav1.ConditionFalse || refreshed.Status.Conditions[0].Reason != gitopsv1alpha1.ReasonValidationFailed {
		t.Fatalf("expected a Ready=False/ValidationFailed condition, got %+v", refreshed.Status.Conditions)
	}
}

func TestVPAReconciler_MissingKeyPathPerContainerFailsValidation(t *testing.T) {
	binding := validBinding("checkout-api-sync", "payments", "checkout-api-vpa")
	binding.Spec.Containers[0].CPUKeyPath = ""
	binding.Spec.Containers[0].MemoryKeyPath = ""
	vpa := vpaWithRecommendation("checkout-api-vpa", "payments")

	scheme := newScheme(t)
	c := fake.NewClientBuilder().WithScheme(scheme).WithStatusSubresource(&gitopsv1alpha1.VpaGitOpsBinding{}).WithObjects(binding, vpa).Build()
	cache := vparecommendation.NewCache()

	runReconcile(t, c, cache, "payments", "checkout-api-sync")

	got, _ := cache.Get("payments", "checkout-api-sync")
	if got.Valid {
		t.Fatalf("expected a container with neither cpuKeyPath nor memoryKeyPath to be invalid")
	}
}

func TestBindingsReferencingVPA_DoesNotCrossNamespaces(t *testing.T) {
	scheme := newScheme(t)
	bindingA := validBinding("sync", "ns-a", "foo")
	bindingB := validBinding("sync", "ns-b", "foo")

	c := fake.NewClientBuilder().
		WithScheme(scheme).
		WithIndex(&gitopsv1alpha1.VpaGitOpsBinding{}, vpaRefIndexField, func(obj client.Object) []string {
			return []string{obj.(*gitopsv1alpha1.VpaGitOpsBinding).Spec.VpaRef.Name}
		}).
		WithObjects(bindingA, bindingB).
		Build()

	mapFunc := bindingsReferencingVPA(c)
	vpaInA := &vpaapi.VerticalPodAutoscaler{ObjectMeta: metav1.ObjectMeta{Name: "foo", Namespace: "ns-a"}}
	reqs := mapFunc(context.Background(), vpaInA)

	if len(reqs) != 1 {
		t.Fatalf("expected exactly one enqueued request, got %d: %+v", len(reqs), reqs)
	}
	if reqs[0].Namespace != "ns-a" || reqs[0].Name != "sync" {
		t.Fatalf("expected namespace ns-a's binding, got %+v", reqs[0])
	}
}
