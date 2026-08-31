package router

import (
	"maps"
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

type recordedCounter struct {
	name string
	tags metrics.Tags
}

type recordingMetricsRegistry struct {
	mu       sync.Mutex
	counters []recordedCounter
}

func (r *recordingMetricsRegistry) Register(metrics.Metric) error { return nil }

func (r *recordingMetricsRegistry) NewCounter() metrics.CounterBuilder {
	return &recordingCounterBuilder{registry: r, tags: make(metrics.Tags)}
}

func (r *recordingMetricsRegistry) NewGauge() metrics.GaugeBuilder {
	return &mocks.MockGaugeBuilder{}
}

func (r *recordingMetricsRegistry) NewHistogram() metrics.HistogramBuilder {
	return &mocks.MockHistogramBuilder{}
}

func (r *recordingMetricsRegistry) NewSummary() metrics.SummaryBuilder {
	return &mocks.MockSummaryBuilder{}
}

func (r *recordingMetricsRegistry) counterSnapshots() []recordedCounter {
	r.mu.Lock()
	defer r.mu.Unlock()
	result := make([]recordedCounter, len(r.counters))
	for i, counter := range r.counters {
		result[i] = recordedCounter{name: counter.name, tags: maps.Clone(counter.tags)}
	}
	return result
}

type recordingCounterBuilder struct {
	registry    *recordingMetricsRegistry
	name        string
	description string
	tags        metrics.Tags
}

func (b *recordingCounterBuilder) Name(name string) metrics.CounterBuilder {
	b.name = name
	return b
}

func (b *recordingCounterBuilder) Description(description string) metrics.CounterBuilder {
	b.description = description
	return b
}

func (b *recordingCounterBuilder) Tag(key, value string) metrics.CounterBuilder {
	b.tags[key] = value
	return b
}

func (b *recordingCounterBuilder) Build() metrics.Counter {
	counter := &recordingCounterMetric{
		name:        b.name,
		description: b.description,
		tags:        maps.Clone(b.tags),
	}
	b.registry.mu.Lock()
	b.registry.counters = append(b.registry.counters, recordedCounter{name: b.name, tags: maps.Clone(b.tags)})
	b.registry.mu.Unlock()
	return counter
}

type recordingCounterMetric struct {
	name        string
	description string
	tags        metrics.Tags
}

func (c *recordingCounterMetric) Name() string             { return c.name }
func (c *recordingCounterMetric) Description() string      { return c.description }
func (c *recordingCounterMetric) Type() metrics.MetricType { return metrics.CounterType }
func (c *recordingCounterMetric) Tags() metrics.Tags       { return maps.Clone(c.tags) }
func (c *recordingCounterMetric) Inc()                     {}
func (c *recordingCounterMetric) Add(_ float64)            {}

// TestMetricsConfigMiddlewareFactory verifies that when MetricsConfig supplies
// a MiddlewareFactory of the router's generic type, the router wraps handlers
// with it (passing the configured ServiceName) and requests flow through it.
func TestMetricsConfigMiddlewareFactory(t *testing.T) {
	factory := &fakeMetricsMiddlewareFactory{}
	registry := &recordingMetricsRegistry{}

	r := NewRouter(RouterConfig{
		ServiceName: "test-service",
		MetricsConfig: &MetricsConfig{
			MiddlewareFactory: factory,
			Collector:         registry,
			EnableQPS:         true,
		},
	}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	r.Route(RouteConfigBase{
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
	if counters := registry.counterSnapshots(); len(counters) != 0 {
		t.Errorf("Collector built %d counters even though MiddlewareFactory should take precedence", len(counters))
	}
}

func TestMetricsConfigMismatchedFactoryFallsBackToCollector(t *testing.T) {
	factory := &fakeMetricsMiddlewareFactory{} // MetricsMiddleware[string, string]
	registry := &recordingMetricsRegistry{}

	r := NewRouter[int, string](RouterConfig{
		ServiceName: "fallback-service",
		MetricsConfig: &MetricsConfig{
			MiddlewareFactory: factory,
			Collector:         registry,
			Namespace:         "accounts",
			Subsystem:         "http",
			EnableQPS:         true,
		},
	}, nil, nil)
	r.Route(RouteConfigBase{
		Path:    "/collector",
		Methods: []HttpMethod{MethodGet},
		Handler: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		},
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/collector", nil))
	if rec.Code != http.StatusNoContent {
		t.Fatalf("status = %d, want %d", rec.Code, http.StatusNoContent)
	}
	if rec.Header().Get("X-Metrics-Factory") != "" {
		t.Fatal("mismatched MiddlewareFactory unexpectedly wrapped the route")
	}

	factory.mu.Lock()
	factoryCalls := len(factory.handlerNames)
	factory.mu.Unlock()
	if factoryCalls != 0 {
		t.Fatalf("mismatched MiddlewareFactory Handler calls = %d, want 0", factoryCalls)
	}

	counters := registry.counterSnapshots()
	if len(counters) != 2 {
		t.Fatalf("Collector built %d counters, want route and global request counters", len(counters))
	}
	var routeCounter, globalCounter bool
	for _, counter := range counters {
		if counter.tags["service"] != "accounts" || counter.tags["subsystem"] != "http" {
			t.Errorf("counter %q tags = %v, want service=accounts and subsystem=http", counter.name, counter.tags)
		}
		switch counter.name {
		case "requests_total":
			routeCounter = counter.tags["route"] == "/collector"
		case "all_requests_total":
			_, hasRoute := counter.tags["route"]
			globalCounter = !hasRoute
		}
	}
	if !routeCounter || !globalCounter {
		t.Errorf("counter mapping = route:%v global:%v; built counters: %v", routeCounter, globalCounter, counters)
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

	got := r.convertRateLimit(src)
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

	// A UserIDFromUser returning a value of the wrong type must fail loudly
	// instead of collapsing unrelated users into the zero-value bucket.
	wrongType := &common.RateLimitConfig[any, any]{
		Limit:  1,
		Window: time.Second,
		UserIDFromUser: func(user any) any {
			return 42 // not a string
		},
	}
	converted := r.convertRateLimit(wrongType)
	if converted == nil || converted.UserIDFromUser == nil {
		t.Fatal("expected converted config with adapted UserIDFromUser")
	}
	defer func() {
		if recover() == nil {
			t.Fatal("expected mismatched user ID type to panic")
		}
	}()
	converted.UserIDFromUser("alice")
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
