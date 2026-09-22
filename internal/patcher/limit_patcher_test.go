package patcher

import (
	"context"
	"strings"
	"testing"

	"github.com/azurebrasil/argocd-vpa-updater/internal/domain"
)

func headroom(p float64) *domain.LimitSpec { return &domain.LimitSpec{HeadroomPercent: &p} }

func absolute(v string) *domain.LimitSpec { return &domain.LimitSpec{Value: qptr(v)} }

// monitorSiteYAML reproduces the Argo CD sync failure that motivated limit
// write-back: a 305Mi recommendation written against a 300Mi limit.
const monitorSiteYAML = `apiVersion: apps/v1
kind: Deployment
metadata:
  name: monitor-site
spec:
  template:
    spec:
      containers:
        - name: app
          resources:
            requests:
              cpu: 100m
              memory: 250Mi
            limits:
              cpu: 200m
              memory: 300Mi
`

func deploymentTargetWithLimits() domain.WriteTarget {
	t := deploymentTarget()
	t.CPULimitKeyPath = "spec.template.spec.containers[app].resources.limits.cpu"
	t.MemoryLimitKeyPath = "spec.template.spec.containers[app].resources.limits.memory"
	return t
}

func TestPatch_LimitHeadroom_RaisesLimitAboveNewRequest(t *testing.T) {
	req := domain.PatchRequest{
		Target:      deploymentTargetWithLimits(),
		ApplyCPU:    true,
		ApplyMemory: true,
		CPULimit:    headroom(20),
		MemoryLimit: headroom(20),
		Recommendation: domain.ContainerRecommendation{
			Target: domain.ResourceAmount{CPU: qptr("250m"), Memory: qptr("305Mi")},
		},
	}

	out, result, err := NewYAMLPatcher().Patch(context.Background(), []byte(monitorSiteYAML), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Changed {
		t.Fatalf("expected Changed=true")
	}
	text := string(out)
	for _, want := range []string{"memory: 305Mi", "memory: 382Mi", "cpu: 250m", "cpu: 313m"} {
		if !strings.Contains(text, want) {
			t.Errorf("expected %q in output, got:\n%s", want, text)
		}
	}
}

func TestPatch_LimitHeadroom_CanLowerLimit(t *testing.T) {
	req := domain.PatchRequest{
		Target:      domain.WriteTarget{FilePath: "values.yaml", MemoryKeyPath: "resources.requests.memory", MemoryLimitKeyPath: "resources.limits.memory"},
		ApplyMemory: true,
		CPULimit:    headroom(20),
		MemoryLimit: headroom(20),
		Recommendation: domain.ContainerRecommendation{
			Target: domain.ResourceAmount{Memory: qptr("200Mi")},
		},
	}

	out, _, err := NewHelmValuesPatcher().Patch(context.Background(), []byte(helmValuesYAML), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 1Gi -> 250Mi: an inexact Gi value steps down to Mi rather than
	// rounding up to a whole Gi (which would make the limit unable to drop).
	if !strings.Contains(string(out), "memory: 200Mi") || !strings.Contains(string(out), "memory: 250Mi") {
		t.Fatalf("unexpected output:\n%s", out)
	}
}

func TestPatch_LimitHeadroom_RoundsLimitUpInCoarserUnit(t *testing.T) {
	const yamlDoc = `resources:
  requests:
    memory: 700Mi
  limits:
    memory: 2Gi
`
	req := domain.PatchRequest{
		Target:      domain.WriteTarget{FilePath: "values.yaml", MemoryKeyPath: "resources.requests.memory", MemoryLimitKeyPath: "resources.limits.memory"},
		ApplyMemory: true,
		CPULimit:    headroom(0),
		MemoryLimit: headroom(0),
		Recommendation: domain.ContainerRecommendation{
			Target: domain.ResourceAmount{Memory: qptr("1100Mi")},
		},
	}
	out, _, err := NewHelmValuesPatcher().Patch(context.Background(), []byte(yamlDoc), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	// 1100Mi at 0% must never become 1Gi (nearest), which would be below the request.
	if !strings.Contains(string(out), "memory: 1100Mi") {
		t.Fatalf("expected limit 1100Mi, got:\n%s", out)
	}
}

func TestPatch_LimitHeadroom_ExactGiValueKeepsGi(t *testing.T) {
	const yamlDoc = `resources:
  requests:
    memory: 1Gi
  limits:
    memory: 1Gi
`
	req := domain.PatchRequest{
		Target:      domain.WriteTarget{FilePath: "values.yaml", MemoryKeyPath: "resources.requests.memory", MemoryLimitKeyPath: "resources.limits.memory"},
		ApplyMemory: true,
		CPULimit:    headroom(50),
		MemoryLimit: headroom(50),
		Recommendation: domain.ContainerRecommendation{
			Target: domain.ResourceAmount{Memory: qptr("2Gi")},
		},
	}
	out, _, err := NewHelmValuesPatcher().Patch(context.Background(), []byte(yamlDoc), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(out), "memory: 2Gi") || !strings.Contains(string(out), "memory: 4Gi") {
		t.Fatalf("unexpected output:\n%s", out)
	}
}

func TestPatch_LimitHeadroom_MissingLimitIsNotCreated(t *testing.T) {
	target := deploymentTargetWithLimits()
	target.MemoryKeyPath = "spec.template.spec.containers[sidecar].resources.requests.memory"
	target.MemoryLimitKeyPath = "spec.template.spec.containers[sidecar].resources.limits.memory"
	req := domain.PatchRequest{
		Target:      target,
		ApplyMemory: true,
		CPULimit:    headroom(20),
		MemoryLimit: headroom(20),
		Recommendation: domain.ContainerRecommendation{
			Target: domain.ResourceAmount{Memory: qptr("80Mi")},
		},
	}

	out, result, err := NewYAMLPatcher().Patch(context.Background(), []byte(deploymentYAML), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Changed || !strings.Contains(string(out), "memory: 80Mi") {
		t.Fatalf("expected request to be patched, got:\n%s", out)
	}
	if strings.Count(string(out), "limits:") != 1 {
		t.Fatalf("expected no limits block to be created for sidecar, got:\n%s", out)
	}
}

func TestPatch_LimitHeadroom_NilLeavesLimitsUntouched(t *testing.T) {
	req := domain.PatchRequest{
		Target:      deploymentTargetWithLimits(),
		ApplyMemory: true,
		Recommendation: domain.ContainerRecommendation{
			Target: domain.ResourceAmount{Memory: qptr("305Mi")},
		},
	}
	out, _, err := NewYAMLPatcher().Patch(context.Background(), []byte(monitorSiteYAML), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(out), "memory: 300Mi") {
		t.Fatalf("expected limit to stay 300Mi, got:\n%s", out)
	}
}

func TestPatch_LimitHeadroom_OnlyAppliedResourceLimitChanges(t *testing.T) {
	req := domain.PatchRequest{
		Target:      deploymentTargetWithLimits(),
		ApplyMemory: true,
		CPULimit:    headroom(20),
		MemoryLimit: headroom(20),
		Recommendation: domain.ContainerRecommendation{
			Target: domain.ResourceAmount{CPU: qptr("999m"), Memory: qptr("305Mi")},
		},
	}
	out, _, err := NewYAMLPatcher().Patch(context.Background(), []byte(monitorSiteYAML), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(out), "cpu: 200m") || !strings.Contains(string(out), "cpu: 100m") {
		t.Fatalf("expected cpu request and limit untouched, got:\n%s", out)
	}
}

func TestPatch_LimitHeadroom_UnchangedRequestStillFixesLimit(t *testing.T) {
	req := domain.PatchRequest{
		Target:      deploymentTargetWithLimits(),
		ApplyMemory: true,
		CPULimit:    headroom(20),
		MemoryLimit: headroom(20),
		Recommendation: domain.ContainerRecommendation{
			Target: domain.ResourceAmount{Memory: qptr("250Mi")},
		},
	}
	out, result, err := NewYAMLPatcher().Patch(context.Background(), []byte(monitorSiteYAML), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !result.Changed || !strings.Contains(string(out), "memory: 313Mi") {
		t.Fatalf("expected limit 250Mi/0.8 -> 313Mi, got:\n%s", out)
	}
}

func TestPatch_AbsoluteLimit_WrittenAsGiven(t *testing.T) {
	req := domain.PatchRequest{
		Target:      deploymentTargetWithLimits(),
		ApplyCPU:    true,
		ApplyMemory: true,
		CPULimit:    headroom(20),
		MemoryLimit: absolute("512Mi"),
		Recommendation: domain.ContainerRecommendation{
			Target: domain.ResourceAmount{CPU: qptr("250m"), Memory: qptr("305Mi")},
		},
	}
	out, _, err := NewYAMLPatcher().Patch(context.Background(), []byte(monitorSiteYAML), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	for _, want := range []string{"memory: 305Mi", "memory: 512Mi", "cpu: 313m"} {
		if !strings.Contains(string(out), want) {
			t.Errorf("expected %q in output, got:\n%s", want, out)
		}
	}
}

func TestPatch_AbsoluteLimit_CanonicalizesOtherUnits(t *testing.T) {
	req := domain.PatchRequest{
		Target:      deploymentTargetWithLimits(),
		ApplyCPU:    true,
		ApplyMemory: true,
		CPULimit:    absolute("1.5"),
		MemoryLimit: absolute("1Gi"),
		Recommendation: domain.ContainerRecommendation{
			Target: domain.ResourceAmount{CPU: qptr("250m"), Memory: qptr("305Mi")},
		},
	}
	out, _, err := NewYAMLPatcher().Patch(context.Background(), []byte(monitorSiteYAML), req)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(string(out), "cpu: 1500m") || !strings.Contains(string(out), "memory: 1Gi") {
		t.Fatalf("unexpected output:\n%s", out)
	}
}

func TestPatch_AbsoluteLimit_BelowRequestIsError(t *testing.T) {
	req := domain.PatchRequest{
		Target:      deploymentTargetWithLimits(),
		ApplyMemory: true,
		MemoryLimit: absolute("300Mi"),
		Recommendation: domain.ContainerRecommendation{
			Target: domain.ResourceAmount{Memory: qptr("305Mi")},
		},
	}
	if _, _, err := NewYAMLPatcher().Patch(context.Background(), []byte(monitorSiteYAML), req); err == nil {
		t.Fatalf("expected an error for a limit below the request")
	}
}
