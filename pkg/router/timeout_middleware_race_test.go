//go:build race

package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/router/internal/mocks"
	"go.uber.org/zap"
)

func TestTimeoutMiddleware_WhenHandlerWritesBetweenHeaderCheckAndTimeoutStore_TakeoverCASFails(t *testing.T) {
	oldProcs := runtime.GOMAXPROCS(2)
	defer runtime.GOMAXPROCS(oldProcs)

	r := NewRouter(RouterConfig{Logger: zap.NewNop()}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)
	timeout := 50 * time.Microsecond

	deadline := time.Now().Add(5 * time.Second)
	attempts := 0

	for time.Now().Before(deadline) {
		attempts++

		mrwCh := make(chan *mutexResponseWriter, 1)
		ctxErrCh := make(chan error, 1)

		handler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if mrw, ok := w.(*mutexResponseWriter); ok {
				select {
				case mrwCh <- mrw:
				default:
				}
			}

			<-req.Context().Done()
			ctxErrCh <- req.Context().Err()

			w.WriteHeader(http.StatusAccepted)
		})

		h := r.timeoutMiddleware(timeout)(handler)

		req := httptest.NewRequest(http.MethodGet, "http://example.com/test", nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)

		mrw := <-mrwCh
		if rr.Code == http.StatusAccepted && mrw.timedOut.Load() {
			select {
			case err := <-ctxErrCh:
				if err != context.DeadlineExceeded {
					t.Fatalf("expected context deadline exceeded, got %v", err)
				}
			default:
				t.Fatalf("expected handler to observe context cancellation")
			}
			return
		}
	}

	t.Fatalf("did not observe timeout takeover CAS failure within deadline (attempts=%d)", attempts)
}

func TestTimeoutMiddleware_WhenHandlerPanicsInCASFailurePath_RethrowsToRecovery(t *testing.T) {
	oldProcs := runtime.GOMAXPROCS(2)
	defer runtime.GOMAXPROCS(oldProcs)

	r := NewRouter(RouterConfig{Logger: zap.NewNop()}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)
	timeout := 50 * time.Microsecond

	deadline := time.Now().Add(5 * time.Second)
	attempts := 0

	for time.Now().Before(deadline) {
		attempts++

		mrwCh := make(chan *mutexResponseWriter, 1)

		handler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if mrw, ok := w.(*mutexResponseWriter); ok {
				select {
				case mrwCh <- mrw:
				default:
				}
			}
			<-req.Context().Done()
			w.WriteHeader(http.StatusAccepted)
			panic("boom")
		})

		h := r.recoveryMiddleware(r.timeoutMiddleware(timeout)(handler))

		req := httptest.NewRequest(http.MethodGet, "http://example.com/test", nil)
		rr := httptest.NewRecorder()
		h.ServeHTTP(rr, req)

		mrw := <-mrwCh
		if rr.Code == http.StatusAccepted && mrw.timedOut.Load() {
			// The handler won the write race (202 was sent) before panicking,
			// so recovery must not append a JSON error to the started response.
			if body := rr.Body.String(); body != "" {
				t.Fatalf("expected no additional body after mid-response panic, got %q", body)
			}
			return
		}
	}

	t.Fatalf("did not observe panic rethrow in CAS-failure path within deadline (attempts=%d)", attempts)
}
