package patcher

import (
	"fmt"
	"strings"

	"k8s.io/apimachinery/pkg/api/resource"
)

// binarySuffixScale/decimalSuffixScale map a Kubernetes quantity suffix to
// the number of bytes (or base units) it represents. See
// https://kubernetes.io/docs/reference/kubernetes-api/common-definitions/quantity/.
var binarySuffixScale = map[string]int64{
	"Ki": 1 << 10,
	"Mi": 1 << 20,
	"Gi": 1 << 30,
	"Ti": 1 << 40,
	"Pi": 1 << 50,
}

var decimalSuffixScale = map[string]int64{
	"k": 1_000,
	"M": 1_000_000,
	"G": 1_000_000_000,
	"T": 1_000_000_000_000,
	"P": 1_000_000_000_000_000,
}

// suffixOf returns the unit suffix a raw quantity string uses -- "256Mi" ->
// "Mi", "250m" -> "m", "4" -> "" -- or ok=false if it doesn't recognize the
// tail as any known Kubernetes quantity suffix (e.g. scientific notation
// like "129e6"), so the caller can fall back to a generic format instead of
// guessing wrong.
func suffixOf(raw string) (suffix string, ok bool) {
	for suf := range binarySuffixScale {
		if strings.HasSuffix(raw, suf) {
			return suf, true
		}
	}
	for suf := range decimalSuffixScale {
		if strings.HasSuffix(raw, suf) {
			return suf, true
		}
	}
	if strings.HasSuffix(raw, "m") {
		return "m", true
	}
	if _, err := resource.ParseQuantity(raw); err == nil && isPlainNumber(raw) {
		return "", true
	}
	return "", false
}

func isPlainNumber(raw string) bool {
	for _, r := range raw {
		if (r < '0' || r > '9') && r != '.' && r != '-' && r != '+' {
			return false
		}
	}
	return raw != ""
}

// formatLikeExisting formats q using the same unit suffix as existingRaw --
// e.g. if the file currently reads "256Mi", a new value is written back as
// "<N>Mi" too, rounded to the nearest whole unit -- rather than replacing it
// with resource.Quantity's own default canonical form. That matters because
// a VPA recommendation is typically an arbitrary exact byte count with no
// clean Ki/Mi/Gi divisor; resource.Quantity.String() on such a value falls
// back to a bare, unsuffixed integer (e.g. "148858618"), which both reads
// nothing like a normal Kubernetes manifest and gets needlessly
// double-quoted by the YAML encoder to keep it typed as a string rather than
// a number. Keeping the existing key's own unit preserves the manifest's own
// convention and produces a normal, human-readable value like "142Mi"
// instead. Falls back to q.String() if existingRaw's suffix isn't
// recognized.
func formatLikeExisting(existingRaw string, q resource.Quantity) string {
	return formatLikeExistingRounded(existingRaw, q, false)
}

// formatLikeExistingRounded is formatLikeExisting, rounding up instead of
// to nearest when roundUp is set (used for limits, which must never end up
// below the value they were computed to cover).
func formatLikeExistingRounded(existingRaw string, q resource.Quantity, roundUp bool) string {
	div := roundDiv
	if roundUp {
		div = ceilDiv
	}

	suf, ok := suffixOf(existingRaw)
	if !ok {
		return q.String()
	}
	if roundUp {
		suf = finerSuffixIfInexact(suf, q.Value())
	}

	switch {
	case suf == "":
		return formatWholeOrDecimal(q)
	case suf == "m":
		return fmt.Sprintf("%dm", q.MilliValue())
	case binarySuffixScale[suf] != 0:
		return fmt.Sprintf("%d%s", div(q.Value(), binarySuffixScale[suf]), suf)
	case decimalSuffixScale[suf] != 0:
		return fmt.Sprintf("%d%s", div(q.Value(), decimalSuffixScale[suf]), suf)
	default:
		return q.String()
	}
}

// formatWholeOrDecimal formats q as a plain, unsuffixed number: a whole
// integer when q has no sub-unit remainder, or a decimal fraction (to milli
// precision, Kubernetes' own finest-grained resolution) otherwise. This must
// never round a sub-1 quantity up to a whole unit -- unlike memory (always a
// whole number of bytes, so this path never truncates anything for it), CPU
// routinely has genuine sub-core recommendations (e.g. 700m), and a
// manifest that happens to write cpu without a suffix (e.g. "cpu: 1") must
// still be able to receive "0.7", not get silently inflated to "1".
func formatWholeOrDecimal(q resource.Quantity) string {
	milli := q.MilliValue()
	if milli%1000 == 0 {
		return fmt.Sprintf("%d", milli/1000)
	}
	s := fmt.Sprintf("%d.%03d", milli/1000, milli%1000)
	s = strings.TrimRight(s, "0")
	return strings.TrimSuffix(s, ".")
}

// roundDiv divides a by b, rounding to the nearest integer (half away from
// zero) rather than truncating -- so e.g. 148858618 bytes against Mi's
// 1048576 scale rounds to 142, not 141.
func roundDiv(a, b int64) int64 {
	if b == 0 {
		return a
	}
	if a >= 0 {
		return (a + b/2) / b
	}
	return -((-a + b/2) / b)
}

// finerSuffixIfInexact steps a unit coarser than Mi/M (Gi, Ti, G, ...) down
// to Mi/M when v isn't a whole multiple of it. Rounding a limit UP in a
// coarse unit can overshoot by almost a whole unit -- 250Mi would become
// 1Gi -- which would make a computed limit unable to ever go down.
func finerSuffixIfInexact(suf string, v int64) string {
	if scale := binarySuffixScale[suf]; scale > binarySuffixScale["Mi"] && v%scale != 0 {
		return "Mi"
	}
	if scale := decimalSuffixScale[suf]; scale > decimalSuffixScale["M"] && v%scale != 0 {
		return "M"
	}
	return suf
}

// ceilDiv divides a by b, rounding up (toward +infinity).
func ceilDiv(a, b int64) int64 {
	if b == 0 {
		return a
	}
	q := a / b
	if a%b != 0 && (a > 0) == (b > 0) {
		q++
	}
	return q
}
