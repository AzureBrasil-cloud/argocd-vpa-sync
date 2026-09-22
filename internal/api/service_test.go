package api

import (
	"context"
	"errors"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/api/resource"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/azurebrasil/argocd-vpa-updater/internal/domain"
	"github.com/azurebrasil/argocd-vpa-updater/internal/resolver"
	"github.com/azurebrasil/argocd-vpa-updater/internal/statestore"
	"github.com/azurebrasil/argocd-vpa-updater/internal/vparecommendation"
	"github.com/azurebrasil/argocd-vpa-updater/internal/workloadresources"
)

// defaultWorkloadValues covers every container name the fixtures below use
// across every test that doesn't override svc.WorkloadReader itself:
// sampleVPA's "app" and cpuOnlyIneligibleVPA's "worker" (the two are
// combined in the bulk-select tests).
func defaultWorkloadValues() map[string]domain.ResourceAmount {
	return map[string]domain.ResourceAmount{
		"app":    {CPU: qptr("100m"), Memory: qptr("256Mi")},
		"worker": {CPU: qptr("100m")},
	}
}

func qptr(s string) *resource.Quantity {
	q := resource.MustParse(s)
	return &q
}

func newTestStateStore(t *testing.T) *statestore.SecretStore {
	t.Helper()
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	return statestore.NewSecretStore(c, "argocd-vpa-updater", "argocd-vpa-updater-state")
}

func sampleVPA() domain.NormalizedVPA {
	return domain.NormalizedVPA{
		Namespace:  "payments",
		Name:       "checkout-api-vpa",
		UpdateMode: "Off",
		Workload:   domain.WorkloadRef{Kind: "Deployment", Name: "checkout-api", Namespace: "payments"},
		Valid:      true,
		Binding: domain.GitOpsBindingSpec{
			ArgoCDApplication: "payments-api",
			RepoURL:           "https://git.example.com/team/app.git",
			RepoBranch:        "main",
			WriteBackPolicy:   domain.WriteBackCommit,
			MinChangePercent:  10,
			Containers: []domain.ContainerWriteBackConfig{
				{
					ContainerName: "app",
					ManifestType:  domain.SourceTypeYAML,
					ManifestPath:  "deploy/checkout-api.yaml",
					CPUKeyPath:    "spec.template.spec.containers[app].resources.requests.cpu",
					MemoryKeyPath: "spec.template.spec.containers[app].resources.requests.memory",
				},
			},
		},
		Containers: []domain.ContainerRecommendation{
			{
				ContainerName: "app",
				Target:        domain.ResourceAmount{CPU: qptr("250m"), Memory: qptr("512Mi")},
				LowerBound:    domain.ResourceAmount{CPU: qptr("150m")},
				UpperBound:    domain.ResourceAmount{CPU: qptr("400m")},
			},
		},
		RecommendationTime: time.Now().Add(-10 * time.Minute),
	}
}

func newTestService(t *testing.T, vpas []domain.NormalizedVPA) *Service {
	t.Helper()
	return &Service{
		Reader:         vparecommendation.FakeReader{VPAs: vpas},
		Resolver:       resolver.NewCRDResolver(nil, "argocd"),
		WorkloadReader: workloadresources.FakeReader{ByContainer: defaultWorkloadValues()},
		State:          newTestStateStore(t),
	}
}

func TestListRecommendations_ComputesDeltaAndEligibility(t *testing.T) {
	svc := newTestService(t, []domain.NormalizedVPA{sampleVPA()})

	items, err := svc.ListRecommendations(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected exactly 1 item, got %d", len(items))
	}

	item := items[0]
	if item.CurrentCPU != "100m" {
		t.Fatalf("expected current cpu 100m, got %q (error=%q)", item.CurrentCPU, item.CurrentValueError)
	}
	if item.RecommendedCPU != "250m" {
		t.Fatalf("expected recommended cpu 250m, got %q", item.RecommendedCPU)
	}
	if item.DeltaCPUAbsoluteMilli != 150 {
		t.Fatalf("expected absolute delta 150m, got %d", item.DeltaCPUAbsoluteMilli)
	}
	if item.DeltaCPUPercent == nil || *item.DeltaCPUPercent != 150 {
		t.Fatalf("expected 150%% cpu delta, got %v", item.DeltaCPUPercent)
	}
	if !item.Eligible {
		t.Fatalf("expected eligible=true, got reasons %v", item.EligibilityReasons)
	}
	if item.Status != "new" {
		t.Fatalf("expected status 'new' with no state store entries, got %q", item.Status)
	}
	if item.RecommendationAgeSeconds < 500 {
		t.Fatalf("expected a recommendation age of roughly 600s, got %d", item.RecommendationAgeSeconds)
	}
}

func TestListRecommendations_InvalidVPASurfacesValidationErrors(t *testing.T) {
	vpa := sampleVPA()
	vpa.Valid = false
	vpa.ValidationErrors = []string{"missing required annotation"}

	svc := newTestService(t, []domain.NormalizedVPA{vpa})
	items, err := svc.ListRecommendations(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("expected exactly 1 item, got %d", len(items))
	}
	if items[0].Status != "failed" || len(items[0].ValidationErrors) == 0 {
		t.Fatalf("expected a failed status with validation errors, got %+v", items[0])
	}
}

func TestListRecommendations_WorkloadReadErrorSurfacedNotSilentlyDropped(t *testing.T) {
	svc := newTestService(t, []domain.NormalizedVPA{sampleVPA()})
	svc.WorkloadReader = workloadresources.FakeReader{Err: errWorkloadNotFound}

	items, err := svc.ListRecommendations(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if items[0].CurrentValueError == "" {
		t.Fatalf("expected a CurrentValueError to be surfaced")
	}
	if items[0].Eligible {
		t.Fatalf("expected ineligible when the current value could not be read")
	}
}

func TestListRecommendations_BelowThresholdIsIneligible(t *testing.T) {
	vpa := sampleVPA()
	vpa.Containers[0].Target.CPU = qptr("102m") // 2% above the 100m current value
	vpa.Containers[0].Target.Memory = nil

	svc := newTestService(t, []domain.NormalizedVPA{vpa})
	items, err := svc.ListRecommendations(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if items[0].Eligible {
		t.Fatalf("expected ineligible below the 10%% threshold, got reasons=%v", items[0].EligibilityReasons)
	}
}

func TestListRecommendations_ReflectsPendingSelectionStatus(t *testing.T) {
	svc := newTestService(t, []domain.NormalizedVPA{sampleVPA()})

	err := svc.State.Update(context.Background(), func(doc *domain.StateDocument) error {
		doc.PendingSelections = append(doc.PendingSelections, domain.PendingSelection{
			IdempotencyKey: "sha256:whatever",
			VPANamespace:   "payments",
			VPAName:        "checkout-api-vpa",
			ContainerName:  "app",
			SelectedAt:     time.Now(),
		})
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	items, err := svc.ListRecommendations(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if items[0].Status != "selected" {
		t.Fatalf("expected status 'selected', got %q", items[0].Status)
	}
}

func TestListRecommendations_MultipleContainersProduceOneItemEach(t *testing.T) {
	vpa := sampleVPA()
	vpa.Binding.Containers = append(vpa.Binding.Containers, domain.ContainerWriteBackConfig{
		ContainerName: "sidecar",
		ManifestType:  domain.SourceTypeYAML,
		ManifestPath:  "deploy/checkout-api.yaml",
		CPUKeyPath:    "spec.template.spec.containers[sidecar].resources.requests.cpu",
		MemoryKeyPath: "spec.template.spec.containers[sidecar].resources.requests.memory",
	})
	vpa.Containers = append(vpa.Containers, domain.ContainerRecommendation{
		ContainerName: "sidecar",
		Target:        domain.ResourceAmount{CPU: qptr("120m"), Memory: qptr("128Mi")},
	})

	svc := newTestService(t, []domain.NormalizedVPA{vpa})
	values := defaultWorkloadValues()
	values["sidecar"] = domain.ResourceAmount{CPU: qptr("50m"), Memory: qptr("64Mi")}
	svc.WorkloadReader = workloadresources.FakeReader{ByContainer: values}

	items, err := svc.ListRecommendations(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(items) != 2 {
		t.Fatalf("expected one item per configured container, got %d: %+v", len(items), items)
	}

	byContainer := map[string]RecommendationDTO{}
	for _, item := range items {
		byContainer[item.ContainerName] = item
	}

	app, ok := byContainer["app"]
	if !ok || app.CurrentCPU != "100m" || app.RecommendedCPU != "250m" {
		t.Fatalf("unexpected app item: %+v", app)
	}
	sidecar, ok := byContainer["sidecar"]
	if !ok || sidecar.CurrentCPU != "50m" || sidecar.RecommendedCPU != "120m" {
		t.Fatalf("unexpected sidecar item: %+v", sidecar)
	}
}

func TestGetRecommendation_NotFound(t *testing.T) {
	svc := newTestService(t, []domain.NormalizedVPA{sampleVPA()})
	_, found, err := svc.GetRecommendation(context.Background(), "payments", "checkout-api-vpa", "sidecar")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if found {
		t.Fatalf("expected not found for a container the VPA doesn't declare")
	}
}

func TestSelectRecommendation_Success(t *testing.T) {
	svc := newTestService(t, []domain.NormalizedVPA{sampleVPA()})

	sel, err := svc.SelectRecommendation(context.Background(), "payments", "checkout-api-vpa", "app", true, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sel.IdempotencyKey == "" {
		t.Fatalf("expected a non-empty idempotency key")
	}
	if sel.VPANamespace != "payments" || sel.VPAName != "checkout-api-vpa" || sel.ContainerName != "app" {
		t.Fatalf("unexpected selection identity: %+v", sel)
	}
	if sel.SelectedAt.IsZero() {
		t.Fatalf("expected SelectedAt to be set")
	}

	items, err := svc.ListRecommendations(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if items[0].Status != "selected" {
		t.Fatalf("expected status 'selected' after SelectRecommendation, got %q", items[0].Status)
	}
}

func TestSelectRecommendation_NotEligible(t *testing.T) {
	vpa := sampleVPA()
	vpa.Containers[0].Target.CPU = qptr("102m")     // 2% above 100m current: below the 10% threshold
	vpa.Containers[0].Target.Memory = qptr("256Mi") // unchanged: also below threshold

	svc := newTestService(t, []domain.NormalizedVPA{vpa})

	_, err := svc.SelectRecommendation(context.Background(), "payments", "checkout-api-vpa", "app", true, true)
	if !errors.Is(err, ErrRecommendationNotEligible) {
		t.Fatalf("expected ErrRecommendationNotEligible, got %v", err)
	}
}

func TestSelectRecommendation_NotFound(t *testing.T) {
	svc := newTestService(t, []domain.NormalizedVPA{sampleVPA()})

	_, err := svc.SelectRecommendation(context.Background(), "payments", "checkout-api-vpa", "sidecar", true, true)
	if !errors.Is(err, ErrRecommendationNotFound) {
		t.Fatalf("expected ErrRecommendationNotFound, got %v", err)
	}
}

func TestSelectRecommendation_ReplacesRatherThanDuplicates(t *testing.T) {
	svc := newTestService(t, []domain.NormalizedVPA{sampleVPA()})

	first, err := svc.SelectRecommendation(context.Background(), "payments", "checkout-api-vpa", "app", true, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	second, err := svc.SelectRecommendation(context.Background(), "payments", "checkout-api-vpa", "app", true, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if first.IdempotencyKey != second.IdempotencyKey {
		t.Fatalf("expected the same recommendation to produce the same idempotency key on reselect")
	}

	doc, _, err := svc.State.Get(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	count := 0
	for _, sel := range doc.PendingSelections {
		if sel.VPANamespace == "payments" && sel.VPAName == "checkout-api-vpa" && sel.ContainerName == "app" {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("expected exactly one pending selection for the container after reselecting, got %d", count)
	}
}

func TestSelectRecommendation_PartialResourceSelectionChangesIdempotencyKey(t *testing.T) {
	svc := newTestService(t, []domain.NormalizedVPA{sampleVPA()})

	cpuOnly, err := svc.SelectRecommendation(context.Background(), "payments", "checkout-api-vpa", "app", true, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	both, err := svc.SelectRecommendation(context.Background(), "payments", "checkout-api-vpa", "app", true, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cpuOnly.IdempotencyKey == both.IdempotencyKey {
		t.Fatalf("expected selecting CPU-only vs. CPU+memory to produce different idempotency keys")
	}
}

func TestSelectRecommendation_RequestedResourceNotConfigured(t *testing.T) {
	vpa := sampleVPA()
	vpa.Binding.Containers[0].MemoryKeyPath = "" // binding no longer writes back memory

	svc := newTestService(t, []domain.NormalizedVPA{vpa})

	_, err := svc.SelectRecommendation(context.Background(), "payments", "checkout-api-vpa", "app", false, true)
	if !errors.Is(err, ErrRecommendationNotEligible) {
		t.Fatalf("expected ErrRecommendationNotEligible for an unconfigured resource, got %v", err)
	}
}

func TestSelectRecommendation_NoResourceRequested(t *testing.T) {
	svc := newTestService(t, []domain.NormalizedVPA{sampleVPA()})

	_, err := svc.SelectRecommendation(context.Background(), "payments", "checkout-api-vpa", "app", false, false)
	if !errors.Is(err, ErrRecommendationNotEligible) {
		t.Fatalf("expected ErrRecommendationNotEligible when neither resource is requested, got %v", err)
	}
}

// cpuOnlyIneligibleVPA is a second fixture for the bulk-select tests: its
// container only configures CPU write-back, and the CPU delta is below the
// 10% threshold -- it should always be skipped, never selected.
func cpuOnlyIneligibleVPA() domain.NormalizedVPA {
	return domain.NormalizedVPA{
		Namespace:  "billing",
		Name:       "invoice-vpa",
		UpdateMode: "Off",
		Workload:   domain.WorkloadRef{Kind: "Deployment", Name: "invoice", Namespace: "billing"},
		Valid:      true,
		Binding: domain.GitOpsBindingSpec{
			RepoURL:          "https://git.example.com/team/app.git",
			RepoBranch:       "main",
			WriteBackPolicy:  domain.WriteBackCommit,
			MinChangePercent: 10,
			Containers: []domain.ContainerWriteBackConfig{
				{
					ContainerName: "worker",
					ManifestType:  domain.SourceTypeYAML,
					ManifestPath:  "deploy/checkout-api.yaml",
					CPUKeyPath:    "spec.template.spec.containers[app].resources.requests.cpu", // current value: defaultWorkloadValues()["worker"], cpu 100m
				},
			},
		},
		Containers: []domain.ContainerRecommendation{
			{ContainerName: "worker", Target: domain.ResourceAmount{CPU: qptr("101m")}}, // ~1%: below threshold
		},
		RecommendationTime: time.Now(),
	}
}

func TestBulkSelectRecommendations_MixedEligibility(t *testing.T) {
	svc := newTestService(t, []domain.NormalizedVPA{sampleVPA(), cpuOnlyIneligibleVPA()})

	result, err := svc.BulkSelectRecommendations(context.Background(), true, true)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Selected) != 1 || result.Selected[0].VPAName != "checkout-api-vpa" {
		t.Fatalf("expected exactly one selection (checkout-api-vpa), got %+v", result.Selected)
	}
	if len(result.Skipped) != 1 || result.Skipped[0].VPAName != "invoice-vpa" {
		t.Fatalf("expected invoice-vpa to be skipped, got %+v", result.Skipped)
	}

	doc, _, err := svc.State.Get(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(doc.PendingSelections) != 1 {
		t.Fatalf("expected exactly one persisted selection, got %d", len(doc.PendingSelections))
	}
}

func TestBulkSelectRecommendations_CPUOnly_SkipsContainersWithoutCPU(t *testing.T) {
	vpa := sampleVPA()
	vpa.Binding.Containers[0].CPUKeyPath = "" // this container only writes back memory

	svc := newTestService(t, []domain.NormalizedVPA{vpa})

	result, err := svc.BulkSelectRecommendations(context.Background(), true, false)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result.Selected) != 0 {
		t.Fatalf("expected no selections when the only container has no CPU configured, got %+v", result.Selected)
	}
	if len(result.Skipped) != 1 {
		t.Fatalf("expected the container to be skipped, got %+v", result.Skipped)
	}
}

func TestBulkSelectRecommendations_NoResourceRequested(t *testing.T) {
	svc := newTestService(t, []domain.NormalizedVPA{sampleVPA()})

	_, err := svc.BulkSelectRecommendations(context.Background(), false, false)
	if !errors.Is(err, ErrRecommendationNotEligible) {
		t.Fatalf("expected ErrRecommendationNotEligible when neither resource is requested, got %v", err)
	}
}

var errWorkloadNotFound = &testError{"workload not found"}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }
