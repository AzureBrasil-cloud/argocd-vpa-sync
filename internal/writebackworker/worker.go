// Package writebackworker drains domain.StateDocument.PendingSelections
// (queued by the dashboard's "Apply" action, see internal/api) and performs
// the actual Git write-back for each one via a
// gitwriteback.GitWriteBackService, recording the outcome as a
// domain.OperationState so the dashboard can show it.
package writebackworker

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sort"
	"time"

	"github.com/azurebrasil/argocd-vpa-updater/internal/credentials"
	"github.com/azurebrasil/argocd-vpa-updater/internal/domain"
	"github.com/azurebrasil/argocd-vpa-updater/internal/gitwriteback"
	"github.com/azurebrasil/argocd-vpa-updater/internal/patcher"
	"github.com/azurebrasil/argocd-vpa-updater/internal/statestore"
)

const (
	// DefaultPollInterval is used when Worker.PollInterval is zero.
	DefaultPollInterval = 20 * time.Second

	// applyingStaleAfter bounds how long an OperationState may sit at
	// Status: applying before processOnce assumes the instance that claimed
	// it crashed mid-flight and gives up on it, rather than leaving the
	// dashboard stuck showing "applying" forever.
	applyingStaleAfter = 5 * time.Minute
)

// Worker periodically drains PendingSelections, applying each one via
// WriteBack and writing the terminal OperationState back to State. It
// implements manager.Runnable (Start) and manager.LeaderElectionRunnable
// (NeedLeaderElection) without importing controller-runtime directly, so it
// shares the controller manager's context and shutdown ordering when
// registered via mgr.Add. StateStore's RBAC only grants get/update (never
// watch) on its one named Secret, so polling -- not a watch -- is the only
// option regardless of how this is wired up.
type Worker struct {
	State       statestore.StateStore
	WriteBack   gitwriteback.GitWriteBackService
	Credentials credentials.RepositoryCredentialsProvider
	Patchers    patcher.ManifestPatcherRegistry
	Logger      *slog.Logger

	// PollInterval defaults to DefaultPollInterval when zero.
	PollInterval time.Duration

	// Now is overridable for deterministic tests; defaults to time.Now.
	Now func() time.Time
}

func (w *Worker) now() time.Time {
	if w.Now != nil {
		return w.Now()
	}
	return time.Now()
}

func (w *Worker) pollInterval() time.Duration {
	if w.PollInterval > 0 {
		return w.PollInterval
	}
	return DefaultPollInterval
}

func (w *Worker) logger() *slog.Logger {
	if w.Logger != nil {
		return w.Logger
	}
	return slog.Default()
}

// NeedLeaderElection reports true: if this process ever runs with more than
// one replica and the controller manager enables leader election, only the
// leader should be draining PendingSelections. Inert today (the manager
// doesn't enable leader election), but costs nothing to declare correctly.
func (w *Worker) NeedLeaderElection() bool { return true }

// Start ticks every PollInterval, calling processOnce, until ctx is
// canceled.
func (w *Worker) Start(ctx context.Context) error {
	ticker := time.NewTicker(w.pollInterval())
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return nil
		case <-ticker.C:
			w.processOnce(ctx)
		}
	}
}

// processOnce runs one full poll tick:
//  1. sweep any OperationState stuck at Status: applying for longer than
//     applyingStaleAfter (the instance that claimed it crashed mid-flight) --
//     marked OperationFailed rather than silently retried, since replaying is
//     the user's decision (and safe: a genuine post-crash replay is caught by
//     GitWriteBackService's own idempotency-trailer scan).
//  2. claim every remaining PendingSelection not already owned by an
//     in-flight attempt, in one atomic StateStore.Update: each moves from
//     PendingSelections into Operations[key] at Status: applying. This
//     single atomic transition is what flips the dashboard from "selected"
//     to "applying" (internal/api's statusFor checks PendingSelections
//     before Operations).
//  3. apply each freshly claimed selection outside the lock (the git
//     operation itself can take seconds), then write its terminal
//     OperationState back in a second, independent Update call.
func (w *Worker) processOnce(ctx context.Context) {
	var claimed []domain.PendingSelection

	err := w.State.Update(ctx, func(doc *domain.StateDocument) error {
		claimed = nil
		now := w.now()

		for key, op := range doc.Operations {
			if op.Status == domain.OperationApplying && now.Sub(op.UpdatedAt) > applyingStaleAfter {
				op.Status = domain.OperationFailed
				op.ErrorMessage = "apply did not complete before the previous instance restarted; please re-select"
				op.UpdatedAt = now
				doc.Operations[key] = op
			}
		}

		var remaining []domain.PendingSelection
		for _, sel := range doc.PendingSelections {
			if existing, ok := doc.Operations[sel.IdempotencyKey]; ok && existing.Status == domain.OperationApplying {
				// Owned by an in-flight (and not yet stale) attempt; leave it
				// queued exactly as-is for a future tick to find again.
				remaining = append(remaining, sel)
				continue
			}

			if sel.Target.RepoURL == "" {
				// Pre-v2 data: persisted before PendingSelection carried
				// enough to build a WriteBackRequest. Surface this as a
				// clear, actionable failure instead of crashing or hanging
				// on it forever.
				doc.Operations[sel.IdempotencyKey] = domain.OperationState{
					IdempotencyKey: sel.IdempotencyKey,
					VPANamespace:   sel.VPANamespace,
					VPAName:        sel.VPAName,
					ContainerName:  sel.ContainerName,
					Status:         domain.OperationFailed,
					ErrorMessage:   "selection predates target-capture (schema v1); re-select from the dashboard",
					CreatedAt:      sel.SelectedAt,
					UpdatedAt:      now,
				}
				continue
			}

			claimed = append(claimed, sel)
			doc.Operations[sel.IdempotencyKey] = domain.OperationState{
				IdempotencyKey:        sel.IdempotencyKey,
				VPANamespace:          sel.VPANamespace,
				VPAName:               sel.VPAName,
				ContainerName:         sel.ContainerName,
				Status:                domain.OperationApplying,
				RecommendationSummary: sel.RecommendationSummary,
				CreatedAt:             sel.SelectedAt,
				UpdatedAt:             now,
			}
		}
		doc.PendingSelections = remaining
		return nil
	})
	if err != nil {
		w.logger().Error("writebackworker: claim tick failed", "error", err)
		return
	}

	for _, group := range groupByTarget(claimed) {
		w.applyGroup(ctx, group)
	}
}

// groupByTarget partitions claimed by (RepoURL, Branch, WriteBackPolicy) --
// everything that can land in a single commit, since that's exactly what
// determines the destination of a git clone/commit/push. Groups are ordered
// deterministically (by key), and each group preserves claimed's original
// relative order, so processing order doesn't depend on Go's map iteration.
func groupByTarget(claimed []domain.PendingSelection) [][]domain.PendingSelection {
	type key struct {
		repoURL, branch string
		policy          domain.WriteBackPolicy
	}
	indexOf := map[key]int{}
	var groups [][]domain.PendingSelection
	var order []key

	for _, sel := range claimed {
		k := key{sel.Target.RepoURL, sel.Target.Branch, sel.Target.WriteBackPolicy}
		i, ok := indexOf[k]
		if !ok {
			i = len(groups)
			indexOf[k] = i
			groups = append(groups, nil)
			order = append(order, k)
		}
		groups[i] = append(groups[i], sel)
	}

	sort.Slice(groups, func(i, j int) bool {
		a, b := order[i], order[j]
		if a.repoURL != b.repoURL {
			return a.repoURL < b.repoURL
		}
		if a.branch != b.branch {
			return a.branch < b.branch
		}
		return a.policy < b.policy
	})
	return groups
}

// applyGroup applies one (RepoURL, Branch, WriteBackPolicy) group of claimed
// selections, combining them into a single commit when WriteBack supports
// it (gitwriteback.BatchGitWriteBackService), or falling back to one Apply
// call per selection otherwise.
func (w *Worker) applyGroup(ctx context.Context, group []domain.PendingSelection) {
	batchSvc, ok := w.WriteBack.(gitwriteback.BatchGitWriteBackService)
	if !ok {
		for _, sel := range group {
			w.applyOne(ctx, sel)
		}
		return
	}
	w.applyBatch(ctx, batchSvc, group)
}

// applyOne performs the actual Git write-back for one claimed selection and
// records its terminal outcome. Called outside State.Update's lock: the git
// operation itself can take seconds, and nothing else in the system
// transitions an "applying" entry except this same call (aside from
// processOnce's staleness sweep, which only fires after applyingStaleAfter,
// comfortably longer than any real apply should take).
func (w *Worker) applyOne(ctx context.Context, sel domain.PendingSelection) {
	result, applyErr := w.doApply(ctx, sel)
	w.recordOutcome(ctx, sel, result, applyErr)
}

// applyBatch combines every selection in group into a single commit via
// svc.ApplyBatch (one clone/commit/push instead of one per selection), then
// records each selection's own terminal outcome from the batch's per-item
// results. Credentials are resolved once (every item in the group shares
// the same RepoURL by construction, see groupByTarget); a patcher is
// resolved per item since items can use different SourceTypes. If
// credentials, a patcher, or the batch call itself fails, every selection in
// the group is recorded as failed with that error -- a group either
// succeeds together or fails together, never partially.
func (w *Worker) applyBatch(ctx context.Context, svc gitwriteback.BatchGitWriteBackService, group []domain.PendingSelection) {
	first := group[0]
	creds, err := w.Credentials.GetCredentials(ctx, first.Target.RepoURL)
	if err != nil {
		w.recordGroupFailure(ctx, group, fmt.Errorf("resolve credentials for %s: %w", first.Target.RepoURL, err))
		return
	}

	req := gitwriteback.BatchWriteBackRequest{
		RepoURL:         first.Target.RepoURL,
		Branch:          first.Target.Branch,
		WriteBackPolicy: first.Target.WriteBackPolicy,
		Credentials:     creds,
	}
	for _, sel := range group {
		p, ok := w.Patchers.For(sel.Target.SourceType)
		if !ok {
			w.recordGroupFailure(ctx, group, fmt.Errorf("no patcher registered for source type %q", sel.Target.SourceType))
			return
		}
		req.Items = append(req.Items, gitwriteback.BatchItem{
			Target: sel.Target,
			Recommendation: domain.ContainerRecommendation{
				ContainerName: sel.ContainerName,
				Target:        sel.RecommendationSummary,
			},
			ApplyCPU:       sel.ApplyCPU,
			ApplyMemory:    sel.ApplyMemory,
			OverrideCPU:    sel.OverrideCPU,
			OverrideMemory: sel.OverrideMemory,
			IdempotencyKey: sel.IdempotencyKey,

			CPULimit:    sel.CPULimit,
			MemoryLimit: sel.MemoryLimit,
			Patcher:     p,
		})
	}

	result, err := svc.ApplyBatch(ctx, req)
	if err != nil {
		w.recordGroupFailure(ctx, group, err)
		return
	}

	byKey := make(map[string]domain.PendingSelection, len(group))
	for _, sel := range group {
		byKey[sel.IdempotencyKey] = sel
	}
	for _, item := range result.Items {
		sel, ok := byKey[item.IdempotencyKey]
		if !ok {
			continue
		}
		w.recordOutcome(ctx, sel, gitwriteback.WriteBackResult{
			Status:         domain.OperationApplied,
			AlreadyApplied: item.AlreadyApplied,
			Branch:         result.Branch,
			CommitSHA:      item.CommitSHA,
			PRURL:          result.PRURL,
		}, nil)
	}
}

func (w *Worker) recordGroupFailure(ctx context.Context, group []domain.PendingSelection, err error) {
	for _, sel := range group {
		w.recordOutcome(ctx, sel, gitwriteback.WriteBackResult{}, err)
	}
}

// recordOutcome writes one selection's terminal OperationState (applied,
// conflict, or failed) back to the StateStore and logs the result. Shared by
// both the single-item (applyOne) and batch (applyBatch) paths so they can
// never disagree on how a result or error maps to dashboard-visible state.
func (w *Worker) recordOutcome(ctx context.Context, sel domain.PendingSelection, result gitwriteback.WriteBackResult, applyErr error) {
	err := w.State.Update(ctx, func(doc *domain.StateDocument) error {
		op, ok := doc.Operations[sel.IdempotencyKey]
		if !ok {
			// Shouldn't happen (the claim step always writes this entry
			// first) -- reconstruct enough to still record the outcome.
			op = domain.OperationState{
				IdempotencyKey: sel.IdempotencyKey,
				VPANamespace:   sel.VPANamespace,
				VPAName:        sel.VPAName,
				ContainerName:  sel.ContainerName,
				CreatedAt:      sel.SelectedAt,
			}
		}
		op.UpdatedAt = w.now()

		switch {
		case applyErr == nil:
			op.Status = domain.OperationApplied
			op.Branch = result.Branch
			op.CommitSHA = result.CommitSHA
			op.PRURL = result.PRURL
			op.ErrorMessage = ""
			recordRequestHeadroom(doc, sel, op.UpdatedAt)
		case errors.Is(applyErr, gitwriteback.ErrConflict):
			op.Status = domain.OperationConflict
			op.ErrorMessage = applyErr.Error()
		default:
			op.Status = domain.OperationFailed
			op.ErrorMessage = applyErr.Error()
		}
		doc.Operations[sel.IdempotencyKey] = op
		return nil
	})
	if err != nil {
		w.logger().Error("writebackworker: failed to record outcome",
			"idempotencyKey", sel.IdempotencyKey, "error", err)
	}

	logAttrs := []any{
		"idempotencyKey", sel.IdempotencyKey,
		"namespace", sel.VPANamespace,
		"vpaName", sel.VPAName,
		"container", sel.ContainerName,
	}
	if applyErr != nil {
		w.logger().Error("writebackworker: apply failed", append(logAttrs, "error", applyErr)...)
		return
	}
	w.logger().Info("writebackworker: applied", append(logAttrs, "branch", result.Branch, "commitSha", result.CommitSHA)...)
}

// recordRequestHeadroom makes the headroom an applied selection was written
// with the container's standing one (see domain.RequestHeadroom), for the
// resources it actually applied only -- the other resource keeps whatever
// it had. Applying with no headroom clears it, and an entry left with none
// is removed.
func recordRequestHeadroom(doc *domain.StateDocument, sel domain.PendingSelection, now time.Time) {
	if !sel.ApplyCPU && !sel.ApplyMemory {
		return
	}
	key := domain.RequestHeadroomKey(sel.VPANamespace, sel.VPAName, sel.ContainerName)
	h := doc.RequestHeadrooms[key]
	if sel.ApplyCPU {
		h.CPUPercent = domain.NonZeroPercent(sel.CPURequestHeadroom)
	}
	if sel.ApplyMemory {
		h.MemoryPercent = domain.NonZeroPercent(sel.MemoryRequestHeadroom)
	}
	if h.CPUPercent == nil && h.MemoryPercent == nil {
		delete(doc.RequestHeadrooms, key)
		return
	}
	h.UpdatedAt = now
	if doc.RequestHeadrooms == nil {
		doc.RequestHeadrooms = map[string]domain.RequestHeadroom{}
	}
	doc.RequestHeadrooms[key] = h
}

func (w *Worker) doApply(ctx context.Context, sel domain.PendingSelection) (gitwriteback.WriteBackResult, error) {
	creds, err := w.Credentials.GetCredentials(ctx, sel.Target.RepoURL)
	if err != nil {
		return gitwriteback.WriteBackResult{}, fmt.Errorf("resolve credentials for %s: %w", sel.Target.RepoURL, err)
	}

	p, ok := w.Patchers.For(sel.Target.SourceType)
	if !ok {
		return gitwriteback.WriteBackResult{}, fmt.Errorf("no patcher registered for source type %q", sel.Target.SourceType)
	}

	req := gitwriteback.WriteBackRequest{
		PatchRequest: domain.PatchRequest{
			Target: sel.Target,
			Recommendation: domain.ContainerRecommendation{
				ContainerName: sel.ContainerName,
				Target:        sel.RecommendationSummary,
			},
			ApplyCPU:       sel.ApplyCPU,
			ApplyMemory:    sel.ApplyMemory,
			OverrideCPU:    sel.OverrideCPU,
			OverrideMemory: sel.OverrideMemory,
			IdempotencyKey: sel.IdempotencyKey,

			CPULimit:    sel.CPULimit,
			MemoryLimit: sel.MemoryLimit,
		},
		CommitMessage: sel.CommitMessage,
		Credentials:   creds,
		Patcher:       p,
	}

	return w.WriteBack.Apply(ctx, req)
}
