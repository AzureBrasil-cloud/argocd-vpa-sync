package patcher

import "testing"

func TestFormatLikeExisting_RoundsToExistingBinarySuffix(t *testing.T) {
	// 148858618 bytes is a typical raw VPA memory recommendation -- not an
	// exact multiple of any binary unit. Against an existing "256Mi" value,
	// it should round to the nearest whole Mi (142), not fall back to a
	// bare byte count.
	got := formatLikeExisting("256Mi", *qptr("148858618"))
	if got != "142Mi" {
		t.Fatalf("expected 142Mi, got %q", got)
	}
}

func TestFormatLikeExisting_ExactBinarySuffixRoundTrips(t *testing.T) {
	got := formatLikeExisting("256Mi", *qptr("512Mi"))
	if got != "512Mi" {
		t.Fatalf("expected 512Mi, got %q", got)
	}
}

func TestFormatLikeExisting_PreservesGiSuffix(t *testing.T) {
	got := formatLikeExisting("1Gi", *qptr("1610612736")) // 1.5 GiB exactly
	if got != "2Gi" {
		// 1610612736 / (1<<30) = 1.5 -> rounds to 2 (round-half-away-from-zero).
		t.Fatalf("expected 2Gi, got %q", got)
	}
}

func TestFormatLikeExisting_PreservesDecimalSuffix(t *testing.T) {
	got := formatLikeExisting("256M", *qptr("148858618"))
	if got != "149M" {
		t.Fatalf("expected 149M, got %q", got)
	}
}

func TestFormatLikeExisting_PreservesMilliSuffixForCPU(t *testing.T) {
	got := formatLikeExisting("100m", *qptr("250m"))
	if got != "250m" {
		t.Fatalf("expected 250m, got %q", got)
	}
}

func TestFormatLikeExisting_PreservesPlainWholeNumber(t *testing.T) {
	got := formatLikeExisting("1", *qptr("2"))
	if got != "2" {
		t.Fatalf("expected 2, got %q", got)
	}
}

// TestFormatLikeExisting_SubUnitCPUNeverRoundsUp is a regression test: a
// no-suffix existing value (e.g. "cpu: 1") must not cause a genuine sub-core
// CPU recommendation to be rounded UP to a whole core -- that would silently
// request far more CPU than the VPA actually recommended.
func TestFormatLikeExisting_SubUnitCPUNeverRoundsUp(t *testing.T) {
	cases := []struct {
		desired string
		want    string
	}{
		{"700m", "0.7"},
		{"333m", "0.333"},
		{"1500m", "1.5"},
		{"2000m", "2"},
		{"50m", "0.05"},
	}
	for _, tc := range cases {
		got := formatLikeExisting("1", *qptr(tc.desired))
		if got != tc.want {
			t.Errorf("formatLikeExisting(%q, %q) = %q, want %q", "1", tc.desired, got, tc.want)
		}
	}
}

func TestFormatLikeExisting_FallsBackToCanonicalFormWhenSuffixUnrecognized(t *testing.T) {
	got := formatLikeExisting("129e6", *qptr("512Mi"))
	want := (*qptr("512Mi")).String()
	if got != want {
		t.Fatalf("expected fallback to canonical form %q, got %q", want, got)
	}
}

func TestSuffixOf(t *testing.T) {
	cases := []struct {
		raw        string
		wantSuffix string
		wantOK     bool
	}{
		{"256Mi", "Mi", true},
		{"1Gi", "Gi", true},
		{"250m", "m", true},
		{"256M", "M", true},
		{"4", "", true},
		{"129e6", "", false},
	}
	for _, tc := range cases {
		suf, ok := suffixOf(tc.raw)
		if suf != tc.wantSuffix || ok != tc.wantOK {
			t.Errorf("suffixOf(%q) = (%q, %v), want (%q, %v)", tc.raw, suf, ok, tc.wantSuffix, tc.wantOK)
		}
	}
}

func TestRoundDiv(t *testing.T) {
	cases := []struct {
		a, b, want int64
	}{
		{148858618, 1 << 20, 142},
		{536870912, 1 << 20, 512},
		{1610612736, 1 << 30, 2},
		{-148858618, 1 << 20, -142},
	}
	for _, tc := range cases {
		got := roundDiv(tc.a, tc.b)
		if got != tc.want {
			t.Errorf("roundDiv(%d, %d) = %d, want %d", tc.a, tc.b, got, tc.want)
		}
	}
}
