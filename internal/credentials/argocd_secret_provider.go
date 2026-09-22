package credentials

import (
	"context"
	"fmt"
	"strings"

	corev1 "k8s.io/api/core/v1"
	"sigs.k8s.io/controller-runtime/pkg/client"

	"github.com/azurebrasil/argocd-vpa-updater/internal/domain"
)

// These match the labels and Secret data keys Argo CD itself uses for
// repository credentials (see argocd-image-updater's git credential
// resolution, which this mirrors). We read the same Secrets rather than
// asking users to duplicate a token into a new one.
const (
	labelSecretType      = "argocd.argoproj.io/secret-type"
	secretTypeRepository = "repository"
	secretTypeRepoCreds  = "repo-creds"
)

// ArgoCDSecretProvider is the default RepositoryCredentialsProvider: it
// looks for a matching Secret in the Argo CD namespace among the Secrets
// already labeled by Argo CD as repository credentials. It never creates,
// modifies or caches a copy of the credential material beyond one lookup.
//
// Match precedence mirrors Argo CD's own: an exact "repository"-type Secret
// for this URL wins; otherwise the "repo-creds"-type Secret whose url is the
// longest matching prefix of repoURL is used.
type ArgoCDSecretProvider struct {
	Client          client.Client
	ArgoCDNamespace string
}

// NewArgoCDSecretProvider builds an ArgoCDSecretProvider. The RBAC granted
// to this controller's ServiceAccount must scope `client`'s access to only
// the Secrets in argoCDNamespace (see deploy/manifests) -- this type itself
// applies no additional filtering beyond the label/prefix match below, so it
// relies on RBAC, not on its own logic, to keep reads scoped.
func NewArgoCDSecretProvider(c client.Client, argoCDNamespace string) *ArgoCDSecretProvider {
	return &ArgoCDSecretProvider{Client: c, ArgoCDNamespace: argoCDNamespace}
}

func (p *ArgoCDSecretProvider) GetCredentials(ctx context.Context, repoURL string) (domain.GitCredentials, error) {
	var list corev1.SecretList
	if err := p.Client.List(ctx, &list, client.InNamespace(p.ArgoCDNamespace)); err != nil {
		return domain.GitCredentials{}, fmt.Errorf("credentials: list secrets in namespace %q: %w", p.ArgoCDNamespace, err)
	}

	target := normalizeRepoURL(repoURL)

	for _, s := range list.Items {
		if s.Labels[labelSecretType] != secretTypeRepository {
			continue
		}
		if normalizeRepoURL(string(s.Data["url"])) == target {
			return credentialsFromSecret(s)
		}
	}

	var best *corev1.Secret
	bestLen := -1
	for i := range list.Items {
		s := &list.Items[i]
		if s.Labels[labelSecretType] != secretTypeRepoCreds {
			continue
		}
		prefix := string(s.Data["url"])
		if prefix != "" && strings.HasPrefix(repoURL, prefix) && len(prefix) > bestLen {
			best = s
			bestLen = len(prefix)
		}
	}
	if best != nil {
		return credentialsFromSecret(*best)
	}

	return domain.GitCredentials{}, fmt.Errorf("%w: %s", ErrCredentialsNotFound, repoURL)
}

func credentialsFromSecret(s corev1.Secret) (domain.GitCredentials, error) {
	if key := s.Data["sshPrivateKey"]; len(key) > 0 {
		return domain.GitCredentials{
			AuthMethod:    domain.AuthSSH,
			SSHPrivateKey: key,
			SSHKnownHosts: s.Data["sshKnownHosts"],
		}, nil
	}
	if token := s.Data["password"]; len(token) > 0 {
		return domain.GitCredentials{
			AuthMethod: domain.AuthBasic,
			Username:   string(s.Data["username"]),
			Secret:     string(token),
		}, nil
	}
	return domain.GitCredentials{}, fmt.Errorf("credentials: secret %s/%s has no recognized auth field (sshPrivateKey or password)", s.Namespace, s.Name)
}

func normalizeRepoURL(u string) string {
	u = strings.TrimSpace(u)
	u = strings.TrimSuffix(u, "/")
	u = strings.TrimSuffix(u, ".git")
	u = strings.TrimSuffix(u, "/")
	return strings.ToLower(u)
}
