package gitexec

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/azurebrasil/argocd-vpa-updater/internal/domain"
	"github.com/azurebrasil/argocd-vpa-updater/internal/gittest"
)

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
