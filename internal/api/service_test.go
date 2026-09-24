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
					ContainerName:      "app",
					ManifestType:       domain.SourceTypeYAML,
					ManifestPath:       "deploy/checkout-api.yaml",
					CPUKeyPath:         "spec.template.spec.containers[app].resources.requests.cpu",
					MemoryKeyPath:      "spec.template.spec.containers[app].resources.requests.memory",
					CPULimitKeyPath:    "spec.template.spec.containers[app].resources.limits.cpu",
					MemoryLimitKeyPath: "spec.template.spec.containers[app].resources.limits.memory",
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

func TestListRecommendations_SurfacesOperationDetailOnceApplied(t *testing.T) {
	svc := newTestService(t, []domain.NormalizedVPA{sampleVPA()})

	err := svc.State.Update(context.Background(), func(doc *domain.StateDocument) error {
		doc.Operations["sha256:whatever"] = domain.OperationState{
			IdempotencyKey: "sha256:whatever",
			VPANamespace:   "payments",
			VPAName:        "checkout-api-vpa",
			ContainerName:  "app",
			Status:         domain.OperationApplied,
			Branch:         "main",
			CommitSHA:      "abc123",
			UpdatedAt:      time.Now(),
		}
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	items, err := svc.ListRecommendations(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if items[0].Status != "applied" {
		t.Fatalf("expected status 'applied', got %q", items[0].Status)
	}
	if items[0].Operation == nil {
		t.Fatalf("expected Operation to be populated")
	}
	if items[0].Operation.Branch != "main" || items[0].Operation.CommitSHA != "abc123" {
		t.Fatalf("expected branch/commit to be surfaced, got %+v", items[0].Operation)
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

	sel, err := svc.SelectRecommendation(context.Background(), "payments", "checkout-api-vpa", "app", SelectOptions{ApplyCPU: true, ApplyMemory: true})
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

	_, err := svc.SelectRecommendation(context.Background(), "payments", "checkout-api-vpa", "app", SelectOptions{ApplyCPU: true, ApplyMemory: true})
	if !errors.Is(err, ErrRecommendationNotEligible) {
		t.Fatalf("expected ErrRecommendationNotEligible, got %v", err)
	}
}

func TestSelectRecommendation_NotFound(t *testing.T) {
	svc := newTestService(t, []domain.NormalizedVPA{sampleVPA()})

	_, err := svc.SelectRecommendation(context.Background(), "payments", "checkout-api-vpa", "sidecar", SelectOptions{ApplyCPU: true, ApplyMemory: true})
	if !errors.Is(err, ErrRecommendationNotFound) {
		t.Fatalf("expected ErrRecommendationNotFound, got %v", err)
	}
}

func TestSelectRecommendation_ReplacesRatherThanDuplicates(t *testing.T) {
	svc := newTestService(t, []domain.NormalizedVPA{sampleVPA()})

	first, err := svc.SelectRecommendation(context.Background(), "payments", "checkout-api-vpa", "app", SelectOptions{ApplyCPU: true, ApplyMemory: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	second, err := svc.SelectRecommendation(context.Background(), "payments", "checkout-api-vpa", "app", SelectOptions{ApplyCPU: true, ApplyMemory: true})
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

	cpuOnly, err := svc.SelectRecommendation(context.Background(), "payments", "checkout-api-vpa", "app", SelectOptions{ApplyCPU: true, ApplyMemory: false})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	both, err := svc.SelectRecommendation(context.Background(), "payments", "checkout-api-vpa", "app", SelectOptions{ApplyCPU: true, ApplyMemory: true})
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

	_, err := svc.SelectRecommendation(context.Background(), "payments", "checkout-api-vpa", "app", SelectOptions{ApplyCPU: false, ApplyMemory: true})
	if !errors.Is(err, ErrRecommendationNotEligible) {
		t.Fatalf("expected ErrRecommendationNotEligible for an unconfigured resource, got %v", err)
	}
}

func TestSelectRecommendation_NoResourceRequested(t *testing.T) {
	svc := newTestService(t, []domain.NormalizedVPA{sampleVPA()})

	_, err := svc.SelectRecommendation(context.Background(), "payments", "checkout-api-vpa", "app", SelectOptions{ApplyCPU: false, ApplyMemory: false})
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

	result, err := svc.BulkSelectRecommendations(context.Background(), SelectOptions{ApplyCPU: true, ApplyMemory: true})
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

	result, err := svc.BulkSelectRecommendations(context.Background(), SelectOptions{ApplyCPU: true, ApplyMemory: false})
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

	_, err := svc.BulkSelectRecommendations(context.Background(), SelectOptions{ApplyCPU: false, ApplyMemory: false})
	if !errors.Is(err, ErrRecommendationNotEligible) {
		t.Fatalf("expected ErrRecommendationNotEligible when neither resource is requested, got %v", err)
	}
}

var errWorkloadNotFound = &testError{"workload not found"}

type testError struct{ msg string }

func (e *testError) Error() string { return e.msg }

func TestSelectRecommendation_CarriesLimitSpecs(t *testing.T) {
	svc := newTestService(t, []domain.NormalizedVPA{sampleVPA()})
	pct := 20.0
	value := resource.MustParse("1Gi")

	withLimits, err := svc.SelectRecommendation(context.Background(), "payments", "checkout-api-vpa", "app", SelectOptions{
		ApplyCPU:    true,
		ApplyMemory: true,
		CPULimit:    &domain.LimitSpec{HeadroomPercent: &pct},
		MemoryLimit: &domain.LimitSpec{Value: &value},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if withLimits.CPULimit == nil || *withLimits.CPULimit.HeadroomPercent != 20 {
		t.Fatalf("expected cpu limit headroom 20 on the selection, got %+v", withLimits.CPULimit)
	}
	if withLimits.MemoryLimit == nil || withLimits.MemoryLimit.Value.String() != "1Gi" {
		t.Fatalf("expected memory limit 1Gi on the selection, got %+v", withLimits.MemoryLimit)
	}
	if withLimits.Target.MemoryLimitKeyPath != "spec.template.spec.containers[app].resources.limits.memory" {
		t.Fatalf("expected derived memory limit key path, got %q", withLimits.Target.MemoryLimitKeyPath)
	}

	without, err := svc.SelectRecommendation(context.Background(), "payments", "checkout-api-vpa", "app", SelectOptions{ApplyCPU: true, ApplyMemory: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if without.IdempotencyKey == withLimits.IdempotencyKey {
		t.Fatalf("expected the limit specs to change the idempotency key")
	}
}

func TestSelectRecommendation_DropsLimitSpecOfUnappliedResource(t *testing.T) {
	svc := newTestService(t, []domain.NormalizedVPA{sampleVPA()})
	pct := 20.0
	sel, err := svc.SelectRecommendation(context.Background(), "payments", "checkout-api-vpa", "app", SelectOptions{
		ApplyCPU:    true,
		MemoryLimit: &domain.LimitSpec{HeadroomPercent: &pct},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sel.MemoryLimit != nil {
		t.Fatalf("expected no memory limit spec when memory isn't applied, got %+v", sel.MemoryLimit)
	}
}

func TestSelectRecommendation_RejectsInvalidLimitSpecs(t *testing.T) {
	svc := newTestService(t, []domain.NormalizedVPA{sampleVPA()})
	pct := 100.0
	below := resource.MustParse("256Mi") // recommendation is 512Mi
	for name, opts := range map[string]SelectOptions{
		"headroom 100":        {ApplyCPU: true, CPULimit: &domain.LimitSpec{HeadroomPercent: &pct}},
		"value below request": {ApplyMemory: true, MemoryLimit: &domain.LimitSpec{Value: &below}},
	} {
		_, err := svc.SelectRecommendation(context.Background(), "payments", "checkout-api-vpa", "app", opts)
		if !errors.Is(err, ErrInvalidSelectRequest) {
			t.Errorf("%s: expected ErrInvalidSelectRequest, got %v", name, err)
		}
	}
}

func TestSelectRequest_OptionsParsesQuantities(t *testing.T) {
	if _, err := (SelectRequest{ApplyMemory: true, MemoryLimit: &LimitSpecDTO{Value: "512Mb"}}).options(); !errors.Is(err, ErrInvalidSelectRequest) {
		t.Fatalf("expected ErrInvalidSelectRequest for an invalid quantity, got %v", err)
	}
	opts, err := (SelectRequest{ApplyMemory: true, MemoryLimit: &LimitSpecDTO{Value: "512Mi"}}).options()
	if err != nil || opts.MemoryLimit == nil || opts.MemoryLimit.Value.String() != "512Mi" {
		t.Fatalf("unexpected result: %+v, %v", opts.MemoryLimit, err)
	}
}

func TestListRecommendations_ExposesLiveLimits(t *testing.T) {
	svc := newTestService(t, []domain.NormalizedVPA{sampleVPA()})
	svc.WorkloadReader = workloadresources.FakeReader{
		ByContainer:       defaultWorkloadValues(),
		LimitsByContainer: map[string]domain.ResourceAmount{"app": {CPU: qptr("500m"), Memory: qptr("300Mi")}},
	}
	items, err := svc.ListRecommendations(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	it := items[0]
	if it.CurrentCPULimit != "500m" || it.CurrentMemoryLimit != "300Mi" || !it.CPULimitConfigured || !it.MemoryLimitConfigured {
		t.Fatalf("unexpected limit fields: %+v", it)
	}
}

func TestLimitRequired_WhenRecommendationExceedsLiveLimit(t *testing.T) {
	// sampleVPA recommends 250m / 512Mi: above a 200m cpu limit, below a 1Gi memory limit.
	svc := newTestService(t, []domain.NormalizedVPA{sampleVPA()})
	svc.WorkloadReader = workloadresources.FakeReader{
		ByContainer:       defaultWorkloadValues(),
		LimitsByContainer: map[string]domain.ResourceAmount{"app": {CPU: qptr("200m"), Memory: qptr("1Gi")}},
	}

	items, err := svc.ListRecommendations(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !items[0].CPULimitRequired || items[0].MemoryLimitRequired {
		t.Fatalf("expected cpu limit required and memory optional, got cpu=%v memory=%v", items[0].CPULimitRequired, items[0].MemoryLimitRequired)
	}

	_, err = svc.SelectRecommendation(context.Background(), "payments", "checkout-api-vpa", "app", SelectOptions{ApplyCPU: true})
	if !errors.Is(err, ErrInvalidSelectRequest) {
		t.Fatalf("expected ErrInvalidSelectRequest for a required cpu limit left unset, got %v", err)
	}

	pct := 20.0
	if _, err := svc.SelectRecommendation(context.Background(), "payments", "checkout-api-vpa", "app", SelectOptions{
		ApplyCPU: true,
		CPULimit: &domain.LimitSpec{HeadroomPercent: &pct},
	}); err != nil {
		t.Fatalf("expected a required cpu limit that is set to be accepted, got %v", err)
	}

	sel, err := svc.SelectRecommendation(context.Background(), "payments", "checkout-api-vpa", "app", SelectOptions{ApplyMemory: true})
	if err != nil {
		t.Fatalf("expected an optional memory limit to be omittable, got %v", err)
	}
	if sel.MemoryLimit != nil {
		t.Fatalf("expected no memory limit spec, got %+v", sel.MemoryLimit)
	}
}

func TestLimitRequired_NotWithoutLiveLimitOrLimitKeyPath(t *testing.T) {
	vpa := sampleVPA()
	vpa.Binding.Containers[0].CPULimitKeyPath = ""
	svc := newTestService(t, []domain.NormalizedVPA{vpa})
	svc.WorkloadReader = workloadresources.FakeReader{
		ByContainer:       defaultWorkloadValues(),
		LimitsByContainer: map[string]domain.ResourceAmount{"app": {CPU: qptr("200m")}},
	}

	items, err := svc.ListRecommendations(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if items[0].CPULimitRequired || items[0].MemoryLimitRequired {
		t.Fatalf("expected no limit required, got cpu=%v memory=%v", items[0].CPULimitRequired, items[0].MemoryLimitRequired)
	}
	// The unmanaged 200m cpu limit is still exceeded by the 250m
	// recommendation (surfaced as a warning); memory declares no limit.
	if !items[0].CPULimitExceeded || items[0].MemoryLimitExceeded {
		t.Fatalf("expected cpu limit exceeded and memory not, got cpu=%v memory=%v", items[0].CPULimitExceeded, items[0].MemoryLimitExceeded)
	}
}

// headroomVPA recommends 100Mi memory for "app", whose live request is set
// per test.
func headroomVPA() domain.NormalizedVPA {
	vpa := sampleVPA()
	vpa.Containers[0].Target = domain.ResourceAmount{CPU: qptr("250m"), Memory: qptr("100Mi")}
	return vpa
}

func seedRequestHeadroom(t *testing.T, svc *Service, memoryPct float64) {
	t.Helper()
	if err := svc.State.Update(context.Background(), func(doc *domain.StateDocument) error {
		doc.RequestHeadrooms = map[string]domain.RequestHeadroom{
			domain.RequestHeadroomKey("payments", "checkout-api-vpa", "app"): {MemoryPercent: &memoryPct},
		}
		return nil
	}); err != nil {
		t.Fatalf("unexpected error seeding headroom: %v", err)
	}
}

func TestRequestHeadroom_RecordedHeadroomMakesCurrentValueOK(t *testing.T) {
	svc := newTestService(t, []domain.NormalizedVPA{headroomVPA()})
	// 149796572 bytes is 100Mi at 30% headroom (100Mi / 0.7); without the
	// recorded headroom that would look like a -30% change.
	svc.WorkloadReader = workloadresources.FakeReader{ByContainer: map[string]domain.ResourceAmount{
		"app": {CPU: qptr("250m"), Memory: qptr("149796572")},
	}}
	seedRequestHeadroom(t, svc, 30)

	items, err := svc.ListRecommendations(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	it := items[0]
	if it.MemoryEligible {
		t.Fatalf("expected memory not eligible at the recorded headroom, got delta %v", it.DeltaMemoryPercent)
	}
	if it.TargetMemory != "149796572" || it.MemoryRequestHeadroomPercent == nil || *it.MemoryRequestHeadroomPercent != 30 {
		t.Fatalf("unexpected target/headroom: %q %v", it.TargetMemory, it.MemoryRequestHeadroomPercent)
	}
}

func TestRequestHeadroom_RecommendationDropStillEligible(t *testing.T) {
	vpa := headroomVPA()
	vpa.Containers[0].Target.Memory = qptr("80Mi") // target 80Mi / 0.7 ~ 114Mi, ~-20%
	svc := newTestService(t, []domain.NormalizedVPA{vpa})
	svc.WorkloadReader = workloadresources.FakeReader{ByContainer: map[string]domain.ResourceAmount{
		"app": {CPU: qptr("250m"), Memory: qptr("149796572")},
	}}
	seedRequestHeadroom(t, svc, 30)

	items, err := svc.ListRecommendations(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !items[0].MemoryEligible {
		t.Fatalf("expected memory eligible once the recommendation drops, got delta %v", items[0].DeltaMemoryPercent)
	}
}

func TestSelectRecommendation_RequestHeadroomOverridesValue(t *testing.T) {
	svc := newTestService(t, []domain.NormalizedVPA{headroomVPA()})

	plain, err := svc.SelectRecommendation(context.Background(), "payments", "checkout-api-vpa", "app", SelectOptions{ApplyMemory: true})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if plain.OverrideMemory != nil || plain.MemoryRequestHeadroom != nil {
		t.Fatalf("expected no override without a headroom, got %+v", plain.OverrideMemory)
	}

	zero := 0.0
	zeroSel, err := svc.SelectRecommendation(context.Background(), "payments", "checkout-api-vpa", "app", SelectOptions{ApplyMemory: true, MemoryRequestHeadroom: &zero})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if zeroSel.IdempotencyKey != plain.IdempotencyKey {
		t.Fatalf("expected a 0%% headroom to keep the idempotency key")
	}

	pct := 30.0
	sel, err := svc.SelectRecommendation(context.Background(), "payments", "checkout-api-vpa", "app", SelectOptions{
		ApplyMemory:           true,
		MemoryRequestHeadroom: &pct,
		CPURequestHeadroom:    &pct, // cpu isn't applied, so this must be dropped
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sel.OverrideMemory == nil || sel.OverrideMemory.Memory.Cmp(resource.MustParse("149796572")) != 0 {
		t.Fatalf("expected memory override 100Mi / 0.7, got %+v", sel.OverrideMemory)
	}
	if sel.MemoryRequestHeadroom == nil || *sel.MemoryRequestHeadroom != 30 || sel.CPURequestHeadroom != nil || sel.OverrideCPU != nil {
		t.Fatalf("unexpected headroom fields: memory=%v cpu=%v overrideCPU=%v", sel.MemoryRequestHeadroom, sel.CPURequestHeadroom, sel.OverrideCPU)
	}
	if sel.IdempotencyKey == plain.IdempotencyKey {
		t.Fatalf("expected the headroom to change the idempotency key")
	}
}

func TestSelectRecommendation_RequiredLimitUsesRequestWithHeadroom(t *testing.T) {
	svc := newTestService(t, []domain.NormalizedVPA{headroomVPA()})
	// 100Mi fits under the 128Mi limit, but 100Mi at 30% (~143Mi) doesn't.
	svc.WorkloadReader = workloadresources.FakeReader{
		ByContainer:       defaultWorkloadValues(),
		LimitsByContainer: map[string]domain.ResourceAmount{"app": {Memory: qptr("128Mi")}},
	}
	if _, err := svc.SelectRecommendation(context.Background(), "payments", "checkout-api-vpa", "app", SelectOptions{ApplyMemory: true}); err != nil {
		t.Fatalf("expected no limit required without a headroom, got %v", err)
	}

	pct := 30.0
	_, err := svc.SelectRecommendation(context.Background(), "payments", "checkout-api-vpa", "app", SelectOptions{ApplyMemory: true, MemoryRequestHeadroom: &pct})
	if !errors.Is(err, ErrInvalidSelectRequest) {
		t.Fatalf("expected a required limit once the headroom pushes the request above it, got %v", err)
	}
}

func TestSelectRecommendation_RejectsInvalidRequestHeadroom(t *testing.T) {
	svc := newTestService(t, []domain.NormalizedVPA{sampleVPA()})
	pct := 100.0
	_, err := svc.SelectRecommendation(context.Background(), "payments", "checkout-api-vpa", "app", SelectOptions{ApplyCPU: true, CPURequestHeadroom: &pct})
	if !errors.Is(err, ErrInvalidSelectRequest) {
		t.Fatalf("expected ErrInvalidSelectRequest, got %v", err)
	}
}
