// Package traceid extracts, validates, and generates request correlation IDs.
package traceid

import (
	"net/http"
	"strings"
)

const (
	HeaderXTraceID        = "X-Trace-ID"
	HeaderXRequestID      = "X-Request-ID"
	HeaderXCorrelationID  = "X-Correlation-ID"
	HeaderCloudflareRayID = "CF-Ray"
	HeaderB3TraceID       = "X-B3-TraceId"
	HeaderTraceparent     = "traceparent"
)

// Source extracts a candidate ID. False indicates no usable candidate.
// Sources must be fast, concurrency-safe, and non-panicking.
type Source func(*http.Request) (string, bool)

// Validator accepts candidate IDs. Validators must be fast, concurrency-safe,
// and non-panicking. The router applies transport safety independently.
type Validator func(string) bool

// FromHeader reads the first value of name without trimming or validating it.
// An absent or empty value returns false. Request headers are never modified.
func FromHeader(name string) Source {
	return func(r *http.Request) (string, bool) {
		id := r.Header.Get(name)
		return id, id != ""
	}
}

// IsValid accepts 1–64 ASCII letters, digits, hyphens, or underscores.
func IsValid(id string) bool {
	if id == "" || len(id) > 64 {
		return false
	}
	for i := range len(id) {
		c := id[i]
		switch {
		case c >= '0' && c <= '9', c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c == '-', c == '_':
		default:
			return false
		}
	}
	return true
}

// FromTraceparent validates the W3C traceparent base format and returns only
// its 32-character trace ID. Version 00 must have exactly 55 characters; future
// versions may append opaque fields after a hyphen. Version ff is forbidden.
// This extracts correlation only; it does not create spans or emit traceparent.
func FromTraceparent(r *http.Request) (string, bool) {
	values := r.Header.Values(HeaderTraceparent)
	if len(values) != 1 {
		return "", false
	}
	v := values[0]
	if len(v) < 55 || v[2] != '-' || v[35] != '-' || v[52] != '-' ||
		!lowerHex(v[:2]) || v[:2] == "ff" || !lowerHex(v[3:35]) ||
		!lowerHex(v[36:52]) || !lowerHex(v[53:55]) ||
		v[3:35] == strings.Repeat("0", 32) || v[36:52] == strings.Repeat("0", 16) {
		return "", false
	}
	if len(v) > 55 {
		if v[:2] == "00" || v[55] != '-' {
			return "", false
		}
		for i := 56; i < len(v); i++ {
			if v[i] <= ' ' || v[i] >= 127 || v[i] == ',' {
				return "", false
			}
		}
	}
	return v[3:35], true
}

func lowerHex(s string) bool {
	for i := range len(s) {
		if (s[i] < '0' || s[i] > '9') && (s[i] < 'a' || s[i] > 'f') {
			return false
		}
	}
	return true
}
