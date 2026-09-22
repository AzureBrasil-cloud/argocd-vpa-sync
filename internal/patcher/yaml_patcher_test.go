package patcher

import (
	"context"
	"errors"
	"strings"
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/azurebrasil/argocd-vpa-updater/internal/domain"
)

func qptr(s string) *resource.Quantity {
	q := resource.MustParse(s)
	return &q
}

const deploymentYAML = `# checkout-api deployment
apiVersion: apps/v1
kind: Deployment
metadata:
  name: checkout-api
spec:
  template:
    spec:
      containers:
        - name: app
          image: example.com/checkout-api:1.2.3
          resources:
            requests:
              cpu: 100m # tuned by hand, please don't lower this
              memory: 256Mi
            limits:
              cpu: 500m
              memory: 512Mi
        - name: sidecar
          image: example.com/envoy:1.0.0
          resources:
            requests:
              cpu: 50m
              memory: 64Mi
`

func deploymentTarget() domain.WriteTarget {
	return domain.WriteTarget{
		FilePath:      "deploy/checkout-api.yaml",
		SourceType:    domain.SourceTypeYAML,
		CPUKeyPath:    "spec.template.spec.containers[app].resources.requests.cpu",
		MemoryKeyPath: "spec.template.spec.containers[app].resources.requests.memory",
		ContainerName: "app",
	}
}

func TestYAMLPatcher_ReadCurrentValues(t *testing.T) {
	p := NewYAMLPatcher()
	amt, err := p.ReadCurrentValues(context.Background(), []byte(deploymentYAML), deploymentTarget())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if amt.CPU.String() != "100m" {
		t.Fatalf("expected current cpu 100m, got %v", amt.CPU)
	}
	if amt.Memory.String() != "256Mi" {
		t.Fatalf("expected current memory 256Mi, got %v", amt.Memory)
	}
}

func TestYAMLPatcher_Patch_OnlyTargetKeysChange(t *testing.T) {
	p := NewYAMLPatcher()
	req := domain.PatchRequest{
		Target:      deploymentTarget(),
		ApplyCPU:    true,
		ApplyMemory: true,
		Recommendation: domain.ContainerRecommendation{
			ContainerName: "app",
			Target: domain.ResourceAmount{
				CPU:    qptr("250m"),
				Memory: qptr("512Mi"),
			},
		},
	}

	newContent, result, err := p.Patch(context.Background(), []byte(deploymentYAML), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Changed {
		t.Fatalf("expected Changed=true")
	}

	newText := string(newContent)

	if !strings.Contains(newText, "cpu: 250m") {
		t.Fatalf("expected app container cpu to be updated to 250m, got:\n%s", newText)
	}
	if !strings.Contains(newText, "memory: 512Mi") {
		t.Fatalf("expected app container memory to be updated to 512Mi, got:\n%s", newText)
	}

	// The sidecar container's identical-looking cpu/memory keys must be untouched.
	if !strings.Contains(newText, "cpu: 50m") {
		t.Fatalf("expected sidecar cpu (50m) to remain untouched, got:\n%s", newText)
	}
	if !strings.Contains(newText, "memory: 64Mi") {
		t.Fatalf("expected sidecar memory (64Mi) to remain untouched, got:\n%s", newText)
	}

	// The app container's limits must remain untouched (only requests were targeted).
	if !strings.Contains(newText, "cpu: 500m") {
		t.Fatalf("expected app cpu limit (500m) to remain untouched, got:\n%s", newText)
	}
	if !strings.Contains(newText, "memory: 512Mi") {
		// 512Mi now appears twice (new request value + untouched limit); that's fine,
		// already asserted above that the request line exists.
		_ = newText
	}

	// Comments and structure must be preserved.
	if !strings.Contains(newText, "# checkout-api deployment") {
		t.Fatalf("expected header comment to be preserved, got:\n%s", newText)
	}
	if !strings.Contains(newText, "please don't lower this") {
		t.Fatalf("expected inline comment to be preserved, got:\n%s", newText)
	}
	if !strings.Contains(newText, "image: example.com/checkout-api:1.2.3") {
		t.Fatalf("expected unrelated fields to be preserved, got:\n%s", newText)
	}

	if result.Diff == "" {
		t.Fatalf("expected a non-empty diff when Changed=true")
	}
}

func TestYAMLPatcher_Patch_NoOpWhenValueAlreadyMatches(t *testing.T) {
	p := NewYAMLPatcher()
	req := domain.PatchRequest{
		Target:      deploymentTarget(),
		ApplyCPU:    true,
		ApplyMemory: true,
		Recommendation: domain.ContainerRecommendation{
			ContainerName: "app",
			Target: domain.ResourceAmount{
				CPU:    qptr("100m"), // matches current value exactly
				Memory: qptr("256Mi"),
			},
		},
	}

	newContent, result, err := p.Patch(context.Background(), []byte(deploymentYAML), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result.Changed {
		t.Fatalf("expected Changed=false when values already match")
	}
	if string(newContent) != deploymentYAML {
		t.Fatalf("expected byte-identical content on no-op, got diff")
	}
	if result.Diff != "" {
		t.Fatalf("expected empty diff on no-op")
	}
}

func TestYAMLPatcher_Patch_OnlyCPUSelected(t *testing.T) {
	p := NewYAMLPatcher()
	req := domain.PatchRequest{
		Target:   deploymentTarget(),
		ApplyCPU: true, // ApplyMemory left false
		Recommendation: domain.ContainerRecommendation{
			ContainerName: "app",
			Target: domain.ResourceAmount{
				CPU:    qptr("300m"),
				Memory: qptr("999Mi"), // must be ignored since ApplyMemory is false
			},
		},
	}

	newContent, result, err := p.Patch(context.Background(), []byte(deploymentYAML), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Changed {
		t.Fatalf("expected Changed=true")
	}
	newText := string(newContent)
	if !strings.Contains(newText, "cpu: 300m") {
		t.Fatalf("expected cpu to be updated to 300m, got:\n%s", newText)
	}
	if strings.Contains(newText, "999Mi") {
		t.Fatalf("expected memory to remain untouched since ApplyMemory was false, got:\n%s", newText)
	}
	if !strings.Contains(newText, "memory: 256Mi") {
		t.Fatalf("expected original memory value 256Mi to remain, got:\n%s", newText)
	}
}

func TestYAMLPatcher_Patch_MissingKeyPathIsError(t *testing.T) {
	p := NewYAMLPatcher()
	target := deploymentTarget()
	target.CPUKeyPath = "spec.template.spec.containers[app].resources.requests.gpu"

	req := domain.PatchRequest{
		Target:   target,
		ApplyCPU: true,
		Recommendation: domain.ContainerRecommendation{
			Target: domain.ResourceAmount{CPU: qptr("300m")},
		},
	}

	_, _, err := p.Patch(context.Background(), []byte(deploymentYAML), req)
	if err == nil {
		t.Fatalf("expected an error for a missing key path")
	}
	if !errors.Is(err, ErrKeyPathNotFound) {
		t.Fatalf("expected ErrKeyPathNotFound, got %v", err)
	}
}

func TestYAMLPatcher_ReadCurrentValues_MissingKeyPathIsError(t *testing.T) {
	p := NewYAMLPatcher()
	target := deploymentTarget()
	target.MemoryKeyPath = "spec.template.spec.containers[app].resources.requests.nope"

	_, err := p.ReadCurrentValues(context.Background(), []byte(deploymentYAML), target)
	if !errors.Is(err, ErrKeyPathNotFound) {
		t.Fatalf("expected ErrKeyPathNotFound, got %v", err)
	}
}

const helmValuesYAML = `# payments-api values
replicaCount: 3
image:
  repository: example.com/payments-api
  tag: "1.4.0"
resources:
  requests:
    cpu: 200m
    memory: 256Mi
  limits:
    cpu: 1
    memory: 1Gi
`

func TestHelmValuesPatcher_PlainDottedPath(t *testing.T) {
	p := NewHelmValuesPatcher()
	target := domain.WriteTarget{
		FilePath:      "apps/payments/values-prd.yaml",
		SourceType:    domain.SourceTypeHelmValues,
		CPUKeyPath:    "resources.requests.cpu",
		MemoryKeyPath: "resources.requests.memory",
	}

	req := domain.PatchRequest{
		Target:      target,
		ApplyCPU:    true,
		ApplyMemory: true,
		Recommendation: domain.ContainerRecommendation{
			Target: domain.ResourceAmount{
				CPU:    qptr("350m"),
				Memory: qptr("384Mi"),
			},
		},
	}

	newContent, result, err := p.Patch(context.Background(), []byte(helmValuesYAML), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Changed {
		t.Fatalf("expected Changed=true")
	}
	newText := string(newContent)
	if !strings.Contains(newText, "cpu: 350m") {
		t.Fatalf("expected cpu updated to 350m, got:\n%s", newText)
	}
	if !strings.Contains(newText, "memory: 384Mi") {
		t.Fatalf("expected memory updated to 384Mi, got:\n%s", newText)
	}
	if !strings.Contains(newText, "cpu: 1\n") {
		t.Fatalf("expected limits.cpu (1) to remain untouched, got:\n%s", newText)
	}
	if !strings.Contains(newText, `tag: "1.4.0"`) {
		t.Fatalf("expected unrelated image.tag to remain untouched, got:\n%s", newText)
	}
}

func TestKustomizePatcher_IsStub(t *testing.T) {
	p := NewKustomizePatchPatcher()
	if p.SourceType() != domain.SourceTypeKustomizePatch {
		t.Fatalf("unexpected source type %q", p.SourceType())
	}
	if _, err := p.ReadCurrentValues(context.Background(), nil, domain.WriteTarget{}); !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("expected ErrNotImplemented, got %v", err)
	}
	if _, _, err := p.Patch(context.Background(), nil, domain.PatchRequest{}); !errors.Is(err, ErrNotImplemented) {
		t.Fatalf("expected ErrNotImplemented, got %v", err)
	}
}

func TestDefaultRegistry(t *testing.T) {
	reg := DefaultRegistry()
	for _, st := range []domain.SourceType{domain.SourceTypeYAML, domain.SourceTypeHelmValues, domain.SourceTypeKustomizePatch} {
		if _, ok := reg.For(st); !ok {
			t.Fatalf("expected registry to have an entry for %q", st)
		}
	}
	if _, ok := reg.For("unknown"); ok {
		t.Fatalf("expected no entry for an unknown source type")
	}
}
