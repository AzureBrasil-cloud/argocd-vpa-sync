package credentials

import (
	"context"

	"github.com/azurebrasil/argocd-vpa-updater/internal/domain"
)

// StaticProvider returns a fixed GitCredentials value for every repo URL.
// Used in tests, and for local file:// repositories where no real
// authentication is needed.
type StaticProvider struct {
	Credentials domain.GitCredentials
}

// NewNoAuthProvider returns a StaticProvider suitable for local file://
// repositories, which need no authentication at all.
func NewNoAuthProvider() StaticProvider {
	return StaticProvider{Credentials: domain.GitCredentials{AuthMethod: domain.AuthNone}}
}

func (p StaticProvider) GetCredentials(_ context.Context, _ string) (domain.GitCredentials, error) {
	return p.Credentials, nil
}
