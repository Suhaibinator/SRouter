package router

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/common"
	"github.com/Suhaibinator/SRouter/pkg/router/internal/mocks"
	"github.com/Suhaibinator/SRouter/pkg/scontext"
	"github.com/julienschmidt/httprouter"
	"go.uber.org/zap"
)

func TestRouterBuildValidationErrors(t *testing.T) {
	handler := func(http.ResponseWriter, *http.Request) {}
	tests := []struct {
		name    string
		config  RouterConfig
		setup   func(*Router[string, string])
		wantErr string
	}{
		{
			name:    "global max body size",
			config:  RouterConfig{GlobalMaxBodySize: -1},
			wantErr: "global max body size must not be negative",
		},
		{
			name:    "nil global middleware",
			config:  RouterConfig{Middlewares: []common.Middleware{nil}},
			wantErr: "router contains nil middleware at index 0",
		},
		{
			name: "negative group max body size",
			setup: func(r *Router[string, string]) {
				r.Group("/api").MaxBodySize(-1)
			},
			wantErr: `route group "/api" max body size must not be negative`,
		},
		{
			name: "invalid group authentication level",
			setup: func(r *Router[string, string]) {
				r.Group("/api").Auth(AuthLevel(99))
			},
			wantErr: `route group "/api" has invalid authentication level 99`,
		},
		{
			name: "nil route",
			setup: func(r *Router[string, string]) {
				r.Route(nil)
			},
			wantErr: `route group "" contains a nil route`,
		},
		{
			name: "invalid route path",
			setup: func(r *Router[string, string]) {
				r.Route(RouteConfigBase{Path: "relative", Methods: []HttpMethod{MethodGet}, Handler: handler})
			},
			wantErr: `route path "relative" must begin with '/'`,
		},
		{
			name: "no HTTP methods",
			setup: func(r *Router[string, string]) {
				r.Route(RouteConfigBase{Path: "/no-methods", Handler: handler})
			},
			wantErr: `route "/no-methods" has no HTTP methods`,
		},
		{
			name: "negative route timeout",
			setup: func(r *Router[string, string]) {
				r.Route(RouteConfigBase{
					Path:      "/negative-timeout",
					Methods:   []HttpMethod{MethodGet},
					Overrides: common.RouteOverrides{Timeout: -time.Second},
					Handler:   handler,
				})
			},
			wantErr: `route "/negative-timeout" timeout must not be negative`,
		},
		{
			name: "negative route max body size",
			setup: func(r *Router[string, string]) {
				r.Route(RouteConfigBase{
					Path:      "/negative-body-size",
					Methods:   []HttpMethod{MethodPost},
					Overrides: common.RouteOverrides{MaxBodySize: -1},
					Handler:   handler,
				})
			},
			wantErr: `route "/negative-body-size" max body size must not be negative`,
		},
		{
			name: "invalid route authentication level",
			setup: func(r *Router[string, string]) {
				invalid := AuthLevel(99)
				r.Route(RouteConfigBase{
					Path:      "/invalid-auth",
					Methods:   []HttpMethod{MethodGet},
					AuthLevel: &invalid,
					Handler:   handler,
				})
			},
			wantErr: `route "/invalid-auth" has invalid authentication level 99`,
		},
		{
			name: "nil route middleware",
			setup: func(r *Router[string, string]) {
				r.Route(RouteConfigBase{
					Path:        "/nil-middleware",
					Methods:     []HttpMethod{MethodGet},
					Middlewares: []common.Middleware{nil},
					Handler:     handler,
				})
			},
			wantErr: `route "/nil-middleware" contains nil middleware at index 0`,
		},
		{
			name: "empty HTTP method",
			setup: func(r *Router[string, string]) {
				r.Route(RouteConfigBase{Path: "/empty-method", Methods: []HttpMethod{""}, Handler: handler})
			},
			wantErr: `route "/empty-method" contains an empty HTTP method`,
		},
		{
			name: "conflicting wildcard routes",
			setup: func(r *Router[string, string]) {
				r.Route(
					RouteConfigBase{Path: "/users/:id", Methods: []HttpMethod{MethodGet}, Handler: handler},
					RouteConfigBase{Path: "/users/:name", Methods: []HttpMethod{MethodGet}, Handler: handler},
				)
			},
			wantErr: "register GET /users/:name:",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.config.Logger = zap.NewNop()
			r := NewRouter[string, string](tt.config, RouterDependencies[string, string]{})
			if tt.setup != nil {
				tt.setup(r)
			}

			err := r.Build()
			if err == nil {
				t.Fatal("Build() succeeded, want an error")
			}
			if !strings.Contains(err.Error(), tt.wantErr) {
				t.Fatalf("Build() error = %q, want it to contain %q", err, tt.wantErr)
			}
		})
	}
}

func TestRouterServeHTTPReportsBuildError(t *testing.T) {
	r := NewRouter[string, string](RouterConfig{
		Logger:        zap.NewNop(),
		GlobalTimeout: -time.Second,
	}, RouterDependencies[string, string]{})

	for range 2 {
		recorder := httptest.NewRecorder()
		r.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
		if recorder.Code != http.StatusInternalServerError {
			t.Fatalf("ServeHTTP status = %d, want %d", recorder.Code, http.StatusInternalServerError)
		}
		if recorder.Body.String() != "Router configuration error\n" {
			t.Fatalf("ServeHTTP body = %q, want router configuration error", recorder.Body.String())
		}
	}
}

func TestRouterBuiltInMetricsRegistryMiddlewareIsApplied(t *testing.T) {
	r := NewRouter(RouterConfig{
		Logger:      zap.NewNop(),
		ServiceName: "router-edge-cases",
		MetricsConfig: &MetricsConfig{
			Collector:     &mocks.MockMetricsRegistry{},
			EnableLatency: true,
		},
	}, RouterDependencies[string, string]{Authenticate: mocks.MockAuthFunction, UserID: mocks.MockUserIDFromUser})

	r.Route(RouteConfigBase{
		Path:    "/metrics",
		Methods: []HttpMethod{MethodGet},
		Handler: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		},
	})

	recorder := httptest.NewRecorder()
	r.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("ServeHTTP status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
}

func TestRouterDirectDispatcherAddsRouteContext(t *testing.T) {
	r := NewRouter[string, string](RouterConfig{Logger: zap.NewNop()}, RouterDependencies[string, string]{})
	called := false
	handle := r.convertToHTTPRouterHandle(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		called = true
		if got := GetParam(req, "id"); got != "42" {
			t.Errorf("GetParam(id) = %q, want %q", got, "42")
		}
		if got, ok := scontext.GetRouteTemplate(req.Context()); !ok || got != "/users/:id" {
			t.Errorf("route template = (%q, %v), want (%q, true)", got, ok, "/users/:id")
		}
		w.WriteHeader(http.StatusNoContent)
	}), "/users/:id")

	recorder := httptest.NewRecorder()
	handle(recorder, httptest.NewRequest(http.MethodGet, "/users/42", nil), httprouter.Params{{Key: "id", Value: "42"}})
	if !called {
		t.Fatal("converted handler was not called")
	}
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("handler status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
}

func TestRouterBuildReturnsErrorCachedWhileWaitingForLock(t *testing.T) {
	r := NewRouter[string, string](RouterConfig{Logger: zap.NewNop()}, RouterDependencies[string, string]{})
	wantErr := errors.New("cached build error")

	previousMaxProcs := runtime.GOMAXPROCS(1)
	defer runtime.GOMAXPROCS(previousMaxProcs)

	r.routeTree.mu.Lock()
	started := make(chan struct{})
	done := make(chan error, 1)
	go func() {
		close(started)
		done <- r.Build()
	}()
	<-started
	// With one active P, yielding lets Build pass its initial ready check and
	// block on routeTree.mu before this goroutine resumes.
	runtime.Gosched()
	r.routeTree.buildErr = wantErr
	r.routeTree.ready.Store(true)
	r.routeTree.mu.Unlock()

	if err := <-done; !errors.Is(err, wantErr) {
		t.Fatalf("Build() error = %v, want cached error %v", err, wantErr)
	}
}

func TestRouterBuildRejectsAuthWithoutUserIDFunction(t *testing.T) {
	auth := AuthRequired
	r := NewRouter[string, string](
		RouterConfig{Logger: zap.NewNop()},
		RouterDependencies[string, string]{
			Authenticate: func(context.Context, string) (*string, bool) { return nil, false },
		},
	)
	r.Route(RouteConfigBase{
		Path:      "/authenticated",
		Methods:   []HttpMethod{MethodGet},
		AuthLevel: &auth,
		Handler:   func(http.ResponseWriter, *http.Request) {},
	})

	err := r.Build()
	if err == nil || !strings.Contains(err.Error(), "without a user ID function") {
		t.Fatalf("Build() error = %v, want missing user ID function error", err)
	}
}

func TestRouterNonPositiveTimeoutCallsHandlerDirectly(t *testing.T) {
	r := NewRouter[string, string](RouterConfig{Logger: zap.NewNop()}, RouterDependencies[string, string]{})
	called := false
	handler := r.timeoutMiddleware(0)(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		called = true
		w.WriteHeader(http.StatusNoContent)
	}))

	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/", nil))
	if !called {
		t.Fatal("handler was not called for a disabled timeout")
	}
	if recorder.Code != http.StatusNoContent {
		t.Fatalf("handler status = %d, want %d", recorder.Code, http.StatusNoContent)
	}
}

func TestRouterTimeoutPropagatesLateHandlerPanic(t *testing.T) {
	r := NewRouter[string, string](RouterConfig{Logger: zap.NewNop()}, RouterDependencies[string, string]{})
	handler := r.timeoutMiddleware(time.Millisecond)(http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
		<-req.Context().Done()
		time.Sleep(10 * time.Millisecond)
		panic("late timeout panic")
	}))

	defer func() {
		if recovered := recover(); recovered != "late timeout panic" {
			t.Fatalf("recovered panic = %v, want %q", recovered, "late timeout panic")
		}
	}()
	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/timeout", nil))
}

func TestRouterShutdownReturnsCanceledContext(t *testing.T) {
	r := NewRouter[string, string](RouterConfig{Logger: zap.NewNop()}, RouterDependencies[string, string]{})
	r.wg.Add(1)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := make(chan error, 1)
	go func() {
		done <- r.Shutdown(ctx)
	}()
	for {
		r.shutdownMu.RLock()
		shuttingDown := r.shutdown
		r.shutdownMu.RUnlock()
		if shuttingDown {
			break
		}
		runtime.Gosched()
	}
	cancel()
	err := <-done
	r.wg.Done()
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("Shutdown() error = %v, want %v", err, context.Canceled)
	}
}

func TestRouterWriteJSONErrorLogsPlainWriterFailure(t *testing.T) {
	r := NewRouter[string, string](RouterConfig{Logger: zap.NewNop()}, RouterDependencies[string, string]{})
	r.writeJSONError(
		&errResponseWriter{},
		httptest.NewRequest(http.MethodGet, "/error", nil),
		http.StatusInternalServerError,
		"Internal Server Error",
		"trace-123",
	)
}

func TestRouterTimedOutWriterDoesNotFlush(t *testing.T) {
	underlying := &flushCountingResponseWriter{header: make(http.Header)}
	writer := &mutexResponseWriter{ResponseWriter: underlying, mu: &sync.Mutex{}}
	writer.timedOut.Store(true)

	writer.Flush()
	if underlying.flushes != 0 {
		t.Fatalf("underlying flush count = %d, want 0", underlying.flushes)
	}
}

type flushCountingResponseWriter struct {
	header  http.Header
	flushes int
}

func (w *flushCountingResponseWriter) Header() http.Header { return w.header }
func (w *flushCountingResponseWriter) WriteHeader(int)     {}
func (w *flushCountingResponseWriter) Write(p []byte) (int, error) {
	return len(p), nil
}
func (w *flushCountingResponseWriter) Flush() { w.flushes++ }
