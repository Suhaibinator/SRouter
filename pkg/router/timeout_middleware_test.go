package router

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/router/internal/mocks"
	"go.uber.org/zap"
)

func parseJSONErrorMessage(t *testing.T, body []byte) string {
	t.Helper()

	var payload struct {
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatalf("expected JSON error payload, got %q: %v", string(body), err)
	}
	return payload.Error.Message
}

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
	if msg := parseJSONErrorMessage(t, rr.Body.Bytes()); msg != "Internal Server Error" {
		t.Fatalf("expected internal server error payload, got %q", msg)
	}
}
