package vparecommendation

import (
	"context"

	"github.com/azurebrasil/argocd-vpa-updater/internal/domain"
)

// CacheReader is the real VpaRecommendationReader implementation: a thin
// accessor over a Cache the controller's reconciler keeps populated. It
// never talks to the Kubernetes API server itself.
type CacheReader struct {
	cache *Cache
}

// NewCacheReader builds a CacheReader backed by cache.
func NewCacheReader(cache *Cache) *CacheReader {
	return &CacheReader{cache: cache}
}

func (r *CacheReader) ListOptedIn(_ context.Context) ([]domain.NormalizedVPA, error) {
	return r.cache.List(), nil
}
