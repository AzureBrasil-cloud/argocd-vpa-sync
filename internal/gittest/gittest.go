// Package gittest provides small helpers for tests that need a real local
// Git repository (bare "remote" + an initial commit), shared between
// internal/gitwriteback's unit tests and the top-level integration test.
package gittest

import (
	"os"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/config"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
)

// DefaultBranch is the branch NewBareRepoWithInitialCommit seeds.
const DefaultBranch = "main"

// NewBareRepoWithInitialCommit creates a bare repository (standing in for a
// Git remote) at a fresh temp directory, seeded with one commit containing
// the given files on DefaultBranch. It returns the bare repo's filesystem
// path, suitable for use as a FakeGitWriteBackService.RemotePath or as a
// go-git clone URL.
func NewBareRepoWithInitialCommit(t *testing.T, files map[string]string) string {
	t.Helper()

	bareDir := t.TempDir()
	if _, err := git.PlainInitWithOptions(bareDir, &git.PlainInitOptions{
		Bare:        true,
		InitOptions: git.InitOptions{DefaultBranch: plumbing.NewBranchReferenceName(DefaultBranch)},
	}); err != nil {
		t.Fatalf("gittest: init bare repo: %v", err)
	}

	seedDir, err := os.MkdirTemp("", "gittest-seed-*")
	if err != nil {
		t.Fatalf("gittest: create seed dir: %v", err)
	}
	defer os.RemoveAll(seedDir)

	seedRepo, err := git.PlainInitWithOptions(seedDir, &git.PlainInitOptions{
		InitOptions: git.InitOptions{DefaultBranch: plumbing.NewBranchReferenceName(DefaultBranch)},
	})
	if err != nil {
		t.Fatalf("gittest: init seed repo: %v", err)
	}

	wt, err := seedRepo.Worktree()
	if err != nil {
		t.Fatalf("gittest: seed worktree: %v", err)
	}
	for path, content := range files {
		WriteFile(t, wt, path, content)
		if _, err := wt.Add(path); err != nil {
			t.Fatalf("gittest: add %s: %v", path, err)
		}
	}
	if _, err := wt.Commit("initial commit", &git.CommitOptions{
		Author: &object.Signature{Name: "gittest", Email: "gittest@local"},
	}); err != nil {
		t.Fatalf("gittest: initial commit: %v", err)
	}

	if _, err := seedRepo.CreateRemote(&config.RemoteConfig{
		Name: "origin",
		URLs: []string{bareDir},
	}); err != nil {
		t.Fatalf("gittest: add remote: %v", err)
	}
	if err := seedRepo.Push(&git.PushOptions{RemoteName: "origin"}); err != nil {
		t.Fatalf("gittest: push seed commit: %v", err)
	}

	return bareDir
}

// WriteFile writes content to path within a worktree's filesystem, creating
// parent directories as needed.
func WriteFile(t *testing.T, wt *git.Worktree, path, content string) {
	t.Helper()
	f, err := wt.Filesystem.Create(path)
	if err != nil {
		t.Fatalf("gittest: create %s: %v", path, err)
	}
	defer f.Close()
	if _, err := f.Write([]byte(content)); err != nil {
		t.Fatalf("gittest: write %s: %v", path, err)
	}
}

// CloneWorktree clones repoPath into a fresh temp directory and returns the
// opened repository, for tests that need to independently verify committed
// content (rather than trusting the writer's own in-process state).
func CloneWorktree(t *testing.T, repoPath string) *git.Repository {
	t.Helper()
	dir := t.TempDir()
	repo, err := git.PlainClone(dir, false, &git.CloneOptions{URL: repoPath})
	if err != nil {
		t.Fatalf("gittest: clone %s: %v", repoPath, err)
	}
	return repo
}

// ReadFile reads path from a worktree's filesystem.
func ReadFile(t *testing.T, wt *git.Worktree, path string) string {
	t.Helper()
	f, err := wt.Filesystem.Open(path)
	if err != nil {
		t.Fatalf("gittest: open %s: %v", path, err)
	}
	defer f.Close()
	buf := make([]byte, 0, 4096)
	tmp := make([]byte, 4096)
	for {
		n, err := f.Read(tmp)
		buf = append(buf, tmp[:n]...)
		if err != nil {
			break
		}
	}
	return string(buf)
}
