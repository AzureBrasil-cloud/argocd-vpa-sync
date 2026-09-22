// Package idempotency computes a deterministic key identifying one specific
// "apply this recommendation to this target" operation, so that replaying
// the same request never produces a duplicate Git commit.
package idempotency

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"

	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/azurebrasil/argocd-vpa-updater/internal/domain"
)

// Key computes a deterministic idempotency key from the VPA/container
// identity, the resolved write target, and the exact values being applied.
// Any change to the applied value, the target file/keys, or which resources
// are selected produces a different key; identical inputs always produce the
// identical key, regardless of call order or process restarts.
//
// The key intentionally does NOT include a timestamp: repeating the exact
// same selection (same recommendation, same target) must yield the exact
// same key so GitWriteBackService can detect and skip a duplicate.
func Key(vpaNamespace, vpaName string, target domain.WriteTarget, req domain.PatchRequest) string {
	h := sha256.New()

	fmt.Fprintf(h, "vpa=%s/%s\n", vpaNamespace, vpaName)
	fmt.Fprintf(h, "container=%s\n", target.ContainerName)
	fmt.Fprintf(h, "repo=%s\n", target.RepoURL)
	fmt.Fprintf(h, "branch=%s\n", target.Branch)
	fmt.Fprintf(h, "file=%s\n", target.FilePath)
	fmt.Fprintf(h, "sourceType=%s\n", target.SourceType)
	fmt.Fprintf(h, "cpuKeyPath=%s\n", target.CPUKeyPath)
	fmt.Fprintf(h, "memoryKeyPath=%s\n", target.MemoryKeyPath)
	fmt.Fprintf(h, "applyCPU=%v\n", req.ApplyCPU)
	fmt.Fprintf(h, "applyMemory=%v\n", req.ApplyMemory)
	fmt.Fprintf(h, "cpu=%s\n", quantityString(req.EffectiveCPU()))
	fmt.Fprintf(h, "memory=%s\n", quantityString(req.EffectiveMemory()))

	return "sha256:" + hex.EncodeToString(h.Sum(nil))
}

// quantityString normalizes through MilliValue rather than String(), since
// two quantities that print differently ("1000m" vs "1") but are numerically
// identical must produce the same key.
func quantityString(q *resource.Quantity) string {
	if q == nil {
		return "<nil>"
	}
	return fmt.Sprintf("%d", q.MilliValue())
}
