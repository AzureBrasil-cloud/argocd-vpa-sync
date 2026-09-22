package patcher

import (
	"context"

	"github.com/azurebrasil/argocd-vpa-updater/internal/domain"
)

// kustomizePatchPatcher is a stub: Kustomize strategic-merge/JSON6902 patch
// files have different structural semantics than a plain manifest or values
// file (a patch document targets a resource by selector, not by a direct key
// path into the file itself), and are not implemented in this phase. It is
// registered now so ManifestPatcherRegistry and the API/dashboard layer
// don't need to change shape when it is implemented.
type kustomizePatchPatcher struct{}

// NewKustomizePatchPatcher returns the (stub) ManifestPatcher for
// SourceTypeKustomizePatch.
func NewKustomizePatchPatcher() ManifestPatcher { return kustomizePatchPatcher{} }

func (kustomizePatchPatcher) SourceType() domain.SourceType { return domain.SourceTypeKustomizePatch }

func (kustomizePatchPatcher) ReadCurrentValues(context.Context, []byte, domain.WriteTarget) (domain.ResourceAmount, error) {
	return domain.ResourceAmount{}, ErrNotImplemented
}

func (kustomizePatchPatcher) Patch(context.Context, []byte, domain.PatchRequest) ([]byte, domain.PatchResult, error) {
	return nil, domain.PatchResult{}, ErrNotImplemented
}
