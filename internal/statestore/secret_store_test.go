package statestore

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/azurebrasil/argocd-vpa-updater/internal/domain"
)

const (
	testNamespace = "argocd-vpa-updater"
	testName      = "argocd-vpa-updater-state"
)

func newFakeClient(objs ...runtime.Object) *fake.ClientBuilder {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	return fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...)
}

func getSecret(t *testing.T, c client.Client) (corev1.Secret, error) {
	t.Helper()
	var secret corev1.Secret
	err := c.Get(context.Background(), client.ObjectKey{Namespace: testNamespace, Name: testName}, &secret)
	return secret, err
}

func TestSecretStore_GetOnMissingSecretReturnsEmptyDocument(t *testing.T) {
	c := newFakeClient().Build()
	store := NewSecretStore(c, testNamespace, testName)

	doc, version, err := store.Get(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if version != "" {
		t.Fatalf("expected empty version token for a missing secret, got %q", version)
	}
	if doc.SchemaVersion != domain.CurrentStateSchemaVersion {
		t.Fatalf("expected a valid empty document, got %+v", doc)
	}
}

func TestSecretStore_UpdateCreatesSecretWhenMissing(t *testing.T) {
	c := newFakeClient().Build()
	store := NewSecretStore(c, testNamespace, testName)

	err := store.Update(context.Background(), func(doc *domain.StateDocument) error {
		doc.PendingSelections = append(doc.PendingSelections, domain.PendingSelection{
			IdempotencyKey: "sha256:abc",
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

	doc, version, err := store.Get(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if version == "" {
		t.Fatalf("expected a non-empty resourceVersion after creation")
	}
	if len(doc.PendingSelections) != 1 {
		t.Fatalf("expected the pending selection to persist, got %+v", doc.PendingSelections)
	}
}

func TestSecretStore_UpdateIsReadModifyWrite(t *testing.T) {
	c := newFakeClient().Build()
	store := NewSecretStore(c, testNamespace, testName)

	addSelection := func(key string) {
		err := store.Update(context.Background(), func(doc *domain.StateDocument) error {
			doc.PendingSelections = append(doc.PendingSelections, domain.PendingSelection{
				IdempotencyKey: key,
				SelectedAt:     time.Now(),
			})
			return nil
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
	}

	addSelection("first")
	addSelection("second")

	doc, _, err := store.Get(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(doc.PendingSelections) != 2 {
		t.Fatalf("expected both selections to accumulate, got %+v", doc.PendingSelections)
	}
}

func TestSecretStore_UpdatePropagatesMutateErrorAndWritesNothing(t *testing.T) {
	c := newFakeClient().Build()
	store := NewSecretStore(c, testNamespace, testName)

	sentinel := errors.New("boom")
	err := store.Update(context.Background(), func(doc *domain.StateDocument) error {
		return sentinel
	})
	if !errors.Is(err, sentinel) {
		t.Fatalf("expected the mutate error to propagate, got %v", err)
	}

	if _, getErr := getSecret(t, c); getErr == nil {
		t.Fatalf("expected no secret to have been created on a failed mutate")
	}
}

func TestSecretStore_UpdateAppliesCompaction(t *testing.T) {
	c := newFakeClient().Build()
	store := NewSecretStore(c, testNamespace, testName)
	store.RetentionPolicy = RetentionPolicy{PendingSelectionTTL: time.Hour}

	err := store.Update(context.Background(), func(doc *domain.StateDocument) error {
		doc.PendingSelections = append(doc.PendingSelections, domain.PendingSelection{
			IdempotencyKey: "stale",
			SelectedAt:     time.Now().Add(-48 * time.Hour),
		})
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	doc, _, err := store.Get(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(doc.PendingSelections) != 0 {
		t.Fatalf("expected compaction to drop the stale selection, got %+v", doc.PendingSelections)
	}
}

func TestSecretStore_UpdateRefusesWhenOverCapacityEvenAfterCompaction(t *testing.T) {
	c := newFakeClient().Build()
	store := NewSecretStore(c, testNamespace, testName)
	store.RetentionPolicy = RetentionPolicy{} // compacts nothing away

	bigMessage := strings.Repeat("x", maxSizeBytes)
	err := store.Update(context.Background(), func(doc *domain.StateDocument) error {
		doc.Operations["key-1"] = domain.OperationState{
			IdempotencyKey: "key-1",
			ErrorMessage:   bigMessage,
			UpdatedAt:      time.Now(),
		}
		return nil
	})
	if !errors.Is(err, ErrNearCapacity) {
		t.Fatalf("expected ErrNearCapacity, got %v", err)
	}

	if _, getErr := getSecret(t, c); getErr == nil {
		t.Fatalf("expected no secret to have been written when over capacity")
	}
}

func TestSecretStore_OnNearCapacityCallback(t *testing.T) {
	c := newFakeClient().Build()
	store := NewSecretStore(c, testNamespace, testName)
	store.RetentionPolicy = RetentionPolicy{}

	var reportedSize int
	store.OnNearCapacity = func(sizeBytes int) { reportedSize = sizeBytes }

	msg := strings.Repeat("x", warnSizeBytes)
	err := store.Update(context.Background(), func(doc *domain.StateDocument) error {
		doc.Operations["key-1"] = domain.OperationState{IdempotencyKey: "key-1", ErrorMessage: msg, UpdatedAt: time.Now()}
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if reportedSize == 0 {
		t.Fatalf("expected OnNearCapacity to be called with a non-zero size")
	}
}

func TestSecretStore_NeverStoresCredentialFields(t *testing.T) {
	// domain.OperationState simply has no field for credential material; this
	// test documents that guarantee by asserting the persisted JSON never
	// contains a recognizable secret-shaped key, even when an error message
	// mentions authentication.
	c := newFakeClient().Build()
	store := NewSecretStore(c, testNamespace, testName)

	err := store.Update(context.Background(), func(doc *domain.StateDocument) error {
		doc.Operations["key-1"] = domain.OperationState{
			IdempotencyKey: "key-1",
			ErrorMessage:   "authentication failed",
			UpdatedAt:      time.Now(),
		}
		return nil
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	secret, err := getSecret(t, c)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	raw := string(secret.Data[stateDataKey])
	for _, forbidden := range []string{`"password"`, `"token"`, `"sshPrivateKey"`, `"secret"`} {
		if strings.Contains(strings.ToLower(raw), forbidden) {
			t.Fatalf("state document unexpectedly contains %q: %s", forbidden, raw)
		}
	}
}
