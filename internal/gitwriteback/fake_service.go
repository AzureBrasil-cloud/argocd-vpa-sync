package gitwriteback

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"
	"github.com/go-git/go-git/v5/plumbing/transport"
	gogithttp "github.com/go-git/go-git/v5/plumbing/transport/http"
	"github.com/go-git/go-git/v5/plumbing/transport/ssh"

	"github.com/azurebrasil/argocd-vpa-updater/internal/domain"
)

// FakeGitWriteBackService is a fully functional GitWriteBackService that
// operates against a real, local Git repository via go-git (no external git
// binary required). It stands in for a real GitHub/GitLab/Azure DevOps
// integration during this phase: same interface, real git operations
// (branch, commit, push, conflict detection), except that a "pull-request"
// write-back only pushes a topic branch and returns a synthetic
// "local://fake-pr/<branch>" URL rather than calling a real hosting API --
// that call is the exact seam a later provider-specific implementation
// fills in.
type FakeGitWriteBackService struct {
	// RemotePath is the filesystem path (or file:// URL) of the repository
	// this instance writes to.
	RemotePath string

	// OnBeforePush, if set, is called once the working commit has been made
	// locally but before Apply attempts to push it. It exists solely so
	// tests can deterministically inject a concurrent remote change into the
	// exact window Apply's conflict handling must detect (real callers leave
	// it nil); see test/integration for the conflict scenario it drives.
	OnBeforePush func()
}

// NewFakeGitWriteBackService builds a FakeGitWriteBackService targeting a
// local repository at remotePath.
func NewFakeGitWriteBackService(remotePath string) *FakeGitWriteBackService {
	return &FakeGitWriteBackService{RemotePath: remotePath}
}

func (s *FakeGitWriteBackService) Apply(ctx context.Context, req WriteBackRequest) (WriteBackResult, error) {
	if req.IdempotencyKey == "" {
		return WriteBackResult{}, fmt.Errorf("gitwriteback: IdempotencyKey is required")
	}
	if req.Patcher == nil {
		return WriteBackResult{}, fmt.Errorf("gitwriteback: Patcher is required")
	}

	workDir, err := os.MkdirTemp("", "argocd-vpa-updater-writeback-*")
	if err != nil {
		return WriteBackResult{}, fmt.Errorf("gitwriteback: create scratch worktree: %w", err)
	}
	defer os.RemoveAll(workDir)

	auth, err := authMethod(req.Credentials)
	if err != nil {
		return WriteBackResult{}, err
	}

	repo, err := git.PlainCloneContext(ctx, workDir, false, &git.CloneOptions{
		URL:  s.RemotePath,
		Auth: auth,
	})
	if err != nil {
		return WriteBackResult{}, fmt.Errorf("gitwriteback: clone %s: %w", s.RemotePath, err)
	}

	workingBranch := workingBranchName(req)
	baseBranch := req.Target.Branch

	wt, err := repo.Worktree()
	if err != nil {
		return WriteBackResult{}, fmt.Errorf("gitwriteback: open worktree: %w", err)
	}

	// If the working branch already exists on the remote, this operation
	// (or one with the same topic branch name) may have run before.
	if workingRef, ok := resolveRemoteBranch(repo, workingBranch); ok {
		if sha, found, err := findIdempotentCommit(repo, workingRef.Hash(), req.IdempotencyKey); err != nil {
			return WriteBackResult{}, fmt.Errorf("gitwriteback: scan %s history: %w", workingBranch, err)
		} else if found {
			return WriteBackResult{
				Status:         domain.OperationApplied,
				AlreadyApplied: true,
				Branch:         workingBranch,
				CommitSHA:      sha.String(),
				PRURL:          prURLFor(req, workingBranch),
			}, nil
		}
		if err := checkoutWorkingBranch(repo, wt, workingBranch, workingRef.Hash()); err != nil {
			return WriteBackResult{}, fmt.Errorf("gitwriteback: checkout %s: %w", workingBranch, err)
		}
	} else {
		baseRef, baseExists := resolveRemoteBranch(repo, baseBranch)
		if !baseExists {
			return WriteBackResult{}, fmt.Errorf("gitwriteback: base branch %q not found on remote %s", baseBranch, s.RemotePath)
		}
		if err := checkoutWorkingBranch(repo, wt, workingBranch, baseRef.Hash()); err != nil {
			return WriteBackResult{}, fmt.Errorf("gitwriteback: create branch %s: %w", workingBranch, err)
		}
	}

	filePath := req.Target.FilePath
	originalContent, err := readWorktreeFile(wt, filePath)
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

	if err := writeWorktreeFile(wt, filePath, newContent); err != nil {
		return WriteBackResult{}, fmt.Errorf("gitwriteback: write %s: %w", filePath, err)
	}
	if _, err := wt.Add(filePath); err != nil {
		return WriteBackResult{}, fmt.Errorf("gitwriteback: git add %s: %w", filePath, err)
	}

	commitMsg := buildCommitMessage(effectiveCommitMessage(req), req.IdempotencyKey)
	commitHash, err := wt.Commit(commitMsg, &git.CommitOptions{
		Author: &object.Signature{
			Name:  "argocd-vpa-updater",
			Email: "argocd-vpa-updater@local",
			When:  time.Now(),
		},
	})
	if err != nil {
		return WriteBackResult{}, fmt.Errorf("gitwriteback: commit: %w", err)
	}

	if s.OnBeforePush != nil {
		s.OnBeforePush()
	}

	refSpec := config.RefSpec(fmt.Sprintf("refs/heads/%s:refs/heads/%s", workingBranch, workingBranch))
	if err := repo.PushContext(ctx, &git.PushOptions{
		RemoteName: "origin",
		RefSpecs:   []config.RefSpec{refSpec},
		Auth:       auth,
	}); err != nil {
		switch {
		case errors.Is(err, git.NoErrAlreadyUpToDate):
			// Nothing to push -- another caller with the same idempotency
			// key already landed this exact commit; fall through as success.
		case isNonFastForward(err):
			return WriteBackResult{}, fmt.Errorf("%w: %v", ErrConflict, err)
		default:
			return WriteBackResult{}, fmt.Errorf("gitwriteback: push %s: %w", workingBranch, err)
		}
	}

	return WriteBackResult{
		Status:    domain.OperationApplied,
		Branch:    workingBranch,
		CommitSHA: commitHash.String(),
		PRURL:     prURLFor(req, workingBranch),
	}, nil
}

// workingBranchName returns the branch commits actually land on: the base
// branch itself for a direct commit, or a deterministic topic branch (so
// replaying the same operation reuses the same topic branch) for a
// pull-request write-back.
func workingBranchName(req WriteBackRequest) string {
	if req.Target.WriteBackPolicy == domain.WriteBackPullRequest {
		return fmt.Sprintf("vpa-updater/%s-%s", req.Target.ContainerName, shortHash(req.IdempotencyKey))
	}
	return req.Target.Branch
}

func shortHash(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])[:10]
}

func prURLFor(req WriteBackRequest, branch string) string {
	if req.Target.WriteBackPolicy != domain.WriteBackPullRequest {
		return ""
	}
	return fmt.Sprintf("local://fake-pr/%s", branch)
}

func effectiveCommitMessage(req WriteBackRequest) string {
	if req.CommitMessage != "" {
		return req.CommitMessage
	}
	return fmt.Sprintf("argocd-vpa-updater: update %s resources for container %q", req.Target.FilePath, req.Target.ContainerName)
}

// checkoutWorkingBranch checks the worktree out onto the local branch
// `name`, pointing at `hash`. It handles all three cases go-git's
// Checkout(Create: true) does not by itself: the branch is already checked
// out (a fresh clone already has its default branch checked out locally,
// which is exactly the case when a "commit" policy write-back's working
// branch equals the base branch), the local branch ref already exists but
// isn't checked out, or the local branch does not exist yet and must be
// created.
func checkoutWorkingBranch(repo *git.Repository, wt *git.Worktree, name string, hash plumbing.Hash) error {
	target := plumbing.NewBranchReferenceName(name)

	if head, err := repo.Head(); err == nil && head.Name() == target {
		return nil
	}

	if _, err := repo.Reference(target, true); err == nil {
		return wt.Checkout(&git.CheckoutOptions{Branch: target})
	}

	return wt.Checkout(&git.CheckoutOptions{Branch: target, Create: true, Hash: hash})
}

func resolveRemoteBranch(repo *git.Repository, branch string) (*plumbing.Reference, bool) {
	ref, err := repo.Reference(plumbing.NewRemoteReferenceName("origin", branch), true)
	if err != nil {
		return nil, false
	}
	return ref, true
}

// findIdempotentCommit walks the commit history starting at `from` looking
// for the Vpa-Idempotency-Key trailer matching idempotencyKey.
func findIdempotentCommit(repo *git.Repository, from plumbing.Hash, idempotencyKey string) (plumbing.Hash, bool, error) {
	iter, err := repo.Log(&git.LogOptions{From: from})
	if err != nil {
		return plumbing.ZeroHash, false, err
	}
	defer iter.Close()

	var found plumbing.Hash
	ok := false
	err = iter.ForEach(func(c *object.Commit) error {
		if key, present := extractIdempotencyKey(c.Message); present && key == idempotencyKey {
			found = c.Hash
			ok = true
			return storer.ErrStop
		}
		return nil
	})
	if err != nil {
		return plumbing.ZeroHash, false, err
	}
	return found, ok, nil
}

func readWorktreeFile(wt *git.Worktree, path string) ([]byte, error) {
	f, err := wt.Filesystem.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	return io.ReadAll(f)
}

func writeWorktreeFile(wt *git.Worktree, path string, content []byte) error {
	f, err := wt.Filesystem.Create(path)
	if err != nil {
		return err
	}
	defer f.Close()
	_, err = f.Write(content)
	return err
}

func authMethod(creds domain.GitCredentials) (transport.AuthMethod, error) {
	switch creds.AuthMethod {
	case domain.AuthNone, "":
		return nil, nil
	case domain.AuthBasic, domain.AuthToken:
		return &gogithttp.BasicAuth{Username: creds.Username, Password: creds.Secret}, nil
	case domain.AuthSSH:
		pk, err := ssh.NewPublicKeys("git", creds.SSHPrivateKey, "")
		if err != nil {
			return nil, fmt.Errorf("gitwriteback: parse ssh private key: %w", err)
		}
		return pk, nil
	default:
		return nil, fmt.Errorf("gitwriteback: unsupported auth method %q", creds.AuthMethod)
	}
}

// isNonFastForward matches go-git's push rejection error. go-git does not
// export a sentinel for this (remote.go builds it with a plain
// fmt.Errorf("non-fast-forward update: %s", ...)), so this matches on the
// stable prefix of that message.
func isNonFastForward(err error) bool {
	return strings.Contains(err.Error(), "non-fast-forward update")
}
