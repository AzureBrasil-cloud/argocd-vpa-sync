package patcher

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
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

// setQuantity sets the scalar node at rawPath to q's canonical string form.
// It returns changed=false, nil when the file already holds a numerically
// equal value, so callers can treat that as a no-op rather than a write.
func setQuantity(root *kyaml.RNode, rawPath string, q resource.Quantity) (bool, error) {
	node, err := lookupNode(root, rawPath)
	if err != nil {
		return false, err
	}

	current, err := resource.ParseQuantity(node.YNode().Value)
	if err == nil && current.MilliValue() == q.MilliValue() {
		return false, nil
	}

	node.YNode().Value = q.String()
	return true, nil
}
