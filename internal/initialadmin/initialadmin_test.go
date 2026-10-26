package initialadmin

import (
	"context"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/azurebrasil/argocd-vpa-updater/internal/auth"
)

const (
	testNamespace = "argocd"
	testName      = "argocd-vpa-sync-initial-admin-secret"
)

func newFakeClient(objs ...runtime.Object) client.Client {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	return fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...).Build()
}

func TestEnsure_CreatesSecretWithMatchingHash(t *testing.T) {
	c := newFakeClient()

	creds, created, err := Ensure(context.Background(), c, testNamespace, testName)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !created {
		t.Fatal("expected the secret to be created")
	}

	var secret corev1.Secret
	if err := c.Get(context.Background(), client.ObjectKey{Namespace: testNamespace, Name: testName}, &secret); err != nil {
		t.Fatalf("get secret: %v", err)
	}
	password := string(secret.Data[PasswordKey])
	if len(password) != passwordLength {
		t.Fatalf("expected a %d-character password, got %q", passwordLength, password)
	}
	hash, err := auth.ParsePasswordHash(creds.PasswordHash)
	if err != nil {
		t.Fatalf("generated hash doesn't parse: %v", err)
	}
	if !hash.Matches(password) {
		t.Fatal("generated hash doesn't match the stored password")
	}
	if len(creds.SigningKey) < auth.MinSigningKeyLength {
		t.Fatalf("signing key too short: %d", len(creds.SigningKey))
	}
}

func TestEnsure_ReusesExistingSecret(t *testing.T) {
	c := newFakeClient()
	first, _, err := Ensure(context.Background(), c, testNamespace, testName)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	second, created, err := Ensure(context.Background(), c, testNamespace, testName)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if created {
		t.Fatal("expected the existing secret to be reused")
	}
	if second != first {
		t.Fatalf("credentials changed across starts: %+v != %+v", second, first)
	}
}

func TestEnsure_WorksWithoutPlaintextPassword(t *testing.T) {
	c := newFakeClient(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: testNamespace, Name: testName},
		Data:       map[string][]byte{PasswordHashKey: []byte("$2a$10$abc")},
	})

	creds, created, err := Ensure(context.Background(), c, testNamespace, testName)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if created || creds.PasswordHash != "$2a$10$abc" || creds.SigningKey != "" {
		t.Fatalf("unexpected result: created=%v creds=%+v", created, creds)
	}
}

func TestEnsure_RejectsSecretWithoutHash(t *testing.T) {
	c := newFakeClient(&corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Namespace: testNamespace, Name: testName},
		Data:       map[string][]byte{PasswordKey: []byte("pw")},
	})

	if _, _, err := Ensure(context.Background(), c, testNamespace, testName); err == nil {
		t.Fatal("expected an error for a secret without a password hash")
	}
}
