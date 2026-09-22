package domain

import (
	"testing"

	"k8s.io/apimachinery/pkg/api/resource"
)

func TestLimitFromRequest(t *testing.T) {
	cases := []struct {
		name    string
		request string
		pct     float64
		isCPU   bool
		want    string
	}{
		{"memory 305Mi at 20% is 381.25Mi", "305Mi", 20, false, "399769600"},
		{"memory 200Mi at 20% is 250Mi", "200Mi", 20, false, "250Mi"},
		{"memory rounds up to a whole byte", "100", 30, false, "143"},
		{"zero headroom means limit == request", "256Mi", 0, false, "256Mi"},
		{"cpu 250m at 20% rounds up to 313m", "250m", 20, true, "313m"},
		{"cpu 1 at 50% is 2", "1", 50, true, "2"},
		{"fractional headroom", "300m", 12.5, true, "343m"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := LimitFromRequest(resource.MustParse(tc.request), tc.pct, tc.isCPU)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			want := resource.MustParse(tc.want)
			if got.Cmp(want) != 0 {
				t.Fatalf("LimitFromRequest(%s, %v) = %s, want %s", tc.request, tc.pct, got.String(), want.String())
			}
		})
	}
}

func TestLimitFromRequest_RejectsOutOfRangeHeadroom(t *testing.T) {
	for _, pct := range []float64{-1, 100, 150} {
		if _, err := LimitFromRequest(resource.MustParse("1"), pct, true); err == nil {
			t.Fatalf("expected an error for headroom %v", pct)
		}
	}
}

func TestLimitSpec_Validate(t *testing.T) {
	pct := func(p float64) *float64 { return &p }
	q := func(s string) *resource.Quantity { v := resource.MustParse(s); return &v }
	cases := []struct {
		name  string
		spec  LimitSpec
		isCPU bool
		ok    bool
	}{
		{"headroom", LimitSpec{HeadroomPercent: pct(20)}, false, true},
		{"headroom 100 rejected", LimitSpec{HeadroomPercent: pct(100)}, false, false},
		{"memory value", LimitSpec{Value: q("512Mi")}, false, true},
		{"cpu value", LimitSpec{Value: q("500m")}, true, true},
		{"memory millibytes rejected", LimitSpec{Value: q("100m")}, false, false},
		{"zero rejected", LimitSpec{Value: q("0")}, true, false},
		{"both rejected", LimitSpec{HeadroomPercent: pct(20), Value: q("1Gi")}, false, false},
		{"neither rejected", LimitSpec{}, false, false},
	}
	for _, tc := range cases {
		if err := tc.spec.Validate(tc.isCPU); (err == nil) != tc.ok {
			t.Errorf("%s: Validate() err = %v, want ok=%v", tc.name, err, tc.ok)
		}
	}
}
