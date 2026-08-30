package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/router/internal/mocks"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestTimeoutMiddleware_WhenHandlerStartedWriting_DoesNotOverrideResponse(t *testing.T) {
	r := NewRouter(RouterConfig{Logger: zap.NewNop()}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	timeout := 25 * time.Millisecond
	wroteHeader := make(chan struct{})
	ctxErrCh := make(chan error, 1)

	handler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusCreated)
		close(wroteHeader)

		<-req.Context().Done()
		ctxErrCh <- req.Context().Err()
		time.Sleep(10 * time.Millisecond)

		_, _ = w.Write([]byte("handler-finished"))
	})

	h := r.recoveryMiddleware(r.timeoutMiddleware(timeout)(handler))

	req := httptest.NewRequest(http.MethodGet, "http://example.com/test", nil)
	rr := httptest.NewRecorder()

	select {
	case <-wroteHeader:
		t.Fatalf("handler should not have executed before ServeHTTP")
	default:
	}

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusCreated {
		t.Fatalf("expected status %d, got %d", http.StatusCreated, rr.Code)
	}
	if rr.Body.String() != "handler-finished" {
		t.Fatalf("expected body %q, got %q", "handler-finished", rr.Body.String())
	}

	select {
	case err := <-ctxErrCh:
		if err != context.DeadlineExceeded {
			t.Fatalf("expected context deadline exceeded, got %v", err)
		}
	default:
		t.Fatalf("expected handler to observe context cancellation")
	}
}

func TestTimeoutMiddleware_WhenHandlerPanicsAfterTimeoutAndStartedWrite_RethrowsToRecovery(t *testing.T) {
	r := NewRouter(RouterConfig{Logger: zap.NewNop()}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	timeout := 15 * time.Millisecond
	handler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusTeapot)
		<-req.Context().Done()
		time.Sleep(10 * time.Millisecond)
		panic("boom")
	})

	h := r.recoveryMiddleware(r.timeoutMiddleware(timeout)(handler))

	req := httptest.NewRequest(http.MethodGet, "http://example.com/test", nil)
	rr := httptest.NewRecorder()

	h.ServeHTTP(rr, req)

	if rr.Code != http.StatusTeapot {
		t.Fatalf("expected status %d, got %d", http.StatusTeapot, rr.Code)
	}
	// The handler already started writing before it panicked, so recovery must
	// not append a second (JSON error) response onto the partial one.
	if body := rr.Body.String(); body != "" {
		t.Fatalf("expected no additional body after mid-response panic, got %q", body)
	}
}

func TestTimeoutMiddleware_LogsStructuredWarning(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)
	r := NewRouter(RouterConfig{Logger: zap.New(core)}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)
	handler := r.timeoutMiddleware(5 * time.Millisecond)(http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
		<-req.Context().Done()
	}))
	req := httptest.NewRequest(http.MethodPut, "http://example.com/slow", nil)
	req.RemoteAddr = "192.0.2.44:9090"
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusRequestTimeout {
		t.Fatalf("response status = %d, want %d", rr.Code, http.StatusRequestTimeout)
	}
	entries := logs.FilterMessage("Request timed out").AllUntimed()
	if len(entries) != 1 {
		t.Fatalf("timeout log entries = %d, want 1: %#v", len(entries), logs.AllUntimed())
	}
	entry := entries[0]
	if entry.Level != zapcore.WarnLevel {
		t.Errorf("timeout log level = %s, want warn", entry.Level)
	}
	fields := entry.ContextMap()
	wants := map[string]any{
		"method":      http.MethodPut,
		"path":        "/slow",
		"client_ip":   "192.0.2.44:9090",
		"status_code": int64(http.StatusRequestTimeout),
	}
	for key, want := range wants {
		if got := fields[key]; got != want {
			t.Errorf("%s = %#v, want %#v", key, got, want)
		}
	}
	if traceID, ok := fields["trace_id"].(string); !ok || traceID == "" {
		t.Errorf("trace_id = %#v, want generated non-empty string", fields["trace_id"])
	}
}
