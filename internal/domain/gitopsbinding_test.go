package domain

import "testing"

func TestGitOpsBindingSpec_ContainerByName(t *testing.T) {
	spec := GitOpsBindingSpec{
		Containers: []ContainerWriteBackConfig{
			{ContainerName: "app"},
			{ContainerName: "sidecar"},
		},
	}

	if _, ok := spec.ContainerByName("app"); !ok {
		t.Fatalf("expected to find container %q", "app")
	}
	if _, ok := spec.ContainerByName("missing"); ok {
		t.Fatalf("expected not to find a container that isn't declared")
	}
}

func TestGitOpsBindingSpec_EffectiveMinChangePercent_ContainerOverrideWins(t *testing.T) {
	override := 25.0
	spec := GitOpsBindingSpec{
		MinChangePercent: 10,
		Containers: []ContainerWriteBackConfig{
			{ContainerName: "app", MinChangePercent: &override},
		},
	}

	if got := spec.EffectiveMinChangePercent("app"); got != 25 {
		t.Fatalf("expected container override 25, got %v", got)
	}
}

func TestGitOpsBindingSpec_EffectiveMinChangePercent_FallsBackToBindingDefault(t *testing.T) {
	spec := GitOpsBindingSpec{
		MinChangePercent: 15,
		Containers: []ContainerWriteBackConfig{
			{ContainerName: "app"},
		},
	}

	if got := spec.EffectiveMinChangePercent("app"); got != 15 {
		t.Fatalf("expected binding default 15, got %v", got)
	}
}

func TestGitOpsBindingSpec_EffectiveMinChangePercent_FallsBackToGlobalDefault(t *testing.T) {
	spec := GitOpsBindingSpec{
		Containers: []ContainerWriteBackConfig{
			{ContainerName: "app"},
		},
	}

	if got := spec.EffectiveMinChangePercent("app"); got != DefaultMinChangePercent {
		t.Fatalf("expected global default %v, got %v", DefaultMinChangePercent, got)
	}
}

func TestGitOpsBindingSpec_EffectiveMinChangePercent_UnknownContainerUsesBindingDefault(t *testing.T) {
	spec := GitOpsBindingSpec{MinChangePercent: 20}
	if got := spec.EffectiveMinChangePercent("does-not-exist"); got != 20 {
		t.Fatalf("expected binding default 20 for an unknown container, got %v", got)
	}
}
