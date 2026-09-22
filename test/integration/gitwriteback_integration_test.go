// Package integration exercises FakeGitWriteBackService end to end against
// real local Git repositories (via go-git, no external git binary), from
// outside the gitwriteback package -- i.e. the way a real caller (the
// future API/controller layer) would use it.
package integration

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing/object"
	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/azurebrasil/argocd-vpa-updater/internal/domain"
	"github.com/azurebrasil/argocd-vpa-updater/internal/gittest"
	"github.com/azurebrasil/argocd-vpa-updater/internal/gitwriteback"
	"github.com/azurebrasil/argocd-vpa-updater/internal/idempotency"
	"github.com/azurebrasil/argocd-vpa-updater/internal/patcher"
)

const manifestWithComments = `# checkout-api deployment -- do not remove this header
apiVersion: apps/v1
kind: Deployment
metadata:
  name: checkout-api
spec:
  template:
    spec:
      containers:
        - name: app
          image: example.com/checkout-api:1.2.3
          resources:
            requests:
              cpu: 100m # tuned by hand
              memory: 256Mi
            limits:
              cpu: 500m
              memory: 512Mi
`

func qptr(s string) *resource.Quantity {
	q := resource.MustParse(s)
	return &q
}

func commitOptions() *git.CommitOptions {
	return &git.CommitOptions{Author: &object.Signature{Name: "concurrent-editor", Email: "concurrent-editor@local"}}
}

func pushOptions() *git.PushOptions {
	return &git.PushOptions{RemoteName: "origin"}
}

func target() domain.WriteTarget {
	return domain.WriteTarget{
		Branch:        gittest.DefaultBranch,
		FilePath:      "deploy/checkout-api.yaml",
		SourceType:    domain.SourceTypeYAML,
		CPUKeyPath:    "spec.template.spec.containers[app].resources.requests.cpu",
		MemoryKeyPath: "spec.template.spec.containers[app].resources.requests.memory",
		ContainerName: "app",
	}
}

func requestFor(tgt domain.WriteTarget, cpu string) gitwriteback.WriteBackRequest {
	req := gitwriteback.WriteBackRequest{
		PatchRequest: domain.PatchRequest{
			Target:   tgt,
			ApplyCPU: true,
			Recommendation: domain.ContainerRecommendation{
				ContainerName: tgt.ContainerName,
				Target:        domain.ResourceAmount{CPU: qptr(cpu)},
			},
		},
		CommitMessage: "argocd-vpa-updater: apply VPA recommendation",
		Patcher:       patcher.NewYAMLPatcher(),
	}
	req.IdempotencyKey = idempotency.Key("payments", "checkout-api-vpa", tgt, req.PatchRequest)
	return req
}

// TestGitWriteBack_FullFlow drives the complete lifecycle the MVP acceptance
// criteria describe: apply a selected recommendation, replay it idempotently,
// have a concurrent edit correctly rejected as a conflict, and independently
// verify the final committed content by re-cloning the repository rather
// than trusting the writer's own in-process state.
func TestGitWriteBack_FullFlow(t *testing.T) {
	remote := gittest.NewBareRepoWithInitialCommit(t, map[string]string{
		"deploy/checkout-api.yaml": manifestWithComments,
	})
	svc := gitwriteback.NewFakeGitWriteBackService(remote)
	tgt := target()

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

		svcWithRace := gitwriteback.NewFakeGitWriteBackService(remote)
		var concurrentCommit string
		svcWithRace.OnBeforePush = func() {
			// Fires after Apply below has already cloned its base and
			// committed locally, but before it pushes -- the exact window a
			// concurrent human edit or a second controller instance could
			// land in.
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
