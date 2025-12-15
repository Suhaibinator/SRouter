package router

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"

	"github.com/Suhaibinator/SRouter/pkg/router/internal/mocks"
	"github.com/Suhaibinator/SRouter/pkg/scontext"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestWriteJSONError_MutexResponseWriter_SetsCORSHeaders(t *testing.T) {
	tests := []struct {
		name               string
		allowedOrigin      string
		credentialsAllowed bool
		wantVaryOrigin     bool
	}{
		{
			name:               "specific_origin_with_credentials_sets_vary",
			allowedOrigin:      "https://example.com",
			credentialsAllowed: true,
			wantVaryOrigin:     true,
		},
		{
			name:               "wildcard_origin_no_vary",
			allowedOrigin:      "*",
			credentialsAllowed: false,
			wantVaryOrigin:     false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := NewRouter[string, string](RouterConfig{Logger: zap.NewNop()}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

			req := httptest.NewRequest(http.MethodGet, "http://example.com/test", nil)
			req = req.WithContext(scontext.WithCORSInfo[string, string](req.Context(), tc.allowedOrigin, tc.credentialsAllowed))

			rr := httptest.NewRecorder()
			var mu sync.Mutex
			mrw := &mutexResponseWriter{ResponseWriter: rr, mu: &mu}

			r.writeJSONError(mrw, req, http.StatusBadRequest, "Bad Request", "")

			if got := rr.Header().Get("Access-Control-Allow-Origin"); got != tc.allowedOrigin {
				t.Fatalf("expected Access-Control-Allow-Origin %q, got %q", tc.allowedOrigin, got)
			}

			if tc.credentialsAllowed {
				if got := rr.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
					t.Fatalf("expected Access-Control-Allow-Credentials %q, got %q", "true", got)
				}
			} else if got := rr.Header().Get("Access-Control-Allow-Credentials"); got != "" {
				t.Fatalf("expected no Access-Control-Allow-Credentials header, got %q", got)
			}

			if tc.wantVaryOrigin {
				if got := rr.Header().Get("Vary"); got != "Origin" {
					t.Fatalf("expected Vary %q, got %q", "Origin", got)
				}
			} else if got := rr.Header().Get("Vary"); got != "" {
				t.Fatalf("expected no Vary header, got %q", got)
			}
		})
	}
}

type errResponseWriter struct {
	header http.Header
}

func (w *errResponseWriter) Header() http.Header {
	if w.header == nil {
		w.header = make(http.Header)
	}
	return w.header
}

func (w *errResponseWriter) WriteHeader(statusCode int) {}

func (w *errResponseWriter) Write([]byte) (int, error) {
	return 0, errors.New("write failed")
}

func TestWriteJSONError_MutexResponseWriter_LogsOnEncodeFailure(t *testing.T) {
	core, logs := observer.New(zap.ErrorLevel)
	logger := zap.New(core)
	r := NewRouter[string, string](RouterConfig{Logger: logger, TraceIDBufferSize: 1}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	req := httptest.NewRequest(http.MethodGet, "http://example.com/test", nil)

	var mu sync.Mutex
	mrw := &mutexResponseWriter{ResponseWriter: &errResponseWriter{}, mu: &mu}

	r.writeJSONError(mrw, req, http.StatusInternalServerError, "Internal Server Error", "trace-123")

	entries := logs.All()
	if len(entries) != 1 {
		t.Fatalf("expected 1 log entry, got %d", len(entries))
	}
	if entries[0].Message != "Failed to write JSON error response" {
		t.Fatalf("expected log message %q, got %q", "Failed to write JSON error response", entries[0].Message)
	}

	var foundStatus, foundMessage, foundTrace bool
	for _, f := range entries[0].Context {
		switch f.Key {
		case "original_status":
			foundStatus = f.Integer == int64(http.StatusInternalServerError)
		case "original_message":
			foundMessage = f.String == "Internal Server Error"
		case "trace_id":
			foundTrace = f.String == "trace-123"
		}
	}

	if !foundStatus {
		t.Fatalf("expected original_status field to be present")
	}
	if !foundMessage {
		t.Fatalf("expected original_message field to be present")
	}
	if !foundTrace {
		t.Fatalf("expected trace_id field to be present")
	}
}
