package vparecommendation

import (
	"sync"

	"github.com/azurebrasil/argocd-vpa-updater/internal/domain"
)

// Cache is a thread-safe, in-memory snapshot of every opted-in VPA's
// normalized state, kept up to date by the controller's reconciler (see
// internal/controller) and read by CacheReader. It intentionally holds only
// what the reconciler has already computed -- no locking or Kubernetes
// client access happens on the read path, so dashboard requests never wait
// on the API server.
type Cache struct {
	mu    sync.RWMutex
	items map[string]domain.NormalizedVPA
}

// NewCache builds an empty Cache.
func NewCache() *Cache {
	return &Cache{items: map[string]domain.NormalizedVPA{}}
}

// Set stores (or replaces) the normalized state under (namespace, name).
// This key identifies the VpaGitOpsBinding that produced the entry, not the
// VPA it describes (domain.NormalizedVPA.Namespace/Name carry the VPA's own
// identity, which the dashboard displays/queries by) -- keeping the cache
// key independent of the stored value's own identity fields is what lets
// Delete evict the right entry even when the VPA lookup itself failed (e.g.
// a VpaGitOpsBinding whose referenced VPA was deleted or renamed).
func (c *Cache) Set(namespace, name string, vpa domain.NormalizedVPA) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.items[cacheKey(namespace, name)] = vpa
}

// Delete removes the entry for (namespace, name), e.g. because the
// VpaGitOpsBinding was deleted or its referenced VPA no longer exists.
func (c *Cache) Delete(namespace, name string) {
	c.mu.Lock()
	defer c.mu.Unlock()
	delete(c.items, cacheKey(namespace, name))
}

// Get returns the entry stored under (namespace, name), if present.
func (c *Cache) Get(namespace, name string) (domain.NormalizedVPA, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	v, ok := c.items[cacheKey(namespace, name)]
	return v, ok
}

// List returns every entry currently in the cache, in no particular order.
func (c *Cache) List() []domain.NormalizedVPA {
	c.mu.RLock()
	defer c.mu.RUnlock()
	out := make([]domain.NormalizedVPA, 0, len(c.items))
	for _, v := range c.items {
		out = append(out, v)
	}
	return out
}

func cacheKey(namespace, name string) string {
	return namespace + "/" + name
}
