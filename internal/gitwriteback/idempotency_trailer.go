package gitwriteback

import (
	"fmt"
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
	summary = strings.TrimRight(summary, "\n")
	return fmt.Sprintf("%s\n\n%s: %s\n", summary, idempotencyTrailerKey, idempotencyKey)
}

// extractIdempotencyKey parses the trailer back out of a commit message, if
// present.
func extractIdempotencyKey(message string) (string, bool) {
	prefix := idempotencyTrailerKey + ":"
	for _, line := range strings.Split(message, "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, prefix) {
			return strings.TrimSpace(strings.TrimPrefix(line, prefix)), true
		}
	}
	return "", false
}
