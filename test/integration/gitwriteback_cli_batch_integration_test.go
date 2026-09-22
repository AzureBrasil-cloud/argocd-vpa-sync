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
	"github.com/azurebrasil/argocd-vpa-updater/internal/gitexec"
	"github.com/azurebrasil/argocd-vpa-updater/internal/gittest"
	"github.com/azurebrasil/argocd-vpa-updater/internal/gitwriteback"
	"github.com/azurebrasil/argocd-vpa-updater/internal/idempotency"
	"github.com/azurebrasil/argocd-vpa-updater/internal/patcher"
)

const secondManifest = `# checkout-worker deployment
apiVersion: apps/v1
kind: Deployment
metadata:
  name: checkout-worker
spec:
  template:
    spec:
      containers:
        - name: worker
          image: example.com/checkout-worker:1.0.0
          resources:
            requests:
              cpu: 100m
              memory: 128Mi
`

func batchTargetA() domain.WriteTarget {
	return domain.WriteTarget{
		Workload:      domain.WorkloadRef{Kind: "Deployment", Name: "checkout-api"},
		Branch:        gittest.DefaultBranch,
		FilePath:      "deploy/checkout-api.yaml",
		SourceType:    domain.SourceTypeYAML,
		CPUKeyPath:    "spec.template.spec.containers[app].resources.requests.cpu",
		MemoryKeyPath: "spec.template.spec.containers[app].resources.requests.memory",
		ContainerName: "app",
	}
}

func batchTargetB() domain.WriteTarget {
	return domain.WriteTarget{
		Workload:      domain.WorkloadRef{Kind: "Deployment", Name: "checkout-worker"},
		Branch:        gittest.DefaultBranch,
		FilePath:      "deploy/checkout-worker.yaml",
		SourceType:    domain.SourceTypeYAML,
		CPUKeyPath:    "spec.template.spec.containers[worker].resources.requests.cpu",
		ContainerName: "worker",
	}
}

func batchItemFor(tgt domain.WriteTarget, vpaNamespace, vpaName, cpu string) gitwriteback.BatchItem {
	patchReq := domain.PatchRequest{
		Target:   tgt,
		ApplyCPU: true,
		Recommendation: domain.ContainerRecommendation{
			ContainerName: tgt.ContainerName,
			Target:        domain.ResourceAmount{CPU: func() *resource.Quantity { q := resource.MustParse(cpu); return &q }()},
		},
	}
	return gitwriteback.BatchItem{
		Target:         tgt,
		Recommendation: patchReq.Recommendation,
		ApplyCPU:       true,
		IdempotencyKey: idempotency.Key(vpaNamespace, vpaName, tgt, patchReq),
		Patcher:        patcher.NewYAMLPatcher(),
	}
}

func TestGitWriteBackCLI_Batch_CombinesMultipleItemsIntoOneCommit(t *testing.T) {
	remote := gittest.NewBareRepoWithInitialCommit(t, map[string]string{
		"deploy/checkout-api.yaml":    manifestWithComments,
		"deploy/checkout-worker.yaml": secondManifest,
	})
	svc := gitwriteback.NewCLIGitWriteBackService(gitexec.NewRunner())

	tgtA, tgtB := batchTargetA(), batchTargetB()
	tgtA.RepoURL, tgtB.RepoURL = remote, remote

	itemA := batchItemFor(tgtA, "payments", "checkout-api-vpa", "250m")
	itemB := batchItemFor(tgtB, "payments", "checkout-worker-vpa", "150m")

	before := gittest.CloneWorktree(t, remote)
	beforeCommits, err := beforeCommitCount(t, before)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result, err := svc.ApplyBatch(context.Background(), gitwriteback.BatchWriteBackRequest{
		RepoURL: remote,
		Branch:  gittest.DefaultBranch,
		Items:   []gitwriteback.BatchItem{itemA, itemB},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.AlreadyApplied {
		t.Fatalf("expected a fresh commit")
	}
	if result.CommitSHA == "" {
		t.Fatalf("expected a commit SHA")
	}
	if len(result.Items) != 2 {
		t.Fatalf("expected 2 item outcomes, got %d", len(result.Items))
	}
	for _, item := range result.Items {
		if item.AlreadyApplied {
			t.Fatalf("expected both items to be freshly applied, got %+v", item)
		}
		if item.CommitSHA != result.CommitSHA {
			t.Fatalf("expected each item's CommitSHA to be the single batch commit %s, got %s", result.CommitSHA, item.CommitSHA)
		}
	}

	after := gittest.CloneWorktree(t, remote)
	afterCommits, err := beforeCommitCount(t, after)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if afterCommits != beforeCommits+1 {
		t.Fatalf("expected exactly 1 new commit for both items combined, went from %d to %d commits", beforeCommits, afterCommits)
	}

	wt, err := after.Worktree()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if content := gittest.ReadFile(t, wt, "deploy/checkout-api.yaml"); !strings.Contains(content, "cpu: 250m") {
		t.Fatalf("expected checkout-api cpu updated to 250m, got:\n%s", content)
	}
	if content := gittest.ReadFile(t, wt, "deploy/checkout-worker.yaml"); !strings.Contains(content, "cpu: 150m") {
		t.Fatalf("expected checkout-worker cpu updated to 150m, got:\n%s", content)
	}

	head, err := after.Head()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	commit, err := after.CommitObject(head.Hash())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(commit.Message, "checkout-api") || !strings.Contains(commit.Message, "checkout-worker") {
		t.Fatalf("expected the commit message to mention both workloads, got:\n%s", commit.Message)
	}
	if strings.Count(commit.Message, "Vpa-Idempotency-Key:") != 2 {
		t.Fatalf("expected one idempotency trailer line per item, got:\n%s", commit.Message)
	}
}

func TestGitWriteBackCLI_Batch_PartialReplaySkipsAlreadyAppliedItems(t *testing.T) {
	remote := gittest.NewBareRepoWithInitialCommit(t, map[string]string{
		"deploy/checkout-api.yaml":    manifestWithComments,
		"deploy/checkout-worker.yaml": secondManifest,
	})
	svc := gitwriteback.NewCLIGitWriteBackService(gitexec.NewRunner())

	tgtA, tgtB := batchTargetA(), batchTargetB()
	tgtA.RepoURL, tgtB.RepoURL = remote, remote
	itemA := batchItemFor(tgtA, "payments", "checkout-api-vpa", "250m")
	itemB := batchItemFor(tgtB, "payments", "checkout-worker-vpa", "150m")

	// First batch applies only itemA.
	first, err := svc.ApplyBatch(context.Background(), gitwriteback.BatchWriteBackRequest{
		RepoURL: remote, Branch: gittest.DefaultBranch, Items: []gitwriteback.BatchItem{itemA},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	before := gittest.CloneWorktree(t, remote)
	beforeCommits, err := beforeCommitCount(t, before)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Second batch replays itemA (already applied) alongside new itemB.
	second, err := svc.ApplyBatch(context.Background(), gitwriteback.BatchWriteBackRequest{
		RepoURL: remote, Branch: gittest.DefaultBranch, Items: []gitwriteback.BatchItem{itemA, itemB},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if second.AlreadyApplied {
		t.Fatalf("expected a new commit for itemB even though itemA was already applied")
	}
	if len(second.Items) != 2 {
		t.Fatalf("expected 2 item outcomes, got %d", len(second.Items))
	}
	if !second.Items[0].AlreadyApplied {
		t.Fatalf("expected itemA to be reported AlreadyApplied=true, got %+v", second.Items[0])
	}
	if second.Items[0].CommitSHA != first.CommitSHA {
		t.Fatalf("expected itemA's CommitSHA to be the FIRST batch's commit %s, got %s", first.CommitSHA, second.Items[0].CommitSHA)
	}
	if second.Items[1].AlreadyApplied {
		t.Fatalf("expected itemB to be freshly applied, got %+v", second.Items[1])
	}
	if second.Items[1].CommitSHA != second.CommitSHA {
		t.Fatalf("expected itemB's CommitSHA to be the second batch's own commit")
	}

	after := gittest.CloneWorktree(t, remote)
	afterCommits, err := beforeCommitCount(t, after)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if afterCommits != beforeCommits+1 {
		t.Fatalf("expected exactly 1 new commit (only itemB), went from %d to %d", beforeCommits, afterCommits)
	}

	commitMsg, err := headCommitMessage(t, after)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Count(commitMsg, "Vpa-Idempotency-Key:") != 1 {
		t.Fatalf("expected the second commit to carry only itemB's trailer (itemA already had its own from the first commit), got:\n%s", commitMsg)
	}
}

func TestGitWriteBackCLI_Batch_AllAlreadyAppliedIsANoOp(t *testing.T) {
	remote := gittest.NewBareRepoWithInitialCommit(t, map[string]string{
		"deploy/checkout-api.yaml":    manifestWithComments,
		"deploy/checkout-worker.yaml": secondManifest,
	})
	svc := gitwriteback.NewCLIGitWriteBackService(gitexec.NewRunner())

	tgtA, tgtB := batchTargetA(), batchTargetB()
	tgtA.RepoURL, tgtB.RepoURL = remote, remote
	items := []gitwriteback.BatchItem{
		batchItemFor(tgtA, "payments", "checkout-api-vpa", "250m"),
		batchItemFor(tgtB, "payments", "checkout-worker-vpa", "150m"),
	}

	if _, err := svc.ApplyBatch(context.Background(), gitwriteback.BatchWriteBackRequest{
		RepoURL: remote, Branch: gittest.DefaultBranch, Items: items,
	}); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	before := gittest.CloneWorktree(t, remote)
	beforeHead, err := before.Head()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	result, err := svc.ApplyBatch(context.Background(), gitwriteback.BatchWriteBackRequest{
		RepoURL: remote, Branch: gittest.DefaultBranch, Items: items,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.AlreadyApplied {
		t.Fatalf("expected AlreadyApplied=true when every item was already applied")
	}
	for _, item := range result.Items {
		if !item.AlreadyApplied {
			t.Fatalf("expected every item to be AlreadyApplied, got %+v", item)
		}
	}

	after := gittest.CloneWorktree(t, remote)
	afterHead, err := after.Head()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if beforeHead.Hash() != afterHead.Hash() {
		t.Fatalf("expected no new commit; remote tip changed from %s to %s", beforeHead.Hash(), afterHead.Hash())
	}
}

func TestGitWriteBackCLI_Batch_ConflictRejectsWholeBatchNotPartially(t *testing.T) {
	remote := gittest.NewBareRepoWithInitialCommit(t, map[string]string{
		"deploy/checkout-api.yaml":    manifestWithComments,
		"deploy/checkout-worker.yaml": secondManifest,
	})
	preConflict := gittest.CloneWorktree(t, remote)
	preConflictHead, err := preConflict.Head()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	svc := gitwriteback.NewCLIGitWriteBackService(gitexec.NewRunner())
	var concurrentCommit string
	svc.OnBeforePush = func() {
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

	tgtA, tgtB := batchTargetA(), batchTargetB()
	tgtA.RepoURL, tgtB.RepoURL = remote, remote
	items := []gitwriteback.BatchItem{
		batchItemFor(tgtA, "payments", "checkout-api-vpa", "250m"),
		batchItemFor(tgtB, "payments", "checkout-worker-vpa", "150m"),
	}

	_, err = svc.ApplyBatch(context.Background(), gitwriteback.BatchWriteBackRequest{
		RepoURL: remote, Branch: gittest.DefaultBranch, Items: items,
	})
	if err == nil {
		t.Fatalf("expected an error due to the concurrent edit")
	}
	if !errors.Is(err, gitwriteback.ErrConflict) {
		t.Fatalf("expected ErrConflict, got %v", err)
	}

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
	// Neither item's change (not even checkout-worker.yaml, unrelated to
	// the conflicting file) may have reached the remote: the batch is one
	// atomic commit, so a conflict must reject it entirely, not partially.
	if content := gittest.ReadFile(t, fwt, "deploy/checkout-worker.yaml"); strings.Contains(content, "cpu: 150m") {
		t.Fatalf("expected checkout-worker.yaml to be untouched by the rejected batch, got:\n%s", content)
	}
}

// beforeCommitCount counts every commit reachable from repo's current HEAD.
func beforeCommitCount(t *testing.T, repo *git.Repository) (int, error) {
	t.Helper()
	head, err := repo.Head()
	if err != nil {
		return 0, err
	}
	iter, err := repo.Log(&git.LogOptions{From: head.Hash()})
	if err != nil {
		return 0, err
	}
	defer iter.Close()

	count := 0
	err = iter.ForEach(func(*object.Commit) error {
		count++
		return nil
	})
	return count, err
}

// headCommitMessage returns repo's current HEAD commit's full message.
func headCommitMessage(t *testing.T, repo *git.Repository) (string, error) {
	t.Helper()
	head, err := repo.Head()
	if err != nil {
		return "", err
	}
	commit, err := repo.CommitObject(head.Hash())
	if err != nil {
		return "", err
	}
	return commit.Message, nil
}
