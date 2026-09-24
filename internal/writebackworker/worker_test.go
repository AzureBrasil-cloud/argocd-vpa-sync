package writebackworker

import (
	"context"
	"errors"
	"testing"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/azurebrasil/argocd-vpa-updater/internal/domain"
	"github.com/azurebrasil/argocd-vpa-updater/internal/gitwriteback"
	"github.com/azurebrasil/argocd-vpa-updater/internal/patcher"
	"github.com/azurebrasil/argocd-vpa-updater/internal/statestore"
)

const (
	testNamespace = "argocd-vpa-updater"
	testName      = "argocd-vpa-updater-state"
)

func newStore() statestore.StateStore {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	c := fake.NewClientBuilder().WithScheme(scheme).Build()
	return statestore.NewSecretStore(c, testNamespace, testName)
}

type fakeCredentialsProvider struct {
	creds domain.GitCredentials
	err   error
}

func (f *fakeCredentialsProvider) GetCredentials(context.Context, string) (domain.GitCredentials, error) {
	return f.creds, f.err
}

type fakeWriteBackService struct {
	calls  int
	result gitwriteback.WriteBackResult
	err    error
}

func (f *fakeWriteBackService) Apply(context.Context, gitwriteback.WriteBackRequest) (gitwriteback.WriteBackResult, error) {
	f.calls++
	return f.result, f.err
}

// fakeBatchWriteBackService implements gitwriteback.BatchGitWriteBackService
// (unlike fakeWriteBackService above, which only implements the single-item
// Apply), so tests using it exercise Worker's batching path. By default it
// echoes back one ItemOutcome per requested item (freshly applied, sharing a
// single commit), matching how a real ApplyBatch call behaves.
type fakeBatchWriteBackService struct {
	calls int
	reqs  []gitwriteback.BatchWriteBackRequest
	err   error

	// alreadyApplied, if set, marks these idempotency keys as
	// AlreadyApplied=true in the synthesized result instead of freshly
	// applied.
	alreadyApplied map[string]bool
}

func (f *fakeBatchWriteBackService) Apply(context.Context, gitwriteback.WriteBackRequest) (gitwriteback.WriteBackResult, error) {
	panic("fakeBatchWriteBackService.Apply should not be called -- Worker must prefer ApplyBatch when available")
}

func (f *fakeBatchWriteBackService) ApplyBatch(_ context.Context, req gitwriteback.BatchWriteBackRequest) (gitwriteback.BatchWriteBackResult, error) {
	f.calls++
	f.reqs = append(f.reqs, req)
	if f.err != nil {
		return gitwriteback.BatchWriteBackResult{}, f.err
	}

	items := make([]gitwriteback.ItemOutcome, len(req.Items))
	for i, it := range req.Items {
		items[i] = gitwriteback.ItemOutcome{
			IdempotencyKey: it.IdempotencyKey,
			AlreadyApplied: f.alreadyApplied[it.IdempotencyKey],
			CommitSHA:      "batch-sha",
		}
	}
	return gitwriteback.BatchWriteBackResult{
		Status:    domain.OperationApplied,
		Branch:    req.Branch,
		CommitSHA: "batch-sha",
		Items:     items,
	}, nil
}

func testSelection(idempotencyKey string) domain.PendingSelection {
	return testSelectionFor(idempotencyKey, "https://example.com/repo.git", "main")
}

func testSelectionFor(idempotencyKey, repoURL, branch string) domain.PendingSelection {
	return domain.PendingSelection{
		IdempotencyKey: idempotencyKey,
		VPANamespace:   "payments",
		VPAName:        "checkout-api-vpa",
		ContainerName:  "app",
		SelectedAt:     time.Now(),
		Target: domain.WriteTarget{
			RepoURL:       repoURL,
			Branch:        branch,
			FilePath:      "deploy.yaml",
			SourceType:    domain.SourceTypeYAML,
			CPUKeyPath:    "resources.requests.cpu",
			ContainerName: "app",
		},
		ApplyCPU: true,
	}
}

func newWorker(store statestore.StateStore, wb gitwriteback.GitWriteBackService) *Worker {
	return &Worker{
		State:       store,
		WriteBack:   wb,
		Credentials: &fakeCredentialsProvider{creds: domain.GitCredentials{AuthMethod: domain.AuthNone}},
		Patchers:    patcher.NewRegistry(patcher.NewYAMLPatcher()),
		Now:         time.Now,
	}
}

func TestWorker_ProcessOnce_ClaimsAndAppliesPendingSelection(t *testing.T) {
	store := newStore()
	sel := testSelection("sha256:key-1")
	if err := store.Update(context.Background(), func(doc *domain.StateDocument) error {
		doc.PendingSelections = append(doc.PendingSelections, sel)
		return nil
	}); err != nil {
		t.Fatalf("unexpected error seeding selection: %v", err)
	}

	wb := &fakeWriteBackService{result: gitwriteback.WriteBackResult{
		Status:    domain.OperationApplied,
		Branch:    "main",
		CommitSHA: "abc123",
	}}
	w := newWorker(store, wb)
	w.processOnce(context.Background())

	if wb.calls != 1 {
		t.Fatalf("expected WriteBack.Apply to be called once, got %d", wb.calls)
	}

	doc, _, err := store.Get(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(doc.PendingSelections) != 0 {
		t.Fatalf("expected the selection to be removed from PendingSelections, got %d remaining", len(doc.PendingSelections))
	}
	op, ok := doc.Operations["sha256:key-1"]
	if !ok {
		t.Fatalf("expected an Operations entry for the applied selection")
	}
	if op.Status != domain.OperationApplied {
		t.Fatalf("expected Status applied, got %q", op.Status)
	}
	if op.Branch != "main" || op.CommitSHA != "abc123" {
		t.Fatalf("expected branch/commit to be recorded, got %+v", op)
	}
}

func TestWorker_ProcessOnce_RecordsFailure(t *testing.T) {
	store := newStore()
	sel := testSelection("sha256:key-2")
	if err := store.Update(context.Background(), func(doc *domain.StateDocument) error {
		doc.PendingSelections = append(doc.PendingSelections, sel)
		return nil
	}); err != nil {
		t.Fatalf("unexpected error seeding selection: %v", err)
	}

	wb := &fakeWriteBackService{err: errors.New("boom: push failed")}
	w := newWorker(store, wb)
	w.processOnce(context.Background())

	doc, _, err := store.Get(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	op, ok := doc.Operations["sha256:key-2"]
	if !ok {
		t.Fatalf("expected an Operations entry")
	}
	if op.Status != domain.OperationFailed {
		t.Fatalf("expected Status failed, got %q", op.Status)
	}
	if op.ErrorMessage == "" {
		t.Fatalf("expected a non-empty ErrorMessage")
	}
}

func TestWorker_ProcessOnce_RecordsConflict(t *testing.T) {
	store := newStore()
	sel := testSelection("sha256:key-3")
	if err := store.Update(context.Background(), func(doc *domain.StateDocument) error {
		doc.PendingSelections = append(doc.PendingSelections, sel)
		return nil
	}); err != nil {
		t.Fatalf("unexpected error seeding selection: %v", err)
	}

	wb := &fakeWriteBackService{err: gitwriteback.ErrConflict}
	w := newWorker(store, wb)
	w.processOnce(context.Background())

	doc, _, err := store.Get(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	op, ok := doc.Operations["sha256:key-3"]
	if !ok {
		t.Fatalf("expected an Operations entry")
	}
	if op.Status != domain.OperationConflict {
		t.Fatalf("expected Status conflict, got %q", op.Status)
	}
}

func TestWorker_ProcessOnce_SkipsAlreadyApplyingEntry(t *testing.T) {
	store := newStore()
	sel := testSelection("sha256:key-4")
	now := time.Now()
	if err := store.Update(context.Background(), func(doc *domain.StateDocument) error {
		doc.PendingSelections = append(doc.PendingSelections, sel)
		doc.Operations[sel.IdempotencyKey] = domain.OperationState{
			IdempotencyKey: sel.IdempotencyKey,
			Status:         domain.OperationApplying,
			UpdatedAt:      now,
		}
		return nil
	}); err != nil {
		t.Fatalf("unexpected error seeding: %v", err)
	}

	wb := &fakeWriteBackService{result: gitwriteback.WriteBackResult{Status: domain.OperationApplied}}
	w := newWorker(store, wb)
	w.Now = func() time.Time { return now }
	w.processOnce(context.Background())

	if wb.calls != 0 {
		t.Fatalf("expected WriteBack.Apply not to be called for an in-flight (not yet stale) selection, got %d calls", wb.calls)
	}

	doc, _, err := store.Get(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(doc.PendingSelections) != 1 {
		t.Fatalf("expected the selection to remain queued, got %d", len(doc.PendingSelections))
	}
}

func TestWorker_ProcessOnce_StaleApplyingEntryMarkedFailed(t *testing.T) {
	store := newStore()
	staleTime := time.Now().Add(-10 * time.Minute)
	if err := store.Update(context.Background(), func(doc *domain.StateDocument) error {
		doc.Operations["sha256:stale"] = domain.OperationState{
			IdempotencyKey: "sha256:stale",
			Status:         domain.OperationApplying,
			UpdatedAt:      staleTime,
		}
		return nil
	}); err != nil {
		t.Fatalf("unexpected error seeding: %v", err)
	}

	wb := &fakeWriteBackService{}
	w := newWorker(store, wb)
	w.processOnce(context.Background())

	if wb.calls != 0 {
		t.Fatalf("expected WriteBack.Apply not to be called for a stale entry with no pending selection to reprocess, got %d", wb.calls)
	}

	doc, _, err := store.Get(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	op := doc.Operations["sha256:stale"]
	if op.Status != domain.OperationFailed {
		t.Fatalf("expected the stale applying entry to be marked failed, got %q", op.Status)
	}
	if op.ErrorMessage == "" {
		t.Fatalf("expected an explanatory ErrorMessage on the stale entry")
	}
}

func TestWorker_ProcessOnce_PreV2SelectionMarkedFailedWithoutApplying(t *testing.T) {
	store := newStore()
	// A pre-v2 selection: no Target captured, so Target.RepoURL is the
	// zero-value empty string.
	sel := domain.PendingSelection{
		IdempotencyKey: "sha256:old",
		VPANamespace:   "payments",
		VPAName:        "checkout-api-vpa",
		ContainerName:  "app",
		SelectedAt:     time.Now(),
	}
	if err := store.Update(context.Background(), func(doc *domain.StateDocument) error {
		doc.PendingSelections = append(doc.PendingSelections, sel)
		return nil
	}); err != nil {
		t.Fatalf("unexpected error seeding: %v", err)
	}

	wb := &fakeWriteBackService{}
	w := newWorker(store, wb)
	w.processOnce(context.Background())

	if wb.calls != 0 {
		t.Fatalf("expected WriteBack.Apply not to be called for a pre-v2 selection with no captured Target, got %d", wb.calls)
	}

	doc, _, err := store.Get(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(doc.PendingSelections) != 0 {
		t.Fatalf("expected the pre-v2 selection to be removed from PendingSelections, got %d remaining", len(doc.PendingSelections))
	}
	op, ok := doc.Operations["sha256:old"]
	if !ok {
		t.Fatalf("expected an Operations entry explaining the failure")
	}
	if op.Status != domain.OperationFailed {
		t.Fatalf("expected Status failed, got %q", op.Status)
	}
}

func TestWorker_ProcessOnce_BatchesSelectionsTargetingTheSameRepoAndBranch(t *testing.T) {
	store := newStore()
	sel1 := testSelectionFor("sha256:key-1", "https://example.com/repo.git", "main")
	sel2 := testSelectionFor("sha256:key-2", "https://example.com/repo.git", "main")
	if err := store.Update(context.Background(), func(doc *domain.StateDocument) error {
		doc.PendingSelections = append(doc.PendingSelections, sel1, sel2)
		return nil
	}); err != nil {
		t.Fatalf("unexpected error seeding: %v", err)
	}

	batchSvc := &fakeBatchWriteBackService{}
	w := newWorker(store, batchSvc)
	w.processOnce(context.Background())

	if batchSvc.calls != 1 {
		t.Fatalf("expected exactly 1 ApplyBatch call for 2 selections sharing repo+branch, got %d", batchSvc.calls)
	}
	if len(batchSvc.reqs[0].Items) != 2 {
		t.Fatalf("expected the single batch call to carry both items, got %d", len(batchSvc.reqs[0].Items))
	}

	doc, _, err := store.Get(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, key := range []string{"sha256:key-1", "sha256:key-2"} {
		op, ok := doc.Operations[key]
		if !ok {
			t.Fatalf("expected an Operations entry for %s", key)
		}
		if op.Status != domain.OperationApplied || op.CommitSHA != "batch-sha" {
			t.Fatalf("expected %s to be applied with the shared batch commit, got %+v", key, op)
		}
	}
}

func TestWorker_ProcessOnce_SplitsSelectionsTargetingDifferentReposIntoSeparateBatches(t *testing.T) {
	store := newStore()
	sel1 := testSelectionFor("sha256:key-1", "https://example.com/repo-a.git", "main")
	sel2 := testSelectionFor("sha256:key-2", "https://example.com/repo-b.git", "main")
	if err := store.Update(context.Background(), func(doc *domain.StateDocument) error {
		doc.PendingSelections = append(doc.PendingSelections, sel1, sel2)
		return nil
	}); err != nil {
		t.Fatalf("unexpected error seeding: %v", err)
	}

	batchSvc := &fakeBatchWriteBackService{}
	w := newWorker(store, batchSvc)
	w.processOnce(context.Background())

	if batchSvc.calls != 2 {
		t.Fatalf("expected 2 separate ApplyBatch calls for selections targeting different repos, got %d", batchSvc.calls)
	}
	for _, req := range batchSvc.reqs {
		if len(req.Items) != 1 {
			t.Fatalf("expected each per-repo batch to carry exactly 1 item, got %d", len(req.Items))
		}
	}
}

func TestWorker_ProcessOnce_BatchReportsPerItemAlreadyApplied(t *testing.T) {
	store := newStore()
	sel1 := testSelectionFor("sha256:key-1", "https://example.com/repo.git", "main")
	sel2 := testSelectionFor("sha256:key-2", "https://example.com/repo.git", "main")
	if err := store.Update(context.Background(), func(doc *domain.StateDocument) error {
		doc.PendingSelections = append(doc.PendingSelections, sel1, sel2)
		return nil
	}); err != nil {
		t.Fatalf("unexpected error seeding: %v", err)
	}

	batchSvc := &fakeBatchWriteBackService{alreadyApplied: map[string]bool{"sha256:key-1": true}}
	w := newWorker(store, batchSvc)
	w.processOnce(context.Background())

	doc, _, err := store.Get(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if doc.Operations["sha256:key-1"].Status != domain.OperationApplied {
		t.Fatalf("expected key-1 to still be recorded as applied even though AlreadyApplied=true, got %+v", doc.Operations["sha256:key-1"])
	}
	// Both items are recorded as domain.OperationApplied regardless of
	// AlreadyApplied -- that flag only affects whether a NEW commit was
	// needed, not whether the dashboard should show the item as done.
	if doc.Operations["sha256:key-2"].Status != domain.OperationApplied {
		t.Fatalf("expected key-2 to be recorded as applied, got %+v", doc.Operations["sha256:key-2"])
	}
}

func TestWorker_ProcessOnce_BatchCredentialsFailureFailsWholeGroup(t *testing.T) {
	store := newStore()
	sel1 := testSelectionFor("sha256:key-1", "https://example.com/repo.git", "main")
	sel2 := testSelectionFor("sha256:key-2", "https://example.com/repo.git", "main")
	if err := store.Update(context.Background(), func(doc *domain.StateDocument) error {
		doc.PendingSelections = append(doc.PendingSelections, sel1, sel2)
		return nil
	}); err != nil {
		t.Fatalf("unexpected error seeding: %v", err)
	}

	batchSvc := &fakeBatchWriteBackService{}
	w := newWorker(store, batchSvc)
	w.Credentials = &fakeCredentialsProvider{err: errors.New("no credentials found")}
	w.processOnce(context.Background())

	if batchSvc.calls != 0 {
		t.Fatalf("expected ApplyBatch never to be called when credential resolution fails, got %d calls", batchSvc.calls)
	}

	doc, _, err := store.Get(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, key := range []string{"sha256:key-1", "sha256:key-2"} {
		if doc.Operations[key].Status != domain.OperationFailed {
			t.Fatalf("expected %s to be marked failed when credentials couldn't be resolved, got %+v", key, doc.Operations[key])
		}
	}
}

func TestWorker_ProcessOnce_BatchApplyErrorFailsWholeGroup(t *testing.T) {
	store := newStore()
	sel1 := testSelectionFor("sha256:key-1", "https://example.com/repo.git", "main")
	sel2 := testSelectionFor("sha256:key-2", "https://example.com/repo.git", "main")
	if err := store.Update(context.Background(), func(doc *domain.StateDocument) error {
		doc.PendingSelections = append(doc.PendingSelections, sel1, sel2)
		return nil
	}); err != nil {
		t.Fatalf("unexpected error seeding: %v", err)
	}

	batchSvc := &fakeBatchWriteBackService{err: gitwriteback.ErrConflict}
	w := newWorker(store, batchSvc)
	w.processOnce(context.Background())

	doc, _, err := store.Get(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, key := range []string{"sha256:key-1", "sha256:key-2"} {
		if doc.Operations[key].Status != domain.OperationConflict {
			t.Fatalf("expected %s to be marked conflict when the batch push conflicts, got %+v", key, doc.Operations[key])
		}
	}
}

func seedSelection(t *testing.T, store statestore.StateStore, sel domain.PendingSelection, headrooms map[string]domain.RequestHeadroom) {
	t.Helper()
	if err := store.Update(context.Background(), func(doc *domain.StateDocument) error {
		doc.PendingSelections = append(doc.PendingSelections, sel)
		doc.RequestHeadrooms = headrooms
		return nil
	}); err != nil {
		t.Fatalf("unexpected error seeding selection: %v", err)
	}
}

func percent(p float64) *float64 { return &p }

func TestWorker_RecordsRequestHeadroomOnlyForAppliedResource(t *testing.T) {
	store := newStore()
	sel := testSelection("sha256:headroom-1")
	sel.CPURequestHeadroom = percent(30)
	key := domain.RequestHeadroomKey(sel.VPANamespace, sel.VPAName, sel.ContainerName)
	// Memory was applied earlier with 20%; this cpu-only selection must keep it.
	seedSelection(t, store, sel, map[string]domain.RequestHeadroom{key: {MemoryPercent: percent(20)}})

	w := newWorker(store, &fakeWriteBackService{result: gitwriteback.WriteBackResult{Status: domain.OperationApplied}})
	w.processOnce(context.Background())

	doc, _, err := store.Get(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	h := doc.RequestHeadrooms[key]
	if h.CPUPercent == nil || *h.CPUPercent != 30 {
		t.Fatalf("expected cpu headroom 30 recorded, got %+v", h.CPUPercent)
	}
	if h.MemoryPercent == nil || *h.MemoryPercent != 20 {
		t.Fatalf("expected memory headroom 20 kept, got %+v", h.MemoryPercent)
	}
}

func TestWorker_RecordsRequestHeadroomWhenAlreadyApplied(t *testing.T) {
	store := newStore()
	sel := testSelection("sha256:headroom-2")
	sel.CPURequestHeadroom = percent(30)
	seedSelection(t, store, sel, nil)

	wb := &fakeBatchWriteBackService{alreadyApplied: map[string]bool{sel.IdempotencyKey: true}}
	newWorker(store, wb).processOnce(context.Background())

	doc, _, err := store.Get(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	h := doc.RequestHeadrooms[domain.RequestHeadroomKey(sel.VPANamespace, sel.VPAName, sel.ContainerName)]
	if h.CPUPercent == nil || *h.CPUPercent != 30 {
		t.Fatalf("expected cpu headroom 30 recorded for an already-applied change, got %+v", h.CPUPercent)
	}
}

func TestWorker_DoesNotRecordRequestHeadroomOnFailure(t *testing.T) {
	store := newStore()
	sel := testSelection("sha256:headroom-3")
	sel.CPURequestHeadroom = percent(30)
	seedSelection(t, store, sel, nil)

	newWorker(store, &fakeWriteBackService{err: errors.New("boom: push failed")}).processOnce(context.Background())

	doc, _, err := store.Get(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(doc.RequestHeadrooms) != 0 {
		t.Fatalf("expected no headroom recorded for a failed write-back, got %+v", doc.RequestHeadrooms)
	}
}

func TestWorker_ApplyingWithoutHeadroomClearsIt(t *testing.T) {
	store := newStore()
	sel := testSelection("sha256:headroom-4")
	key := domain.RequestHeadroomKey(sel.VPANamespace, sel.VPAName, sel.ContainerName)
	seedSelection(t, store, sel, map[string]domain.RequestHeadroom{key: {CPUPercent: percent(30)}})

	newWorker(store, &fakeWriteBackService{result: gitwriteback.WriteBackResult{Status: domain.OperationApplied}}).processOnce(context.Background())

	doc, _, err := store.Get(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, ok := doc.RequestHeadrooms[key]; ok {
		t.Fatalf("expected the headroom entry to be removed, got %+v", doc.RequestHeadrooms[key])
	}
}
