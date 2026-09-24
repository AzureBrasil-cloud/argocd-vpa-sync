package domain

import (
	"encoding/json"
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"
)

func pct(p float64) *float64 { return &p }

func TestRequestWithHeadroom(t *testing.T) {
	cases := []struct {
		name     string
		base     string
		headroom *float64
		isCPU    bool
		want     string
	}{
		// 100Mi must end up as 70% of the request: 100Mi / 0.7, rounded up to a byte.
		{"memory 100Mi at 30% makes it 70% of the request", "100Mi", pct(30), false, "149796572"},
		{"cpu 100m at 30% rounds up to 143m", "100m", pct(30), true, "143m"},
		{"nil headroom returns the recommendation", "100Mi", nil, false, "100Mi"},
		{"zero headroom returns the recommendation", "100m", pct(0), true, "100m"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			base := resource.MustParse(tc.base)
			got, err := RequestWithHeadroom(&base, tc.headroom, tc.isCPU)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if want := resource.MustParse(tc.want); got.Cmp(want) != 0 {
				t.Fatalf("RequestWithHeadroom(%s, %v) = %s, want %s", tc.base, tc.headroom, got.String(), want.String())
			}
		})
	}
}

func TestRequestWithHeadroom_NilRecommendationAndInvalidHeadroom(t *testing.T) {
	if got, err := RequestWithHeadroom(nil, pct(30), false); err != nil || got != nil {
		t.Fatalf("expected nil, nil for a nil recommendation, got %v, %v", got, err)
	}
	base := resource.MustParse("100Mi")
	if _, err := RequestWithHeadroom(&base, pct(100), false); err == nil {
		t.Fatalf("expected an error for a 100%% headroom")
	}
}

func TestStateDocument_RequestHeadroomFor(t *testing.T) {
	var doc StateDocument
	// A document written before RequestHeadrooms existed decodes with it nil.
	if err := json.Unmarshal([]byte(`{"schemaVersion":2,"pendingSelections":[],"operations":{}}`), &doc); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if h := doc.RequestHeadroomFor("ns", "vpa", "app"); h.CPUPercent != nil || h.MemoryPercent != nil {
		t.Fatalf("expected no headroom, got %+v", h)
	}

	doc.RequestHeadrooms = map[string]RequestHeadroom{RequestHeadroomKey("ns", "vpa", "app"): {MemoryPercent: pct(30)}}
	if h := doc.RequestHeadroomFor("ns", "vpa", "app"); h.MemoryPercent == nil || *h.MemoryPercent != 30 {
		t.Fatalf("expected a 30%% memory headroom, got %+v", h)
	}
	if h := (*StateDocument)(nil).RequestHeadroomFor("ns", "vpa", "app"); h.MemoryPercent != nil {
		t.Fatalf("expected no headroom from a nil document, got %+v", h)
	}
}
