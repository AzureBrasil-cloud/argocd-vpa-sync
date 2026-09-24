package eligibility

import (
	"math"
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"
)

func qty(s string) *resource.Quantity {
	q := resource.MustParse(s)
	return &q
}

func TestComputeDelta_Basic(t *testing.T) {
	d := ComputeDelta(qty("100m"), qty("150m"))
	if !d.HasCurrent || !d.HasRecommended {
		t.Fatalf("expected both sides present")
	}
	if d.AbsoluteMilli != 50 {
		t.Fatalf("expected absolute delta 50, got %d", d.AbsoluteMilli)
	}
	if math.Abs(d.PercentChange-50) > 0.0001 {
		t.Fatalf("expected 50%% change, got %f", d.PercentChange)
	}
}

func TestComputeDelta_Decrease(t *testing.T) {
	d := ComputeDelta(qty("200m"), qty("100m"))
	if d.AbsoluteMilli != -100 {
		t.Fatalf("expected absolute delta -100, got %d", d.AbsoluteMilli)
	}
	if math.Abs(d.PercentChange-(-50)) > 0.0001 {
		t.Fatalf("expected -50%% change, got %f", d.PercentChange)
	}
}

func TestComputeDelta_ZeroCurrent(t *testing.T) {
	d := ComputeDelta(qty("0"), qty("100m"))
	if !math.IsInf(d.PercentChange, 1) {
		t.Fatalf("expected +Inf percent change from zero current, got %f", d.PercentChange)
	}
	if !d.MeetsThreshold(10) {
		t.Fatalf("expected zero-current increase to meet any finite threshold")
	}
}

func TestComputeDelta_BothZero(t *testing.T) {
	d := ComputeDelta(qty("0"), qty("0"))
	if d.PercentChange != 0 {
		t.Fatalf("expected 0%% change when both sides are zero, got %f", d.PercentChange)
	}
	if !d.IsZero() {
		t.Fatalf("expected IsZero to be true")
	}
}

func TestComputeDelta_MissingSide(t *testing.T) {
	d := ComputeDelta(nil, qty("100m"))
	if d.HasCurrent {
		t.Fatalf("expected HasCurrent to be false")
	}
	if d.MeetsThreshold(1) {
		t.Fatalf("a delta missing a side must never meet a threshold")
	}
}

func TestMeetsThreshold_Boundary(t *testing.T) {
	d := ComputeDelta(qty("100m"), qty("110m")) // exactly 10%
	if !d.MeetsThreshold(10) {
		t.Fatalf("expected exactly-at-threshold delta to meet a >= threshold")
	}
	if d.MeetsThreshold(10.0001) {
		t.Fatalf("expected delta just under threshold to not meet it")
	}
}

func TestEvaluate_Eligible(t *testing.T) {
	res := Evaluate(Input{
		CPU:              ComputeDelta(qty("100m"), qty("200m")),
		ApplyCPU:         true,
		MinChangePercent: 10,
	})
	if !res.Eligible {
		t.Fatalf("expected eligible, got reasons %v", res.Reasons)
	}
}

func TestEvaluate_BelowThreshold(t *testing.T) {
	res := Evaluate(Input{
		CPU:              ComputeDelta(qty("100m"), qty("102m")), // 2%
		ApplyCPU:         true,
		MinChangePercent: 10,
	})
	if res.Eligible {
		t.Fatalf("expected ineligible")
	}
	if len(res.Reasons) != 1 || res.Reasons[0] != ReasonBelowThreshold {
		t.Fatalf("expected ReasonBelowThreshold, got %v", res.Reasons)
	}
}

func TestEvaluate_ContainerExcluded(t *testing.T) {
	res := Evaluate(Input{
		CPU:               ComputeDelta(qty("100m"), qty("200m")),
		ApplyCPU:          true,
		MinChangePercent:  10,
		ContainerExcluded: true,
	})
	if res.Eligible {
		t.Fatalf("expected ineligible for excluded container")
	}
	if res.Reasons[0] != ReasonContainerExcluded {
		t.Fatalf("expected ReasonContainerExcluded, got %v", res.Reasons)
	}
}

func TestEvaluate_NoResourceSelected(t *testing.T) {
	res := Evaluate(Input{
		CPU:              ComputeDelta(qty("100m"), qty("200m")),
		MinChangePercent: 10,
	})
	if res.Eligible || res.Reasons[0] != ReasonNoResourceSelected {
		t.Fatalf("expected ReasonNoResourceSelected, got eligible=%v reasons=%v", res.Eligible, res.Reasons)
	}
}

func TestEvaluate_OneOfTwoEligibleIsEnough(t *testing.T) {
	res := Evaluate(Input{
		CPU:              ComputeDelta(qty("100m"), qty("101m")),   // below threshold
		Memory:           ComputeDelta(qty("100Mi"), qty("200Mi")), // well above threshold
		ApplyCPU:         true,
		ApplyMemory:      true,
		MinChangePercent: 10,
	})
	if !res.Eligible {
		t.Fatalf("expected eligible because memory alone clears the bar, got reasons %v", res.Reasons)
	}
}

func TestEvaluate_MinMaxAllowedBounds(t *testing.T) {
	min := qty("50m")
	max := qty("500m")
	res := Evaluate(Input{
		CPU:              ComputeDelta(qty("100m"), qty("600m")),
		ApplyCPU:         true,
		MinChangePercent: 1,
		CPUMinAllowed:    min,
		CPUMaxAllowed:    max,
	})
	if res.Eligible {
		t.Fatalf("expected ineligible: recommendation exceeds maxAllowed")
	}
	if res.Reasons[0] != ReasonAboveMaxAllowed {
		t.Fatalf("expected ReasonAboveMaxAllowed, got %v", res.Reasons)
	}
}

func TestEvaluate_NoCurrentValue(t *testing.T) {
	res := Evaluate(Input{
		CPU:              ComputeDelta(nil, qty("200m")),
		ApplyCPU:         true,
		MinChangePercent: 10,
	})
	if res.Eligible || res.Reasons[0] != ReasonNoCurrentValue {
		t.Fatalf("expected ReasonNoCurrentValue, got eligible=%v reasons=%v", res.Eligible, res.Reasons)
	}
}

func TestEvaluate_PerResourceBreakdown_BothEligible(t *testing.T) {
	res := Evaluate(Input{
		CPU:              ComputeDelta(qty("100m"), qty("200m")),
		Memory:           ComputeDelta(qty("100Mi"), qty("200Mi")),
		ApplyCPU:         true,
		ApplyMemory:      true,
		MinChangePercent: 10,
	})
	if !res.CPU.Requested || !res.CPU.Eligible {
		t.Fatalf("expected CPU requested and eligible, got %+v", res.CPU)
	}
	if !res.Memory.Requested || !res.Memory.Eligible {
		t.Fatalf("expected Memory requested and eligible, got %+v", res.Memory)
	}
}

func TestEvaluate_PerResourceBreakdown_MixedEligibility(t *testing.T) {
	// CPU is below threshold; memory clears it. The combined Eligible must
	// stay true (one clears the bar), but the per-resource breakdown must
	// still reflect that CPU individually did not -- this is exactly what
	// stops a UI from letting someone tick a "CPU" checkbox that shouldn't
	// be selectable just because memory happened to qualify.
	res := Evaluate(Input{
		CPU:              ComputeDelta(qty("100m"), qty("101m")),   // ~1%
		Memory:           ComputeDelta(qty("100Mi"), qty("200Mi")), // 100%
		ApplyCPU:         true,
		ApplyMemory:      true,
		MinChangePercent: 10,
	})
	if !res.Eligible {
		t.Fatalf("expected combined Eligible=true (memory clears the bar)")
	}
	if res.CPU.Eligible {
		t.Fatalf("expected CPU individually ineligible, got %+v", res.CPU)
	}
	if len(res.CPU.Reasons) == 0 || res.CPU.Reasons[0] != ReasonBelowThreshold {
		t.Fatalf("expected CPU reasons to include ReasonBelowThreshold, got %v", res.CPU.Reasons)
	}
	if !res.Memory.Eligible {
		t.Fatalf("expected Memory individually eligible, got %+v", res.Memory)
	}
}

func TestEvaluate_PerResourceBreakdown_NotRequestedLeavesZeroValue(t *testing.T) {
	res := Evaluate(Input{
		CPU:              ComputeDelta(qty("100m"), qty("200m")),
		ApplyCPU:         true,
		MinChangePercent: 10,
		// ApplyMemory intentionally false.
	})
	if res.Memory.Requested {
		t.Fatalf("expected Memory.Requested=false when ApplyMemory was false")
	}
}

func TestEvaluate_WithinVPABandIsNotEligible(t *testing.T) {
	// 300Mi is 25% below the 400Mi target -- well past the 10% threshold --
	// but inside the VPA's own 250Mi..500Mi range.
	res := Evaluate(Input{
		Memory:           ComputeDelta(qty("300Mi"), qty("400Mi")),
		ApplyMemory:      true,
		MinChangePercent: 10,
		MemoryBand:       Band{Lower: qty("250Mi"), Upper: qty("500Mi")},
	})
	if res.Memory.Eligible || len(res.Memory.Reasons) != 1 || res.Memory.Reasons[0] != ReasonWithinVPABounds {
		t.Fatalf("expected ineligible with %q, got %+v", ReasonWithinVPABounds, res.Memory)
	}
}

func TestEvaluate_OutsideVPABand(t *testing.T) {
	band := Band{Lower: qty("250Mi"), Upper: qty("500Mi")}
	res := Evaluate(Input{
		Memory:           ComputeDelta(qty("200Mi"), qty("400Mi")),
		ApplyMemory:      true,
		MinChangePercent: 10,
		MemoryBand:       band,
	})
	if !res.Memory.Eligible {
		t.Fatalf("expected eligible below the band, got %+v", res.Memory)
	}

	// Outside the band but under the threshold: the threshold still applies.
	res = Evaluate(Input{
		Memory:           ComputeDelta(qty("520Mi"), qty("500Mi")),
		ApplyMemory:      true,
		MinChangePercent: 10,
		MemoryBand:       band,
	})
	if res.Memory.Eligible || res.Memory.Reasons[0] != ReasonBelowThreshold {
		t.Fatalf("expected %q, got %+v", ReasonBelowThreshold, res.Memory)
	}
}

func TestEvaluate_IncompleteVPABandIsIgnored(t *testing.T) {
	res := Evaluate(Input{
		CPU:              ComputeDelta(qty("300m"), qty("400m")),
		ApplyCPU:         true,
		MinChangePercent: 10,
		CPUBand:          Band{Lower: qty("250m")},
	})
	if !res.CPU.Eligible {
		t.Fatalf("expected a band missing its upper bound to be ignored, got %+v", res.CPU)
	}
}
