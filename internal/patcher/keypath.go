package patcher

import (
	"fmt"
	"strings"
)

// parseKeyPath turns an annotation-declared dotted key path into the
// segment list consumed by kyaml's yaml.Lookup/PathGetter.
//
// Plain segments are dot-separated field names: "resources.requests.cpu".
// A segment may carry a bracketed predicate to select one element of a
// sequence by its "name" field, matching how Kubernetes container lists are
// shaped: "containers[app].resources.requests.cpu" selects, within the
// "containers" sequence, the element whose "name" field equals "app", then
// descends into resources.requests.cpu on that element.
//
// This never infers structure: an absent segment is a hard error surfaced
// by the caller (ReadCurrentValues/Patch), not silently created.
func parseKeyPath(raw string) ([]string, error) {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil, fmt.Errorf("empty key path")
	}

	var out []string
	for _, part := range strings.Split(raw, ".") {
		if part == "" {
			return nil, fmt.Errorf("key path %q has an empty segment", raw)
		}
		open := strings.IndexByte(part, '[')
		if open == -1 {
			out = append(out, part)
			continue
		}
		if !strings.HasSuffix(part, "]") {
			return nil, fmt.Errorf("key path %q has an unterminated bracket in segment %q", raw, part)
		}
		field := part[:open]
		predicate := part[open+1 : len(part)-1]
		if field == "" || predicate == "" {
			return nil, fmt.Errorf("key path %q has a malformed indexed segment %q", raw, part)
		}
		out = append(out, field, fmt.Sprintf("[name=%s]", predicate))
	}
	return out, nil
}
