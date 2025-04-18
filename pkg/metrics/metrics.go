// Package metrics provides an interface-based metrics system for SRouter.
// It defines interfaces for metrics collection and exposition, allowing users to provide
// their own implementations while the framework uses the methods exposed by these interfaces
// to aggregate metrics. This approach maintains separation of concerns, where the framework
// defines the interfaces and the users provide the implementations.
package metrics

import (
	"net/http"
	"time"
)

// MetricType represents the type of a metric.
type MetricType string

const (
	// CounterType represents a counter metric.
	CounterType MetricType = "counter"

	// GaugeType represents a gauge metric.
	GaugeType MetricType = "gauge"

	// HistogramType represents a histogram metric.
	HistogramType MetricType = "histogram"

	// SummaryType represents a summary metric.
	SummaryType MetricType = "summary"
)

// Tags represents a map of key-value pairs for metric tags.
type Tags map[string]string

// Metric is the base interface for all metrics.
type Metric interface {
	// Name returns the metric name.
	Name() string

	// Description returns the metric description.
	Description() string

	// Type returns the metric type.
	Type() MetricType

	// Tags returns the metric tags.
	Tags() Tags
}

// Counter is a metric that represents a monotonically increasing value.
type Counter interface {
	Metric

	// Inc increments the counter by 1.
	Inc()

	// Add adds the given value to the counter.
	Add(value float64)
}

// Gauge is a metric that represents a value that can go up and down.
type Gauge interface {
	Metric

	// Set sets the gauge to the given value.
	Set(value float64)

	// Inc increments the gauge by 1.
	Inc()

	// Dec decrements the gauge by 1.
	Dec()

	// Add adds the given value to the gauge.
	Add(value float64)

	// Sub subtracts the given value from the gauge.
	Sub(value float64)
}

// Histogram is a metric that samples observations and counts them in configurable buckets.
type Histogram interface {
	Metric

	// Observe adds a single observation to the histogram.
	Observe(value float64)
}

// Summary is a metric that samples observations and calculates quantiles over a sliding time window.
type Summary interface {
	Metric

	// Observe adds a single observation to the summary.
	Observe(value float64)
}

// CounterBuilder is a builder for creating counters.
type CounterBuilder interface {
	// Name sets the counter name.
	Name(name string) CounterBuilder

	// Description sets the counter description.
	Description(desc string) CounterBuilder

	// Tag adds a tag to the counter.
	Tag(key, value string) CounterBuilder

	// Build creates the counter.
	Build() Counter
}

// GaugeBuilder is a builder for creating gauges.
type GaugeBuilder interface {
	// Name sets the gauge name.
	Name(name string) GaugeBuilder

	// Description sets the gauge description.
	Description(desc string) GaugeBuilder

	// Tag adds a tag to the gauge.
	Tag(key, value string) GaugeBuilder

	// Build creates the gauge.
	Build() Gauge
}

// HistogramBuilder is a builder for creating histograms.
type HistogramBuilder interface {
	// Name sets the histogram name.
	Name(name string) HistogramBuilder

	// Description sets the histogram description.
	Description(desc string) HistogramBuilder

	// Tag adds a tag to the histogram.
	Tag(key, value string) HistogramBuilder

	// Buckets sets the bucket boundaries.
	Buckets(buckets []float64) HistogramBuilder

	// Build creates the histogram.
	Build() Histogram
}

// SummaryBuilder is a builder for creating summaries.
type SummaryBuilder interface {
	// Name sets the summary name.
	Name(name string) SummaryBuilder

	// Description sets the summary description.
	Description(desc string) SummaryBuilder

	// Tag adds a tag to the summary.
	Tag(key, value string) SummaryBuilder

	// Objectives sets the quantile objectives.
	Objectives(objectives map[float64]float64) SummaryBuilder

	// MaxAge sets the maximum age of observations.
	MaxAge(maxAge time.Duration) SummaryBuilder

	// AgeBuckets sets the number of age buckets.
	AgeBuckets(ageBuckets int) SummaryBuilder

	// Build creates the summary.
	Build() Summary
}

// MetricsRegistry is a registry for metrics.
type MetricsRegistry interface {
	// Register a metric with the registry.
	Register(metric Metric) error

	// Create a new counter builder.
	NewCounter() CounterBuilder

	// Create a new gauge builder.
	NewGauge() GaugeBuilder

	// Create a new histogram builder.
	NewHistogram() HistogramBuilder

	// Create a new summary builder.
	NewSummary() SummaryBuilder
}

// MetricsMiddleware is a generic middleware for collecting metrics.
// T is the UserID type (comparable), U is the User object type (any).
type MetricsMiddleware[T comparable, U any] interface {
	// Wrap an HTTP handler with metrics collection.
	Handler(name string, handler http.Handler) http.Handler

	// Configure the middleware.
	Configure(config MetricsMiddlewareConfig) MetricsMiddleware[T, U]

	// Add a filter to the middleware.
	WithFilter(filter MetricsFilter) MetricsMiddleware[T, U]

	// Add a sampler to the middleware.
	WithSampler(sampler MetricsSampler) MetricsMiddleware[T, U]
}

// MetricsMiddlewareConfig is the configuration for metrics middleware.
// This config itself doesn't need to be generic, as the T and U types
// are relevant to the middleware instance, not the configuration values.
type MetricsMiddlewareConfig struct {
	// EnableLatency enables latency metrics.
	EnableLatency bool

	// EnableThroughput enables throughput metrics.
	EnableThroughput bool

	// EnableQPS enables queries per second metrics.
	EnableQPS bool

	// EnableErrors enables error metrics.
	EnableErrors bool

	// SamplingRate is the rate at which to sample requests.
	SamplingRate float64

	// DefaultTags are tags to add to all metrics.
	DefaultTags Tags
}

// MetricsFilter is a filter for metrics collection.
type MetricsFilter interface {
	// Filter returns true if metrics should be collected for the request.
	Filter(r *http.Request) bool
}

// MetricsSampler is a sampler for metrics collection.
type MetricsSampler interface {
	// Sample returns true if the request should be sampled.
	Sample() bool
}

// RandomSampler is a sampler that randomly samples requests.
type RandomSampler struct {
	rate float64
}

// NewRandomSampler creates a new random sampler.
func NewRandomSampler(rate float64) *RandomSampler {
	return &RandomSampler{
		rate: rate,
	}
}

// Sample returns true if the request should be sampled.
func (s *RandomSampler) Sample() bool {
	return s.rate >= 1.0
}

// MetricsMiddlewareImpl is a concrete generic implementation of the MetricsMiddleware interface.
// T is the UserID type (comparable), U is the User object type (any).
type MetricsMiddlewareImpl[T comparable, U any] struct {
	registry MetricsRegistry
	config   MetricsMiddlewareConfig
	filter   MetricsFilter
	sampler  MetricsSampler
}

// NewMetricsMiddleware creates a new generic MetricsMiddlewareImpl.
// T is the UserID type (comparable), U is the User object type (any).
func NewMetricsMiddleware[T comparable, U any](registry MetricsRegistry, config MetricsMiddlewareConfig) *MetricsMiddlewareImpl[T, U] {
	return &MetricsMiddlewareImpl[T, U]{
		registry: registry,
		config:   config,
	}
}

// Configure configures the middleware.
func (m *MetricsMiddlewareImpl[T, U]) Configure(config MetricsMiddlewareConfig) MetricsMiddleware[T, U] {
	m.config = config
	return m
}

// WithFilter adds a filter to the middleware.
func (m *MetricsMiddlewareImpl[T, U]) WithFilter(filter MetricsFilter) MetricsMiddleware[T, U] {
	m.filter = filter
	return m
}

// WithSampler adds a sampler to the middleware.
func (m *MetricsMiddlewareImpl[T, U]) WithSampler(sampler MetricsSampler) MetricsMiddleware[T, U] {
	m.sampler = sampler
	return m
}

// Handler method needs to be moved to handler_method.go as it's part of the implementation.
// We will update it there to use the generic types T and U.
