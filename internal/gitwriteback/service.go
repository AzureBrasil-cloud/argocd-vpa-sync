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

// BatchItem is one selection's contribution to a BatchWriteBackRequest --
// everything WriteBackRequest carries except the fields that are shared
// across the whole batch (RepoURL/Branch/WriteBackPolicy/Credentials, all on
// BatchWriteBackRequest itself, since every item in one batch targets the
// same repo/branch by construction -- see the caller's grouping).
type BatchItem struct {
	Target         domain.WriteTarget
	Recommendation domain.ContainerRecommendation

	ApplyCPU    bool
	ApplyMemory bool

	OverrideCPU    *domain.ResourceAmount
	OverrideMemory *domain.ResourceAmount

	CPULimit    *domain.LimitSpec
	MemoryLimit *domain.LimitSpec

	IdempotencyKey string

	// Patcher performs this item's file edit; resolved by the caller from a
	// patcher.ManifestPatcherRegistry based on Target.SourceType. Items in
	// the same batch may use different patchers (e.g. one YAML, one
	// Helm-values file in the same repo).
	Patcher patcher.ManifestPatcher
}

// BatchWriteBackRequest groups multiple selections that should land in a
// single commit: all Items must share the same destination repo, branch,
// and write-back policy (the caller is responsible for that grouping --
// see internal/writebackworker).
type BatchWriteBackRequest struct {
	RepoURL         string
	Branch          string
	WriteBackPolicy domain.WriteBackPolicy
	Credentials     domain.GitCredentials

	Items []BatchItem

	// CommitMessage is the human-readable summary; if empty, a default is
	// built from the distinct workload names across Items. Each item's own
	// idempotency trailer is appended by the implementation.
	CommitMessage string
}

// ItemOutcome is one BatchItem's result within a BatchWriteBackResult.
type ItemOutcome struct {
	IdempotencyKey string

	// AlreadyApplied is true when this item's IdempotencyKey was already
	// present in the target branch's history (a previous batch, possibly a
	// different grouping, already applied it), or its requested value
	// already matched the file -- either way, this item contributed nothing
	// to a new commit.
	AlreadyApplied bool

	// CommitSHA is the commit (new or historical) that contains this item's
	// change.
	CommitSHA string
}

// BatchWriteBackResult is the outcome of a successful (non-error) ApplyBatch
// call.
type BatchWriteBackResult struct {
	Status domain.OperationStatus // OperationApplied on success

	// AlreadyApplied is true only when EVERY item was already applied --
	// i.e. no new commit was made at all.
	AlreadyApplied bool

	Branch    string
	CommitSHA string
	PRURL     string

	// Items carries one ItemOutcome per BatchWriteBackRequest.Items, in the
	// same order.
	Items []ItemOutcome
}

// BatchGitWriteBackService is implemented by a GitWriteBackService that can
// combine several selections into a single commit instead of one commit per
// selection. Not every implementation needs to support this (e.g.
// FakeGitWriteBackService doesn't); callers that want batching type-assert
// for this interface and fall back to per-item Apply calls when a
// GitWriteBackService doesn't implement it.
type BatchGitWriteBackService interface {
	// ApplyBatch is idempotent per-item on each Items[i].IdempotencyKey: an
	// item already found in the target branch's history (from this or an
	// earlier batch) is skipped, not reapplied or duplicated. It never
	// partially pushes -- either every newly-needed change lands in one
	// commit, or (on ErrConflict) none of them do.
	ApplyBatch(ctx context.Context, req BatchWriteBackRequest) (BatchWriteBackResult, error)
}
