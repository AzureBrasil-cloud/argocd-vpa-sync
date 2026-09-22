package resolver

import (
	"context"
	"errors"
	"testing"

	"github.com/azurebrasil/argocd-vpa-updater/internal/domain"
)

func validVPA() domain.NormalizedVPA {
	binding := domain.GitOpsBindingSpec{
		ArgoCDApplication:          "payments-api",
		ArgoCDApplicationNamespace: "argocd",
		RepoURL:                    "https://git.example.com/team/app.git",
		RepoBranch:                 "main",
		WriteBackPolicy:            domain.WriteBackCommit,
		MinChangePercent:           10,
		Containers: []domain.ContainerWriteBackConfig{
			{
				ContainerName: "app",
				ManifestType:  domain.SourceTypeHelmValues,
				ManifestPath:  "apps/payments/values-prd.yaml",
				CPUKeyPath:    "resources.requests.cpu",
				MemoryKeyPath: "resources.requests.memory",

				CPULimitKeyPath:    "resources.limits.cpu",
				MemoryLimitKeyPath: "resources.limits.memory",
			},
		},
	}
	return domain.NormalizedVPA{
		Namespace: "payments",
		Name:      "checkout-api-vpa",
		Workload:  domain.WorkloadRef{Kind: "Deployment", Name: "checkout-api", Namespace: "payments"},
		Binding:   binding,
		Valid:     true,
	}
}

type fakeArgoApps struct {
	repoURLs map[string][]string
	err      error
}

func (f fakeArgoApps) GetRepoURLs(ctx context.Context, namespace, name string) ([]string, error) {
	if f.err != nil {
		return nil, f.err
	}
	return f.repoURLs[namespace+"/"+name], nil
}

func TestResolve_HappyPath(t *testing.T) {
	r := NewCRDResolver(nil, "argocd")
	target, err := r.Resolve(context.Background(), validVPA(), "app")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target.RepoURL != "https://git.example.com/team/app.git" || target.Branch != "main" || target.FilePath != "apps/payments/values-prd.yaml" {
		t.Fatalf("unexpected target: %+v", target)
	}
	if target.ContainerName != "app" {
		t.Fatalf("expected container app, got %s", target.ContainerName)
	}
	if len(target.Warnings) != 0 {
		t.Fatalf("expected no warnings without an ArgoCDApps getter, got %v", target.Warnings)
	}
}

func TestResolve_InvalidVPA(t *testing.T) {
	vpa := validVPA()
	vpa.Valid = false
	vpa.ValidationErrors = []string{"missing repo-url"}
	r := NewCRDResolver(nil, "argocd")
	if _, err := r.Resolve(context.Background(), vpa, "app"); err == nil {
		t.Fatalf("expected error for an invalid VPA")
	}
}

func TestResolve_UnknownContainer(t *testing.T) {
	r := NewCRDResolver(nil, "argocd")
	if _, err := r.Resolve(context.Background(), validVPA(), "sidecar"); err == nil {
		t.Fatalf("expected error when the binding declares no config for the requested container")
	}
}

func TestResolve_MultipleContainers_ResolvesEachIndependently(t *testing.T) {
	vpa := validVPA()
	vpa.Binding.Containers = append(vpa.Binding.Containers, domain.ContainerWriteBackConfig{
		ContainerName: "sidecar",
		ManifestType:  domain.SourceTypeYAML,
		ManifestPath:  "deploy/sidecar.yaml",
		CPUKeyPath:    "spec.containers[sidecar].resources.requests.cpu",
	})

	r := NewCRDResolver(nil, "argocd")

	appTarget, err := r.Resolve(context.Background(), vpa, "app")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if appTarget.FilePath != "apps/payments/values-prd.yaml" {
		t.Fatalf("unexpected app target: %+v", appTarget)
	}

	sidecarTarget, err := r.Resolve(context.Background(), vpa, "sidecar")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if sidecarTarget.FilePath != "deploy/sidecar.yaml" || sidecarTarget.SourceType != domain.SourceTypeYAML {
		t.Fatalf("unexpected sidecar target: %+v", sidecarTarget)
	}
}

func TestResolve_ArgoCDRepoURLMatch_NoWarning(t *testing.T) {
	apps := fakeArgoApps{repoURLs: map[string][]string{
		"argocd/payments-api": {"https://git.example.com/team/app.git"},
	}}
	r := NewCRDResolver(apps, "argocd")
	target, err := r.Resolve(context.Background(), validVPA(), "app")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(target.Warnings) != 0 {
		t.Fatalf("expected no warnings on repo-url match, got %v", target.Warnings)
	}
}

func TestResolve_ArgoCDRepoURLMismatch_ProducesWarningNotError(t *testing.T) {
	apps := fakeArgoApps{repoURLs: map[string][]string{
		"argocd/payments-api": {"https://git.example.com/other/repo.git"},
	}}
	r := NewCRDResolver(apps, "argocd")
	target, err := r.Resolve(context.Background(), validVPA(), "app")
	if err != nil {
		t.Fatalf("a repo-url mismatch must not be a hard error, got %v", err)
	}
	if len(target.Warnings) != 1 {
		t.Fatalf("expected exactly one warning, got %v", target.Warnings)
	}
	if target.RepoURL != "https://git.example.com/team/app.git" {
		t.Fatalf("resolver must never substitute the declared repo-url, got %q", target.RepoURL)
	}
}

func TestResolve_ArgoCDLookupError_ProducesWarningNotError(t *testing.T) {
	apps := fakeArgoApps{err: errors.New("application not found")}
	r := NewCRDResolver(apps, "argocd")
	target, err := r.Resolve(context.Background(), validVPA(), "app")
	if err != nil {
		t.Fatalf("an Argo CD lookup failure must not fail resolution, got %v", err)
	}
	if len(target.Warnings) != 1 {
		t.Fatalf("expected exactly one warning, got %v", target.Warnings)
	}
}

func TestResolve_LimitKeyPathsAreNotInferred(t *testing.T) {
	vpa := validVPA()
	vpa.Binding.Containers[0].CPULimitKeyPath = ""
	vpa.Binding.Containers[0].MemoryLimitKeyPath = ""
	target, err := NewCRDResolver(nil, "argocd").Resolve(context.Background(), vpa, "app")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target.CPULimitKeyPath != "" || target.MemoryLimitKeyPath != "" {
		t.Fatalf("expected no limit key paths, got %+v", target)
	}
	if len(target.Warnings) != 2 {
		t.Fatalf("expected a warning per resource without a limit key path, got %v", target.Warnings)
	}
}

func TestResolve_DeclaredLimitKeyPaths(t *testing.T) {
	vpa := validVPA()
	vpa.Binding.Containers[0].CPULimitKeyPath = "resources.limits.cpu"
	vpa.Binding.Containers[0].MemoryLimitKeyPath = "app.memoryLimit"
	target, err := NewCRDResolver(nil, "argocd").Resolve(context.Background(), vpa, "app")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if target.CPULimitKeyPath != "resources.limits.cpu" || target.MemoryLimitKeyPath != "app.memoryLimit" {
		t.Fatalf("unexpected limit key paths: %+v", target)
	}
	if len(target.Warnings) != 0 {
		t.Fatalf("expected no warnings, got %v", target.Warnings)
	}
}
