package gitrepo

import (
	"context"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"github.com/azurebrasil/argocd-vpa-updater/internal/domain"
	"github.com/azurebrasil/argocd-vpa-updater/internal/gittest"
)

func TestGitCLIReader_ReadFile(t *testing.T) {
	remote := gittest.NewBareRepoWithInitialCommit(t, map[string]string{
		"deploy/checkout-api.yaml": "resources:\n  requests:\n    cpu: 100m\n",
	})

	r := NewGitCLIReader()
	content, err := r.ReadFile(context.Background(), remote, gittest.DefaultBranch, "deploy/checkout-api.yaml", domain.GitCredentials{AuthMethod: domain.AuthNone})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(content), "cpu: 100m") {
		t.Fatalf("unexpected content: %s", content)
	}
}

func TestGitCLIReader_MissingFile(t *testing.T) {
	remote := gittest.NewBareRepoWithInitialCommit(t, map[string]string{"deploy.yaml": "a: b\n"})

	r := NewGitCLIReader()
	if _, err := r.ReadFile(context.Background(), remote, gittest.DefaultBranch, "does-not-exist.yaml", domain.GitCredentials{AuthMethod: domain.AuthNone}); err == nil {
		t.Fatalf("expected an error for a missing file")
	}
}

func TestGitCLIReader_NonDefaultBranch(t *testing.T) {
	remote := gittest.NewBareRepoWithInitialCommit(t, map[string]string{"deploy.yaml": "cpu: 100m\n"})

	// Push a second branch that is never checked out locally by a plain
	// clone (only reachable via refs/remotes/origin/<branch>), exercising
	// resolveBranch's fallback path.
	repo := gittest.CloneWorktree(t, remote)
	wt, err := repo.Worktree()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := wt.Checkout(&git.CheckoutOptions{Branch: plumbing.NewBranchReferenceName("feature"), Create: true}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	gittest.WriteFile(t, wt, "deploy.yaml", "cpu: 200m\n")
	if _, err := wt.Add("deploy.yaml"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := wt.Commit("feature branch commit", &git.CommitOptions{Author: &object.Signature{Name: "t", Email: "t@local"}}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := repo.Push(&git.PushOptions{RemoteName: "origin"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r := NewGitCLIReader()
	content, err := r.ReadFile(context.Background(), remote, "feature", "deploy.yaml", domain.GitCredentials{AuthMethod: domain.AuthNone})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(content), "cpu: 200m") {
		t.Fatalf("expected the feature branch's content, got: %s", content)
	}
}

func TestGitCLIReader_UnknownBranch(t *testing.T) {
	remote := gittest.NewBareRepoWithInitialCommit(t, map[string]string{"deploy.yaml": "a: b\n"})

	r := NewGitCLIReader()
	if _, err := r.ReadFile(context.Background(), remote, "does-not-exist", "deploy.yaml", domain.GitCredentials{AuthMethod: domain.AuthNone}); err == nil {
		t.Fatalf("expected an error for a nonexistent branch")
	}
}
