package router

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/Suhaibinator/SRouter/pkg/router/internal/mocks"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func observedErrorRouter() (*Router[string, string], *observer.ObservedLogs) {
	core, logs := observer.New(zap.DebugLevel)
	r := NewRouter(RouterConfig{Logger: zap.New(core)}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)
	return r, logs
}

func TestHandleErrorClassifiesFinalOutcome(t *testing.T) {
	tests := []struct {
		name       string
		err        error
		statusCode int
		message    string
		wantStatus int
		wantLevel  zapcore.Level
	}{
		{name: "bad request", err: NewHTTPError(400, "bad request"), statusCode: 500, message: "internal", wantStatus: 400, wantLevel: zapcore.InfoLevel},
		{name: "unauthorized", err: NewHTTPError(401, "unauthorized"), statusCode: 500, message: "internal", wantStatus: 401, wantLevel: zapcore.InfoLevel},
		{name: "forbidden", err: NewHTTPError(403, "forbidden"), statusCode: 500, message: "internal", wantStatus: 403, wantLevel: zapcore.InfoLevel},
		{name: "not found", err: NewHTTPError(404, "not found"), statusCode: 500, message: "internal", wantStatus: 404, wantLevel: zapcore.InfoLevel},
		{name: "conflict", err: NewHTTPError(409, "conflict"), statusCode: 500, message: "internal", wantStatus: 409, wantLevel: zapcore.InfoLevel},
		{name: "payload too large", err: NewHTTPError(413, "too large"), statusCode: 500, message: "internal", wantStatus: 413, wantLevel: zapcore.InfoLevel},
		{name: "rate limited", err: NewHTTPError(429, "rate limited"), statusCode: 500, message: "internal", wantStatus: 429, wantLevel: zapcore.InfoLevel},
		{name: "untyped client error", err: errors.New("decode failed"), statusCode: 400, message: "bad request", wantStatus: 400, wantLevel: zapcore.InfoLevel},
		{name: "max bytes error", err: &http.MaxBytesError{Limit: 12}, statusCode: 500, message: "internal", wantStatus: 413, wantLevel: zapcore.InfoLevel},
		{name: "deadline", err: fmt.Errorf("wrapped: %w", context.DeadlineExceeded), statusCode: 500, message: "internal", wantStatus: 408, wantLevel: zapcore.WarnLevel},
		{name: "canceled", err: fmt.Errorf("wrapped: %w", context.Canceled), statusCode: 500, message: "internal", wantStatus: 500, wantLevel: zapcore.DebugLevel},
		{name: "server error", err: errors.New("database failed"), statusCode: 500, message: "internal", wantStatus: 500, wantLevel: zapcore.ErrorLevel},
		{name: "invalid low status", err: NewHTTPError(200, "not an error status"), statusCode: 500, message: "internal", wantStatus: 500, wantLevel: zapcore.ErrorLevel},
		{name: "invalid high status", err: NewHTTPError(600, "invalid"), statusCode: 500, message: "internal", wantStatus: 500, wantLevel: zapcore.ErrorLevel},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r, logs := observedErrorRouter()
			req := httptest.NewRequest(http.MethodPost, "/resource", nil)
			rr := httptest.NewRecorder()

			r.handleError(rr, req, tt.err, tt.statusCode, tt.message)

			if rr.Code != tt.wantStatus {
				t.Fatalf("response status = %d, want %d", rr.Code, tt.wantStatus)
			}
			entries := logs.AllUntimed()
			if len(entries) != 1 {
				t.Fatalf("log entries = %d, want 1", len(entries))
			}
			if entries[0].Level != tt.wantLevel {
				t.Errorf("log level = %s, want %s", entries[0].Level, tt.wantLevel)
			}
			fields := entries[0].ContextMap()
			if fields["status_code"] != int64(tt.wantStatus) {
				t.Errorf("status_code = %#v, want %d", fields["status_code"], tt.wantStatus)
			}
			if fields["method"] != http.MethodPost || fields["path"] != "/resource" {
				t.Errorf("request fields = %#v", fields)
			}
			if traceID, ok := fields["trace_id"].(string); !ok || traceID == "" {
				t.Errorf("trace_id = %#v, want non-empty string", fields["trace_id"])
			}
		})
	}
}

func TestHTTPErrorCauseFieldsOverrideAndNonDisclosure(t *testing.T) {
	cause := errors.New("database connection secret")
	err := NewHTTPErrorWithCause(http.StatusConflict, "email already registered", cause).
		WithFields(
			zap.String("email", "inner@example.com"),
			zap.String("method", "SPOOFED"),
			zap.String("path", "/spoofed"),
			zap.Int("status_code", 299),
			zap.String("trace_id", "spoofed-trace"),
			zap.String("error", "spoofed-error"),
		).
		WithFields(
			zap.String("email", "outer@example.com"),
			zap.Uint64("user_id", 42),
		).
		WithLogLevel(zapcore.ErrorLevel)

	if !errors.Is(err, cause) {
		t.Fatal("HTTPError did not preserve errors.Is through Unwrap")
	}
	if err.Cause() != cause {
		t.Fatal("Cause did not return the original error")
	}

	r, logs := observedErrorRouter()
	req := httptest.NewRequest(http.MethodPut, "/accounts/42", nil)
	rr := httptest.NewRecorder()
	r.handleError(rr, req, err, http.StatusInternalServerError, "internal")

	entries := logs.AllUntimed()
	if len(entries) != 1 {
		t.Fatalf("log entries = %d, want 1", len(entries))
	}
	entry := entries[0]
	if entry.Level != zapcore.ErrorLevel {
		t.Errorf("log level = %s, want error override", entry.Level)
	}
	fields := entry.ContextMap()
	wants := map[string]any{
		"email":       "outer@example.com",
		"user_id":     uint64(42),
		"method":      http.MethodPut,
		"path":        "/accounts/42",
		"status_code": int64(http.StatusConflict),
		"error":       cause.Error(),
	}
	for key, want := range wants {
		if got := fields[key]; got != want {
			t.Errorf("%s = %#v, want %#v", key, got, want)
		}
	}
	if fields["trace_id"] == "spoofed-trace" {
		t.Error("attached trace_id overrode boundary trace_id")
	}
	for _, key := range []string{"email", "method", "path", "status_code", "trace_id", "error"} {
		count := 0
		for _, field := range entry.Context {
			if field.Key == key {
				count++
			}
		}
		if count != 1 {
			t.Errorf("field %q appeared %d times, want once", key, count)
		}
	}

	body := rr.Body.String()
	for _, secret := range []string{cause.Error(), "outer@example.com", "42"} {
		if strings.Contains(body, secret) {
			t.Errorf("response disclosed diagnostic value %q: %s", secret, body)
		}
	}
	if !strings.Contains(body, "email already registered") {
		t.Errorf("response omitted public message: %s", body)
	}
}

func TestHTTPErrorFieldSnapshotsAreIndependent(t *testing.T) {
	input := []zap.Field{zap.String("email", "first@example.com")}
	base := NewHTTPErrorWithCause(http.StatusBadRequest, "bad request", errors.New("cause"))
	withFields := base.WithFields(input...)
	input[0] = zap.String("email", "mutated@example.com")

	fields := withFields.Fields()
	if len(fields) != 1 || fields[0].String != "first@example.com" {
		t.Fatalf("stored fields changed with input: %#v", fields)
	}
	fields[0] = zap.String("email", "returned-slice@example.com")
	if got := withFields.Fields()[0].String; got != "first@example.com" {
		t.Fatalf("stored fields changed with returned slice: %q", got)
	}
	if len(base.Fields()) != 0 {
		t.Fatal("WithFields mutated its receiver")
	}

	const goroutines = 64
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := range goroutines {
		go func() {
			defer wg.Done()
			derived := withFields.WithFields(zap.Int("attempt", i)).WithLogLevel(zapcore.WarnLevel)
			if level, ok := derived.LogLevel(); !ok || level != zapcore.WarnLevel {
				t.Errorf("derived level = %s, %v", level, ok)
			}
			_ = derived.Fields()
		}()
	}
	wg.Wait()

	if _, ok := withFields.LogLevel(); ok {
		t.Fatal("WithLogLevel mutated its receiver")
	}
}
