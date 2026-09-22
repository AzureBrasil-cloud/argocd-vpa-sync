package gitwriteback

import (
	"context"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/azurebrasil/argocd-vpa-updater/internal/domain"
	"github.com/azurebrasil/argocd-vpa-updater/internal/gittest"
	"github.com/azurebrasil/argocd-vpa-updater/internal/idempotency"
	"github.com/azurebrasil/argocd-vpa-updater/internal/patcher"
)

const sampleManifest = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: checkout-api
spec:
  template:
    spec:
      containers:
        - name: app
          resources:
            requests:
              cpu: 100m
              memory: 256Mi
`

func qptr(s string) *resource.Quantity {
	q := resource.MustParse(s)
	return &q
}

func baseTarget(filePath string) domain.WriteTarget {
	return domain.WriteTarget{
		Branch:        gittest.DefaultBranch,
		FilePath:      filePath,
		SourceType:    domain.SourceTypeYAML,
		CPUKeyPath:    "spec.template.spec.containers[app].resources.requests.cpu",
		MemoryKeyPath: "spec.template.spec.containers[app].resources.requests.memory",
		ContainerName: "app",
	}
}

func newRequest(target domain.WriteTarget, cpu string) WriteBackRequest {
	req := WriteBackRequest{
		PatchRequest: domain.PatchRequest{
			Target:   target,
			ApplyCPU: true,
			Recommendation: domain.ContainerRecommendation{
				ContainerName: target.ContainerName,
				Target:        domain.ResourceAmount{CPU: qptr(cpu)},
			},
		},
		Patcher: patcher.NewYAMLPatcher(),
	}
	req.IdempotencyKey = idempotency.Key("payments", "checkout-api-vpa", target, req.PatchRequest)
	return req
}

func TestFakeGitWriteBackService_CommitPolicy_CreatesCommitOnBranch(t *testing.T) {
	remote := gittest.NewBareRepoWithInitialCommit(t, map[string]string{"deploy.yaml": sampleManifest})
	svc := NewFakeGitWriteBackService(remote)

	target := baseTarget("deploy.yaml")
	req := newRequest(target, "250m")

	result, err := svc.Apply(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.AlreadyApplied {
		t.Fatalf("expected a fresh commit, not AlreadyApplied")
	}
	if result.Branch != gittest.DefaultBranch {
		t.Fatalf("expected commit-policy write-back to land on %q, got %q", gittest.DefaultBranch, result.Branch)
	}
	if result.CommitSHA == "" {
		t.Fatalf("expected a commit SHA")
	}
	if result.PRURL != "" {
		t.Fatalf("expected no PR URL for commit policy, got %q", result.PRURL)
	}

	clone := gittest.CloneWorktree(t, remote)
	wt, err := clone.Worktree()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if content := gittest.ReadFile(t, wt, "deploy.yaml"); !strings.Contains(content, "cpu: 250m") {
		t.Fatalf("expected the pushed commit to declare cpu: 250m, got:\n%s", content)
	}
}

func TestFakeGitWriteBackService_PullRequestPolicy_PushesTopicBranch(t *testing.T) {
	remote := gittest.NewBareRepoWithInitialCommit(t, map[string]string{"deploy.yaml": sampleManifest})
	svc := NewFakeGitWriteBackService(remote)

	target := baseTarget("deploy.yaml")
	target.WriteBackPolicy = domain.WriteBackPullRequest
	req := newRequest(target, "250m")

	result, err := svc.Apply(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Branch == gittest.DefaultBranch {
		t.Fatalf("expected a topic branch distinct from %q, got %q", gittest.DefaultBranch, result.Branch)
	}
	if !strings.HasPrefix(result.Branch, "vpa-updater/app-") {
		t.Fatalf("expected topic branch name to start with vpa-updater/app-, got %q", result.Branch)
	}
	if !strings.HasPrefix(result.PRURL, "local://fake-pr/") {
		t.Fatalf("expected a synthetic local PR URL, got %q", result.PRURL)
	}

	// The base branch itself must be untouched by a pull-request write-back.
	clone := gittest.CloneWorktree(t, remote)
	wt, err := clone.Worktree()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	content := gittest.ReadFile(t, wt, "deploy.yaml")
	if !strings.Contains(content, "cpu: 100m") {
		t.Fatalf("expected base branch to still declare cpu: 100m, got:\n%s", content)
	}
}

func TestFakeGitWriteBackService_IdempotentReplay_NoDuplicateCommit(t *testing.T) {
	remote := gittest.NewBareRepoWithInitialCommit(t, map[string]string{"deploy.yaml": sampleManifest})
	svc := NewFakeGitWriteBackService(remote)

	target := baseTarget("deploy.yaml")
	req := newRequest(target, "250m")

	first, err := svc.Apply(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error on first apply: %v", err)
	}

	second, err := svc.Apply(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error on replay: %v", err)
	}
	if !second.AlreadyApplied {
		t.Fatalf("expected replay to report AlreadyApplied")
	}
	if second.CommitSHA != first.CommitSHA {
		t.Fatalf("expected replay to reference the same commit %q, got %q", first.CommitSHA, second.CommitSHA)
	}
}

func TestFakeGitWriteBackService_NoOpWhenValueAlreadyMatches(t *testing.T) {
	remote := gittest.NewBareRepoWithInitialCommit(t, map[string]string{"deploy.yaml": sampleManifest})
	svc := NewFakeGitWriteBackService(remote)

	target := baseTarget("deploy.yaml")
	req := newRequest(target, "100m") // matches the seeded value exactly

	result, err := svc.Apply(context.Background(), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.AlreadyApplied {
		t.Fatalf("expected AlreadyApplied=true when the value already matches")
	}
	if result.CommitSHA != "" {
		t.Fatalf("expected no commit to be made for a no-op patch, got %q", result.CommitSHA)
	}
}

func TestFakeGitWriteBackService_MissingBaseBranchIsError(t *testing.T) {
	remote := gittest.NewBareRepoWithInitialCommit(t, map[string]string{"deploy.yaml": sampleManifest})
	svc := NewFakeGitWriteBackService(remote)

	target := baseTarget("deploy.yaml")
	target.Branch = "does-not-exist"
	req := newRequest(target, "250m")

	if _, err := svc.Apply(context.Background(), req); err == nil {
		t.Fatalf("expected an error for a nonexistent base branch")
	}
}

func TestFakeGitWriteBackService_RequiresIdempotencyKey(t *testing.T) {
	remote := gittest.NewBareRepoWithInitialCommit(t, map[string]string{"deploy.yaml": sampleManifest})
	svc := NewFakeGitWriteBackService(remote)

	req := newRequest(baseTarget("deploy.yaml"), "250m")
	req.IdempotencyKey = ""

	if _, err := svc.Apply(context.Background(), req); err == nil {
		t.Fatalf("expected an error when IdempotencyKey is empty")
	}
}

func TestExtractIdempotencyKey_RoundTrip(t *testing.T) {
	msg := buildCommitMessage("update resources", "sha256:abc123")
	key, ok := extractIdempotencyKey(msg)
	if !ok || key != "sha256:abc123" {
		t.Fatalf("expected round-trip trailer extraction, got key=%q ok=%v (message=%q)", key, ok, msg)
	}
}

func TestExtractIdempotencyKey_AbsentReturnsFalse(t *testing.T) {
	if _, ok := extractIdempotencyKey("just a plain commit message\n"); ok {
		t.Fatalf("expected no trailer to be found")
	}
}
