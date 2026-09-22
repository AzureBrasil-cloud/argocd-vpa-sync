// Package resolver turns a NormalizedVPA + container name into a fully
// resolved domain.WriteTarget, using only explicit configuration from the
// VPA's VpaGitOpsBinding. It never infers a repo, branch or file path from
// the VPA/Deployment name.
package resolver

import (
	"context"
	"fmt"

	"github.com/azurebrasil/argocd-vpa-updater/internal/domain"
)

// GitOpsTargetResolver resolves the Git write-back target for one
// container's recommendation.
type GitOpsTargetResolver interface {
	Resolve(ctx context.Context, vpa domain.NormalizedVPA, containerName string) (domain.WriteTarget, error)
}

// ArgoCDApplicationRepoURLGetter is consulted, when available, purely to
// cross-check the binding-declared RepoURL against the source Argo CD
// Application's own spec.source(s).repoURL. A mismatch is surfaced as a
// WriteTarget warning, never as an error and never as a silent substitution.
type ArgoCDApplicationRepoURLGetter interface {
	GetRepoURLs(ctx context.Context, namespace, name string) ([]string, error)
}

// CRDResolver is the default GitOpsTargetResolver implementation. It is a
// pure function of domain.GitOpsBindingSpec (built by internal/controller
// from a VpaGitOpsBinding CR), optionally augmented by an Argo CD
// Application cross-check.
type CRDResolver struct {
	// ArgoCDApps is optional; when nil the Argo CD cross-check is skipped.
	ArgoCDApps ArgoCDApplicationRepoURLGetter

	// DefaultArgoCDNamespace is used when a binding declares
	// ArgoCDApplicationRef.Name but not ArgoCDApplicationRef.Namespace.
	DefaultArgoCDNamespace string
}

// NewCRDResolver builds a CRDResolver. argoCDApps may be nil.
func NewCRDResolver(argoCDApps ArgoCDApplicationRepoURLGetter, defaultArgoCDNamespace string) *CRDResolver {
	return &CRDResolver{ArgoCDApps: argoCDApps, DefaultArgoCDNamespace: defaultArgoCDNamespace}
}

// Resolve implements GitOpsTargetResolver.
func (r *CRDResolver) Resolve(ctx context.Context, vpa domain.NormalizedVPA, containerName string) (domain.WriteTarget, error) {
	if !vpa.Valid {
		return domain.WriteTarget{}, fmt.Errorf("resolver: VPA %s/%s failed binding validation: %v", vpa.Namespace, vpa.Name, vpa.ValidationErrors)
	}
	cc, found := vpa.Binding.ContainerByName(containerName)
	if !found {
		return domain.WriteTarget{}, fmt.Errorf("resolver: VpaGitOpsBinding for VPA %s/%s declares no container config named %q", vpa.Namespace, vpa.Name, containerName)
	}

	target := domain.WriteTarget{
		RepoURL:         vpa.Binding.RepoURL,
		Branch:          vpa.Binding.RepoBranch,
		FilePath:        cc.ManifestPath,
		SourceType:      cc.ManifestType,
		CPUKeyPath:      cc.CPUKeyPath,
		MemoryKeyPath:   cc.MemoryKeyPath,
		WriteBackPolicy: vpa.Binding.WriteBackPolicy,
		Workload:        vpa.Workload,
		ContainerName:   containerName,

		CPULimitKeyPath:    cc.CPULimitKeyPath,
		MemoryLimitKeyPath: cc.MemoryLimitKeyPath,
	}
	warnMissingLimitKeyPath(&target, "cpu", cc.CPUKeyPath, cc.CPULimitKeyPath)
	warnMissingLimitKeyPath(&target, "memory", cc.MemoryKeyPath, cc.MemoryLimitKeyPath)

	if r.ArgoCDApps != nil && vpa.Binding.ArgoCDApplication != "" {
		ns := vpa.Binding.ArgoCDApplicationNamespace
		if ns == "" {
			ns = r.DefaultArgoCDNamespace
		}
		repoURLs, err := r.ArgoCDApps.GetRepoURLs(ctx, ns, vpa.Binding.ArgoCDApplication)
		if err != nil {
			target.Warnings = append(target.Warnings, fmt.Sprintf("could not verify source Argo CD Application %s/%s: %v", ns, vpa.Binding.ArgoCDApplication, err))
		} else if len(repoURLs) > 0 && !contains(repoURLs, target.RepoURL) {
			target.Warnings = append(target.Warnings, fmt.Sprintf("declared repo-url %q does not match any spec.source(s).repoURL of Argo CD Application %s/%s (%v)", target.RepoURL, ns, vpa.Binding.ArgoCDApplication, repoURLs))
		}
	}

	return target, nil
}

// warnMissingLimitKeyPath records a warning when a resource's request is
// written back but its limit key path isn't declared. Limit key paths are
// never inferred -- a Helm values layout can put the limit anywhere -- so
// write-back will update the request alone, which Kubernetes rejects if
// the new request exceeds the limit already in the file.
func warnMissingLimitKeyPath(target *domain.WriteTarget, resourceName, requestKeyPath, limitKeyPath string) {
	if requestKeyPath == "" || limitKeyPath != "" {
		return
	}
	target.Warnings = append(target.Warnings, fmt.Sprintf("%sLimitKeyPath is not set: write-back updates the %s request only and leaves its limit untouched", resourceName, resourceName))
}

func contains(list []string, s string) bool {
	for _, v := range list {
		if v == s {
			return true
		}
	}
	return false
}
