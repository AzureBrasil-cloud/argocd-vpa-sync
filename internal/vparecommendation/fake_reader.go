package vparecommendation

import (
	"context"

	"github.com/azurebrasil/argocd-vpa-updater/internal/domain"
)

// FakeReader is a static VpaRecommendationReader for tests.
type FakeReader struct {
	VPAs []domain.NormalizedVPA
	Err  error
}

func (r FakeReader) ListOptedIn(_ context.Context) ([]domain.NormalizedVPA, error) {
	return r.VPAs, r.Err
}
