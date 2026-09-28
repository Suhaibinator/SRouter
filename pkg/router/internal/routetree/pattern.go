package routetree

import (
	"fmt"
)

type partKind uint8

const (
	staticPart partKind = iota
	paramPart
	catchAllPart
)

// part is a run of a route pattern. A static part is one or more static
// segments with their leading '/', sliced from the pattern, so a pattern
// such as "/a/b/:c" has the static part "/a/b" and the parameter "c". For
// wildcards, text is the name without its ':' or '*' marker.
type part struct {
	kind partKind
	text string
}

// parsePattern splits pattern into parts and validates the syntax described
// in docs/plans/route-table.md. It appends to buf, which lets callers keep
// short patterns on the stack, and reports whether every part is static.
func parsePattern(pattern string, buf []part) (parts []part, static bool, err error) {
	if pattern == "" || pattern[0] != '/' {
		return nil, false, fmt.Errorf("pattern %q must begin with '/'", pattern)
	}

	parts = buf[:0]
	static = true
	var nameBuf [8]string
	names := nameBuf[:0]
	run := 0 // start of the static run that has not been emitted yet
	for start := 0; start < len(pattern); {
		// One scan finds the segment's end and whether it holds a marker.
		end, markers := start+1, 0
		for end < len(pattern) && pattern[end] != '/' {
			if c := pattern[end]; c == ':' || c == '*' {
				markers++
			}
			end++
		}
		text := pattern[start+1 : end]
		last := end == len(pattern)

		if markers == 0 {
			start = end
			continue
		}
		if text[0] != ':' && text[0] != '*' {
			return nil, false, fmt.Errorf("pattern %q: wildcard in segment %q must fill the whole segment", pattern, text)
		}
		if run < start {
			parts = append(parts, part{kind: staticPart, text: pattern[run:start]})
		}
		run = end

		kind := paramPart
		if text[0] == '*' {
			kind = catchAllPart
			if !last {
				return nil, false, fmt.Errorf("pattern %q: catch-all %q must be the final segment", pattern, text)
			}
		}
		name := text[1:]
		if name == "" {
			return nil, false, fmt.Errorf("pattern %q: wildcard %q must have a non-empty name", pattern, text)
		}
		if markers > 1 {
			return nil, false, fmt.Errorf("pattern %q: segment %q must contain only one wildcard", pattern, text)
		}
		for _, seen := range names {
			if seen == name {
				return nil, false, fmt.Errorf("pattern %q: wildcard name %q is used more than once", pattern, name)
			}
		}
		names = append(names, name)
		parts = append(parts, part{kind: kind, text: name})
		static = false
		start = end
	}
	if run < len(pattern) {
		parts = append(parts, part{kind: staticPart, text: pattern[run:]})
	}
	return parts, static, nil
}
