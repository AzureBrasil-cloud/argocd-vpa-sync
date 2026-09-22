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
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/azurebrasil/argocd-vpa-updater/internal/domain"
)

// ErrNonFastForward is returned by Push when the remote rejected the push
// because its branch tip moved since the local commit was based on it (a
// concurrent edit landed first). Callers must never retry with --force;
// internal/gitwriteback wraps this as ErrConflict.
var ErrNonFastForward = errors.New("gitexec: push rejected: remote has diverged (non-fast-forward)")

// GitIdentity is the author/committer identity Commit records. Real callers
// always pass a fixed identity (see internal/gitwriteback); it's a parameter
// only so tests can use their own.
type GitIdentity struct {
	Name  string
	Email string
}

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

// CloneFull is Clone without --depth 1 --single-branch: a full clone with
// complete history on branch. Callers must use this (not Clone) whenever
// they need to walk a branch's full commit history -- e.g. scanning an
// existing working branch for a prior idempotency-trailer match, where a
// shallow clone would silently truncate the scan to just the tip commit.
func (r *Runner) CloneFull(ctx context.Context, dir, repoURL, branch string, creds domain.GitCredentials) error {
	env, cleanup, err := r.prepareCredentials(creds)
	if err != nil {
		return fmt.Errorf("gitexec: prepare credentials: %w", err)
	}
	defer cleanup()

	return r.run(ctx, env,
		"-c", "safe.directory=*",
		"-c", "core.autocrlf=false",
		"clone",
		"--branch", branch,
		"--",
		repoURL,
		dir,
	)
}

// RemoteBranchExists reports whether branch exists on repoURL, without
// cloning.
func (r *Runner) RemoteBranchExists(ctx context.Context, repoURL, branch string, creds domain.GitCredentials) (bool, error) {
	env, cleanup, err := r.prepareCredentials(creds)
	if err != nil {
		return false, fmt.Errorf("gitexec: prepare credentials: %w", err)
	}
	defer cleanup()

	args := []string{"ls-remote", "--exit-code", "--heads", "--", repoURL, branch}
	res, err := r.runCapture(ctx, env, args...)
	if err != nil {
		return false, err
	}
	switch res.exitCode {
	case 0:
		return true, nil
	case 2:
		// "ls-remote --exit-code" reserves exit code 2 specifically for "no
		// matching refs" -- not an error, just "doesn't exist yet".
		return false, nil
	default:
		return false, exitError(args, res)
	}
}

// CheckoutNewBranch creates and checks out a new local branch at whatever
// commit is currently checked out in dir. Used only when a brand-new
// pull-request topic branch is being created (never for the "commit" policy,
// where the working branch already equals the freshly cloned base branch).
func (r *Runner) CheckoutNewBranch(ctx context.Context, dir, branch string) error {
	return r.run(ctx, nil, "-c", "safe.directory=*", "-C", dir, "checkout", "-b", branch)
}

// Add stages path (relative to dir) in the repository at dir.
func (r *Runner) Add(ctx context.Context, dir, path string) error {
	return r.run(ctx, nil, "-c", "safe.directory=*", "-C", dir, "add", "--", path)
}

// Commit creates a commit in the repository at dir with an explicit
// author/committer identity (passed via env, never the ambient git config --
// HOME isolation during Clone would leave that unset anyway) and returns the
// new commit's SHA.
func (r *Runner) Commit(ctx context.Context, dir, message string, author GitIdentity) (string, error) {
	env := []string{
		"GIT_AUTHOR_NAME=" + author.Name,
		"GIT_AUTHOR_EMAIL=" + author.Email,
		"GIT_COMMITTER_NAME=" + author.Name,
		"GIT_COMMITTER_EMAIL=" + author.Email,
	}
	if err := r.run(ctx, env, "-c", "safe.directory=*", "-C", dir, "commit", "--no-gpg-sign", "-m", message); err != nil {
		return "", err
	}
	return r.HeadCommitSHA(ctx, dir)
}

// HeadCommitSHA returns the SHA of the commit currently checked out in dir.
func (r *Runner) HeadCommitSHA(ctx context.Context, dir string) (string, error) {
	args := []string{"-c", "safe.directory=*", "-C", dir, "rev-parse", "HEAD"}
	res, err := r.runCapture(ctx, nil, args...)
	if err != nil {
		return "", err
	}
	if res.exitCode != 0 {
		return "", exitError(args, res)
	}
	return strings.TrimSpace(res.stdout), nil
}

// Push pushes dir's local branch localBranch to remoteBranch on repoURL,
// never force. Returns ErrNonFastForward (wrapped) specifically when the
// remote rejected the push because its tip moved since dir was cloned; any
// other push failure (auth, network, hook rejection) is returned unwrapped,
// since force-pushing or retrying wouldn't fix those and callers must not
// confuse them with a benign concurrent-edit race.
func (r *Runner) Push(ctx context.Context, dir, repoURL, localBranch, remoteBranch string, creds domain.GitCredentials) error {
	env, cleanup, err := r.prepareCredentials(creds)
	if err != nil {
		return fmt.Errorf("gitexec: prepare credentials: %w", err)
	}
	defer cleanup()

	refSpec := localBranch + ":refs/heads/" + remoteBranch
	args := []string{"-c", "safe.directory=*", "-C", dir, "push", "--porcelain", "--", repoURL, refSpec}
	res, err := r.runCapture(ctx, env, args...)
	if err != nil {
		return err
	}
	if res.exitCode == 0 {
		return nil
	}

	remoteRef := "refs/heads/" + remoteBranch
	for _, line := range strings.Split(res.stdout, "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) != 3 || fields[0] != "!" {
			continue
		}
		if !strings.HasSuffix(fields[1], ":"+remoteRef) && fields[1] != localBranch+":"+remoteRef {
			continue
		}
		reason := fields[2]
		if strings.Contains(reason, "non-fast-forward") || strings.Contains(reason, "fetch first") || strings.Contains(reason, "stale info") {
			return fmt.Errorf("%w: %s", ErrNonFastForward, strings.TrimSpace(res.stderr))
		}
	}
	return exitError(args, res)
}

// CommitMessage is one entry returned by LogMessages: a commit's full hash
// and its complete message body.
type CommitMessage struct {
	SHA  string
	Body string
}

// LogMessages returns the full message body of every commit reachable from
// ref (most recent first), for a caller to scan for the write-back
// idempotency trailer. maxCount<=0 means unbounded.
func (r *Runner) LogMessages(ctx context.Context, dir, ref string, maxCount int) ([]CommitMessage, error) {
	args := []string{"-c", "safe.directory=*", "-C", dir, "log", "--format=%H%x00%B%x02"}
	if maxCount > 0 {
		args = append(args, fmt.Sprintf("--max-count=%d", maxCount))
	}
	args = append(args, ref)

	res, err := r.runCapture(ctx, nil, args...)
	if err != nil {
		return nil, err
	}
	if res.exitCode != 0 {
		return nil, exitError(args, res)
	}

	var out []CommitMessage
	for _, record := range strings.Split(res.stdout, "\x02") {
		// git's default pretty-printing adds a newline after each record
		// (before %H of the next one); strip exactly that one separator, not
		// any newline that's part of the commit body itself.
		record = strings.TrimPrefix(record, "\n")
		if record == "" {
			continue
		}
		parts := strings.SplitN(record, "\x00", 2)
		if len(parts) != 2 {
			continue
		}
		out = append(out, CommitMessage{SHA: parts[0], Body: parts[1]})
	}
	return out, nil
}

type execResult struct {
	stdout   string
	stderr   string
	exitCode int
}

// runCapture runs git with both stdout and stderr captured, and never treats
// a non-zero exit code alone as a Go error -- callers that care about exit
// codes (RemoteBranchExists, Push) inspect res.exitCode themselves. Only a
// failure to start/run the process at all (e.g. context cancellation) is
// returned as an error here.
func (r *Runner) runCapture(ctx context.Context, extraEnv []string, args ...string) (execResult, error) {
	cmd := exec.CommandContext(ctx, r.gitBinary(), args...)
	cmd.Env = append(baseEnv(), extraEnv...)

	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err := cmd.Run()
	res := execResult{stdout: stdout.String(), stderr: stderr.String()}

	var exitErr *exec.ExitError
	if err == nil {
		return res, nil
	}
	if errors.As(err, &exitErr) {
		res.exitCode = exitErr.ExitCode()
		return res, nil
	}
	// args never contain credential material (see prepareCredentials), only
	// repo URLs/branches/paths, so it's always safe to include them here.
	return res, fmt.Errorf("git %s: %w", strings.Join(args, " "), err)
}

func exitError(args []string, res execResult) error {
	msg := strings.TrimSpace(res.stderr)
	if msg == "" {
		msg = fmt.Sprintf("exit status %d", res.exitCode)
	}
	return fmt.Errorf("git %s: %s", strings.Join(args, " "), msg)
}

func (r *Runner) run(ctx context.Context, extraEnv []string, args ...string) error {
	res, err := r.runCapture(ctx, extraEnv, args...)
	if err != nil {
		return err
	}
	if res.exitCode != 0 {
		return exitError(args, res)
	}
	return nil
}

func baseEnv() []string {
	return []string{
		"PATH=" + os.Getenv("PATH"),
		// Never prompt interactively -- a hung prompt would otherwise block
		// the calling goroutine indefinitely.
		"GIT_TERMINAL_PROMPT=0",
		// Force English, untranslated git output -- callers (Push) match on
		// specific English substrings in porcelain/stderr output to detect a
		// non-fast-forward rejection; a localized git build would otherwise
		// make that matching silently locale-dependent.
		"LC_ALL=C",
		"LANG=C",
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
