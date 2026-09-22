package idempotency

import (
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"

	"github.com/azurebrasil/argocd-vpa-updater/internal/domain"
)

func qty(s string) *resource.Quantity {
	q := resource.MustParse(s)
	return &q
}

func baseTarget() domain.WriteTarget {
	return domain.WriteTarget{
		RepoURL:       "https://git.example.com/team/app.git",
		Branch:        "main",
		FilePath:      "apps/payments/values-prd.yaml",
		SourceType:    domain.SourceTypeHelmValues,
		CPUKeyPath:    "resources.requests.cpu",
		MemoryKeyPath: "resources.requests.memory",
		ContainerName: "app",
	}
}

func baseRequest() domain.PatchRequest {
	return domain.PatchRequest{
		ApplyCPU:    true,
		ApplyMemory: true,
		Recommendation: domain.ContainerRecommendation{
			ContainerName: "app",
			Target: domain.ResourceAmount{
				CPU:    qty("250m"),
				Memory: qty("512Mi"),
			},
		},
	}
}

func TestKey_StableForIdenticalInputs(t *testing.T) {
	k1 := Key("payments", "checkout-api-vpa", baseTarget(), baseRequest())
	k2 := Key("payments", "checkout-api-vpa", baseTarget(), baseRequest())
	if k1 != k2 {
		t.Fatalf("expected identical inputs to produce identical keys, got %q vs %q", k1, k2)
	}
}

func TestKey_ChangesWithRecommendedValue(t *testing.T) {
	req1 := baseRequest()
	req2 := baseRequest()
	req2.Recommendation.Target.CPU = qty("300m")

	k1 := Key("payments", "checkout-api-vpa", baseTarget(), req1)
	k2 := Key("payments", "checkout-api-vpa", baseTarget(), req2)
	if k1 == k2 {
		t.Fatalf("expected different recommended CPU to produce different keys")
	}
}

func TestKey_EquivalentQuantityRepresentationsMatch(t *testing.T) {
	req1 := baseRequest()
	req1.Recommendation.Target.CPU = qty("1000m")
	req2 := baseRequest()
	req2.Recommendation.Target.CPU = qty("1")

	k1 := Key("payments", "checkout-api-vpa", baseTarget(), req1)
	k2 := Key("payments", "checkout-api-vpa", baseTarget(), req2)
	if k1 != k2 {
		t.Fatalf("expected numerically-equal quantities (1000m vs 1) to produce the same key")
	}
}

func TestKey_ChangesWithTarget(t *testing.T) {
	target1 := baseTarget()
	target2 := baseTarget()
	target2.FilePath = "apps/payments/values-stg.yaml"

	k1 := Key("payments", "checkout-api-vpa", target1, baseRequest())
	k2 := Key("payments", "checkout-api-vpa", target2, baseRequest())
	if k1 == k2 {
		t.Fatalf("expected different file path to produce different keys")
	}
}

func TestKey_ChangesWithApplySelection(t *testing.T) {
	req1 := baseRequest()
	req2 := baseRequest()
	req2.ApplyMemory = false

	k1 := Key("payments", "checkout-api-vpa", baseTarget(), req1)
	k2 := Key("payments", "checkout-api-vpa", baseTarget(), req2)
	if k1 == k2 {
		t.Fatalf("expected different ApplyMemory selection to produce different keys")
	}
}

func TestKey_NoCollisionAcrossContainers(t *testing.T) {
	target1 := baseTarget()
	target1.ContainerName = "app"
	target2 := baseTarget()
	target2.ContainerName = "sidecar"

	k1 := Key("payments", "checkout-api-vpa", target1, baseRequest())
	k2 := Key("payments", "checkout-api-vpa", target2, baseRequest())
	if k1 == k2 {
		t.Fatalf("expected different container name to produce different keys")
	}
}

func TestKey_OverrideChangesKey(t *testing.T) {
	req1 := baseRequest()
	req2 := baseRequest()
	req2.OverrideCPU = &domain.ResourceAmount{CPU: qty("400m")}

	k1 := Key("payments", "checkout-api-vpa", baseTarget(), req1)
	k2 := Key("payments", "checkout-api-vpa", baseTarget(), req2)
	if k1 == k2 {
		t.Fatalf("expected an edited/overridden proposed value to change the key")
	}
}
