// Package controller watches VpaGitOpsBinding custom resources and the
// VerticalPodAutoscaler objects they reference, keeping a
// vparecommendation.Cache up to date with normalized state. It creates,
// modifies and owns no Kubernetes resources of its own beyond its own
// VpaGitOpsBinding status subresource -- it never mutates a VPA, Pod,
// Deployment or StatefulSet.
package controller

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/meta"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"

	gitopsv1alpha1 "github.com/azurebrasil/argocd-vpa-updater/internal/apis/vpagitopsbinding/v1alpha1"
	"github.com/azurebrasil/argocd-vpa-updater/internal/domain"
	"github.com/azurebrasil/argocd-vpa-updater/internal/vpaapi"
	"github.com/azurebrasil/argocd-vpa-updater/internal/vparecommendation"
)

// VPAReconciler keeps Cache in sync with every VpaGitOpsBinding and the
// VerticalPodAutoscaler it references.
type VPAReconciler struct {
	client.Client
	Cache *vparecommendation.Cache
}

func (r *VPAReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	var binding gitopsv1alpha1.VpaGitOpsBinding
	if err := r.Get(ctx, req.NamespacedName, &binding); err != nil {
		if apierrors.IsNotFound(err) {
			r.Cache.Delete(req.Namespace, req.Name)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, fmt.Errorf("controller: get VpaGitOpsBinding %s: %w", req.NamespacedName, err)
	}

	var vpa vpaapi.VerticalPodAutoscaler
	vpaKey := client.ObjectKey{Namespace: binding.Namespace, Name: binding.Spec.VpaRef.Name}
	if err := r.Get(ctx, vpaKey, &vpa); err != nil {
		if apierrors.IsNotFound(err) {
			r.Cache.Delete(binding.Namespace, binding.Name)
			msg := fmt.Sprintf("VerticalPodAutoscaler %q not found in namespace %q", binding.Spec.VpaRef.Name, binding.Namespace)
			return ctrl.Result{}, r.setCondition(ctx, &binding, metav1.ConditionFalse, gitopsv1alpha1.ReasonVpaNotFound, msg)
		}
		return ctrl.Result{}, fmt.Errorf("controller: get VerticalPodAutoscaler %s: %w", vpaKey, err)
	}

	validationErrs := validateSpec(binding.Spec)

	updateMode := string(vpaapi.UpdateModeOff)
	if vpa.Spec.UpdatePolicy != nil && vpa.Spec.UpdatePolicy.UpdateMode != nil {
		updateMode = string(*vpa.Spec.UpdatePolicy.UpdateMode)
	}
	// The CRD schema cannot validate this -- updateMode lives on the VPA,
	// not the binding -- so it stays a runtime check.
	if updateMode != string(vpaapi.UpdateModeOff) {
		validationErrs = append(validationErrs, fmt.Sprintf(
			"expected updateMode %q, got %q -- argocd-vpa-updater assumes the VPA never mutates Pods directly",
			vpaapi.UpdateModeOff, updateMode))
	}

	normalized := normalize(vpa, toDomainSpec(binding.Spec), updateMode, validationErrs)
	r.Cache.Set(binding.Namespace, binding.Name, normalized)

	if len(validationErrs) > 0 {
		msg := fmt.Sprintf("%d validation error(s): %v", len(validationErrs), validationErrs)
		return ctrl.Result{}, r.setCondition(ctx, &binding, metav1.ConditionFalse, gitopsv1alpha1.ReasonValidationFailed, msg)
	}
	return ctrl.Result{}, r.setCondition(ctx, &binding, metav1.ConditionTrue, gitopsv1alpha1.ReasonReady, "recommendation cache up to date")
}

// validateSpec re-checks what the CRD's structural schema already enforces
// at admission (required fields, enums, MinItems) plus the one rule that
// cannot be expressed declaratively: each container needs at least one of
// cpuKeyPath/memoryKeyPath. Re-checking defensively (not just trusting
// admission) protects against CRs written before a schema tightening
// shipped, or created via tooling that bypasses validation.
func validateSpec(spec gitopsv1alpha1.VpaGitOpsBindingSpec) []string {
	var errs []string
	if spec.VpaRef.Name == "" {
		errs = append(errs, "spec.vpaRef.name is required")
	}
	if spec.RepoURL == "" {
		errs = append(errs, "spec.repoURL is required")
	}
	if spec.RepoBranch == "" {
		errs = append(errs, "spec.repoBranch is required")
	}
	if len(spec.Containers) == 0 {
		errs = append(errs, "spec.containers must declare at least one entry")
	}

	seen := make(map[string]bool, len(spec.Containers))
	for i, c := range spec.Containers {
		switch {
		case c.Name == "":
			errs = append(errs, fmt.Sprintf("spec.containers[%d].name is required", i))
		case seen[c.Name]:
			errs = append(errs, fmt.Sprintf("spec.containers[%d].name %q is duplicated", i, c.Name))
		}
		seen[c.Name] = true

		switch c.ManifestType {
		case gitopsv1alpha1.SourceTypeHelmValues, gitopsv1alpha1.SourceTypeKustomizePatch, gitopsv1alpha1.SourceTypeYAML:
		default:
			errs = append(errs, fmt.Sprintf("spec.containers[%d].manifestType has unknown value %q", i, c.ManifestType))
		}
		if c.ManifestPath == "" {
			errs = append(errs, fmt.Sprintf("spec.containers[%d].manifestPath is required", i))
		}
		if c.CPUKeyPath == "" && c.MemoryKeyPath == "" {
			errs = append(errs, fmt.Sprintf("spec.containers[%d]: at least one of cpuKeyPath or memoryKeyPath must be set", i))
		}
	}

	switch spec.WriteBackPolicy {
	case "", gitopsv1alpha1.WriteBackCommit, gitopsv1alpha1.WriteBackPullRequest:
	default:
		errs = append(errs, fmt.Sprintf("spec.writeBackPolicy has unknown value %q", spec.WriteBackPolicy))
	}
	return errs
}

func (r *VPAReconciler) setCondition(ctx context.Context, binding *gitopsv1alpha1.VpaGitOpsBinding, status metav1.ConditionStatus, reason, message string) error {
	meta.SetStatusCondition(&binding.Status.Conditions, metav1.Condition{
		Type:               gitopsv1alpha1.ConditionTypeReady,
		Status:             status,
		Reason:             reason,
		Message:            message,
		ObservedGeneration: binding.Generation,
	})
	binding.Status.ObservedGeneration = binding.Generation
	if err := r.Status().Update(ctx, binding); err != nil {
		return fmt.Errorf("controller: update VpaGitOpsBinding %s/%s status: %w", binding.Namespace, binding.Name, err)
	}
	return nil
}

func toDomainSpec(s gitopsv1alpha1.VpaGitOpsBindingSpec) domain.GitOpsBindingSpec {
	out := domain.GitOpsBindingSpec{
		RepoURL:          s.RepoURL,
		RepoBranch:       s.RepoBranch,
		WriteBackPolicy:  domain.WriteBackPolicy(s.WriteBackPolicy),
		MinChangePercent: s.MinChangePercent,
	}
	if out.MinChangePercent <= 0 {
		out.MinChangePercent = domain.DefaultMinChangePercent
	}
	if s.ArgoCDApplicationRef != nil {
		out.ArgoCDApplication = s.ArgoCDApplicationRef.Name
		out.ArgoCDApplicationNamespace = s.ArgoCDApplicationRef.Namespace
	}
	out.Containers = make([]domain.ContainerWriteBackConfig, 0, len(s.Containers))
	for _, c := range s.Containers {
		out.Containers = append(out.Containers, domain.ContainerWriteBackConfig{
			ContainerName:    c.Name,
			ManifestType:     domain.SourceType(c.ManifestType),
			ManifestPath:     c.ManifestPath,
			CPUKeyPath:       c.CPUKeyPath,
			MemoryKeyPath:    c.MemoryKeyPath,
			MinChangePercent: c.MinChangePercent,
		})
	}
	return out
}

func normalize(vpa vpaapi.VerticalPodAutoscaler, spec domain.GitOpsBindingSpec, updateMode string, validationErrs []string) domain.NormalizedVPA {
	workload := domain.WorkloadRef{Namespace: vpa.Namespace}
	if vpa.Spec.TargetRef != nil {
		workload.Kind = vpa.Spec.TargetRef.Kind
		workload.Name = vpa.Spec.TargetRef.Name
	}

	var containers []domain.ContainerRecommendation
	if vpa.Status.Recommendation != nil {
		for _, cr := range vpa.Status.Recommendation.ContainerRecommendations {
			containers = append(containers, domain.ContainerRecommendation{
				ContainerName:  cr.ContainerName,
				Target:         toResourceAmount(cr.Target),
				LowerBound:     toResourceAmount(cr.LowerBound),
				UpperBound:     toResourceAmount(cr.UpperBound),
				UncappedTarget: toResourceAmount(cr.UncappedTarget),
			})
		}
	}

	var recommendationTime time.Time
	for _, cond := range vpa.Status.Conditions {
		if cond.Type == vpaapi.RecommendationProvided && cond.Status == corev1.ConditionTrue {
			recommendationTime = cond.LastTransitionTime.Time
		}
	}

	return domain.NormalizedVPA{
		Cluster:            "in-cluster",
		Namespace:          vpa.Namespace,
		Name:               vpa.Name,
		UpdateMode:         updateMode,
		Workload:           workload,
		Binding:            spec,
		Containers:         containers,
		RecommendationTime: recommendationTime,
		Valid:              len(validationErrs) == 0,
		ValidationErrors:   validationErrs,
	}
}

func toResourceAmount(rl corev1.ResourceList) domain.ResourceAmount {
	var out domain.ResourceAmount
	if cpu, ok := rl[corev1.ResourceCPU]; ok {
		c := cpu.DeepCopy()
		out.CPU = &c
	}
	if mem, ok := rl[corev1.ResourceMemory]; ok {
		m := mem.DeepCopy()
		out.Memory = &m
	}
	return out
}
