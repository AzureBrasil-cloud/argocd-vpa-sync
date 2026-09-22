package patcher

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"

	"k8s.io/apimachinery/pkg/api/resource"
	kyaml "sigs.k8s.io/kustomize/kyaml/yaml"

	"github.com/azurebrasil/argocd-vpa-updater/internal/domain"
)

// keyPathPatcher is a ManifestPatcher that edits scalar values at
// dot-separated (optionally indexed) key paths within a YAML document, using
// kyaml's node-level API so comments, key order and any untouched content
// survive unchanged. It backs both SourceTypeYAML (plain Deployment/
// StatefulSet/CronJob manifests, where a key path typically indexes into a
// container list, e.g. "spec.template.spec.containers[app].resources.requests.cpu")
// and SourceTypeHelmValues (a values file, where a key path is usually a
// plain dotted path, e.g. "resources.requests.cpu") -- both are, at the
// YAML level, the same operation: find a scalar node by path, replace its
// value, re-serialize.
type keyPathPatcher struct {
	sourceType domain.SourceType
}

// NewYAMLPatcher returns the ManifestPatcher for SourceTypeYAML.
func NewYAMLPatcher() ManifestPatcher { return keyPathPatcher{sourceType: domain.SourceTypeYAML} }

// NewHelmValuesPatcher returns the ManifestPatcher for SourceTypeHelmValues.
func NewHelmValuesPatcher() ManifestPatcher {
	return keyPathPatcher{sourceType: domain.SourceTypeHelmValues}
}

func (p keyPathPatcher) SourceType() domain.SourceType { return p.sourceType }

func (p keyPathPatcher) ReadCurrentValues(_ context.Context, fileContent []byte, target domain.WriteTarget) (domain.ResourceAmount, error) {
	root, err := kyaml.Parse(string(fileContent))
	if err != nil {
		return domain.ResourceAmount{}, fmt.Errorf("patcher: parse yaml: %w", err)
	}

	var out domain.ResourceAmount
	if target.CPUKeyPath != "" {
		q, err := lookupQuantity(root, target.CPUKeyPath)
		if err != nil {
			return domain.ResourceAmount{}, err
		}
		out.CPU = q
	}
	if target.MemoryKeyPath != "" {
		q, err := lookupQuantity(root, target.MemoryKeyPath)
		if err != nil {
			return domain.ResourceAmount{}, err
		}
		out.Memory = q
	}
	return out, nil
}

func (p keyPathPatcher) Patch(_ context.Context, fileContent []byte, req domain.PatchRequest) ([]byte, domain.PatchResult, error) {
	root, err := kyaml.Parse(string(fileContent))
	if err != nil {
		return nil, domain.PatchResult{}, fmt.Errorf("patcher: parse yaml: %w", err)
	}

	changed := false

	if req.ApplyCPU && req.Target.CPUKeyPath != "" {
		desired := req.EffectiveCPU()
		if desired == nil {
			return nil, domain.PatchResult{}, fmt.Errorf("patcher: ApplyCPU is set but no CPU value is available to apply")
		}
		c, err := setQuantity(root, req.Target.CPUKeyPath, *desired)
		if err != nil {
			return nil, domain.PatchResult{}, err
		}
		changed = changed || c

		c, err = syncLimit(root, req.Target.CPUKeyPath, req.Target.CPULimitKeyPath, req.CPULimit, true)
		if err != nil {
			return nil, domain.PatchResult{}, err
		}
		changed = changed || c
	}

	if req.ApplyMemory && req.Target.MemoryKeyPath != "" {
		desired := req.EffectiveMemory()
		if desired == nil {
			return nil, domain.PatchResult{}, fmt.Errorf("patcher: ApplyMemory is set but no memory value is available to apply")
		}
		c, err := setQuantity(root, req.Target.MemoryKeyPath, *desired)
		if err != nil {
			return nil, domain.PatchResult{}, err
		}
		changed = changed || c

		c, err = syncLimit(root, req.Target.MemoryKeyPath, req.Target.MemoryLimitKeyPath, req.MemoryLimit, false)
		if err != nil {
			return nil, domain.PatchResult{}, err
		}
		changed = changed || c
	}

	if !changed {
		sum := sha256.Sum256(fileContent)
		return fileContent, domain.PatchResult{Changed: false, NewContentSHA256: hex.EncodeToString(sum[:])}, nil
	}

	newText, err := root.String()
	if err != nil {
		return nil, domain.PatchResult{}, fmt.Errorf("patcher: serialize patched yaml: %w", err)
	}
	newContent := []byte(newText)

	sum := sha256.Sum256(newContent)
	diff := UnifiedDiff(req.Target.FilePath, string(fileContent), newText)

	return newContent, domain.PatchResult{
		Changed:          true,
		NewContentSHA256: hex.EncodeToString(sum[:]),
		Diff:             diff,
	}, nil
}

// lookupQuantity finds the scalar node at rawPath and parses it as a
// resource.Quantity. It returns ErrKeyPathNotFound (wrapped) if the path
// does not exist -- it is never created.
func lookupQuantity(root *kyaml.RNode, rawPath string) (*resource.Quantity, error) {
	node, err := lookupNode(root, rawPath)
	if err != nil {
		return nil, err
	}
	val := node.YNode().Value
	q, err := resource.ParseQuantity(val)
	if err != nil {
		return nil, fmt.Errorf("patcher: value %q at key path %q is not a valid quantity: %w", val, rawPath, err)
	}
	return &q, nil
}

func lookupNode(root *kyaml.RNode, rawPath string) (*kyaml.RNode, error) {
	segs, err := parseKeyPath(rawPath)
	if err != nil {
		return nil, fmt.Errorf("patcher: %w", err)
	}
	node, err := root.Pipe(kyaml.Lookup(segs...))
	if err != nil {
		return nil, fmt.Errorf("patcher: lookup key path %q: %w", rawPath, err)
	}
	if node == nil {
		return nil, fmt.Errorf("%w: %q", ErrKeyPathNotFound, rawPath)
	}
	return node, nil
}

// setQuantity sets the scalar node at rawPath to q, formatted using the same
// unit suffix the file's existing value already uses (see
// formatLikeExisting). It returns changed=false, nil when the file already
// holds a numerically equal value, so callers can treat that as a no-op
// rather than a write.
func setQuantity(root *kyaml.RNode, rawPath string, q resource.Quantity) (bool, error) {
	return setQuantityRounded(root, rawPath, q, false)
}

// setQuantityRounded is setQuantity with a choice of rounding when q has to
// be expressed in a coarser unit than it carries: nearest (roundUp=false,
// right for a request) or up (roundUp=true, right for a limit, which must
// never land below the request it was computed from).
func setQuantityRounded(root *kyaml.RNode, rawPath string, q resource.Quantity, roundUp bool) (bool, error) {
	return setQuantityFormatted(root, rawPath, func(existingRaw string) string {
		return formatLikeExistingRounded(existingRaw, q, roundUp)
	})
}

// setQuantityFormatted sets the scalar at rawPath to format(existingRaw);
// a no-op when that's numerically what's already there.
func setQuantityFormatted(root *kyaml.RNode, rawPath string, format func(existingRaw string) string) (bool, error) {
	node, err := lookupNode(root, rawPath)
	if err != nil {
		return false, err
	}

	existingRaw := node.YNode().Value
	newRaw := format(existingRaw)
	current, err := resource.ParseQuantity(existingRaw)
	if written, werr := resource.ParseQuantity(newRaw); err == nil && werr == nil && current.MilliValue() == written.MilliValue() {
		return false, nil
	}

	node.YNode().Value = newRaw
	// The node's Tag was resolved from the OLD value (e.g. "!!int" for a
	// plain "1"); left in place, a new value of a different implicit kind
	// (e.g. "0.7", a float) would force the encoder to emit an incorrect
	// explicit tag ("!!int 0.7", not valid YAML/int). Clearing it lets the
	// encoder re-infer the correct implicit tag from the new value, exactly
	// as if a human had hand-written it.
	node.YNode().Tag = ""
	return true, nil
}

// syncLimit rewrites the limit at limitPath per spec (see domain.LimitSpec):
// by headroom, derived from the request as it was actually written -- after
// rounding to the file's unit -- so the two can never disagree the way
// Kubernetes rejects (request > limit), even when the request and limit use
// different units; or to an absolute value, written as given, which is an
// error if it's below that request. It is a no-op when spec is nil (limit
// left alone) or limitPath is empty, and also when the limit key doesn't
// exist in the file: a container without a limit is already valid, and a
// limit is never created.
func syncLimit(root *kyaml.RNode, requestPath, limitPath string, spec *domain.LimitSpec, isCPU bool) (bool, error) {
	if spec == nil || limitPath == "" {
		return false, nil
	}
	if _, err := lookupNode(root, limitPath); err != nil {
		if errors.Is(err, ErrKeyPathNotFound) {
			return false, nil
		}
		return false, err
	}

	request, err := lookupQuantity(root, requestPath)
	if err != nil {
		return false, err
	}
	limit, err := spec.Limit(*request, isCPU)
	if err != nil {
		return false, fmt.Errorf("patcher: %s: %w", limitPath, err)
	}
	if spec.Value != nil {
		return setQuantityFormatted(root, limitPath, func(string) string { return limit.String() })
	}
	return setQuantityRounded(root, limitPath, limit, true)
}
