package mocks

import (
	"context"
	"net/http"
	"net/http/httptest"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/metrics"
)

// MockMetricsRegistry is a mock implementation of metrics.MetricsRegistry for testing
type MockMetricsRegistry struct{}

func (r *MockMetricsRegistry) Register(metric metrics.Metric) error               { return nil }
func (r *MockMetricsRegistry) Get(name string) (metrics.Metric, bool)             { return nil, false }
func (r *MockMetricsRegistry) Unregister(name string) bool                        { return true }
func (r *MockMetricsRegistry) Clear()                                             {}
func (r *MockMetricsRegistry) Snapshot() metrics.MetricsSnapshot                  { return nil }
func (r *MockMetricsRegistry) WithTags(tags metrics.Tags) metrics.MetricsRegistry { return r }
func (r *MockMetricsRegistry) NewCounter() metrics.CounterBuilder                 { return &MockCounterBuilder{} }
func (r *MockMetricsRegistry) NewGauge() metrics.GaugeBuilder                     { return &MockGaugeBuilder{} }
func (r *MockMetricsRegistry) NewHistogram() metrics.HistogramBuilder             { return &MockHistogramBuilder{} }
func (r *MockMetricsRegistry) NewSummary() metrics.SummaryBuilder                 { return &MockSummaryBuilder{} }

// MockCounterBuilder is a mock implementation of metrics.CounterBuilder for testing
type MockCounterBuilder struct{}

func (b *MockCounterBuilder) Name(name string) metrics.CounterBuilder        { return b }
func (b *MockCounterBuilder) Description(desc string) metrics.CounterBuilder { return b }
func (b *MockCounterBuilder) Tag(key, value string) metrics.CounterBuilder   { return b }
func (b *MockCounterBuilder) Build() metrics.Counter                         { return &MockCounter{} }

// MockCounter is a mock implementation of metrics.Counter for testing
type MockCounter struct{}

func (c *MockCounter) Name() string                              { return "mock_counter" }
func (c *MockCounter) Description() string                       { return "Mock counter for testing" }
func (c *MockCounter) Type() metrics.MetricType                  { return metrics.CounterType }
func (c *MockCounter) Tags() metrics.Tags                        { return metrics.Tags{} }
func (c *MockCounter) WithTags(tags metrics.Tags) metrics.Metric { return c }
func (c *MockCounter) Inc()                                      {}
func (c *MockCounter) Add(value float64)                         {}
func (c *MockCounter) Value() float64                            { return 0 }

// MockGaugeBuilder is a mock implementation of metrics.GaugeBuilder for testing
type MockGaugeBuilder struct{}

func (b *MockGaugeBuilder) Name(name string) metrics.GaugeBuilder        { return b }
func (b *MockGaugeBuilder) Description(desc string) metrics.GaugeBuilder { return b }
func (b *MockGaugeBuilder) Tag(key, value string) metrics.GaugeBuilder   { return b }
func (b *MockGaugeBuilder) Build() metrics.Gauge                         { return &MockGauge{} }

// MockGauge is a mock implementation of metrics.Gauge for testing
type MockGauge struct{}

func (g *MockGauge) Name() string                              { return "mock_gauge" }
func (g *MockGauge) Description() string                       { return "Mock gauge for testing" }
func (g *MockGauge) Type() metrics.MetricType                  { return metrics.GaugeType }
func (g *MockGauge) Tags() metrics.Tags                        { return metrics.Tags{} }
func (g *MockGauge) WithTags(tags metrics.Tags) metrics.Metric { return g }
func (g *MockGauge) Set(value float64)                         {}
func (g *MockGauge) Inc()                                      {}
func (g *MockGauge) Dec()                                      {}
func (g *MockGauge) Add(value float64)                         {}
func (g *MockGauge) Sub(value float64)                         {}
func (g *MockGauge) Value() float64                            { return 0 }

// MockHistogramBuilder is a mock implementation of metrics.HistogramBuilder for testing
type MockHistogramBuilder struct{}

func (b *MockHistogramBuilder) Name(name string) metrics.HistogramBuilder          { return b }
func (b *MockHistogramBuilder) Description(desc string) metrics.HistogramBuilder   { return b }
func (b *MockHistogramBuilder) Tag(key, value string) metrics.HistogramBuilder     { return b }
func (b *MockHistogramBuilder) Buckets(buckets []float64) metrics.HistogramBuilder { return b }
func (b *MockHistogramBuilder) Build() metrics.Histogram                           { return &MockHistogram{} }

// MockHistogram is a mock implementation of metrics.Histogram for testing
type MockHistogram struct{}

func (h *MockHistogram) Name() string                              { return "mock_histogram" }
func (h *MockHistogram) Description() string                       { return "Mock histogram for testing" }
func (h *MockHistogram) Type() metrics.MetricType                  { return metrics.HistogramType }
func (h *MockHistogram) Tags() metrics.Tags                        { return metrics.Tags{} }
func (h *MockHistogram) WithTags(tags metrics.Tags) metrics.Metric { return h }
func (h *MockHistogram) Observe(value float64)                     {}
func (h *MockHistogram) Buckets() []float64                        { return []float64{} }

// MockSummaryBuilder is a mock implementation of metrics.SummaryBuilder for testing
type MockSummaryBuilder struct{}

func (b *MockSummaryBuilder) Name(name string) metrics.SummaryBuilder        { return b }
func (b *MockSummaryBuilder) Description(desc string) metrics.SummaryBuilder { return b }
func (b *MockSummaryBuilder) Tag(key, value string) metrics.SummaryBuilder   { return b }
func (b *MockSummaryBuilder) Objectives(objectives map[float64]float64) metrics.SummaryBuilder {
	return b
}
func (b *MockSummaryBuilder) MaxAge(maxAge time.Duration) metrics.SummaryBuilder { return b }
func (b *MockSummaryBuilder) AgeBuckets(ageBuckets int) metrics.SummaryBuilder   { return b }
func (b *MockSummaryBuilder) Build() metrics.Summary                             { return &MockSummary{} }

// MockSummary is a mock implementation of metrics.Summary for testing
type MockSummary struct{}

func (s *MockSummary) Name() string                              { return "mock_summary" }
func (s *MockSummary) Description() string                       { return "Mock summary for testing" }
func (s *MockSummary) Type() metrics.MetricType                  { return metrics.SummaryType }
func (s *MockSummary) Tags() metrics.Tags                        { return metrics.Tags{} }
func (s *MockSummary) WithTags(tags metrics.Tags) metrics.Metric { return s }
func (s *MockSummary) Observe(value float64)                     {}
func (s *MockSummary) Objectives() map[float64]float64           { return map[float64]float64{} }

// MockMetricsExporter is a mock implementation of metrics.MetricsExporter for testing
type MockMetricsExporter struct{}

func (e *MockMetricsExporter) Export(snapshot metrics.MetricsSnapshot) error { return nil }
func (e *MockMetricsExporter) Start() error                                  { return nil }
func (e *MockMetricsExporter) Stop() error                                   { return nil }
func (e *MockMetricsExporter) Handler() http.Handler                         { return http.NotFoundHandler() }

// FlusherRecorder is a test response recorder that implements http.Flusher
type FlusherRecorder struct {
	ResponseRecorder *httptest.ResponseRecorder
	Flushed          bool
	Code             int // Must be a field, not a method, to match how it's used in tests
}

// NewFlusherRecorder creates a new FlusherRecorder
func NewFlusherRecorder() *FlusherRecorder {
	return &FlusherRecorder{
		ResponseRecorder: httptest.NewRecorder(),
		Flushed:          false,
		Code:             0,
	}
}

// Header returns the response headers - required to implement http.ResponseWriter
func (fr *FlusherRecorder) Header() http.Header {
	return fr.ResponseRecorder.Header()
}

// WriteHeader captures the status code
func (fr *FlusherRecorder) WriteHeader(statusCode int) {
	fr.Code = statusCode
	fr.ResponseRecorder.WriteHeader(statusCode)
}

// Write captures the response body
func (fr *FlusherRecorder) Write(b []byte) (int, error) {
	return fr.ResponseRecorder.Write(b)
}

// Flush implements the http.Flusher interface
func (fr *FlusherRecorder) Flush() {
	fr.Flushed = true
}

// MockAuthFunction is a simple mock auth function for testing
func MockAuthFunction(ctx context.Context, token string) (string, bool) {
	if token == "valid-token" {
		return "user123", true
	}
	return "", false
}

// MockUserIDFromUser is a simple mock function to get user ID from user object
func MockUserIDFromUser(user string) string {
	return user
}

// MockGenericRouteRegistrar is a mock implementation of GenericRouteRegistrar for testing
type MockGenericRouteRegistrar struct {
	RegisterWithError bool
	TypeCastError     bool
}

func (m MockGenericRouteRegistrar) RegisterWith(router interface{}, pathPrefix string) error {
	if m.RegisterWithError {
		return context.Canceled // Use a standard error for testing
	}
	if m.TypeCastError {
		// Intentionally try to cast to the wrong type to simulate an error
		_, ok := router.(int)
		if !ok {
			return context.DeadlineExceeded // Use a different standard error
		}
	}
	// Simulate successful registration
	return nil
}
