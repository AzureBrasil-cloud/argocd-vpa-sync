package controller

import (
	"context"
	"fmt"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/handler"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	gitopsv1alpha1 "github.com/azurebrasil/argocd-vpa-updater/internal/apis/vpagitopsbinding/v1alpha1"
	"github.com/azurebrasil/argocd-vpa-updater/internal/vpaapi"
	"github.com/azurebrasil/argocd-vpa-updater/internal/vparecommendation"
)

// vpaRefIndexField is the field-indexer name under which every
// VpaGitOpsBinding is indexed by the (unqualified) VPA name it references.
// The indexer function intentionally returns the raw name, not a
// namespace-qualified key: controller-runtime's cache automatically
// namespaces indexed values using the indexed object's own namespace when a
// List call combines client.InNamespace with client.MatchingFields on this
// field (see bindingsReferencingVPA below) -- the same pattern the
// kubebuilder book's CronJob tutorial uses for owned-object indexes.
const vpaRefIndexField = ".spec.vpaRef.name"

// AddToManager registers the VpaGitOpsBinding reconciler with mgr. The
// controller watches VpaGitOpsBinding as its primary resource and the
// VerticalPodAutoscaler it references as a secondary resource: a VPA
// recommendation update must re-trigger reconciliation of whichever
// binding(s) point at it, even though no VpaGitOpsBinding object itself
// changed.
func AddToManager(mgr ctrl.Manager, cache *vparecommendation.Cache) error {
	if err := mgr.GetFieldIndexer().IndexField(context.Background(), &gitopsv1alpha1.VpaGitOpsBinding{}, vpaRefIndexField,
		func(obj client.Object) []string {
			binding, ok := obj.(*gitopsv1alpha1.VpaGitOpsBinding)
			if !ok || binding.Spec.VpaRef.Name == "" {
				return nil
			}
			return []string{binding.Spec.VpaRef.Name}
		},
	); err != nil {
		return fmt.Errorf("controller: index VpaGitOpsBinding field %s: %w", vpaRefIndexField, err)
	}

	r := &VPAReconciler{Client: mgr.GetClient(), Cache: cache}
	return ctrl.NewControllerManagedBy(mgr).
		For(&gitopsv1alpha1.VpaGitOpsBinding{}).
		Watches(
			&vpaapi.VerticalPodAutoscaler{},
			handler.EnqueueRequestsFromMapFunc(bindingsReferencingVPA(mgr.GetClient())),
		).
		Complete(r)
}

// bindingsReferencingVPA maps a VerticalPodAutoscaler event to a
// reconcile.Request for every VpaGitOpsBinding, in the same namespace, whose
// spec.vpaRef.name names that VPA. It uses the field index above instead of
// listing and filtering every binding in the cluster on every VPA event.
func bindingsReferencingVPA(c client.Client) handler.MapFunc {
	return func(ctx context.Context, obj client.Object) []reconcile.Request {
		var bindings gitopsv1alpha1.VpaGitOpsBindingList
		if err := c.List(ctx, &bindings,
			client.InNamespace(obj.GetNamespace()),
			client.MatchingFields{vpaRefIndexField: obj.GetName()},
		); err != nil {
			return nil
		}

		reqs := make([]reconcile.Request, 0, len(bindings.Items))
		for i := range bindings.Items {
			reqs = append(reqs, reconcile.Request{
				NamespacedName: client.ObjectKeyFromObject(&bindings.Items[i]),
			})
		}
		return reqs
	}
}
