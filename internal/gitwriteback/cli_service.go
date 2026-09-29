package gitwriteback

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/azurebrasil/argocd-vpa-updater/internal/domain"
	"github.com/azurebrasil/argocd-vpa-updater/internal/gitexec"
)

// CLIGitWriteBackService is the production GitWriteBackService: it shells
// out to the real git binary via *gitexec.Runner instead of go-git, for the
// same reason gitexec itself does (see gitexec's package doc) -- go-git has
// been observed, in production, to fail against a real Azure DevOps
// repository. It reproduces FakeGitWriteBackService's exact contract
// (idempotent replay, no-op detection, conflict rejection via ErrConflict,
// never force-pushing), sharing this package's effectiveCommitMessage/
// buildCommitMessage/extractIdempotencyKey helpers so the two
// implementations can never silently diverge in policy.
//
// One deliberate behavioral difference from the fake service: "already up to
// date" on push cannot occur here. Every path that reaches Push has just
// created a brand-new local commit (Commit always advances the branch tip by
// exactly one commit once a caller decides to proceed past the no-op
// checks), so the local tip can never already equal a previously observed
// remote tip -- the no-op case that would otherwise cause that is already
// handled earlier, by the PatchResult.Changed check.
type CLIGitWriteBackService struct {
	Runner *gitexec.Runner

	// Now is overridable for deterministic tests; defaults to time.Now.
	Now func() time.Time

	// OnBeforePush, if set, is called once the working commit has been made
	// locally but before Apply attempts to push it. It exists solely so
	// tests can deterministically inject a concurrent remote change into the
	// exact window Apply's conflict handling must detect (real callers leave
	// it nil), mirroring FakeGitWriteBackService.
	OnBeforePush func()
}

// NewCLIGitWriteBackService builds a CLIGitWriteBackService using runner for
// every git operation.
func NewCLIGitWriteBackService(runner *gitexec.Runner) *CLIGitWriteBackService {
	return &CLIGitWriteBackService{Runner: runner}
}

func (s *CLIGitWriteBackService) runner() *gitexec.Runner {
	if s.Runner != nil {
		return s.Runner
	}
	return gitexec.NewRunner()
}

func (s *CLIGitWriteBackService) Apply(ctx context.Context, req WriteBackRequest) (WriteBackResult, error) {
	if req.IdempotencyKey == "" {
		return WriteBackResult{}, fmt.Errorf("gitwriteback: IdempotencyKey is required")
	}
	if req.Patcher == nil {
		return WriteBackResult{}, fmt.Errorf("gitwriteback: Patcher is required")
	}

	runner := s.runner()

	workDir, err := os.MkdirTemp("", "argocd-vpa-updater-writeback-*")
	if err != nil {
		return WriteBackResult{}, fmt.Errorf("gitwriteback: create scratch worktree: %w", err)
	}
	defer os.RemoveAll(workDir)

	branch := req.Target.Branch
	repoURL := req.Target.RepoURL

	if err := cloneTargetBranch(ctx, runner, workDir, repoURL, branch, req.Credentials); err != nil {
		return WriteBackResult{}, err
	}

	commits, err := runner.LogMessages(ctx, workDir, "HEAD", 0)
	if err != nil {
		return WriteBackResult{}, fmt.Errorf("gitwriteback: scan %s history: %w", branch, err)
	}
	for _, c := range commits {
		if key, present := extractIdempotencyKey(c.Body); present && key == req.IdempotencyKey {
			return WriteBackResult{
				Status:         domain.OperationApplied,
				AlreadyApplied: true,
				Branch:         branch,
				CommitSHA:      c.SHA,
			}, nil
		}
	}

	filePath := req.Target.FilePath
	absPath := filepath.Join(workDir, filepath.FromSlash(filePath))

	originalContent, err := os.ReadFile(absPath)
	if err != nil {
		return WriteBackResult{}, fmt.Errorf("gitwriteback: read %s: %w", filePath, err)
	}

	newContent, patchResult, err := req.Patcher.Patch(ctx, originalContent, req.PatchRequest)
	if err != nil {
		return WriteBackResult{}, fmt.Errorf("gitwriteback: patch %s: %w", filePath, err)
	}
	if !patchResult.Changed {
		return WriteBackResult{
			Status:         domain.OperationApplied,
			AlreadyApplied: true,
			Branch:         branch,
		}, nil
	}

	if err := os.WriteFile(absPath, newContent, 0o644); err != nil {
		return WriteBackResult{}, fmt.Errorf("gitwriteback: write %s: %w", filePath, err)
	}
	if err := runner.Add(ctx, workDir, filePath); err != nil {
		return WriteBackResult{}, fmt.Errorf("gitwriteback: git add %s: %w", filePath, err)
	}

	commitMsg := buildCommitMessage(effectiveCommitMessage(req), req.IdempotencyKey)
	sha, err := runner.Commit(ctx, workDir, commitMsg, gitexec.GitIdentity{
		Name:  "argocd-vpa-updater",
		Email: "argocd-vpa-updater@local",
	})
	if err != nil {
		return WriteBackResult{}, fmt.Errorf("gitwriteback: commit: %w", err)
	}

	if s.OnBeforePush != nil {
		s.OnBeforePush()
	}

	if err := runner.Push(ctx, workDir, repoURL, branch, branch, req.Credentials); err != nil {
		if errors.Is(err, gitexec.ErrNonFastForward) {
			return WriteBackResult{}, fmt.Errorf("%w: %v", ErrConflict, err)
		}
		return WriteBackResult{}, fmt.Errorf("gitwriteback: push %s: %w", branch, err)
	}

	return WriteBackResult{
		Status:    domain.OperationApplied,
		Branch:    branch,
		CommitSHA: sha,
	}, nil
}

// cloneTargetBranch clones branch of repoURL into workDir, failing clearly
// if the branch doesn't exist. The clone is full (non-shallow), not Clone's
// shallow default: an operation may have landed on this branch before, and
// the idempotency-trailer scan that follows must see its complete history,
// not just its tip commit.
func cloneTargetBranch(ctx context.Context, runner *gitexec.Runner, workDir, repoURL, branch string, creds domain.GitCredentials) error {
	exists, err := runner.RemoteBranchExists(ctx, repoURL, branch, creds)
	if err != nil {
		return fmt.Errorf("gitwriteback: check remote branch %s: %w", branch, err)
	}
	if !exists {
		return fmt.Errorf("gitwriteback: branch %q not found on remote %s", branch, repoURL)
	}
	if err := runner.CloneFull(ctx, workDir, repoURL, branch, creds); err != nil {
		return fmt.Errorf("gitwriteback: clone %s: %w", branch, err)
	}
	return nil
}

// ApplyBatch combines every item in req into a single commit (one clone, one
// commit, one push), instead of the one-clone-commit-push-per-item cost
// (and one-commit-per-selection noise) that calling Apply once per item
// would incur. It mirrors Apply's contract per item: an item whose
// IdempotencyKey is already found in the target branch's history, or whose
// requested value already matches the file, contributes nothing to a new
// commit and is reported as AlreadyApplied. If every item turns out that
// way, no commit or push happens at all.
func (s *CLIGitWriteBackService) ApplyBatch(ctx context.Context, req BatchWriteBackRequest) (BatchWriteBackResult, error) {
	if len(req.Items) == 0 {
		return BatchWriteBackResult{}, fmt.Errorf("gitwriteback: ApplyBatch requires at least one item")
	}
	for _, item := range req.Items {
		if item.IdempotencyKey == "" {
			return BatchWriteBackResult{}, fmt.Errorf("gitwriteback: IdempotencyKey is required for every batch item")
		}
		if item.Patcher == nil {
			return BatchWriteBackResult{}, fmt.Errorf("gitwriteback: Patcher is required for every batch item")
		}
	}

	runner := s.runner()

	workDir, err := os.MkdirTemp("", "argocd-vpa-updater-writeback-batch-*")
	if err != nil {
		return BatchWriteBackResult{}, fmt.Errorf("gitwriteback: create scratch worktree: %w", err)
	}
	defer os.RemoveAll(workDir)

	branch := req.Branch
	repoURL := req.RepoURL

	// idempotencyCommits maps an already-applied item's key to the (earlier)
	// commit SHA that contains it, found by scanning the branch's full
	// history -- a batch replay (or a batch that happens to overlap with an
	// earlier one) must skip exactly those items, not reapply or duplicate
	// them.
	idempotencyCommits := map[string]string{}

	if err := cloneTargetBranch(ctx, runner, workDir, repoURL, branch, req.Credentials); err != nil {
		return BatchWriteBackResult{}, err
	}

	wanted := make(map[string]bool, len(req.Items))
	for _, item := range req.Items {
		wanted[item.IdempotencyKey] = true
	}

	commits, err := runner.LogMessages(ctx, workDir, "HEAD", 0)
	if err != nil {
		return BatchWriteBackResult{}, fmt.Errorf("gitwriteback: scan %s history: %w", branch, err)
	}
	for _, c := range commits {
		for _, key := range extractIdempotencyKeys(c.Body) {
			if wanted[key] {
				if _, already := idempotencyCommits[key]; !already {
					idempotencyCommits[key] = c.SHA
				}
			}
		}
	}

	outcomes := make([]ItemOutcome, len(req.Items))
	var newKeys []string
	var workloadNames []string
	seenWorkload := map[string]bool{}
	anyChange := false

	for i, item := range req.Items {
		if sha, already := idempotencyCommits[item.IdempotencyKey]; already {
			outcomes[i] = ItemOutcome{IdempotencyKey: item.IdempotencyKey, AlreadyApplied: true, CommitSHA: sha}
			continue
		}

		filePath := item.Target.FilePath
		absPath := filepath.Join(workDir, filepath.FromSlash(filePath))

		originalContent, err := os.ReadFile(absPath)
		if err != nil {
			return BatchWriteBackResult{}, fmt.Errorf("gitwriteback: read %s: %w", filePath, err)
		}

		patchReq := domain.PatchRequest{
			Target:         item.Target,
			Recommendation: item.Recommendation,
			ApplyCPU:       item.ApplyCPU,
			ApplyMemory:    item.ApplyMemory,
			OverrideCPU:    item.OverrideCPU,
			OverrideMemory: item.OverrideMemory,
			IdempotencyKey: item.IdempotencyKey,

			CPULimit:    item.CPULimit,
			MemoryLimit: item.MemoryLimit,
		}
		newContent, patchResult, err := item.Patcher.Patch(ctx, originalContent, patchReq)
		if err != nil {
			return BatchWriteBackResult{}, fmt.Errorf("gitwriteback: patch %s: %w", filePath, err)
		}
		if !patchResult.Changed {
			outcomes[i] = ItemOutcome{IdempotencyKey: item.IdempotencyKey, AlreadyApplied: true}
			continue
		}

		if err := os.WriteFile(absPath, newContent, 0o644); err != nil {
			return BatchWriteBackResult{}, fmt.Errorf("gitwriteback: write %s: %w", filePath, err)
		}
		if err := runner.Add(ctx, workDir, filePath); err != nil {
			return BatchWriteBackResult{}, fmt.Errorf("gitwriteback: git add %s: %w", filePath, err)
		}

		anyChange = true
		newKeys = append(newKeys, item.IdempotencyKey)
		outcomes[i] = ItemOutcome{IdempotencyKey: item.IdempotencyKey}
		if name := item.Target.Workload.Name; name != "" && !seenWorkload[name] {
			seenWorkload[name] = true
			workloadNames = append(workloadNames, name)
		}
	}

	if !anyChange {
		return BatchWriteBackResult{
			Status:         domain.OperationApplied,
			AlreadyApplied: true,
			Branch:         branch,
			Items:          outcomes,
		}, nil
	}

	summary := req.CommitMessage
	if summary == "" {
		summary = fmt.Sprintf("build: automatic update of %s", strings.Join(workloadNames, ", "))
	}
	commitMsg := buildBatchCommitMessage(summary, newKeys)

	sha, err := runner.Commit(ctx, workDir, commitMsg, gitexec.GitIdentity{
		Name:  "argocd-vpa-updater",
		Email: "argocd-vpa-updater@local",
	})
	if err != nil {
		return BatchWriteBackResult{}, fmt.Errorf("gitwriteback: commit: %w", err)
	}

	if s.OnBeforePush != nil {
		s.OnBeforePush()
	}

	if err := runner.Push(ctx, workDir, repoURL, branch, branch, req.Credentials); err != nil {
		if errors.Is(err, gitexec.ErrNonFastForward) {
			return BatchWriteBackResult{}, fmt.Errorf("%w: %v", ErrConflict, err)
		}
		return BatchWriteBackResult{}, fmt.Errorf("gitwriteback: push %s: %w", branch, err)
	}

	for i := range outcomes {
		if outcomes[i].CommitSHA == "" && !outcomes[i].AlreadyApplied {
			outcomes[i].CommitSHA = sha
		}
	}

	return BatchWriteBackResult{
		Status:    domain.OperationApplied,
		Branch:    branch,
		CommitSHA: sha,
		Items:     outcomes,
	}, nil
}
