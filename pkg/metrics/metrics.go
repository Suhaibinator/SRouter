// Package metrics defines backend-neutral metric instruments, builders, registries,
// and HTTP request middleware for SRouter.
package metrics

import (
	"math/rand"
	"net/http"
	"sync"
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

// MetricsRegistry creates and registers metric instruments. A backend may register
// an instrument during Build, so the exact Register semantics are backend-specific.
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

// MetricsMiddleware wraps HTTP handlers with request metric collection.
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

// MetricsMiddlewareConfig configures the built-in request metrics middleware.
type MetricsMiddlewareConfig struct {
	// EnableLatency enables latency metrics.
	EnableLatency bool

	// EnableThroughput records positive request Content-Length values. It does
	// not measure response bytes or calculate a bytes-per-second rate.
	EnableThroughput bool

	// EnableQPS enables cumulative request counters. Derive a per-second rate
	// in the metrics backend.
	EnableQPS bool

	// EnableErrors enables error metrics.
	EnableErrors bool

	// SamplingRate installs a RandomSampler only when strictly between 0 and 1.
	// Values outside that interval mean no configured sampler, so all requests
	// that pass the filter are collected.
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

// RandomSampler independently samples requests at a configured probability.
type RandomSampler struct {
	rate float64
	rng  *rand.Rand
}

// NewRandomSampler creates a sampler with the given probability. Rates at or
// below 0 reject every sample; rates at or above 1 accept every sample.
func NewRandomSampler(rate float64) *RandomSampler {
	return &RandomSampler{
		rate: rate,
		rng:  rand.New(rand.NewSource(time.Now().UnixNano())),
	}
}

// NewRandomSamplerWithRand returns a RandomSampler that uses r for deterministic
// sampling. The caller must provide a non-nil rand.Rand. Rates at or below 0
// reject every sample; rates at or above 1 accept every sample.
func NewRandomSamplerWithRand(rate float64, r *rand.Rand) *RandomSampler {
	return &RandomSampler{
		rate: rate,
		rng:  r,
	}
}

// Sample reports whether the next request should be included.
func (s *RandomSampler) Sample() bool {
	switch {
	case s.rate <= 0:
		return false
	case s.rate >= 1:
		return true
	default:
		return s.rng.Float64() < s.rate
	}
}

// MetricsMiddlewareImpl is the built-in MetricsMiddleware implementation.
type MetricsMiddlewareImpl[T comparable, U any] struct {
	registry    MetricsRegistry
	config      MetricsMiddlewareConfig
	filter      MetricsFilter
	sampler     MetricsSampler
	metricCache sync.Map // cache key (string) -> once-protected Counter/Histogram builder
}

// samplerFromConfig returns a sampler implementing the configured SamplingRate,
// or nil when no sampling is needed (rate <= 0 means "not configured" and
// rate >= 1 means "always sample", both of which need no sampler).
func samplerFromConfig(config MetricsMiddlewareConfig) MetricsSampler {
	if config.SamplingRate > 0 && config.SamplingRate < 1 {
		return NewRandomSampler(config.SamplingRate)
	}
	return nil
}

// NewMetricsMiddleware creates a request metrics middleware backed by registry.
// A SamplingRate strictly between 0 and 1 installs a RandomSampler; WithSampler
// can replace it.
func NewMetricsMiddleware[T comparable, U any](registry MetricsRegistry, config MetricsMiddlewareConfig) *MetricsMiddlewareImpl[T, U] {
	return &MetricsMiddlewareImpl[T, U]{
		registry: registry,
		config:   config,
		sampler:  samplerFromConfig(config),
	}
}

// Configure replaces the middleware configuration and derives a new sampler
// from SamplingRate. Call it before serving requests: already-built cached
// instruments retain their original names and tags. Call WithSampler after
// Configure when custom sampling behavior is needed.
func (m *MetricsMiddlewareImpl[T, U]) Configure(config MetricsMiddlewareConfig) MetricsMiddleware[T, U] {
	m.config = config
	m.sampler = samplerFromConfig(config)
	return m
}

// WithFilter sets the request filter. A nil filter collects every request that
// passes sampling.
func (m *MetricsMiddlewareImpl[T, U]) WithFilter(filter MetricsFilter) MetricsMiddleware[T, U] {
	m.filter = filter
	return m
}

// WithSampler sets the request sampler. A nil sampler collects every request
// that passes filtering.
func (m *MetricsMiddlewareImpl[T, U]) WithSampler(sampler MetricsSampler) MetricsMiddleware[T, U] {
	m.sampler = sampler
	return m
}
