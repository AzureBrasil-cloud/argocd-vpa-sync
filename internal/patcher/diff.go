package patcher

import (
	"fmt"
	"strings"
)

// UnifiedDiff produces a minimal unified-diff-style rendering of the
// line-based changes between old and new, for use in the dashboard preview
// and commit message body. It is a small LCS-based line differ -- adequate
// for the manifest/values files this project patches (typically well under
// a few thousand lines); it is not intended for arbitrarily large inputs.
func UnifiedDiff(path, oldContent, newContent string) string {
	oldLines := splitLines(oldContent)
	newLines := splitLines(newContent)

	ops := diffLines(oldLines, newLines)
	if !hasChange(ops) {
		return ""
	}

	var b strings.Builder
	fmt.Fprintf(&b, "--- a/%s\n", path)
	fmt.Fprintf(&b, "+++ b/%s\n", path)

	const context = 3
	hunks := groupHunks(ops, context)
	for _, h := range hunks {
		writeHunk(&b, h)
	}
	return b.String()
}

func splitLines(s string) []string {
	if s == "" {
		return nil
	}
	lines := strings.Split(s, "\n")
	// A trailing newline produces a spurious empty final element; drop it so
	// line counts match what a human/editor would call "N lines".
	if len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	return lines
}

type opKind int

const (
	opEqual opKind = iota
	opDelete
	opInsert
)

type diffOp struct {
	kind opKind
	text string
	// oldIdx/newIdx are 0-based line indices into oldLines/newLines,
	// meaningful only for the corresponding kind (or opEqual, where both are
	// meaningful).
	oldIdx, newIdx int
}

// diffLines computes a line-level diff using dynamic-programming LCS,
// backtracked into a flat operation list.
func diffLines(a, b []string) []diffOp {
	n, m := len(a), len(b)
	dp := make([][]int, n+1)
	for i := range dp {
		dp[i] = make([]int, m+1)
	}
	for i := n - 1; i >= 0; i-- {
		for j := m - 1; j >= 0; j-- {
			if a[i] == b[j] {
				dp[i][j] = dp[i+1][j+1] + 1
			} else if dp[i+1][j] >= dp[i][j+1] {
				dp[i][j] = dp[i+1][j]
			} else {
				dp[i][j] = dp[i][j+1]
			}
		}
	}

	var ops []diffOp
	i, j := 0, 0
	for i < n && j < m {
		switch {
		case a[i] == b[j]:
			ops = append(ops, diffOp{kind: opEqual, text: a[i], oldIdx: i, newIdx: j})
			i++
			j++
		case dp[i+1][j] >= dp[i][j+1]:
			ops = append(ops, diffOp{kind: opDelete, text: a[i], oldIdx: i})
			i++
		default:
			ops = append(ops, diffOp{kind: opInsert, text: b[j], newIdx: j})
			j++
		}
	}
	for ; i < n; i++ {
		ops = append(ops, diffOp{kind: opDelete, text: a[i], oldIdx: i})
	}
	for ; j < m; j++ {
		ops = append(ops, diffOp{kind: opInsert, text: b[j], newIdx: j})
	}
	return ops
}

func hasChange(ops []diffOp) bool {
	for _, o := range ops {
		if o.kind != opEqual {
			return true
		}
	}
	return false
}

type hunk struct {
	ops                []diffOp
	oldStart, newStart int
	oldLines, newLines int
}

// groupHunks splits a full op list into hunks separated by more than
// 2*context unchanged lines, each carrying up to `context` lines of
// surrounding unchanged content, matching conventional unified-diff output.
func groupHunks(ops []diffOp, context int) []hunk {
	var hunks []hunk
	var cur []diffOp
	equalRun := 0

	flush := func() {
		if len(cur) == 0 {
			return
		}
		// Trim trailing pure-equal tail beyond `context` lines.
		end := len(cur)
		trail := 0
		for end > 0 && cur[end-1].kind == opEqual && trail < context {
			end--
			trail++
		}
		trimTrailEqual := len(cur) - end
		for trimTrailEqual > context {
			cur = cur[:len(cur)-1]
			trimTrailEqual--
		}
		hunks = append(hunks, buildHunk(cur))
		cur = nil
	}

	for idx, op := range ops {
		if op.kind == opEqual {
			equalRun++
			// Look ahead: if this run of equals is long enough to separate
			// hunks, cut here (after adding `context` lines of leading
			// context to the *next* hunk, handled by not adding this line to
			// `cur` once the run exceeds `context` on both sides).
			if equalRun > context {
				if len(cur) > 0 {
					flush()
				}
				// Skip lines until within `context` of the next change.
				remaining := ops[idx+1:]
				nextChangeOffset := -1
				for k, o2 := range remaining {
					if o2.kind != opEqual {
						nextChangeOffset = k
						break
					}
				}
				if nextChangeOffset == -1 || nextChangeOffset >= context {
					continue // still just skipping equal lines
				}
			}
			cur = append(cur, op)
		} else {
			equalRun = 0
			cur = append(cur, op)
		}
	}
	flush()
	return hunks
}

func buildHunk(ops []diffOp) hunk {
	h := hunk{ops: ops}
	oldStart, newStart := -1, -1
	for _, o := range ops {
		switch o.kind {
		case opEqual:
			if oldStart == -1 {
				oldStart = o.oldIdx
			}
			if newStart == -1 {
				newStart = o.newIdx
			}
			h.oldLines++
			h.newLines++
		case opDelete:
			if oldStart == -1 {
				oldStart = o.oldIdx
			}
			h.oldLines++
		case opInsert:
			if newStart == -1 {
				newStart = o.newIdx
			}
			h.newLines++
		}
	}
	if oldStart == -1 {
		oldStart = 0
	}
	if newStart == -1 {
		newStart = 0
	}
	h.oldStart = oldStart
	h.newStart = newStart
	return h
}

func writeHunk(b *strings.Builder, h hunk) {
	fmt.Fprintf(b, "@@ -%d,%d +%d,%d @@\n", h.oldStart+1, h.oldLines, h.newStart+1, h.newLines)
	for _, op := range h.ops {
		switch op.kind {
		case opEqual:
			fmt.Fprintf(b, " %s\n", op.text)
		case opDelete:
			fmt.Fprintf(b, "-%s\n", op.text)
		case opInsert:
			fmt.Fprintf(b, "+%s\n", op.text)
		}
	}
}
