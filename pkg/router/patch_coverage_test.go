package router

import (
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/common"
	"github.com/Suhaibinator/SRouter/pkg/metrics"
	"github.com/Suhaibinator/SRouter/pkg/router/internal/mocks"
)

// fakeMetricsMiddlewareFactory implements metrics.MetricsMiddleware[string, string]
// so tests can verify that a user-supplied MiddlewareFactory takes precedence
// over building middleware from the Collector.
type fakeMetricsMiddlewareFactory struct {
	mu           sync.Mutex
	handlerNames []string
	requests     int
}

func (f *fakeMetricsMiddlewareFactory) Handler(name string, handler http.Handler) http.Handler {
	f.mu.Lock()
	f.handlerNames = append(f.handlerNames, name)
	f.mu.Unlock()
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		f.mu.Lock()
		f.requests++
		f.mu.Unlock()
		w.Header().Set("X-Metrics-Factory", "invoked")
		handler.ServeHTTP(w, r)
	})
}

func (f *fakeMetricsMiddlewareFactory) Configure(config metrics.MetricsMiddlewareConfig) metrics.MetricsMiddleware[string, string] {
	return f
}

func (f *fakeMetricsMiddlewareFactory) WithFilter(filter metrics.MetricsFilter) metrics.MetricsMiddleware[string, string] {
	return f
}

func (f *fakeMetricsMiddlewareFactory) WithSampler(sampler metrics.MetricsSampler) metrics.MetricsMiddleware[string, string] {
	return f
}

// TestMetricsConfigMiddlewareFactory verifies that when MetricsConfig supplies
// a MiddlewareFactory of the router's generic type, the router wraps handlers
// with it (passing the configured ServiceName) and requests flow through it.
func TestMetricsConfigMiddlewareFactory(t *testing.T) {
	factory := &fakeMetricsMiddlewareFactory{}

	r := NewRouter(RouterConfig{
		ServiceName: "test-service",
		MetricsConfig: &MetricsConfig{
			MiddlewareFactory: factory,
		},
	}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	r.RegisterRoute(RouteConfigBase{
		Path:    "/factory",
		Methods: []HttpMethod{MethodGet},
		Handler: func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		},
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest("GET", "/factory", nil))

	if rec.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusOK)
	}
	if rec.Header().Get("X-Metrics-Factory") != "invoked" {
		t.Error("request did not pass through the factory-provided metrics middleware")
	}

	factory.mu.Lock()
	defer factory.mu.Unlock()
	if factory.requests != 1 {
		t.Errorf("factory middleware handled %d requests, want 1", factory.requests)
	}
	if len(factory.handlerNames) == 0 {
		t.Fatal("factory Handler was never called when wrapping routes")
	}
	for _, name := range factory.handlerNames {
		if name != "test-service" {
			t.Errorf("factory Handler called with name %q, want configured ServiceName %q", name, "test-service")
		}
	}
}

// TestGetEffectiveRateLimitConvertsUserIDFunctions verifies that converting a
// RateLimitConfig[any, any] override to the router's concrete types adapts
// UserIDFromUser and UserIDToString so user-based rate limiting keeps working.
func TestGetEffectiveRateLimitConvertsUserIDFunctions(t *testing.T) {
	r := NewRouter(RouterConfig{}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	src := &common.RateLimitConfig[any, any]{
		BucketName: "user-bucket",
		Limit:      7,
		Window:     time.Minute,
		Strategy:   common.StrategyUser,
		UserIDFromUser: func(user any) any {
			return "id-" + user.(string)
		},
		UserIDToString: func(userID any) string {
			return "key:" + userID.(string)
		},
	}

	got := r.getEffectiveRateLimit(src, nil)
	if got == nil {
		t.Fatal("expected a converted rate limit config, got nil")
	}
	if got.BucketName != "user-bucket" || got.Limit != 7 || got.Window != time.Minute || got.Strategy != common.StrategyUser {
		t.Errorf("converted config lost fields: %+v", got)
	}
	if got.UserIDFromUser == nil {
		t.Fatal("UserIDFromUser was not adapted across the type conversion")
	}
	if id := got.UserIDFromUser("alice"); id != "id-alice" {
		t.Errorf("UserIDFromUser(\"alice\") = %q, want %q", id, "id-alice")
	}
	if got.UserIDToString == nil {
		t.Fatal("UserIDToString was not adapted across the type conversion")
	}
	if key := got.UserIDToString("bob"); key != "key:bob" {
		t.Errorf("UserIDToString(\"bob\") = %q, want %q", key, "key:bob")
	}

	// A UserIDFromUser returning a value of the wrong type must degrade to the
	// zero user ID instead of panicking. Passed as the sub-router override to
	// exercise that precedence level too.
	wrongType := &common.RateLimitConfig[any, any]{
		Limit:  1,
		Window: time.Second,
		UserIDFromUser: func(user any) any {
			return 42 // not a string
		},
	}
	converted := r.getEffectiveRateLimit(nil, wrongType)
	if converted == nil || converted.UserIDFromUser == nil {
		t.Fatal("expected converted sub-router config with adapted UserIDFromUser")
	}
	if id := converted.UserIDFromUser("alice"); id != "" {
		t.Errorf("UserIDFromUser with mismatched return type = %q, want zero value", id)
	}
}

// TestExtractIPFromXForwardedForBlankEntries verifies that an X-Forwarded-For
// header containing only blank entries (commas and whitespace) yields no IP,
// so the caller falls back to RemoteAddr instead of using an empty key.
func TestExtractIPFromXForwardedForBlankEntries(t *testing.T) {
	for _, xff := range []string{" ", ",", " , ", ",,  ,"} {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("X-Forwarded-For", xff)
		if ip := extractIPFromXForwardedFor(req); ip != "" {
			t.Errorf("X-Forwarded-For %q: got %q, want empty string", xff, ip)
		}
	}

	// End to end: with a blank XFF the extracted client IP must fall back to
	// RemoteAddr even when proxy headers are trusted.
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "203.0.113.9:1234"
	req.Header.Set("X-Forwarded-For", " , ")
	ip := extractClientIP(req, &IPConfig{Source: IPSourceXForwardedFor, TrustProxy: true})
	if ip != "203.0.113.9" {
		t.Errorf("extractClientIP with blank XFF = %q, want RemoteAddr fallback %q", ip, "203.0.113.9")
	}
}

// TestRouterMutexResponseWriterWriteRecheckUnderLock verifies the router's
// timeout response writer rejects a handler write that passed the initial
// timeout check but lost the race to the timeout response: the re-check under
// the lock must fail the write instead of corrupting the response.
func TestRouterMutexResponseWriterWriteRecheckUnderLock(t *testing.T) {
	rec := httptest.NewRecorder()
	var mu sync.Mutex
	rw := &mutexResponseWriter{ResponseWriter: rec, mu: &mu}

	// Hold the lock as the timeout path does while writing its response.
	mu.Lock()
	writeErr := make(chan error)
	go func() {
		_, err := rw.Write([]byte("late"))
		writeErr <- err
	}()

	// Let the handler write pass the initial check and block on the mutex,
	// then mark the timeout before releasing the lock.
	time.Sleep(50 * time.Millisecond)
	rw.timedOut.Store(true)
	mu.Unlock()

	if err := <-writeErr; err != http.ErrHandlerTimeout {
		t.Errorf("late Write = %v, want http.ErrHandlerTimeout", err)
	}
	if rec.Body.Len() != 0 {
		t.Errorf("late Write reached the underlying writer: body = %q", rec.Body.String())
	}
}
