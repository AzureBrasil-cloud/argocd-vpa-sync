package credentials

import (
	"context"
	"errors"
	"testing"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	"github.com/azurebrasil/argocd-vpa-updater/internal/domain"
)

func newFakeClient(objs ...runtime.Object) *fake.ClientBuilder {
	scheme := runtime.NewScheme()
	_ = clientgoscheme.AddToScheme(scheme)
	return fake.NewClientBuilder().WithScheme(scheme).WithRuntimeObjects(objs...)
}

func repoSecret(name, url, username, password string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "argocd",
			Labels:    map[string]string{labelSecretType: secretTypeRepository},
		},
		Data: map[string][]byte{
			"url":      []byte(url),
			"username": []byte(username),
			"password": []byte(password),
		},
	}
}

func repoCredsSecret(name, urlPrefix, username, password string) *corev1.Secret {
	return &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: "argocd",
			Labels:    map[string]string{labelSecretType: secretTypeRepoCreds},
		},
		Data: map[string][]byte{
			"url":      []byte(urlPrefix),
			"username": []byte(username),
			"password": []byte(password),
		},
	}
}

func TestArgoCDSecretProvider_ExactRepositoryMatch(t *testing.T) {
	c := newFakeClient(
		repoSecret("repo-payments", "https://git.example.com/team/payments.git", "git", "token-1"),
	).Build()

	p := NewArgoCDSecretProvider(c, "argocd")
	creds, err := p.GetCredentials(context.Background(), "https://git.example.com/team/payments.git")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creds.AuthMethod != domain.AuthBasic || creds.Username != "git" || creds.Secret != "token-1" {
		t.Fatalf("unexpected credentials: %+v", creds)
	}
}

func TestArgoCDSecretProvider_ExactMatchIgnoresTrailingSlashAndGitSuffix(t *testing.T) {
	c := newFakeClient(
		repoSecret("repo-payments", "https://git.example.com/team/payments", "git", "token-1"),
	).Build()

	p := NewArgoCDSecretProvider(c, "argocd")
	creds, err := p.GetCredentials(context.Background(), "https://git.example.com/team/payments.git/")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creds.Secret != "token-1" {
		t.Fatalf("expected normalized match, got %+v", creds)
	}
}

func TestArgoCDSecretProvider_RepoCredsPrefixFallback(t *testing.T) {
	c := newFakeClient(
		repoCredsSecret("org-creds", "https://git.example.com/team/", "git", "org-token"),
	).Build()

	p := NewArgoCDSecretProvider(c, "argocd")
	creds, err := p.GetCredentials(context.Background(), "https://git.example.com/team/payments.git")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creds.Secret != "org-token" {
		t.Fatalf("expected repo-creds fallback, got %+v", creds)
	}
}

func TestArgoCDSecretProvider_ExactMatchWinsOverPrefix(t *testing.T) {
	c := newFakeClient(
		repoCredsSecret("org-creds", "https://git.example.com/team/", "git", "org-token"),
		repoSecret("repo-payments", "https://git.example.com/team/payments.git", "git", "specific-token"),
	).Build()

	p := NewArgoCDSecretProvider(c, "argocd")
	creds, err := p.GetCredentials(context.Background(), "https://git.example.com/team/payments.git")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creds.Secret != "specific-token" {
		t.Fatalf("expected exact repository secret to win over repo-creds prefix, got %+v", creds)
	}
}

func TestArgoCDSecretProvider_LongestPrefixWins(t *testing.T) {
	c := newFakeClient(
		repoCredsSecret("org-creds", "https://git.example.com/", "git", "org-token"),
		repoCredsSecret("team-creds", "https://git.example.com/team/", "git", "team-token"),
	).Build()

	p := NewArgoCDSecretProvider(c, "argocd")
	creds, err := p.GetCredentials(context.Background(), "https://git.example.com/team/payments.git")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if creds.Secret != "team-token" {
		t.Fatalf("expected longest-prefix match to win, got %+v", creds)
	}
}

func TestArgoCDSecretProvider_NotFound(t *testing.T) {
	c := newFakeClient().Build()
	p := NewArgoCDSecretProvider(c, "argocd")
	_, err := p.GetCredentials(context.Background(), "https://git.example.com/team/payments.git")
	if !errors.Is(err, ErrCredentialsNotFound) {
		t.Fatalf("expected ErrCredentialsNotFound, got %v", err)
	}
}

func TestArgoCDSecretProvider_IgnoresUnrelatedSecrets(t *testing.T) {
	unrelated := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{Name: "state", Namespace: "argocd-vpa-updater"},
		Data:       map[string][]byte{"state.json": []byte("{}")},
	}
	c := newFakeClient(unrelated).Build()
	p := NewArgoCDSecretProvider(c, "argocd")
	_, err := p.GetCredentials(context.Background(), "https://git.example.com/team/payments.git")
	if !errors.Is(err, ErrCredentialsNotFound) {
		t.Fatalf("expected ErrCredentialsNotFound for a namespace with only unrelated secrets, got %v", err)
	}
}

func TestGitCredentials_NeverSerializesSecretMaterial(t *testing.T) {
	creds := domain.GitCredentials{AuthMethod: domain.AuthBasic, Username: "git", Secret: "super-secret-token"}
	if got := creds.String(); got == creds.Secret || len(got) == 0 || got != "[REDACTED]" {
		t.Fatalf("String() must be redacted, got %q", got)
	}
	b, err := creds.MarshalJSON()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(b) != `"[REDACTED]"` {
		t.Fatalf("MarshalJSON must be redacted, got %s", b)
	}
}
