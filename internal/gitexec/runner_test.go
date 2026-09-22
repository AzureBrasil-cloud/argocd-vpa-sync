package gitexec

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/azurebrasil/argocd-vpa-updater/internal/domain"
	"github.com/azurebrasil/argocd-vpa-updater/internal/gittest"
)

func noAuth() domain.GitCredentials {
	return domain.GitCredentials{AuthMethod: domain.AuthNone}
}

func cloneInto(t *testing.T, remote, branch string) string {
	t.Helper()
	dir := filepath.Join(t.TempDir(), "clone")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := NewRunner().Clone(context.Background(), dir, remote, branch, noAuth()); err != nil {
		t.Fatalf("unexpected error cloning: %v", err)
	}
	return dir
}

func TestRunner_Clone_AuthNone(t *testing.T) {
	remote := gittest.NewBareRepoWithInitialCommit(t, map[string]string{
		"deploy.yaml": "cpu: 100m\n",
	})

	dir := t.TempDir()
	cloneDir := filepath.Join(dir, "clone")
	if err := os.MkdirAll(cloneDir, 0o755); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	r := NewRunner()
	if err := r.Clone(context.Background(), cloneDir, remote, gittest.DefaultBranch, domain.GitCredentials{AuthMethod: domain.AuthNone}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	content, err := os.ReadFile(filepath.Join(cloneDir, "deploy.yaml"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(content) != "cpu: 100m\n" {
		t.Fatalf("unexpected content: %q", content)
	}
}

func TestRunner_Clone_UnknownBranch(t *testing.T) {
	remote := gittest.NewBareRepoWithInitialCommit(t, map[string]string{"deploy.yaml": "a: b\n"})

	r := NewRunner()
	err := r.Clone(context.Background(), t.TempDir(), remote, "does-not-exist", domain.GitCredentials{AuthMethod: domain.AuthNone})
	if err == nil {
		t.Fatalf("expected an error for a nonexistent branch")
	}
}

func TestRunner_Clone_UnsupportedAuthMethod(t *testing.T) {
	r := NewRunner()
	err := r.Clone(context.Background(), t.TempDir(), "https://example.com/repo.git", "main", domain.GitCredentials{AuthMethod: "carrier-pigeon"})
	if err == nil {
		t.Fatalf("expected an error for an unsupported auth method")
	}
}

func TestPrepareCredentials_SSHWritesKeyFileWithRestrictedPermissions(t *testing.T) {
	r := NewRunner()
	env, cleanup, err := r.prepareCredentials(domain.GitCredentials{
		AuthMethod:    domain.AuthSSH,
		SSHPrivateKey: []byte("fake-key-material"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cleanup()

	var keyDir string
	for _, kv := range env {
		if len(kv) > 5 && kv[:5] == "HOME=" {
			keyDir = kv[5:]
		}
	}
	if keyDir == "" {
		t.Fatalf("expected a HOME env var to be set")
	}

	info, err := os.Stat(filepath.Join(keyDir, "id"))
	if err != nil {
		t.Fatalf("expected the private key file to exist: %v", err)
	}
	// Windows/NTFS doesn't map POSIX permission bits the way os.WriteFile's
	// mode argument requests; this check is only meaningful on POSIX.
	if runtime.GOOS != "windows" && info.Mode().Perm()&0o077 != 0 {
		t.Fatalf("expected the private key file to not be group/world accessible, got mode %v", info.Mode())
	}
}

func TestNormalizePEM(t *testing.T) {
	cases := []struct {
		name string
		in   string
		want string
	}{
		{"already normalized", "line1\nline2\n", "line1\nline2\n"},
		{"CRLF line endings", "line1\r\nline2\r\n", "line1\nline2\n"},
		{"missing trailing newline", "line1\nline2", "line1\nline2\n"},
		{"trailing blank lines collapsed", "line1\nline2\n\n\n", "line1\nline2\n"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := string(normalizePEM([]byte(tc.in)))
			if got != tc.want {
				t.Fatalf("normalizePEM(%q) = %q, want %q", tc.in, got, tc.want)
			}
		})
	}
}

func TestPrepareCredentials_SSHKeyIsNormalizedOnDisk(t *testing.T) {
	r := NewRunner()
	env, cleanup, err := r.prepareCredentials(domain.GitCredentials{
		AuthMethod:    domain.AuthSSH,
		SSHPrivateKey: []byte("line1\r\nline2\r\n"),
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	defer cleanup()

	var homeDir string
	for _, kv := range env {
		if len(kv) > 5 && kv[:5] == "HOME=" {
			homeDir = kv[5:]
		}
	}
	content, err := os.ReadFile(filepath.Join(homeDir, "id"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(content) != "line1\nline2\n" {
		t.Fatalf("expected CRLF to be normalized on disk, got %q", content)
	}
}

func TestBaseEnv_ForcesCLocale(t *testing.T) {
	env := baseEnv()
	wantLCAll, wantLang := false, false
	for _, kv := range env {
		if kv == "LC_ALL=C" {
			wantLCAll = true
		}
		if kv == "LANG=C" {
			wantLang = true
		}
	}
	if !wantLCAll || !wantLang {
		t.Fatalf("expected baseEnv() to force LC_ALL=C and LANG=C, got %v", env)
	}
}

func TestRunner_RemoteBranchExists(t *testing.T) {
	remote := gittest.NewBareRepoWithInitialCommit(t, map[string]string{"f.txt": "a\n"})
	r := NewRunner()

	exists, err := r.RemoteBranchExists(context.Background(), remote, gittest.DefaultBranch, noAuth())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !exists {
		t.Fatalf("expected %q to exist", gittest.DefaultBranch)
	}

	exists, err = r.RemoteBranchExists(context.Background(), remote, "does-not-exist", noAuth())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if exists {
		t.Fatalf("expected does-not-exist to not exist")
	}
}

func TestRunner_AddCommitPush_FullFlow(t *testing.T) {
	remote := gittest.NewBareRepoWithInitialCommit(t, map[string]string{"f.txt": "a\n"})
	dir := cloneInto(t, remote, gittest.DefaultBranch)
	r := NewRunner()
	ctx := context.Background()

	if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte("b\n"), 0o644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := r.Add(ctx, dir, "f.txt"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	sha, err := r.Commit(ctx, dir, "update f.txt\n\nVpa-Idempotency-Key: sha256:test\n", GitIdentity{Name: "argocd-vpa-updater", Email: "argocd-vpa-updater@local"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sha == "" {
		t.Fatalf("expected a non-empty commit SHA")
	}

	head, err := r.HeadCommitSHA(ctx, dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if head != sha {
		t.Fatalf("expected HeadCommitSHA %q to equal Commit's returned SHA %q", head, sha)
	}

	if err := r.Push(ctx, dir, remote, gittest.DefaultBranch, gittest.DefaultBranch, noAuth()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	verify := cloneInto(t, remote, gittest.DefaultBranch)
	content, err := os.ReadFile(filepath.Join(verify, "f.txt"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(content) != "b\n" {
		t.Fatalf("expected pushed content %q, got %q", "b\n", content)
	}

	msgs, err := r.LogMessages(ctx, dir, "HEAD", 0)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 2 {
		t.Fatalf("expected 2 commits in history, got %d", len(msgs))
	}
	if msgs[0].SHA != sha {
		t.Fatalf("expected the most recent LogMessages entry to be the new commit %q, got %q", sha, msgs[0].SHA)
	}
	if !strings.Contains(msgs[0].Body, "Vpa-Idempotency-Key: sha256:test") {
		t.Fatalf("expected the commit body to contain the idempotency trailer, got %q", msgs[0].Body)
	}
}

func TestRunner_LogMessages_MaxCount(t *testing.T) {
	remote := gittest.NewBareRepoWithInitialCommit(t, map[string]string{"f.txt": "a\n"})
	dir := cloneInto(t, remote, gittest.DefaultBranch)
	r := NewRunner()
	ctx := context.Background()

	for i, content := range []string{"b\n", "c\n"} {
		if err := os.WriteFile(filepath.Join(dir, "f.txt"), []byte(content), 0o644); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if err := r.Add(ctx, dir, "f.txt"); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if _, err := r.Commit(ctx, dir, "commit body only, no trailer\n", GitIdentity{Name: "t", Email: "t@local"}); err != nil {
			t.Fatalf("unexpected error committing #%d: %v", i, err)
		}
	}

	msgs, err := r.LogMessages(ctx, dir, "HEAD", 1)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(msgs) != 1 {
		t.Fatalf("expected exactly 1 message with maxCount=1, got %d", len(msgs))
	}
}

// TestRunner_Push_NonFastForward reproduces a real concurrent-edit race: two
// independent clones of the same repo both commit locally, the first one
// pushes successfully, and the second one's push must be rejected as
// ErrNonFastForward -- never force-pushed, never silently dropped.
func TestRunner_Push_NonFastForward(t *testing.T) {
	remote := gittest.NewBareRepoWithInitialCommit(t, map[string]string{"f.txt": "a\n"})
	r := NewRunner()
	ctx := context.Background()

	dirA := cloneInto(t, remote, gittest.DefaultBranch)
	dirB := cloneInto(t, remote, gittest.DefaultBranch)

	if err := os.WriteFile(filepath.Join(dirA, "f.txt"), []byte("from-a\n"), 0o644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := r.Add(ctx, dirA, "f.txt"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := r.Commit(ctx, dirA, "change from a", GitIdentity{Name: "a", Email: "a@local"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := r.Push(ctx, dirA, remote, gittest.DefaultBranch, gittest.DefaultBranch, noAuth()); err != nil {
		t.Fatalf("unexpected error pushing from a: %v", err)
	}

	if err := os.WriteFile(filepath.Join(dirB, "f.txt"), []byte("from-b\n"), 0o644); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := r.Add(ctx, dirB, "f.txt"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if _, err := r.Commit(ctx, dirB, "change from b", GitIdentity{Name: "b", Email: "b@local"}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	err := r.Push(ctx, dirB, remote, gittest.DefaultBranch, gittest.DefaultBranch, noAuth())
	if err == nil {
		t.Fatalf("expected an error pushing b's stale branch")
	}
	if !errors.Is(err, ErrNonFastForward) {
		t.Fatalf("expected ErrNonFastForward, got %v", err)
	}

	verify := cloneInto(t, remote, gittest.DefaultBranch)
	content, err := os.ReadFile(filepath.Join(verify, "f.txt"))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if string(content) != "from-a\n" {
		t.Fatalf("expected the remote to still reflect only a's change, got %q", content)
	}
}

func TestRunner_CheckoutNewBranch(t *testing.T) {
	remote := gittest.NewBareRepoWithInitialCommit(t, map[string]string{"f.txt": "a\n"})
	dir := cloneInto(t, remote, gittest.DefaultBranch)
	r := NewRunner()

	if err := r.CheckoutNewBranch(context.Background(), dir, "vpa-updater/app-abc123"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if err := r.Push(context.Background(), dir, remote, "vpa-updater/app-abc123", "vpa-updater/app-abc123", noAuth()); err != nil {
		t.Fatalf("unexpected error pushing new topic branch: %v", err)
	}

	exists, err := r.RemoteBranchExists(context.Background(), remote, "vpa-updater/app-abc123", noAuth())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !exists {
		t.Fatalf("expected the new topic branch to exist on the remote after push")
	}
}

func TestPrepareCredentials_CleanupRemovesTempDir(t *testing.T) {
	r := NewRunner()
	env, cleanup, err := r.prepareCredentials(domain.GitCredentials{AuthMethod: domain.AuthNone})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	var homeDir string
	for _, kv := range env {
		if len(kv) > 5 && kv[:5] == "HOME=" {
			homeDir = kv[5:]
		}
	}
	cleanup()
	if _, err := os.Stat(homeDir); !os.IsNotExist(err) {
		t.Fatalf("expected the temp credential dir to be removed after cleanup")
	}
}
