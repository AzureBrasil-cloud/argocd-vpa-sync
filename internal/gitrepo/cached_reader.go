package gitrepo

import (
	"context"
	"sync"
	"time"

	"github.com/azurebrasil/argocd-vpa-updater/internal/domain"
)

// CachedReader wraps a Reader with a short-TTL cache keyed by
// repo+branch+path, so a caller reading the same file repeatedly (e.g.
// write-back re-reading a file it's about to patch) doesn't trigger a fresh
// clone every time. It deliberately does not share a cache across
// different repo URLs' credentials -- the key includes repoURL, so a
// credentials change takes effect on the next natural expiry rather than
// needing explicit invalidation.
type CachedReader struct {
	inner Reader
	ttl   time.Duration

	mu      sync.Mutex
	entries map[string]cacheEntry

	// now is overridable for deterministic tests; defaults to time.Now.
	now func() time.Time
}

type cacheEntry struct {
	content   []byte
	err       error
	expiresAt time.Time
}

// NewCachedReader wraps inner with a cache of the given TTL.
func NewCachedReader(inner Reader, ttl time.Duration) *CachedReader {
	return &CachedReader{inner: inner, ttl: ttl, entries: map[string]cacheEntry{}, now: time.Now}
}

func (c *CachedReader) ReadFile(ctx context.Context, repoURL, branch, path string, creds domain.GitCredentials) ([]byte, error) {
	key := repoURL + "@" + branch + ":" + path

	c.mu.Lock()
	if e, ok := c.entries[key]; ok && c.now().Before(e.expiresAt) {
		c.mu.Unlock()
		return e.content, e.err
	}
	c.mu.Unlock()

	content, err := c.inner.ReadFile(ctx, repoURL, branch, path, creds)

	c.mu.Lock()
	c.entries[key] = cacheEntry{content: content, err: err, expiresAt: c.now().Add(c.ttl)}
	c.mu.Unlock()

	return content, err
}
