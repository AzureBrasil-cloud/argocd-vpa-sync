package integration

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"

	"github.com/azurebrasil/argocd-vpa-updater/internal/domain"
	"github.com/azurebrasil/argocd-vpa-updater/internal/gitexec"
	"github.com/azurebrasil/argocd-vpa-updater/internal/gittest"
	"github.com/azurebrasil/argocd-vpa-updater/internal/gitwriteback"
)

// TestGitWriteBackCLI_FullFlow is TestGitWriteBack_FullFlow's counterpart for
// the real, git-CLI-backed CLIGitWriteBackService: the same three
// guarantees (fresh commit patches only the declared keys; exact replay is a
// provable no-op; a concurrent conflicting push is rejected, never
// overwritten), proven against a local repository over the filesystem
// transport -- a real, non-mocked git transport that exercises the actual
// git binary end to end, the same way TestRunner_Clone_AuthNone already
// does, without any network dependency or SSH/daemon setup.
func TestGitWriteBackCLI_FullFlow(t *testing.T) {
	remote := gittest.NewBareRepoWithInitialCommit(t, map[string]string{
		"deploy/checkout-api.yaml": manifestWithComments,
	})
	svc := gitwriteback.NewCLIGitWriteBackService(gitexec.NewRunner())
	tgt := target()
	tgt.RepoURL = remote

	t.Run("apply changes only the targeted keys and preserves everything else", func(t *testing.T) {
		req := requestFor(tgt, "250m")

		result, err := svc.Apply(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if result.AlreadyApplied {
			t.Fatalf("expected a fresh commit")
		}
		if result.CommitSHA == "" {
			t.Fatalf("expected a commit SHA")
		}

		repo := gittest.CloneWorktree(t, remote)
		wt, err := repo.Worktree()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		content := gittest.ReadFile(t, wt, "deploy/checkout-api.yaml")

		if !strings.Contains(content, "cpu: 250m") {
			t.Fatalf("expected cpu to be updated to 250m, got:\n%s", content)
		}
		if !strings.Contains(content, "memory: 256Mi") {
			t.Fatalf("expected memory to remain untouched (ApplyMemory was false), got:\n%s", content)
		}
		if !strings.Contains(content, "cpu: 500m") {
			t.Fatalf("expected the cpu limit to remain untouched, got:\n%s", content)
		}
		if !strings.Contains(content, "# checkout-api deployment -- do not remove this header") {
			t.Fatalf("expected the header comment to survive, got:\n%s", content)
		}
		if !strings.Contains(content, "image: example.com/checkout-api:1.2.3") {
			t.Fatalf("expected unrelated fields to survive, got:\n%s", content)
		}
	})

	t.Run("replaying the identical selection does not create a duplicate commit", func(t *testing.T) {
		before := gittest.CloneWorktree(t, remote)
		beforeHead, err := before.Head()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		req := requestFor(tgt, "250m") // identical recommendation to the previous subtest
		result, err := svc.Apply(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !result.AlreadyApplied {
			t.Fatalf("expected AlreadyApplied=true on replay")
		}

		after := gittest.CloneWorktree(t, remote)
		afterHead, err := after.Head()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if beforeHead.Hash() != afterHead.Hash() {
			t.Fatalf("expected the remote's main tip to be unchanged by a replay, before=%s after=%s", beforeHead.Hash(), afterHead.Hash())
		}
	})

	t.Run("a concurrent edit between clone and push is rejected, not overwritten", func(t *testing.T) {
		preConflict := gittest.CloneWorktree(t, remote)
		preConflictHead, err := preConflict.Head()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}

		svcWithRace := gitwriteback.NewCLIGitWriteBackService(gitexec.NewRunner())
		var concurrentCommit string
		svcWithRace.OnBeforePush = func() {
			// Fires after Apply below has already cloned its base and
			// committed locally, but before it pushes -- the exact window a
			// concurrent human edit or a second controller instance could
			// land in. The concurrent actor itself uses go-git (via
			// gittest), simulating "some other process pushed meanwhile" --
			// it need not be the same git implementation as the SUT.
			concurrentRepo := gittest.CloneWorktree(t, remote)
			cwt, err := concurrentRepo.Worktree()
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			gittest.WriteFile(t, cwt, "deploy/checkout-api.yaml", strings.ReplaceAll(manifestWithComments, "cpu: 100m # tuned by hand", "cpu: 999m # tuned by hand"))
			if _, err := cwt.Add("deploy/checkout-api.yaml"); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			hash, err := cwt.Commit("concurrent human edit", commitOptions())
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			concurrentCommit = hash.String()
			if err := concurrentRepo.Push(pushOptions()); err != nil {
				t.Fatalf("unexpected error pushing the concurrent edit: %v", err)
			}
		}

		req := requestFor(tgt, "300m") // a different value than the concurrent edit
		_, err = svcWithRace.Apply(context.Background(), req)
		if err == nil {
			t.Fatalf("expected an error due to the concurrent edit")
		}
		if !errors.Is(err, gitwriteback.ErrConflict) {
			t.Fatalf("expected ErrConflict, got %v", err)
		}

		// The remote must reflect only the concurrent edit -- Apply's own
		// (conflicting) commit must never have been pushed.
		final := gittest.CloneWorktree(t, remote)
		finalHead, err := final.Head()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if finalHead.Hash().String() != concurrentCommit {
			t.Fatalf("expected remote tip to be exactly the concurrent commit %s, got %s", concurrentCommit, finalHead.Hash())
		}
		if finalHead.Hash() == preConflictHead.Hash() {
			t.Fatalf("sanity check failed: the concurrent edit itself did not land")
		}

		fwt, err := final.Worktree()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		content := gittest.ReadFile(t, fwt, "deploy/checkout-api.yaml")
		if !strings.Contains(content, "cpu: 999m") {
			t.Fatalf("expected the concurrent edit's value (999m) to be what's on the remote, got:\n%s", content)
		}
		if strings.Contains(content, "cpu: 300m") {
			t.Fatalf("Apply's conflicting value (300m) must never have reached the remote, got:\n%s", content)
		}
	})
}

// TestGitWriteBackCLI_IdempotencyScanFindsNonTipCommit is a regression test
// for CLIGitWriteBackService's use of CloneFull (full history) rather than
// Clone (shallow, --depth 1) when resuming an existing working branch: it
// pushes two different idempotency-keyed commits onto the same pull-request
// topic branch, then replays the *first* one (no longer the branch tip) and
// asserts it's still found. A regression to a shallow clone here would only
// see the tip commit and incorrectly conclude "not yet applied", producing a
// duplicate commit.
func TestGitWriteBackCLI_IdempotencyScanFindsNonTipCommit(t *testing.T) {
	remote := gittest.NewBareRepoWithInitialCommit(t, map[string]string{
		"deploy/checkout-api.yaml": manifestWithComments,
	})
	svc := gitwriteback.NewCLIGitWriteBackService(gitexec.NewRunner())

	tgt := target()
	tgt.RepoURL = remote
	tgt.WriteBackPolicy = domain.WriteBackPullRequest

	firstReq := requestFor(tgt, "250m")
	firstResult, err := svc.Apply(context.Background(), firstReq)
	if err != nil {
		t.Fatalf("unexpected error applying first request: %v", err)
	}
	if firstResult.AlreadyApplied {
		t.Fatalf("expected a fresh commit for the first request")
	}
	if firstResult.Branch == tgt.Branch {
		t.Fatalf("expected a pull-request policy to use a topic branch distinct from %q, got %q", tgt.Branch, firstResult.Branch)
	}

	// A second, different container on the *same* topic-branch-naming scheme
	// would normally get its own branch (the name is derived from
	// ContainerName+IdempotencyKey), so to land two different commits on the
	// exact same branch here, apply a second, different recommended value
	// for the same container -- this changes the IdempotencyKey (different
	// requested value), producing a different topic branch name too. To
	// force both commits onto one branch (proving the history scan walks
	// past the tip), commit a second, unrelated change directly onto that
	// same branch out-of-band, tagged with a *different* idempotency key,
	// then apply-and-replay the *first* request again.
	topicBranch := firstResult.Branch
	otherKey := "sha256:unrelated-commit-for-this-test"
	pushUnrelatedCommit(t, remote, topicBranch, "deploy/checkout-api.yaml", strings.ReplaceAll(manifestWithComments, "memory: 256Mi", "memory: 300Mi"), otherKey)

	replayResult, err := svc.Apply(context.Background(), firstReq)
	if err != nil {
		t.Fatalf("unexpected error replaying the first request: %v", err)
	}
	if !replayResult.AlreadyApplied {
		t.Fatalf("expected the first request's idempotency key to still be found even though it is no longer the branch tip")
	}
	if replayResult.CommitSHA != firstResult.CommitSHA {
		t.Fatalf("expected the replay to report the original commit %s, got %s", firstResult.CommitSHA, replayResult.CommitSHA)
	}
}

// pushUnrelatedCommit commits content directly onto branch (via go-git,
// simulating some other actor) tagged with its own idempotency trailer, so
// the target branch ends up with more than one write-back-shaped commit.
func pushUnrelatedCommit(t *testing.T, remotePath, branch, path, content, idempotencyKey string) {
	t.Helper()

	dir := t.TempDir()
	repo, err := git.PlainClone(dir, false, &git.CloneOptions{
		URL:           remotePath,
		ReferenceName: plumbing.NewBranchReferenceName(branch),
		SingleBranch:  true,
	})
	if err != nil {
		t.Fatalf("pushUnrelatedCommit: clone: %v", err)
	}
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("pushUnrelatedCommit: worktree: %v", err)
	}
	gittest.WriteFile(t, wt, path, content)
	if _, err := wt.Add(path); err != nil {
		t.Fatalf("pushUnrelatedCommit: add: %v", err)
	}
	msg := "unrelated commit\n\nVpa-Idempotency-Key: " + idempotencyKey + "\n"
	if _, err := wt.Commit(msg, commitOptions()); err != nil {
		t.Fatalf("pushUnrelatedCommit: commit: %v", err)
	}
	if err := repo.Push(pushOptions()); err != nil {
		t.Fatalf("pushUnrelatedCommit: push: %v", err)
	}
}
