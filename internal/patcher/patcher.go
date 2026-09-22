// Package patcher edits the resources.requests/limits CPU and memory values
// declared in a Git-tracked file, changing only the annotation-declared key
// paths and preserving everything else (comments, key order, unrelated
// content) byte-for-byte where untouched.
package patcher

import (
	"context"
	"errors"

	"github.com/azurebrasil/argocd-vpa-updater/internal/domain"
)

// ErrNotImplemented is returned by patchers that are registered (so the
// registry and API layer have a stable shape) but not yet implemented.
var ErrNotImplemented = errors.New("patcher: not implemented")

// ErrKeyPathNotFound is returned when an annotation-declared key path does
// not exist in the target file. Patchers never create a missing path --
// that would be exactly the kind of unsafe inference this project avoids.
var ErrKeyPathNotFound = errors.New("patcher: key path not found in file")

// ManifestPatcher reads and rewrites the resource values at a WriteTarget's
// declared key paths within one file's content.
type ManifestPatcher interface {
	SourceType() domain.SourceType

	// ReadCurrentValues extracts the resources currently declared at the
	// target's key paths, used by the dashboard to compute delta vs. the
	// VPA recommendation.
	ReadCurrentValues(ctx context.Context, fileContent []byte, target domain.WriteTarget) (domain.ResourceAmount, error)

	// Patch returns new file content with only the requested key paths
	// modified. If the requested value(s) already match what's in the file,
	// PatchResult.Changed is false and the returned bytes are byte-identical
	// to fileContent.
	Patch(ctx context.Context, fileContent []byte, req domain.PatchRequest) ([]byte, domain.PatchResult, error)
}

// ManifestPatcherRegistry looks up the ManifestPatcher for a SourceType.
type ManifestPatcherRegistry interface {
	For(sourceType domain.SourceType) (ManifestPatcher, bool)
}

type registry map[domain.SourceType]ManifestPatcher

// NewRegistry builds a ManifestPatcherRegistry from a set of patchers, keyed
// by each patcher's own SourceType().
func NewRegistry(patchers ...ManifestPatcher) ManifestPatcherRegistry {
	r := make(registry, len(patchers))
	for _, p := range patchers {
		r[p.SourceType()] = p
	}
	return r
}

func (r registry) For(sourceType domain.SourceType) (ManifestPatcher, bool) {
	p, ok := r[sourceType]
	return p, ok
}

// DefaultRegistry wires up every known SourceType: real implementations for
// yaml and helm-values, a stub (ErrNotImplemented on use) for
// kustomize-patch -- kept registered so callers never need a type switch and
// so wiring this in later isn't a breaking change.
func DefaultRegistry() ManifestPatcherRegistry {
	return NewRegistry(
		NewYAMLPatcher(),
		NewHelmValuesPatcher(),
		NewKustomizePatchPatcher(),
	)
}
