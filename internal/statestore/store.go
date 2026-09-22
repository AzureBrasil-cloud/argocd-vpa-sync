// Package statestore persists argocd-vpa-updater's small operational state
// (pending selections, applied/failed write-back operations) without any
// external database, by default in a single Kubernetes Secret.
package statestore

import (
	"context"
	"errors"

	"github.com/azurebrasil/argocd-vpa-updater/internal/domain"
)

// ErrOptimisticLockConflict is returned by Update when the underlying
// storage's version token changed between Get and write, and retries (if
// any) were exhausted.
var ErrOptimisticLockConflict = errors.New("statestore: version conflict, retry")

// ErrNearCapacity is returned by Update when the document, even after
// compaction, is too close to the underlying storage's size limit to write
// safely. Retention settings need tightening, or the caller should reduce
// what it is trying to record.
var ErrNearCapacity = errors.New("statestore: state document is nearing the storage size limit even after compaction")

// StateStore persists a single domain.StateDocument. Implementations must
// never store credential material, and must retain only recent/bounded
// history (see Compact in compaction.go) so the document never grows
// unbounded.
//
// The default implementation (SecretStore) backs this with a single
// Kubernetes Secret; this interface exists so it can be swapped for e.g. a
// PostgreSQL-backed implementation later without changing any business
// logic that calls it.
type StateStore interface {
	// Get returns the current document and an opaque storage-native version
	// token (a Kubernetes Secret's resourceVersion for SecretStore), for
	// diagnostic use; most callers should use Update instead of hand-rolling
	// a read-modify-write.
	Get(ctx context.Context) (*domain.StateDocument, string, error)

	// Update reads the current document, applies mutate, compacts it (see
	// compaction.go), and writes it back using optimistic concurrency. It
	// retries internally on version conflicts (bounded); ErrOptimisticLockConflict
	// is returned only once retries are exhausted. If mutate returns an
	// error, Update returns it unchanged and writes nothing.
	Update(ctx context.Context, mutate func(*domain.StateDocument) error) error
}
