// Package credentials resolves Git repository credentials without ever
// duplicating or persisting a separate copy of them: the default
// implementation reads the credential Secrets Argo CD itself already
// manages.
package credentials

import (
	"context"
	"errors"

	"github.com/azurebrasil/argocd-vpa-updater/internal/domain"
)

// ErrCredentialsNotFound is returned when no usable credential Secret could
// be located for a repository URL.
var ErrCredentialsNotFound = errors.New("credentials: no repository credentials found")

// RepositoryCredentialsProvider resolves the credentials needed to read from
// and write to a Git repository. Implementations must never log or persist
// the returned domain.GitCredentials beyond the lifetime of the operation
// that requested them.
type RepositoryCredentialsProvider interface {
	GetCredentials(ctx context.Context, repoURL string) (domain.GitCredentials, error)
}
