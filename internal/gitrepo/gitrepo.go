// Package gitrepo provides read-only Git access to a file's declared
// content -- needed by internal/gitwriteback's real (not yet wired up)
// implementation before it can patch a file. The dashboard's own "current
// value" comparison no longer goes through this package: it reads the live
// workload instead (see internal/workloadresources), since that's what a
// VPA recommendation is actually about, and a pending Argo CD sync can
// leave Git and the cluster briefly disagreeing. Write access lives in
// internal/gitwriteback; this package is deliberately read-only.
package gitrepo

import (
	"context"
	"fmt"
	"os"
	"path/filepath"

	"github.com/azurebrasil/argocd-vpa-updater/internal/domain"
	"github.com/azurebrasil/argocd-vpa-updater/internal/gitexec"
)

// Reader reads a single file's content from a Git repo at a given branch.
type Reader interface {
	ReadFile(ctx context.Context, repoURL, branch, path string, creds domain.GitCredentials) ([]byte, error)
}

// GitCLIReader is the default Reader. It shells out to the real git binary
// (via internal/gitexec) rather than using a pure-Go Git implementation --
// see internal/gitexec's package doc for why. Each call performs a fresh,
// shallow, single-branch clone into a temp directory that is removed before
// returning; there is no long-lived local checkout to keep in sync.
type GitCLIReader struct {
	runner *gitexec.Runner
}

// NewGitCLIReader builds a GitCLIReader.
func NewGitCLIReader() *GitCLIReader {
	return &GitCLIReader{runner: gitexec.NewRunner()}
}

func (r *GitCLIReader) ReadFile(ctx context.Context, repoURL, branch, path string, creds domain.GitCredentials) ([]byte, error) {
	dir, err := os.MkdirTemp("", "argocd-vpa-updater-read-*")
	if err != nil {
		return nil, fmt.Errorf("gitrepo: create scratch dir: %w", err)
	}
	defer os.RemoveAll(dir)

	if err := r.runner.Clone(ctx, dir, repoURL, branch, creds); err != nil {
		return nil, fmt.Errorf("gitrepo: clone %s@%s: %w", repoURL, branch, err)
	}

	content, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(path)))
	if err != nil {
		return nil, fmt.Errorf("gitrepo: read %s: %w", path, err)
	}
	return content, nil
}
