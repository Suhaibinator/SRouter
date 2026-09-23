package router

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/Suhaibinator/SRouter/pkg/scontext"
	"github.com/Suhaibinator/SRouter/pkg/traceid"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestTraceResolution(t *testing.T) {
	for _, tc := range []struct {
		name, contextID, candidate, want string
		present                          bool
		validator                        traceid.Validator
		calls                            int
	}{
		{name: "context wins", contextID: "context", candidate: "source", present: true, want: "context"},
		{name: "source", candidate: "source", present: true, want: "source", calls: 1},
		{name: "invalid context replaced", contextID: "bad id", candidate: "source", present: true, want: "source", calls: 1},
		{name: "absent", candidate: "ignored", calls: 1},
		{name: "empty", present: true, calls: 1},
		{name: "default rejects punctuation", candidate: "a.b", present: true, calls: 1},
		{name: "custom accepts punctuation", candidate: "a.b", present: true, validator: func(s string) bool { return s == "a.b" }, want: "a.b", calls: 1},
		{name: "custom rejects context", contextID: "context", candidate: "a.b", present: true, validator: func(s string) bool { return s == "a.b" }, want: "a.b", calls: 1},
		{name: "custom rejects all", candidate: "source", present: true, validator: func(string) bool { return false }, calls: 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			calls := 0
			core, logs := observer.New(zap.DebugLevel)
			config := &TraceIDConfig{Validator: tc.validator, ResponseHeader: "X-Canonical", Source: func(req *http.Request) (string, bool) {
				calls++
				if _, ok := scontext.GetLogger(req.Context()); !ok {
					t.Error("logger not initialized")
				}
				return tc.candidate, tc.present
			}}
			r := NewRouter(RouterConfig{Logger: zap.New(core), TraceIDConfig: config}, RouterDependencies[string, string]{})
			t.Cleanup(func() { _ = r.Shutdown(context.Background()) })
			var got string
			r.Route(RouteConfigBase{Path: "/", Methods: []HttpMethod{MethodGet}, Handler: func(w http.ResponseWriter, req *http.Request) {
				got = scontext.GetTraceID(req.Context())
				logger, _ := scontext.GetLogger(req.Context())
				logger.Info("handler")
				r.handleError(w, req, errors.New("test error"), http.StatusBadRequest, "bad request")
			}})
			req := httptest.NewRequest("GET", "/", nil)
			// Also exercise explicitly set empty IDs and previously cached loggers.
			ctx := scontext.SetTraceID[string, string](req.Context(), tc.contextID)
			ctx = scontext.WithRequestLogger[string, string](ctx, scontext.NewRequestLoggerSource[string](zap.New(core), nil))
			_, _ = scontext.GetLogger(ctx)
			req = req.WithContext(ctx)
			req.Header.Set(traceid.HeaderXTraceID, "untouched-inbound")
			rr := httptest.NewRecorder()
			rr.Header().Set(traceid.HeaderXTraceID, "untouched-response")
			r.ServeHTTP(rr, req)
			if tc.want != "" && got != tc.want {
				t.Fatalf("ID = %q, want %q", got, tc.want)
			}
			if tc.want == "" && (len(got) != 32 || !traceid.IsValid(got) || got[12] != '7') {
				t.Fatalf("fallback = %q", got)
			}
			if calls != tc.calls {
				t.Errorf("source calls = %d, want %d", calls, tc.calls)
			}
			if rr.Header().Get("X-Canonical") != got || !strings.Contains(rr.Body.String(), `"trace_id":"`+got+`"`) {
				t.Fatalf("inconsistent response: %v %s", rr.Header(), rr.Body)
			}
			if req.Header.Get(traceid.HeaderXTraceID) != "untouched-inbound" || rr.Header().Get(traceid.HeaderXTraceID) != "untouched-response" {
				t.Fatal("noncanonical header changed")
			}
			if logs.Len() != 3 {
				t.Fatalf("logs = %d, want handler/error/summary", logs.Len())
			}
			for _, entry := range logs.All() {
				if entry.ContextMap()["trace_id"] != got {
					t.Errorf("%s trace = %v, want %s", entry.Message, entry.ContextMap()["trace_id"], got)
				}
			}
		})
	}
}

func TestTraceTransportSafety(t *testing.T) {
	for _, unsafe := range []string{"", strings.Repeat("a", 65), "a b", "a\t", "a\r", "a\n", "a\x00", "a\x1f", "a\x7f", "a\u0085", "a\u00a0", "a\u2003", "a\u2028", "a\xff"} {
		t.Run(fmt.Sprintf("%q", unsafe), func(t *testing.T) {
			calls := 0
			r := NewRouter(RouterConfig{Logger: zap.NewNop(), TraceIDConfig: &TraceIDConfig{
				Source:    func(*http.Request) (string, bool) { return unsafe, true },
				Validator: func(string) bool { calls++; return true },
			}}, RouterDependencies[string, string]{})
			defer func() { _ = r.Shutdown(context.Background()) }()
			req := httptest.NewRequest("GET", "/", nil)
			req = req.WithContext(scontext.SetTraceID[string, string](req.Context(), unsafe))
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)
			id := rr.Header().Get(traceid.HeaderXTraceID)
			if id == unsafe || !traceid.IsValid(id) || len(id) != 32 {
				t.Fatalf("unsafe fallback = %q", id)
			}
			if calls != 0 {
				t.Errorf("validator called %d times on unsafe IDs", calls)
			}
		})
	}
	for _, safe := range []string{strings.Repeat("x", 64), "a.b/c:1", "café"} {
		if !safeTraceID(safe) {
			t.Errorf("safe ID rejected: %q", safe)
		}
	}
}

func TestTraceBoundaryOutcomes(t *testing.T) {
	for _, tc := range []struct {
		name, method, path string
		status             int
	}{
		{"matched", "GET", "/", 200}, {"not found", "GET", "/missing", 404},
		{"method not allowed", "POST", "/", 405}, {"CORS", "OPTIONS", "/", 204},
		{"CORS rejected", "OPTIONS", "/", 204}, {"shutdown", "GET", "/", 503},
		{"build failure", "GET", "/", 500}, {"negative buffer", "GET", "/", 500},
	} {
		for _, size := range []int{0, 4} {
			t.Run(fmt.Sprintf("%s/%d", tc.name, size), func(t *testing.T) {
				core, logs := observer.New(zap.DebugLevel)
				config := RouterConfig{Logger: zap.New(core), TraceIDConfig: &TraceIDConfig{BufferSize: size}}
				if strings.HasPrefix(tc.name, "CORS") {
					config.CORSConfig = &CORSConfig{Origins: []string{"https://example.com"}}
				}
				if tc.name == "build failure" {
					config.GlobalTimeout = -1
				}
				if tc.name == "negative buffer" {
					config.TraceIDConfig.BufferSize = -1
				}
				r := NewRouter(config, RouterDependencies[string, string]{})
				defer func() { _ = r.Shutdown(context.Background()) }()
				r.Route(RouteConfigBase{Path: "/", Methods: []HttpMethod{MethodGet}, Handler: func(w http.ResponseWriter, req *http.Request) {
					if id := scontext.GetTraceID(req.Context()); id == "" || id != w.Header().Get(traceid.HeaderXTraceID) {
						t.Error("handler missing trace")
					}
					w.WriteHeader(200)
				}})
				if tc.name == "shutdown" {
					_ = r.Shutdown(context.Background())
				}
				req := httptest.NewRequest(tc.method, tc.path, nil)
				req.Header.Set(traceid.HeaderXTraceID, "upstream")
				req.Header.Set("Origin", "https://example.com")
				req.Header.Set("Access-Control-Request-Method", "GET")
				if tc.name == "CORS rejected" {
					req.Header.Set("Origin", "https://other.example")
				}
				rr := httptest.NewRecorder()
				r.ServeHTTP(rr, req)
				if rr.Code != tc.status || rr.Header().Get(traceid.HeaderXTraceID) != "upstream" {
					t.Fatalf("response = %d %v", rr.Code, rr.Header())
				}
				summaries := logs.FilterMessage("Request summary statistics").All()
				if len(summaries) != 1 || summaries[0].ContextMap()["status"] != int64(tc.status) {
					t.Fatalf("summaries = %v", summaries)
				}
				for _, entry := range logs.All() {
					if entry.ContextMap()["trace_id"] != "upstream" {
						t.Errorf("%s missing trace", entry.Message)
					}
				}
				if tc.status == 500 && r.Build() == nil {
					t.Error("Build accepted invalid config")
				}
				// The fallback must also work on every early return, even after Stop.
				req.Header.Del(traceid.HeaderXTraceID)
				rr = httptest.NewRecorder()
				r.ServeHTTP(rr, req)
				if id := rr.Header().Get(traceid.HeaderXTraceID); len(id) != 32 || !traceid.IsValid(id) {
					t.Errorf("missing generated fallback: %q", id)
				}
			})
		}
	}
}

func TestTraceSourcesAndHeaderPreservation(t *testing.T) {
	const parent = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	for _, tc := range []struct {
		header, value, want string
		source              traceid.Source
	}{
		{traceid.HeaderXRequestID, "request-id", "request-id", traceid.FromHeader(traceid.HeaderXRequestID)},
		{traceid.HeaderTraceparent, parent, parent[3:35], traceid.FromTraceparent},
	} {
		t.Run(tc.header, func(t *testing.T) {
			r := NewRouter(RouterConfig{Logger: zap.NewNop(), TraceIDConfig: &TraceIDConfig{Source: tc.source}}, RouterDependencies[string, string]{})
			defer func() { _ = r.Shutdown(context.Background()) }()
			req := httptest.NewRequest("GET", "/", nil)
			req.Header.Set(tc.header, tc.value)
			req.Header.Set(traceid.HeaderXTraceID, "ignored-default")
			rr := httptest.NewRecorder()
			rr.Header().Set(tc.header, "preserve-response")
			r.ServeHTTP(rr, req)
			if rr.Header().Get(traceid.HeaderXTraceID) != tc.want || rr.Header().Get(tc.header) != "preserve-response" || req.Header.Get(tc.header) != tc.value {
				t.Fatalf("headers = %v / %v", req.Header, rr.Header())
			}
		})
	}
}

func TestTraceDisabledPreservesContextWithoutResponse(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)
	r := NewRouter(RouterConfig{Logger: zap.New(core), EnableTraceLogging: true}, RouterDependencies[string, string]{})
	r.Route(RouteConfigBase{Path: "/", Methods: []HttpMethod{MethodGet}, Handler: func(w http.ResponseWriter, req *http.Request) {
		if got := scontext.GetTraceID(req.Context()); got != "existing" {
			t.Errorf("context = %q", got)
		}
		r.handleError(w, req, errors.New("test"), 400, "bad request")
	}})
	req := httptest.NewRequest("GET", "/", nil)
	req = req.WithContext(scontext.WithTraceID[string, string](req.Context(), "existing"))
	req.Header.Set(traceid.HeaderXTraceID, "ignored")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Header().Get(traceid.HeaderXTraceID) != "" || strings.Contains(rr.Body.String(), "trace_id") {
		t.Fatal("disabled tracing modified response")
	}
	for _, entry := range logs.All() {
		if entry.ContextMap()["trace_id"] != "existing" {
			t.Error("existing trace omitted")
		}
	}
}

func TestTraceConfigSnapshotAndConcurrentResolution(t *testing.T) {
	for _, size := range []int{0, 8} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			var calls atomic.Int64
			config := &TraceIDConfig{BufferSize: size, Source: func(*http.Request) (string, bool) { calls.Add(1); return "", false }}
			r := NewRouter(RouterConfig{Logger: zap.NewNop(), TraceIDConfig: config}, RouterDependencies[string, string]{})
			defer func() { _ = r.Shutdown(context.Background()) }()
			config.BufferSize = -1
			config.ResponseHeader = "X-Changed"
			var wg sync.WaitGroup
			var ids sync.Map
			for range 32 {
				wg.Go(func() {
					rr := httptest.NewRecorder()
					r.ServeHTTP(rr, httptest.NewRequest("GET", "/", nil))
					id := rr.Header().Get(traceid.HeaderXTraceID)
					if rr.Code != 404 || !traceid.IsValid(id) {
						t.Errorf("response = %d, %q", rr.Code, id)
					}
					if _, exists := ids.LoadOrStore(id, true); exists {
						t.Errorf("duplicate ID %q", id)
					}
				})
			}
			wg.Wait()
			if calls.Load() != 32 {
				t.Errorf("source calls = %d", calls.Load())
			}
		})
	}
}
