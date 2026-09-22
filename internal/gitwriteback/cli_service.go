package gitwriteback

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
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
// never force-pushing), sharing this package's workingBranchName/shortHash/
// prURLFor/effectiveCommitMessage/buildCommitMessage/extractIdempotencyKey
// helpers so the two implementations can never silently diverge in policy.
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

	workingBranch := workingBranchName(req)
	baseBranch := req.Target.Branch
	repoURL := req.Target.RepoURL

	workingExists, err := runner.RemoteBranchExists(ctx, repoURL, workingBranch, req.Credentials)
	if err != nil {
		return WriteBackResult{}, fmt.Errorf("gitwriteback: check remote branch %s: %w", workingBranch, err)
	}

	if workingExists {
		// A full (non-shallow) clone is required here, not Clone's shallow
		// default: this operation (or one with the same topic branch name)
		// may have run before, and the idempotency-trailer scan below must
		// see the branch's complete history, not just its tip commit.
		if err := runner.CloneFull(ctx, workDir, repoURL, workingBranch, req.Credentials); err != nil {
			return WriteBackResult{}, fmt.Errorf("gitwriteback: clone %s: %w", workingBranch, err)
		}

		commits, err := runner.LogMessages(ctx, workDir, "HEAD", 0)
		if err != nil {
			return WriteBackResult{}, fmt.Errorf("gitwriteback: scan %s history: %w", workingBranch, err)
		}
		for _, c := range commits {
			if key, present := extractIdempotencyKey(c.Body); present && key == req.IdempotencyKey {
				return WriteBackResult{
					Status:         domain.OperationApplied,
					AlreadyApplied: true,
					Branch:         workingBranch,
					CommitSHA:      c.SHA,
					PRURL:          prURLFor(req, workingBranch),
				}, nil
			}
		}
	} else {
		baseExists, err := runner.RemoteBranchExists(ctx, repoURL, baseBranch, req.Credentials)
		if err != nil {
			return WriteBackResult{}, fmt.Errorf("gitwriteback: check remote branch %s: %w", baseBranch, err)
		}
		if !baseExists {
			return WriteBackResult{}, fmt.Errorf("gitwriteback: base branch %q not found on remote %s", baseBranch, repoURL)
		}
		if err := runner.Clone(ctx, workDir, repoURL, baseBranch, req.Credentials); err != nil {
			return WriteBackResult{}, fmt.Errorf("gitwriteback: clone %s: %w", baseBranch, err)
		}
		if workingBranch != baseBranch {
			if err := runner.CheckoutNewBranch(ctx, workDir, workingBranch); err != nil {
				return WriteBackResult{}, fmt.Errorf("gitwriteback: create branch %s: %w", workingBranch, err)
			}
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
			Branch:         workingBranch,
			PRURL:          prURLFor(req, workingBranch),
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

	if err := runner.Push(ctx, workDir, repoURL, workingBranch, workingBranch, req.Credentials); err != nil {
		if errors.Is(err, gitexec.ErrNonFastForward) {
			return WriteBackResult{}, fmt.Errorf("%w: %v", ErrConflict, err)
		}
		return WriteBackResult{}, fmt.Errorf("gitwriteback: push %s: %w", workingBranch, err)
	}

	return WriteBackResult{
		Status:    domain.OperationApplied,
		Branch:    workingBranch,
		CommitSHA: sha,
		PRURL:     prURLFor(req, workingBranch),
	}, nil
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

	workingBranch := batchWorkingBranchName(req)
	baseBranch := req.Branch
	repoURL := req.RepoURL

	// idempotencyCommits maps an already-applied item's key to the (earlier)
	// commit SHA that contains it, found by scanning the branch's full
	// history -- a batch replay (or a batch that happens to overlap with an
	// earlier one) must skip exactly those items, not reapply or duplicate
	// them.
	idempotencyCommits := map[string]string{}

	workingExists, err := runner.RemoteBranchExists(ctx, repoURL, workingBranch, req.Credentials)
	if err != nil {
		return BatchWriteBackResult{}, fmt.Errorf("gitwriteback: check remote branch %s: %w", workingBranch, err)
	}

	if workingExists {
		if err := runner.CloneFull(ctx, workDir, repoURL, workingBranch, req.Credentials); err != nil {
			return BatchWriteBackResult{}, fmt.Errorf("gitwriteback: clone %s: %w", workingBranch, err)
		}

		wanted := make(map[string]bool, len(req.Items))
		for _, item := range req.Items {
			wanted[item.IdempotencyKey] = true
		}

		commits, err := runner.LogMessages(ctx, workDir, "HEAD", 0)
		if err != nil {
			return BatchWriteBackResult{}, fmt.Errorf("gitwriteback: scan %s history: %w", workingBranch, err)
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
	} else {
		baseExists, err := runner.RemoteBranchExists(ctx, repoURL, baseBranch, req.Credentials)
		if err != nil {
			return BatchWriteBackResult{}, fmt.Errorf("gitwriteback: check remote branch %s: %w", baseBranch, err)
		}
		if !baseExists {
			return BatchWriteBackResult{}, fmt.Errorf("gitwriteback: base branch %q not found on remote %s", baseBranch, repoURL)
		}
		if err := runner.Clone(ctx, workDir, repoURL, baseBranch, req.Credentials); err != nil {
			return BatchWriteBackResult{}, fmt.Errorf("gitwriteback: clone %s: %w", baseBranch, err)
		}
		if workingBranch != baseBranch {
			if err := runner.CheckoutNewBranch(ctx, workDir, workingBranch); err != nil {
				return BatchWriteBackResult{}, fmt.Errorf("gitwriteback: create branch %s: %w", workingBranch, err)
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
			Branch:         workingBranch,
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

	if err := runner.Push(ctx, workDir, repoURL, workingBranch, workingBranch, req.Credentials); err != nil {
		if errors.Is(err, gitexec.ErrNonFastForward) {
			return BatchWriteBackResult{}, fmt.Errorf("%w: %v", ErrConflict, err)
		}
		return BatchWriteBackResult{}, fmt.Errorf("gitwriteback: push %s: %w", workingBranch, err)
	}

	for i := range outcomes {
		if outcomes[i].CommitSHA == "" && !outcomes[i].AlreadyApplied {
			outcomes[i].CommitSHA = sha
		}
	}

	return BatchWriteBackResult{
		Status:    domain.OperationApplied,
		Branch:    workingBranch,
		CommitSHA: sha,
		PRURL:     batchPRURLFor(req, workingBranch),
		Items:     outcomes,
	}, nil
}

// batchWorkingBranchName is workingBranchName's batch counterpart: for a
// direct commit it's simply the shared base branch, same as a single item;
// for a pull-request policy it's a deterministic name derived from every
// item's IdempotencyKey (sorted, so member order never affects the name),
// so replaying the identical batch reuses the same topic branch.
func batchWorkingBranchName(req BatchWriteBackRequest) string {
	if req.WriteBackPolicy != domain.WriteBackPullRequest {
		return req.Branch
	}
	keys := make([]string, len(req.Items))
	for i, item := range req.Items {
		keys[i] = item.IdempotencyKey
	}
	sort.Strings(keys)
	return fmt.Sprintf("vpa-updater/batch-%s", shortHash(strings.Join(keys, "|")))
}

func batchPRURLFor(req BatchWriteBackRequest, branch string) string {
	if req.WriteBackPolicy != domain.WriteBackPullRequest {
		return ""
	}
	return fmt.Sprintf("local://fake-pr/%s", branch)
}
