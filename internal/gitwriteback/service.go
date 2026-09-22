// Package gitwriteback applies a selected VPA recommendation to a Git
// repository: branch, commit (or push a topic branch for a pull/merge
// request), idempotently and without ever force-overwriting a conflicting
// concurrent change.
package gitwriteback

import (
	"context"
	"errors"

	"github.com/azurebrasil/argocd-vpa-updater/internal/domain"
	"github.com/azurebrasil/argocd-vpa-updater/internal/patcher"
)

// ErrConflict is returned when the target branch changed since the base
// used to compute the patch, in a way that can't be safely reconciled.
// Apply never force-pushes and never partially writes when this occurs.
var ErrConflict = errors.New("gitwriteback: target changed since base was read; refusing to force")

// WriteBackRequest is everything GitWriteBackService.Apply needs to perform
// one write-back operation. It embeds domain.PatchRequest (target, the
// recommendation, which resources to apply, optional overrides, and the
// idempotency key computed by internal/idempotency).
type WriteBackRequest struct {
	domain.PatchRequest

	// CommitMessage is the human-readable summary; the idempotency trailer
	// is appended by the implementation, callers do not need to include it.
	CommitMessage string

	// Credentials authenticate against Target.RepoURL. Obtained from a
	// credentials.RepositoryCredentialsProvider by the caller.
	Credentials domain.GitCredentials

	// Patcher performs the actual file edit; resolved by the caller from a
	// patcher.ManifestPatcherRegistry based on Target.SourceType.
	Patcher patcher.ManifestPatcher
}

// WriteBackResult is the outcome of a successful (non-error) Apply call.
type WriteBackResult struct {
	Status domain.OperationStatus // OperationApplied on success

	// AlreadyApplied is true when this exact IdempotencyKey was already
	// applied (or the requested values already match the file) and no new
	// commit was made.
	AlreadyApplied bool

	Branch    string
	CommitSHA string

	// PRURL is set only when Target.WriteBackPolicy is WriteBackPullRequest.
	PRURL string
}

// GitWriteBackService applies one resolved recommendation to Git.
type GitWriteBackService interface {
	// Apply is idempotent on req.IdempotencyKey: replaying the same request
	// never produces a duplicate commit. It never touches content outside
	// req.Target.FilePath's declared key paths, and never force-pushes over
	// a conflicting concurrent change -- see ErrConflict.
	Apply(ctx context.Context, req WriteBackRequest) (WriteBackResult, error)
}
