package metrics

import (
	"net/http"
	"testing"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/scontext"
)

// TestRandomSampler tests the RandomSampler
func TestRandomSampler(t *testing.T) {
	// Test with rate 1.0 (always sample)
	sampler := NewRandomSampler(1.0)
	if !sampler.Sample() {
		t.Error("Expected Sample() to return true with rate 1.0")
	}

	// Test with rate 0.0 (never sample)
	sampler = NewRandomSampler(0.0)
	if sampler.Sample() {
		t.Error("Expected Sample() to return false with rate 0.0")
	}
}

// TestTags tests the Tags map
func TestTags(t *testing.T) {
	// Create a new Tags map
	tags := Tags{
		"key1": "value1",
		"key2": "value2",
	}

	// Check that the map has the expected values
	if tags["key1"] != "value1" {
		t.Errorf("Expected tags[\"key1\"] to be \"value1\", got %q", tags["key1"])
	}
	if tags["key2"] != "value2" {
		t.Errorf("Expected tags[\"key2\"] to be \"value2\", got %q", tags["key2"])
	}

	// Check that a non-existent key returns the zero value
	if tags["key3"] != "" {
		t.Errorf("Expected tags[\"key3\"] to be \"\", got %q", tags["key3"])
	}
}

// TestMetricType tests the MetricType constants
func TestMetricType(t *testing.T) {
	// Check that the MetricType constants have the expected values
	if CounterType != "counter" {
		t.Errorf("Expected CounterType to be \"counter\", got %q", CounterType)
	}
	if GaugeType != "gauge" {
		t.Errorf("Expected GaugeType to be \"gauge\", got %q", GaugeType)
	}
	if HistogramType != "histogram" {
		t.Errorf("Expected HistogramType to be \"histogram\", got %q", HistogramType)
	}
	if SummaryType != "summary" {
		t.Errorf("Expected SummaryType to be \"summary\", got %q", SummaryType)
	}
}

// TestMetricsMiddlewareConfig tests the MetricsMiddlewareConfig struct
func TestMetricsMiddlewareConfig(t *testing.T) {
	// Create a new MetricsMiddlewareConfig
	config := MetricsMiddlewareConfig{
		EnableLatency:    true,
		EnableThroughput: true,
		EnableQPS:        true,
		EnableErrors:     true,
		SamplingRate:     0.5,
		DefaultTags: Tags{
			"key1": "value1",
			"key2": "value2",
		},
	}

	// Check that the config has the expected values
	if !config.EnableLatency {
		t.Error("Expected EnableLatency to be true")
	}
	if !config.EnableThroughput {
		t.Error("Expected EnableThroughput to be true")
	}
	if !config.EnableQPS {
		t.Error("Expected EnableQPS to be true")
	}
	if !config.EnableErrors {
		t.Error("Expected EnableErrors to be true")
	}
	if config.SamplingRate != 0.5 {
		t.Errorf("Expected SamplingRate to be 0.5, got %f", config.SamplingRate)
	}
	if config.DefaultTags["key1"] != "value1" {
		t.Errorf("Expected DefaultTags[\"key1\"] to be \"value1\", got %q", config.DefaultTags["key1"])
	}
	if config.DefaultTags["key2"] != "value2" {
		t.Errorf("Expected DefaultTags[\"key2\"] to be \"value2\", got %q", config.DefaultTags["key2"])
	}
}

// MockMetricsRegistry is a mock implementation of MetricsRegistry for testing
type MockMetricsRegistry struct {
	counters   map[string]Counter
	gauges     map[string]Gauge
	histograms map[string]Histogram
	summaries  map[string]Summary
}

// NewMockMetricsRegistry creates a new MockMetricsRegistry
func NewMockMetricsRegistry() *MockMetricsRegistry {
	return &MockMetricsRegistry{
		counters:   make(map[string]Counter),
		gauges:     make(map[string]Gauge),
		histograms: make(map[string]Histogram),
		summaries:  make(map[string]Summary),
	}
}

// Register registers a metric with the registry
func (r *MockMetricsRegistry) Register(metric Metric) error {
	return nil
}

// Get gets a metric by name
func (r *MockMetricsRegistry) Get(name string) (Metric, bool) {
	return nil, false
}

// Unregister unregisters a metric from the registry
func (r *MockMetricsRegistry) Unregister(name string) bool {
	return true
}

// Clear clears all metrics from the registry
func (r *MockMetricsRegistry) Clear() {
}

// WithTags creates a new registry with the given tags
func (r *MockMetricsRegistry) WithTags(tags Tags) MetricsRegistry {
	return r
}

// NewCounter creates a new counter builder
func (r *MockMetricsRegistry) NewCounter() CounterBuilder {
	return &MockCounterBuilder{registry: r}
}

// NewGauge creates a new gauge builder
func (r *MockMetricsRegistry) NewGauge() GaugeBuilder {
	return &MockGaugeBuilder{registry: r}
}

// NewHistogram creates a new histogram builder
func (r *MockMetricsRegistry) NewHistogram() HistogramBuilder {
	return &MockHistogramBuilder{registry: r}
}

// NewSummary creates a new summary builder
func (r *MockMetricsRegistry) NewSummary() SummaryBuilder {
	return &MockSummaryBuilder{registry: r}
}

// MockCounterBuilder is a mock implementation of CounterBuilder for testing
type MockCounterBuilder struct {
	registry    *MockMetricsRegistry
	name        string
	description string
	tags        Tags
}

// Name sets the counter name
func (b *MockCounterBuilder) Name(name string) CounterBuilder {
	b.name = name
	return b
}

// Description sets the counter description
func (b *MockCounterBuilder) Description(desc string) CounterBuilder {
	b.description = desc
	return b
}

// Tag adds a tag to the counter
func (b *MockCounterBuilder) Tag(key, value string) CounterBuilder {
	if b.tags == nil {
		b.tags = make(Tags)
	}
	b.tags[key] = value
	return b
}

// Build creates the counter
func (b *MockCounterBuilder) Build() Counter {
	counter := &MockCounter{
		name:        b.name,
		description: b.description,
		tags:        b.tags,
	}
	b.registry.counters[b.name] = counter
	return counter
}

// MockCounter is a mock implementation of Counter for testing
type MockCounter struct {
	name        string
	description string
	tags        Tags
	value       float64
}

// Name returns the counter name
func (c *MockCounter) Name() string {
	return c.name
}

// Description returns the counter description
func (c *MockCounter) Description() string {
	return c.description
}

// Type returns the metric type
func (c *MockCounter) Type() MetricType {
	return CounterType
}

// Tags returns the metric tags
func (c *MockCounter) Tags() Tags {
	return c.tags
}

// WithTags returns a new metric with the given tags
func (c *MockCounter) WithTags(tags Tags) Metric {
	newCounter := &MockCounter{
		name:        c.name,
		description: c.description,
		tags:        make(Tags),
		value:       c.value,
	}
	for k, v := range c.tags {
		newCounter.tags[k] = v
	}
	for k, v := range tags {
		newCounter.tags[k] = v
	}
	return newCounter
}

// Inc increments the counter by 1
func (c *MockCounter) Inc() {
	c.value++
}

// Add adds the given value to the counter
func (c *MockCounter) Add(value float64) {
	c.value += value
}

// Value returns the current value of the counter
func (c *MockCounter) Value() float64 {
	return c.value
}

// MockGaugeBuilder is a mock implementation of GaugeBuilder for testing
type MockGaugeBuilder struct {
	registry    *MockMetricsRegistry
	name        string
	description string
	tags        Tags
}

// Name sets the gauge name
func (b *MockGaugeBuilder) Name(name string) GaugeBuilder {
	b.name = name
	return b
}

// Description sets the gauge description
func (b *MockGaugeBuilder) Description(desc string) GaugeBuilder {
	b.description = desc
	return b
}

// Tag adds a tag to the gauge
func (b *MockGaugeBuilder) Tag(key, value string) GaugeBuilder {
	if b.tags == nil {
		b.tags = make(Tags)
	}
	b.tags[key] = value
	return b
}

// Build creates the gauge
func (b *MockGaugeBuilder) Build() Gauge {
	gauge := &MockGauge{
		name:        b.name,
		description: b.description,
		tags:        b.tags,
	}
	b.registry.gauges[b.name] = gauge
	return gauge
}

// MockGauge is a mock implementation of Gauge for testing
type MockGauge struct {
	name        string
	description string
	tags        Tags
	value       float64
}

// Name returns the gauge name
func (g *MockGauge) Name() string {
	return g.name
}

// Description returns the gauge description
func (g *MockGauge) Description() string {
	return g.description
}

// Type returns the metric type
func (g *MockGauge) Type() MetricType {
	return GaugeType
}

// Tags returns the metric tags
func (g *MockGauge) Tags() Tags {
	return g.tags
}

// WithTags returns a new metric with the given tags
func (g *MockGauge) WithTags(tags Tags) Metric {
	newGauge := &MockGauge{
		name:        g.name,
		description: g.description,
		tags:        make(Tags),
		value:       g.value,
	}
	for k, v := range g.tags {
		newGauge.tags[k] = v
	}
	for k, v := range tags {
		newGauge.tags[k] = v
	}
	return newGauge
}

// Set sets the gauge to the given value
func (g *MockGauge) Set(value float64) {
	g.value = value
}

// Inc increments the gauge by 1
func (g *MockGauge) Inc() {
	g.value++
}

// Dec decrements the gauge by 1
func (g *MockGauge) Dec() {
	g.value--
}

// Add adds the given value to the gauge
func (g *MockGauge) Add(value float64) {
	g.value += value
}

// Sub subtracts the given value from the gauge
func (g *MockGauge) Sub(value float64) {
	g.value -= value
}

// Value returns the current value of the gauge
func (g *MockGauge) Value() float64 {
	return g.value
}

// MockHistogramBuilder is a mock implementation of HistogramBuilder for testing
type MockHistogramBuilder struct {
	registry    *MockMetricsRegistry
	name        string
	description string
	tags        Tags
	buckets     []float64
}

// Name sets the histogram name
func (b *MockHistogramBuilder) Name(name string) HistogramBuilder {
	b.name = name
	return b
}

// Description sets the histogram description
func (b *MockHistogramBuilder) Description(desc string) HistogramBuilder {
	b.description = desc
	return b
}

// Tag adds a tag to the histogram
func (b *MockHistogramBuilder) Tag(key, value string) HistogramBuilder {
	if b.tags == nil {
		b.tags = make(Tags)
	}
	b.tags[key] = value
	return b
}

// Buckets sets the bucket boundaries
func (b *MockHistogramBuilder) Buckets(buckets []float64) HistogramBuilder {
	b.buckets = buckets
	return b
}

// Build creates the histogram
func (b *MockHistogramBuilder) Build() Histogram {
	histogram := &MockHistogram{
		name:        b.name,
		description: b.description,
		tags:        b.tags,
		buckets:     b.buckets,
	}
	b.registry.histograms[b.name] = histogram
	return histogram
}

// MockHistogram is a mock implementation of Histogram for testing
type MockHistogram struct {
	name         string
	description  string
	tags         Tags
	buckets      []float64
	observations []float64
}

// Name returns the histogram name
func (h *MockHistogram) Name() string {
	return h.name
}

// Description returns the histogram description
func (h *MockHistogram) Description() string {
	return h.description
}

// Type returns the metric type
func (h *MockHistogram) Type() MetricType {
	return HistogramType
}

// Tags returns the metric tags
func (h *MockHistogram) Tags() Tags {
	return h.tags
}

// WithTags returns a new metric with the given tags
func (h *MockHistogram) WithTags(tags Tags) Metric {
	newHistogram := &MockHistogram{
		name:         h.name,
		description:  h.description,
		tags:         make(Tags),
		buckets:      h.buckets,
		observations: h.observations,
	}
	for k, v := range h.tags {
		newHistogram.tags[k] = v
	}
	for k, v := range tags {
		newHistogram.tags[k] = v
	}
	return newHistogram
}

// Observe adds a single observation to the histogram
func (h *MockHistogram) Observe(value float64) {
	h.observations = append(h.observations, value)
}

// Buckets returns the bucket boundaries
func (h *MockHistogram) Buckets() []float64 {
	return h.buckets
}

// MockSummaryBuilder is a mock implementation of SummaryBuilder for testing
type MockSummaryBuilder struct {
	registry    *MockMetricsRegistry
	name        string
	description string
	tags        Tags
	objectives  map[float64]float64
	maxAge      time.Duration
	ageBuckets  int
}

// Name sets the summary name
func (b *MockSummaryBuilder) Name(name string) SummaryBuilder {
	b.name = name
	return b
}

// Description sets the summary description
func (b *MockSummaryBuilder) Description(desc string) SummaryBuilder {
	b.description = desc
	return b
}

// Tag adds a tag to the summary
func (b *MockSummaryBuilder) Tag(key, value string) SummaryBuilder {
	if b.tags == nil {
		b.tags = make(Tags)
	}
	b.tags[key] = value
	return b
}

// Objectives sets the quantile objectives
func (b *MockSummaryBuilder) Objectives(objectives map[float64]float64) SummaryBuilder {
	b.objectives = objectives
	return b
}

// MaxAge sets the maximum age of observations
func (b *MockSummaryBuilder) MaxAge(maxAge time.Duration) SummaryBuilder {
	b.maxAge = maxAge
	return b
}

// AgeBuckets sets the number of age buckets
func (b *MockSummaryBuilder) AgeBuckets(ageBuckets int) SummaryBuilder {
	b.ageBuckets = ageBuckets
	return b
}

// Build creates the summary
func (b *MockSummaryBuilder) Build() Summary {
	summary := &MockSummary{
		name:        b.name,
		description: b.description,
		tags:        b.tags,
		objectives:  b.objectives,
	}
	b.registry.summaries[b.name] = summary
	return summary
}

// MockSummary is a mock implementation of Summary for testing
type MockSummary struct {
	name         string
	description  string
	tags         Tags
	objectives   map[float64]float64
	observations []float64
}

// Name returns the summary name
func (s *MockSummary) Name() string {
	return s.name
}

// Description returns the summary description
func (s *MockSummary) Description() string {
	return s.description
}

// Type returns the metric type
func (s *MockSummary) Type() MetricType {
	return SummaryType
}

// Tags returns the metric tags
func (s *MockSummary) Tags() Tags {
	return s.tags
}

// WithTags returns a new metric with the given tags
func (s *MockSummary) WithTags(tags Tags) Metric {
	newSummary := &MockSummary{
		name:         s.name,
		description:  s.description,
		tags:         make(Tags),
		objectives:   s.objectives,
		observations: s.observations,
	}
	for k, v := range s.tags {
		newSummary.tags[k] = v
	}
	for k, v := range tags {
		newSummary.tags[k] = v
	}
	return newSummary
}

// Observe adds a single observation to the summary
func (s *MockSummary) Observe(value float64) {
	s.observations = append(s.observations, value)
}

// Objectives returns the quantile objectives
func (s *MockSummary) Objectives() map[float64]float64 {
	return s.objectives
}

// MockMetricsFilter is a mock implementation of MetricsFilter for testing
type MockMetricsFilter struct {
	filterFunc func(*http.Request) bool
}

// Filter returns true if metrics should be collected for the request
func (f *MockMetricsFilter) Filter(r *http.Request) bool {
	if f.filterFunc != nil {
		return f.filterFunc(r)
	}
	return true
}

// NewMockMetricsFilter creates a new MockMetricsFilter
func NewMockMetricsFilter(filterFunc func(*http.Request) bool) *MockMetricsFilter {
	return &MockMetricsFilter{
		filterFunc: filterFunc,
	}
}

// TestNewMetricsMiddleware tests the NewMetricsMiddleware function
func TestNewMetricsMiddleware(t *testing.T) {
	// Create a mock registry
	registry := NewMockMetricsRegistry()

	// Create a config
	config := MetricsMiddlewareConfig{
		EnableLatency:    true,
		EnableThroughput: true,
		EnableQPS:        true,
		EnableErrors:     true,
		SamplingRate:     0.5,
		DefaultTags: Tags{
			"key1": "value1",
			"key2": "value2",
		},
	}

	// Create a middleware with placeholder types [string, any]
	middleware := NewMetricsMiddleware[string, any](registry, config)

	// Check that the middleware was created with the correct registry and config
	if middleware.registry != registry {
		t.Error("Expected middleware.registry to be the mock registry")
	}
	if middleware.config.EnableLatency != config.EnableLatency {
		t.Errorf("Expected middleware.config.EnableLatency to be %v, got %v", config.EnableLatency, middleware.config.EnableLatency)
	}
	if middleware.config.EnableThroughput != config.EnableThroughput {
		t.Errorf("Expected middleware.config.EnableThroughput to be %v, got %v", config.EnableThroughput, middleware.config.EnableThroughput)
	}
	if middleware.config.EnableQPS != config.EnableQPS {
		t.Errorf("Expected middleware.config.EnableQPS to be %v, got %v", config.EnableQPS, middleware.config.EnableQPS)
	}
	if middleware.config.EnableErrors != config.EnableErrors {
		t.Errorf("Expected middleware.config.EnableErrors to be %v, got %v", config.EnableErrors, middleware.config.EnableErrors)
	}
	if middleware.config.SamplingRate != config.SamplingRate {
		t.Errorf("Expected middleware.config.SamplingRate to be %v, got %v", config.SamplingRate, middleware.config.SamplingRate)
	}
	if middleware.config.DefaultTags["key1"] != config.DefaultTags["key1"] {
		t.Errorf("Expected middleware.config.DefaultTags[\"key1\"] to be %v, got %v", config.DefaultTags["key1"], middleware.config.DefaultTags["key1"])
	}
	if middleware.config.DefaultTags["key2"] != config.DefaultTags["key2"] {
		t.Errorf("Expected middleware.config.DefaultTags[\"key2\"] to be %v, got %v", config.DefaultTags["key2"], middleware.config.DefaultTags["key2"])
	}
}

// TestMetricsMiddlewareImpl_Configure tests the Configure method of MetricsMiddlewareImpl
func TestMetricsMiddlewareImpl_Configure(t *testing.T) {
	// Create a mock registry
	registry := NewMockMetricsRegistry()

	// Create a middleware with placeholder types [string, any]
	middleware := NewMetricsMiddleware[string, any](registry, MetricsMiddlewareConfig{})

	// Create a new config
	config := MetricsMiddlewareConfig{
		EnableLatency:    true,
		EnableThroughput: true,
		EnableQPS:        true,
		EnableErrors:     true,
		SamplingRate:     0.5,
		DefaultTags: Tags{
			"key1": "value1",
			"key2": "value2",
		},
	}

	// Configure the middleware
	result := middleware.Configure(config)

	// Check that the middleware was configured correctly
	if result != middleware {
		t.Error("Expected Configure to return the middleware")
	}
	if middleware.config.EnableLatency != config.EnableLatency {
		t.Errorf("Expected middleware.config.EnableLatency to be %v, got %v", config.EnableLatency, middleware.config.EnableLatency)
	}
	if middleware.config.EnableThroughput != config.EnableThroughput {
		t.Errorf("Expected middleware.config.EnableThroughput to be %v, got %v", config.EnableThroughput, middleware.config.EnableThroughput)
	}
	if middleware.config.EnableQPS != config.EnableQPS {
		t.Errorf("Expected middleware.config.EnableQPS to be %v, got %v", config.EnableQPS, middleware.config.EnableQPS)
	}
	if middleware.config.EnableErrors != config.EnableErrors {
		t.Errorf("Expected middleware.config.EnableErrors to be %v, got %v", config.EnableErrors, middleware.config.EnableErrors)
	}
	if middleware.config.SamplingRate != config.SamplingRate {
		t.Errorf("Expected middleware.config.SamplingRate to be %v, got %v", config.SamplingRate, middleware.config.SamplingRate)
	}
	if middleware.config.DefaultTags["key1"] != config.DefaultTags["key1"] {
		t.Errorf("Expected middleware.config.DefaultTags[\"key1\"] to be %v, got %v", config.DefaultTags["key1"], middleware.config.DefaultTags["key1"])
	}
	if middleware.config.DefaultTags["key2"] != config.DefaultTags["key2"] {
		t.Errorf("Expected middleware.config.DefaultTags[\"key2\"] to be %v, got %v", config.DefaultTags["key2"], middleware.config.DefaultTags["key2"])
	}
}

// TestMetricsMiddlewareImpl_WithFilter tests the WithFilter method of MetricsMiddlewareImpl
func TestMetricsMiddlewareImpl_WithFilter(t *testing.T) {
	// Create a mock registry
	registry := NewMockMetricsRegistry()

	// Create a middleware with placeholder types [string, any]
	middleware := NewMetricsMiddleware[string, any](registry, MetricsMiddlewareConfig{})

	// Create a mock filter
	filter := NewMockMetricsFilter(func(r *http.Request) bool {
		return true
	})

	// Add the filter to the middleware
	result := middleware.WithFilter(filter)

	// Check that the middleware was configured correctly
	if result != middleware {
		t.Error("Expected WithFilter to return the middleware")
	}
	if middleware.filter != filter {
		t.Error("Expected middleware.filter to be the mock filter")
	}
}

// TestMetricsMiddlewareImpl_WithSampler tests the WithSampler method of MetricsMiddlewareImpl
func TestMetricsMiddlewareImpl_WithSampler(t *testing.T) {
	// Create a mock registry
	registry := NewMockMetricsRegistry()

	// Create a middleware with placeholder types [string, any]
	middleware := NewMetricsMiddleware[string, any](registry, MetricsMiddlewareConfig{})

	// Create a sampler
	sampler := NewRandomSampler(0.5)

	// Add the sampler to the middleware
	result := middleware.WithSampler(sampler)

	// Check that the middleware was configured correctly
	if result != middleware {
		t.Error("Expected WithSampler to return the middleware")
	}
	if middleware.sampler != sampler {
		t.Error("Expected middleware.sampler to be the random sampler")
	}
}

// MockResponseWriter is a mock implementation of http.ResponseWriter for testing
type MockResponseWriter struct {
	headers     http.Header
	statusCode  int
	writtenData []byte
}

// Header returns the response headers
func (w *MockResponseWriter) Header() http.Header {
	return w.headers
}

// Write writes the data to the response
func (w *MockResponseWriter) Write(data []byte) (int, error) {
	w.writtenData = append(w.writtenData, data...)
	return len(data), nil
}

// WriteHeader sets the response status code
func (w *MockResponseWriter) WriteHeader(statusCode int) {
	w.statusCode = statusCode
}

// NewMockResponseWriter creates a new MockResponseWriter
func NewMockResponseWriter() *MockResponseWriter {
	return &MockResponseWriter{
		headers:     make(http.Header),
		statusCode:  http.StatusOK,
		writtenData: []byte{},
	}
}

// TestMetricsMiddlewareImpl_Handler tests the Handler method of MetricsMiddlewareImpl
func TestMetricsMiddlewareImpl_Handler(t *testing.T) {
	// Create a mock registry
	registry := NewMockMetricsRegistry()

	// Create a middleware with all metrics enabled, using placeholder types [string, any]
	middleware := NewMetricsMiddleware[string, any](registry, MetricsMiddlewareConfig{
		EnableLatency:    true,
		EnableThroughput: true,
		EnableQPS:        true,
		EnableErrors:     true,
	})

	// Create a test handler
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Hello, World!"))
	})

	// Wrap the handler with the middleware
	wrappedHandler := middleware.Handler("test", testHandler)

	// Create a test request
	req, err := http.NewRequest("GET", "/test", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	// Create a mock response writer
	rw := NewMockResponseWriter()

	// Serve the request
	wrappedHandler.ServeHTTP(rw, req)

	// Check that the response was written correctly
	if rw.statusCode != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rw.statusCode)
	}
	if string(rw.writtenData) != "Hello, World!" {
		t.Errorf("Expected response body %q, got %q", "Hello, World!", string(rw.writtenData))
	}
}

// TestMetricsMiddlewareImpl_Handler_WithFilter tests the Handler method with a filter
func TestMetricsMiddlewareImpl_Handler_WithFilter(t *testing.T) {
	// Create a mock registry
	registry := NewMockMetricsRegistry()

	// Create a middleware with all metrics enabled, using placeholder types [string, any]
	middleware := NewMetricsMiddleware[string, any](registry, MetricsMiddlewareConfig{
		EnableLatency:    true,
		EnableThroughput: true,
		EnableQPS:        true,
		EnableErrors:     true,
	})

	// Create a filter that rejects all requests
	filter := NewMockMetricsFilter(func(r *http.Request) bool {
		return false
	})

	// Add the filter to the middleware
	middleware.WithFilter(filter)

	// Create a test handler that increments a counter
	handlerCalled := false
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Hello, World!"))
	})

	// Wrap the handler with the middleware
	wrappedHandler := middleware.Handler("test", testHandler)

	// Create a test request
	req, err := http.NewRequest("GET", "/test", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	// Create a mock response writer
	rw := NewMockResponseWriter()

	// Serve the request
	wrappedHandler.ServeHTTP(rw, req)

	// Check that the handler was called
	if !handlerCalled {
		t.Error("Expected handler to be called")
	}

	// Check that the response was written correctly
	if rw.statusCode != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rw.statusCode)
	}
	if string(rw.writtenData) != "Hello, World!" {
		t.Errorf("Expected response body %q, got %q", "Hello, World!", string(rw.writtenData))
	}
}

// TestMetricsMiddlewareImpl_Handler_WithSampler tests the Handler method with a sampler
func TestMetricsMiddlewareImpl_Handler_WithSampler(t *testing.T) {
	// Create a mock registry
	registry := NewMockMetricsRegistry()

	// Create a middleware with all metrics enabled, using placeholder types [string, any]
	middleware := NewMetricsMiddleware[string, any](registry, MetricsMiddlewareConfig{
		EnableLatency:    true,
		EnableThroughput: true,
		EnableQPS:        true,
		EnableErrors:     true,
	})

	// Create a sampler that rejects all requests
	sampler := NewRandomSampler(0.0)

	// Add the sampler to the middleware
	middleware.WithSampler(sampler)

	// Create a test handler that increments a counter
	handlerCalled := false
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Hello, World!"))
	})

	// Wrap the handler with the middleware
	wrappedHandler := middleware.Handler("test", testHandler)

	// Create a test request
	req, err := http.NewRequest("GET", "/test", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	// Create a mock response writer
	rw := NewMockResponseWriter()

	// Serve the request
	wrappedHandler.ServeHTTP(rw, req)

	// Check that the handler was called
	if !handlerCalled {
		t.Error("Expected handler to be called")
	}

	// Check that the response was written correctly
	if rw.statusCode != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rw.statusCode)
	}
	if string(rw.writtenData) != "Hello, World!" {
		t.Errorf("Expected response body %q, got %q", "Hello, World!", string(rw.writtenData))
	}
}

// TestMetricsMiddlewareImpl_Handler_WithRouteTemplate tests that route templates are used when available
func TestMetricsMiddlewareImpl_Handler_WithRouteTemplate(t *testing.T) {
	// Create a mock registry to track metric creation
	registry := NewMockMetricsRegistry()

	// Create a middleware with all metrics enabled, using placeholder types [string, any]
	middleware := NewMetricsMiddleware[string, any](registry, MetricsMiddlewareConfig{
		EnableLatency:    true,
		EnableThroughput: true,
		EnableQPS:        true,
		EnableErrors:     true,
	})

	// Create a test handler
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Hello, World!"))
	})

	// Wrap the handler with the middleware
	wrappedHandler := middleware.Handler("fallback-name", testHandler)

	// Create a test request with a route template in the context
	req, err := http.NewRequest("GET", "/users/123", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	// Add route template to context
	expectedTemplate := "/users/:id"
	ctx := req.Context()
	ctx = scontext.WithRouteInfo[string, interface{}](ctx, nil, expectedTemplate)
	req = req.WithContext(ctx)

	// Set Content-Length to test throughput metrics
	req.ContentLength = 100

	// Create a mock response writer
	rw := NewMockResponseWriter()

	// Serve the request
	wrappedHandler.ServeHTTP(rw, req)

	// Verify that both route-specific and global metrics were created

	// Check for route-specific latency metric
	var foundRouteLatency bool
	for _, histogram := range registry.histograms {
		if histogram.Name() == "request_latency_seconds" {
			if tags := histogram.Tags(); tags != nil {
				if routeTag, ok := tags["route"]; ok && routeTag == expectedTemplate {
					foundRouteLatency = true
					break
				}
			}
		}
	}
	if !foundRouteLatency {
		t.Error("Route-specific latency metric not found")
	}

	// Check for global latency metric
	var foundGlobalLatency bool
	for _, histogram := range registry.histograms {
		if histogram.Name() == "request_latency_seconds_total" {
			foundGlobalLatency = true
			break
		}
	}
	if !foundGlobalLatency {
		t.Error("Global latency metric not found")
	}

	// Check for route-specific QPS metric
	var foundRouteQPS bool
	for _, counter := range registry.counters {
		if counter.Name() == "requests_total" {
			if tags := counter.Tags(); tags != nil {
				if routeTag, ok := tags["route"]; ok && routeTag == expectedTemplate {
					foundRouteQPS = true
					break
				}
			}
		}
	}
	if !foundRouteQPS {
		t.Error("Route-specific QPS metric not found")
	}

	// Check for global QPS metric
	var foundGlobalQPS bool
	for _, counter := range registry.counters {
		if counter.Name() == "all_requests_total" {
			foundGlobalQPS = true
			break
		}
	}
	if !foundGlobalQPS {
		t.Error("Global QPS metric not found")
	}

	// Check for route-specific throughput metric
	var foundRouteThroughput bool
	for _, counter := range registry.counters {
		if counter.Name() == "request_throughput_bytes" {
			if tags := counter.Tags(); tags != nil {
				if routeTag, ok := tags["route"]; ok && routeTag == expectedTemplate {
					foundRouteThroughput = true
					break
				}
			}
		}
	}
	if !foundRouteThroughput {
		t.Error("Route-specific throughput metric not found")
	}

	// Check for global throughput metric
	var foundGlobalThroughput bool
	for _, counter := range registry.counters {
		if counter.Name() == "request_throughput_bytes_total" {
			foundGlobalThroughput = true
			break
		}
	}
	if !foundGlobalThroughput {
		t.Error("Global throughput metric not found")
	}
}

// TestMetricsMiddlewareImpl_Handler_Error tests the Handler method with an error response
func TestMetricsMiddlewareImpl_Handler_Error(t *testing.T) {
	// Create a mock registry
	registry := NewMockMetricsRegistry()

	// Create a middleware with all metrics enabled, using placeholder types [string, any]
	middleware := NewMetricsMiddleware[string, any](registry, MetricsMiddlewareConfig{
		EnableLatency:    true,
		EnableThroughput: true,
		EnableQPS:        true,
		EnableErrors:     true,
	})

	// Create a test handler that returns an error
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte("Internal Server Error"))
	})

	// Wrap the handler with the middleware
	wrappedHandler := middleware.Handler("test", testHandler)

	// Create a test request
	req, err := http.NewRequest("GET", "/test", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	// Create a mock response writer
	rw := NewMockResponseWriter()

	// Serve the request
	wrappedHandler.ServeHTTP(rw, req)

	// Check that the response was written correctly
	if rw.statusCode != http.StatusInternalServerError {
		t.Errorf("Expected status code %d, got %d", http.StatusInternalServerError, rw.statusCode)
	}
	if string(rw.writtenData) != "Internal Server Error" {
		t.Errorf("Expected response body %q, got %q", "Internal Server Error", string(rw.writtenData))
	}
}

// Removed TestGetRouteTemplateFromRequest as the helper function was removed.

// TestResponseWriter tests the responseWriter struct
func TestResponseWriter(t *testing.T) {
	// Create a mock response writer
	mockRw := NewMockResponseWriter()

	// Create a responseWriter
	rw := &responseWriter{
		ResponseWriter: mockRw,
		statusCode:     http.StatusOK,
	}

	// Set a status code
	rw.WriteHeader(http.StatusNotFound)

	// Check that the status code was set
	if rw.statusCode != http.StatusNotFound {
		t.Errorf("Expected status code %d, got %d", http.StatusNotFound, rw.statusCode)
	}
	if mockRw.statusCode != http.StatusNotFound {
		t.Errorf("Expected underlying status code %d, got %d", http.StatusNotFound, mockRw.statusCode)
	}

	// Write some data
	data := []byte("Test data")
	n, err := rw.Write(data)
	if err != nil {
		t.Errorf("Unexpected error: %v", err)
	}
	if n != len(data) {
		t.Errorf("Expected to write %d bytes, wrote %d", len(data), n)
	}
	if string(mockRw.writtenData) != "Test data" {
		t.Errorf("Expected underlying data %q, got %q", "Test data", string(mockRw.writtenData))
	}
}
