// Package gitexec shells out to the real `git` binary rather than using a
// pure-Go Git implementation. This exists specifically because go-git was
// found, in production, to fail against at least one real Azure DevOps
// repository with an opaque "object not found" error even on a full clone
// with valid credentials -- while the organization's own git-based tooling
// (which shells out to real git) works against the same repository. Nothing
// here is Azure-DevOps-specific; it is a generic git-CLI wrapper.
package gitexec

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/azurebrasil/argocd-vpa-updater/internal/domain"
)

// Runner shells out to the git CLI, materializing SSH/HTTP credentials into
// a per-invocation temp directory (private key file, known_hosts, askpass
// script) that is removed as soon as the call returns. Credential material
// is never written to a persistent location, never appears in a command-line
// argument (so it can't leak via /proc/<pid>/cmdline or process listings),
// and never appears in a returned error message.
type Runner struct {
	// GitBinary is the git executable to invoke; defaults to "git" resolved
	// via PATH.
	GitBinary string
}

// NewRunner builds a Runner using "git" from PATH.
func NewRunner() *Runner {
	return &Runner{GitBinary: "git"}
}

func (r *Runner) gitBinary() string {
	if r.GitBinary != "" {
		return r.GitBinary
	}
	return "git"
}

// Clone clones repoURL at branch into dir (a fresh, caller-owned, empty
// directory) with a shallow, single-branch checkout.
func (r *Runner) Clone(ctx context.Context, dir, repoURL, branch string, creds domain.GitCredentials) error {
	env, cleanup, err := r.prepareCredentials(creds)
	if err != nil {
		return fmt.Errorf("gitexec: prepare credentials: %w", err)
	}
	defer cleanup()

	return r.run(ctx, env,
		"-c", "safe.directory=*",
		// Never let git rewrite line endings on checkout -- the files this
		// reads are parsed byte-for-byte by internal/patcher, and silent
		// CRLF rewriting would corrupt that comparison regardless of
		// platform.
		"-c", "core.autocrlf=false",
		"clone",
		"--depth", "1",
		"--single-branch",
		"--branch", branch,
		"--",
		repoURL,
		dir,
	)
}

func (r *Runner) run(ctx context.Context, extraEnv []string, args ...string) error {
	cmd := exec.CommandContext(ctx, r.gitBinary(), args...)
	cmd.Env = append(baseEnv(), extraEnv...)

	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		msg := strings.TrimSpace(stderr.String())
		if msg == "" {
			msg = err.Error()
		}
		// args never contain credential material (see prepareCredentials),
		// only repo URLs/branches/paths, so it's always safe to include them
		// here.
		return fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
	}
	return nil
}

func baseEnv() []string {
	return []string{
		"PATH=" + os.Getenv("PATH"),
		// Never prompt interactively -- a hung prompt would otherwise block
		// the calling goroutine indefinitely.
		"GIT_TERMINAL_PROMPT=0",
	}
}

// prepareCredentials returns the extra environment variables needed to
// authenticate this one git invocation, plus a cleanup func the caller must
// always invoke (even on error) to remove any temp files it created.
func (r *Runner) prepareCredentials(creds domain.GitCredentials) ([]string, func(), error) {
	dir, err := os.MkdirTemp("", "argocd-vpa-updater-gitexec-*")
	if err != nil {
		return nil, func() {}, err
	}
	cleanup := func() { _ = os.RemoveAll(dir) }

	// HOME is overridden to this fresh, empty temp dir so git never reads a
	// real user's ~/.gitconfig or ~/.ssh -- each invocation is isolated.
	env := []string{"HOME=" + dir}

	switch creds.AuthMethod {
	case domain.AuthNone, "":
		return env, cleanup, nil

	case domain.AuthSSH:
		keyPath := filepath.Join(dir, "id")
		if err := os.WriteFile(keyPath, normalizePEM(creds.SSHPrivateKey), 0o600); err != nil {
			cleanup()
			return nil, func() {}, fmt.Errorf("write ssh private key: %w", err)
		}

		sshCmd := "ssh -i " + shellQuote(keyPath) + " -o IdentitiesOnly=yes -o StrictHostKeyChecking=yes"
		switch {
		case len(creds.SSHKnownHosts) > 0:
			khPath := filepath.Join(dir, "known_hosts")
			if err := os.WriteFile(khPath, creds.SSHKnownHosts, 0o600); err != nil {
				cleanup()
				return nil, func() {}, fmt.Errorf("write known_hosts: %w", err)
			}
			sshCmd += " -o UserKnownHostsFile=" + shellQuote(khPath)
		case os.Getenv("SSH_KNOWN_HOSTS") != "":
			// Falls back to the same known_hosts file the rest of the
			// process already uses (see deploy/manifests: mounted from
			// Argo CD's own argocd-ssh-known-hosts-cm ConfigMap).
			sshCmd += " -o UserKnownHostsFile=" + shellQuote(os.Getenv("SSH_KNOWN_HOSTS"))
		}

		env = append(env, "GIT_SSH_COMMAND="+sshCmd)
		return env, cleanup, nil

	case domain.AuthBasic, domain.AuthToken:
		scriptPath := filepath.Join(dir, "askpass.sh")
		script := "#!/bin/sh\ncase \"$1\" in\nUsername*) echo \"$GIT_ASKPASS_USERNAME\" ;;\n*) echo \"$GIT_ASKPASS_PASSWORD\" ;;\nesac\n"
		if err := os.WriteFile(scriptPath, []byte(script), 0o700); err != nil {
			cleanup()
			return nil, func() {}, fmt.Errorf("write askpass script: %w", err)
		}

		env = append(env,
			"GIT_ASKPASS="+scriptPath,
			"GIT_ASKPASS_USERNAME="+creds.Username,
			"GIT_ASKPASS_PASSWORD="+creds.Secret,
		)
		return env, cleanup, nil

	default:
		cleanup()
		return nil, func() {}, fmt.Errorf("unsupported auth method %q", creds.AuthMethod)
	}
}

// shellQuote POSIX-single-quotes s for safe embedding in GIT_SSH_COMMAND,
// which git parses as a shell command string.
func shellQuote(s string) string {
	return "'" + strings.ReplaceAll(s, "'", `'\''`) + "'"
}

// normalizePEM defensively normalizes PEM-encoded key material before it
// hits disk: real OpenSSH (unlike go-git's pure-Go SSH client, which is
// far more lenient) can reject an otherwise-valid private key with an
// opaque "error in libcrypto" if it contains CR line endings or is missing
// its final newline -- both are easy to end up with when key material has
// passed through a Kubernetes Secret, a YAML manifest, or a copy/paste on
// Windows at some point in its life.
func normalizePEM(key []byte) []byte {
	key = bytes.ReplaceAll(key, []byte("\r\n"), []byte("\n"))
	key = bytes.TrimRight(key, "\n")
	return append(key, '\n')
}
