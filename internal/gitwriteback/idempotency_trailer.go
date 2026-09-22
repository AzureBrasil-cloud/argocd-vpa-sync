package gitwriteback

import (
	"strings"
)

// idempotencyTrailerKey is the Git trailer written into every write-back
// commit message so a later Apply call, even from a fresh clone, can detect
// "this exact recommendation was already applied" without needing any
// external state.
const idempotencyTrailerKey = "Vpa-Idempotency-Key"

// buildCommitMessage appends the idempotency trailer to a human-readable
// summary, following the "key: value" Git trailer convention (a blank line
// then "Key: value", as used for e.g. "Signed-off-by").
func buildCommitMessage(summary, idempotencyKey string) string {
	return buildBatchCommitMessage(summary, []string{idempotencyKey})
}

// buildBatchCommitMessage is buildCommitMessage's multi-item counterpart: a
// single commit covering several selections carries one trailer line per
// idempotency key (the same repeated-trailer-name convention Git itself uses
// for e.g. multiple "Co-authored-by:" lines), so each item can still be
// independently detected as already-applied on a later replay.
func buildBatchCommitMessage(summary string, idempotencyKeys []string) string {
	summary = strings.TrimRight(summary, "\n")
	var b strings.Builder
	b.WriteString(summary)
	b.WriteString("\n\n")
	for _, key := range idempotencyKeys {
		b.WriteString(idempotencyTrailerKey)
		b.WriteString(": ")
		b.WriteString(key)
		b.WriteString("\n")
	}
	return b.String()
}

// extractIdempotencyKey parses the first trailer out of a commit message, if
// present.
func extractIdempotencyKey(message string) (string, bool) {
	keys := extractIdempotencyKeys(message)
	if len(keys) == 0 {
		return "", false
	}
	return keys[0], true
}

// extractIdempotencyKeys parses every idempotency trailer line out of a
// commit message -- a batch commit carries one per item (see
// buildBatchCommitMessage).
func extractIdempotencyKeys(message string) []string {
	prefix := idempotencyTrailerKey + ":"
	var keys []string
	for _, line := range strings.Split(message, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			keys = append(keys, strings.TrimSpace(strings.TrimPrefix(line, prefix)))
		}
	}
	return keys
}
