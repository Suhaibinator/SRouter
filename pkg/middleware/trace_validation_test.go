package middleware

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Suhaibinator/SRouter/pkg/scontext"
)

// TestIsValidTraceID exercises the character-class and length validation for
// inbound X-Trace-ID values.
func TestIsValidTraceID(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want bool
	}{
		{"empty", "", false},
		{"digits only", "0123456789", true},
		{"lowercase letters", "abcxyz", true},
		{"uppercase letters", "ABCXYZ", true},
		{"dash and underscore", "trace-id_1", true},
		{"mixed valid", "REQ-123_abcXYZ", true},
		{"max length (64)", strings.Repeat("a", 64), true},
		{"too long (65)", strings.Repeat("a", 65), false},
		{"disallowed punctuation", "abc!123", false},
		{"embedded newline", "abc\n123", false},
		{"embedded space", "abc 123", false},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			if got := isValidTraceID(tc.id); got != tc.want {
				t.Errorf("isValidTraceID(%q) = %v, want %v", tc.id, got, tc.want)
			}
		})
	}
}

// TestTraceMiddlewarePropagatesValidInboundTraceID verifies that a valid
// client-supplied X-Trace-ID is propagated unchanged to both the request
// context and the response header.
func TestTraceMiddlewarePropagatesValidInboundTraceID(t *testing.T) {
	generator := NewIDGenerator(4)
	defer generator.Stop()

	const inbound = "REQ-123_abcXYZ"
	var handlerTraceID string
	handler := CreateTraceMiddleware[string, any](generator)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerTraceID = scontext.GetTraceIDFromRequest[string, any](r)
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Trace-ID", inbound)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if handlerTraceID != inbound {
		t.Errorf("handler saw trace ID %q, want propagated inbound ID %q", handlerTraceID, inbound)
	}
	if got := rec.Header().Get("X-Trace-ID"); got != inbound {
		t.Errorf("response X-Trace-ID = %q, want propagated inbound ID %q", got, inbound)
	}
}

// TestTraceMiddlewareReplacesInvalidInboundTraceID verifies that invalid
// client-supplied trace IDs (oversized or containing unsafe characters) are
// not propagated: the middleware substitutes a freshly generated, valid ID.
func TestTraceMiddlewareReplacesInvalidInboundTraceID(t *testing.T) {
	generator := NewIDGenerator(4)
	defer generator.Stop()

	tests := []struct {
		name    string
		inbound string
	}{
		{"oversized", strings.Repeat("a", 65)},
		{"log injection attempt", "abc\nINJECTED"},
		{"disallowed characters", "abc!123"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			var handlerTraceID string
			handler := CreateTraceMiddleware[string, any](generator)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				handlerTraceID = scontext.GetTraceIDFromRequest[string, any](r)
				w.WriteHeader(http.StatusOK)
			}))

			req := httptest.NewRequest("GET", "/", nil)
			req.Header.Set("X-Trace-ID", tc.inbound)
			rec := httptest.NewRecorder()
			handler.ServeHTTP(rec, req)

			got := rec.Header().Get("X-Trace-ID")
			if got == tc.inbound {
				t.Fatalf("invalid inbound trace ID %q was propagated; it should have been replaced", tc.inbound)
			}
			if got == "" {
				t.Fatal("expected a generated trace ID in the response header, got empty")
			}
			if !isValidTraceID(got) {
				t.Errorf("generated replacement trace ID %q is not itself valid", got)
			}
			if handlerTraceID != got {
				t.Errorf("handler saw trace ID %q but response header has %q; they must match", handlerTraceID, got)
			}
		})
	}
}
