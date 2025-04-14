package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/metrics"
	"github.com/Suhaibinator/SRouter/pkg/router"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	"go.uber.org/zap"
)

// PrometheusRegistry implements the metrics.MetricsRegistry interface.
type PrometheusRegistry struct {
	registry *prometheus.Registry
}

// NewPrometheusRegistry creates a new PrometheusRegistry.
func NewPrometheusRegistry() *PrometheusRegistry {
	return &PrometheusRegistry{
		registry: prometheus.NewRegistry(),
	}
}

// Register registers a metric with the registry.
func (r *PrometheusRegistry) Register(metric metrics.Metric) error {
	// This is a no-op since Prometheus metrics are registered when they are created
	return nil
}

// Get gets a metric by name.
func (r *PrometheusRegistry) Get(name string) (metrics.Metric, bool) {
	// This is not directly supported by Prometheus
	return nil, false
}

// Unregister unregisters a metric from the registry.
func (r *PrometheusRegistry) Unregister(name string) bool {
	// This is not directly supported by Prometheus
	return false
}

// Clear clears all metrics from the registry.
func (r *PrometheusRegistry) Clear() {
	// This is not directly supported by Prometheus
}

// WithTags creates a new registry with the given tags.
func (r *PrometheusRegistry) WithTags(tags metrics.Tags) metrics.MetricsRegistry {
	// Create a new registry with the same underlying Prometheus registry
	return r
}

// NewCounter creates a new counter builder.
func (r *PrometheusRegistry) NewCounter() metrics.CounterBuilder {
	return &PrometheusCounterBuilder{
		registry: r,
		labels:   make(prometheus.Labels),
	}
}

// NewGauge creates a new gauge builder.
func (r *PrometheusRegistry) NewGauge() metrics.GaugeBuilder {
	return &PrometheusGaugeBuilder{
		registry: r,
		labels:   make(prometheus.Labels),
	}
}

// NewHistogram creates a new histogram builder.
func (r *PrometheusRegistry) NewHistogram() metrics.HistogramBuilder {
	return &PrometheusHistogramBuilder{
		registry: r,
		labels:   make(prometheus.Labels),
		buckets:  prometheus.DefBuckets,
	}
}

// NewSummary creates a new summary builder.
func (r *PrometheusRegistry) NewSummary() metrics.SummaryBuilder {
	return &PrometheusSummaryBuilder{
		registry:   r,
		labels:     make(prometheus.Labels),
		objectives: map[float64]float64{0.5: 0.05, 0.9: 0.01, 0.99: 0.001},
	}
}

// Handler returns an HTTP handler for exposing metrics.
func (r *PrometheusRegistry) Handler() http.Handler {
	return promhttp.HandlerFor(r.registry, promhttp.HandlerOpts{})
}

// PrometheusCounterBuilder implements the metrics.CounterBuilder interface.
type PrometheusCounterBuilder struct {
	registry    *PrometheusRegistry
	name        string
	description string
	labels      prometheus.Labels
}

// Name sets the counter name.
func (b *PrometheusCounterBuilder) Name(name string) metrics.CounterBuilder {
	b.name = name
	return b
}

// Description sets the counter description.
func (b *PrometheusCounterBuilder) Description(desc string) metrics.CounterBuilder {
	b.description = desc
	return b
}

// Tag adds a tag to the counter.
func (b *PrometheusCounterBuilder) Tag(key, value string) metrics.CounterBuilder {
	b.labels[key] = value
	return b
}

// Build creates the counter.
func (b *PrometheusCounterBuilder) Build() metrics.Counter {
	counter := prometheus.NewCounter(prometheus.CounterOpts{
		Name:        b.name,
		Help:        b.description,
		ConstLabels: b.labels,
	})

	b.registry.registry.MustRegister(counter)

	return &PrometheusCounter{
		counter:     counter,
		name:        b.name,
		description: b.description,
		tags:        convertLabelsToTags(b.labels),
	}
}

// PrometheusCounter implements the metrics.Counter interface.
type PrometheusCounter struct {
	counter     prometheus.Counter
	name        string
	description string
	tags        metrics.Tags
}

// Name returns the counter name.
func (c *PrometheusCounter) Name() string {
	return c.name
}

// Description returns the counter description.
func (c *PrometheusCounter) Description() string {
	return c.description
}

// Type returns the metric type.
func (c *PrometheusCounter) Type() metrics.MetricType {
	return metrics.CounterType
}

// Tags returns the metric tags.
func (c *PrometheusCounter) Tags() metrics.Tags {
	return c.tags
}

// WithTags returns a new metric with the given tags.
func (c *PrometheusCounter) WithTags(tags metrics.Tags) metrics.Metric {
	// This is not directly supported by Prometheus
	return c
}

// Inc increments the counter by 1.
func (c *PrometheusCounter) Inc() {
	c.counter.Inc()
}

// Add adds the given value to the counter.
func (c *PrometheusCounter) Add(value float64) {
	c.counter.Add(value)
}

// Value returns the current value of the counter.
func (c *PrometheusCounter) Value() float64 {
	// This is not directly supported by Prometheus
	return 0
}

// PrometheusGaugeBuilder implements the metrics.GaugeBuilder interface.
type PrometheusGaugeBuilder struct {
	registry    *PrometheusRegistry
	name        string
	description string
	labels      prometheus.Labels
}

// Name sets the gauge name.
func (b *PrometheusGaugeBuilder) Name(name string) metrics.GaugeBuilder {
	b.name = name
	return b
}

// Description sets the gauge description.
func (b *PrometheusGaugeBuilder) Description(desc string) metrics.GaugeBuilder {
	b.description = desc
	return b
}

// Tag adds a tag to the gauge.
func (b *PrometheusGaugeBuilder) Tag(key, value string) metrics.GaugeBuilder {
	b.labels[key] = value
	return b
}

// Build creates the gauge.
func (b *PrometheusGaugeBuilder) Build() metrics.Gauge {
	gauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        b.name,
		Help:        b.description,
		ConstLabels: b.labels,
	})

	b.registry.registry.MustRegister(gauge)

	return &PrometheusGauge{
		gauge:       gauge,
		name:        b.name,
		description: b.description,
		tags:        convertLabelsToTags(b.labels),
	}
}

// PrometheusGauge implements the metrics.Gauge interface.
type PrometheusGauge struct {
	gauge       prometheus.Gauge
	name        string
	description string
	tags        metrics.Tags
}

// Name returns the gauge name.
func (g *PrometheusGauge) Name() string {
	return g.name
}

// Description returns the gauge description.
func (g *PrometheusGauge) Description() string {
	return g.description
}

// Type returns the metric type.
func (g *PrometheusGauge) Type() metrics.MetricType {
	return metrics.GaugeType
}

// Tags returns the metric tags.
func (g *PrometheusGauge) Tags() metrics.Tags {
	return g.tags
}

// WithTags returns a new metric with the given tags.
func (g *PrometheusGauge) WithTags(tags metrics.Tags) metrics.Metric {
	// This is not directly supported by Prometheus
	return g
}

// Set sets the gauge to the given value.
func (g *PrometheusGauge) Set(value float64) {
	g.gauge.Set(value)
}

// Inc increments the gauge by 1.
func (g *PrometheusGauge) Inc() {
	g.gauge.Inc()
}

// Dec decrements the gauge by 1.
func (g *PrometheusGauge) Dec() {
	g.gauge.Dec()
}

// Add adds the given value to the gauge.
func (g *PrometheusGauge) Add(value float64) {
	g.gauge.Add(value)
}

// Sub subtracts the given value from the gauge.
func (g *PrometheusGauge) Sub(value float64) {
	g.gauge.Sub(value)
}

// Value returns the current value of the gauge.
func (g *PrometheusGauge) Value() float64 {
	// This is not directly supported by Prometheus
	return 0
}

// PrometheusHistogramBuilder implements the metrics.HistogramBuilder interface.
type PrometheusHistogramBuilder struct {
	registry    *PrometheusRegistry
	name        string
	description string
	labels      prometheus.Labels
	buckets     []float64
}

// Name sets the histogram name.
func (b *PrometheusHistogramBuilder) Name(name string) metrics.HistogramBuilder {
	b.name = name
	return b
}

// Description sets the histogram description.
func (b *PrometheusHistogramBuilder) Description(desc string) metrics.HistogramBuilder {
	b.description = desc
	return b
}

// Tag adds a tag to the histogram.
func (b *PrometheusHistogramBuilder) Tag(key, value string) metrics.HistogramBuilder {
	b.labels[key] = value
	return b
}

// Buckets sets the bucket boundaries.
func (b *PrometheusHistogramBuilder) Buckets(buckets []float64) metrics.HistogramBuilder {
	b.buckets = buckets
	return b
}

// Build creates the histogram.
func (b *PrometheusHistogramBuilder) Build() metrics.Histogram {
	histogram := prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:        b.name,
		Help:        b.description,
		ConstLabels: b.labels,
		Buckets:     b.buckets,
	})

	b.registry.registry.MustRegister(histogram)

	return &PrometheusHistogram{
		histogram:   histogram,
		name:        b.name,
		description: b.description,
		tags:        convertLabelsToTags(b.labels),
		buckets:     b.buckets,
	}
}

// PrometheusHistogram implements the metrics.Histogram interface.
type PrometheusHistogram struct {
	histogram   prometheus.Histogram
	name        string
	description string
	tags        metrics.Tags
	buckets     []float64
}

// Name returns the histogram name.
func (h *PrometheusHistogram) Name() string {
	return h.name
}

// Description returns the histogram description.
func (h *PrometheusHistogram) Description() string {
	return h.description
}

// Type returns the metric type.
func (h *PrometheusHistogram) Type() metrics.MetricType {
	return metrics.HistogramType
}

// Tags returns the metric tags.
func (h *PrometheusHistogram) Tags() metrics.Tags {
	return h.tags
}

// WithTags returns a new metric with the given tags.
func (h *PrometheusHistogram) WithTags(tags metrics.Tags) metrics.Metric {
	// This is not directly supported by Prometheus
	return h
}

// Observe adds a single observation to the histogram.
func (h *PrometheusHistogram) Observe(value float64) {
	h.histogram.Observe(value)
}

// Buckets returns the bucket boundaries.
func (h *PrometheusHistogram) Buckets() []float64 {
	return h.buckets
}

// PrometheusSummaryBuilder implements the metrics.SummaryBuilder interface.
type PrometheusSummaryBuilder struct {
	registry    *PrometheusRegistry
	name        string
	description string
	labels      prometheus.Labels
	objectives  map[float64]float64
	maxAge      time.Duration
	ageBuckets  int
}

// Name sets the summary name.
func (b *PrometheusSummaryBuilder) Name(name string) metrics.SummaryBuilder {
	b.name = name
	return b
}

// Description sets the summary description.
func (b *PrometheusSummaryBuilder) Description(desc string) metrics.SummaryBuilder {
	b.description = desc
	return b
}

// Tag adds a tag to the summary.
func (b *PrometheusSummaryBuilder) Tag(key, value string) metrics.SummaryBuilder {
	b.labels[key] = value
	return b
}

// Objectives sets the quantile objectives.
func (b *PrometheusSummaryBuilder) Objectives(objectives map[float64]float64) metrics.SummaryBuilder {
	b.objectives = objectives
	return b
}

// MaxAge sets the maximum age of observations.
func (b *PrometheusSummaryBuilder) MaxAge(maxAge time.Duration) metrics.SummaryBuilder {
	b.maxAge = maxAge
	return b
}

// AgeBuckets sets the number of age buckets.
func (b *PrometheusSummaryBuilder) AgeBuckets(ageBuckets int) metrics.SummaryBuilder {
	b.ageBuckets = ageBuckets
	return b
}

// Build creates the summary.
func (b *PrometheusSummaryBuilder) Build() metrics.Summary {
	summary := prometheus.NewSummary(prometheus.SummaryOpts{
		Name:        b.name,
		Help:        b.description,
		ConstLabels: b.labels,
		Objectives:  b.objectives,
		MaxAge:      b.maxAge,
		AgeBuckets:  uint32(b.ageBuckets),
	})

	b.registry.registry.MustRegister(summary)

	return &PrometheusSummary{
		summary:     summary,
		name:        b.name,
		description: b.description,
		tags:        convertLabelsToTags(b.labels),
		objectives:  b.objectives,
	}
}

// PrometheusSummary implements the metrics.Summary interface.
type PrometheusSummary struct {
	summary     prometheus.Summary
	name        string
	description string
	tags        metrics.Tags
	objectives  map[float64]float64
}

// Name returns the summary name.
func (s *PrometheusSummary) Name() string {
	return s.name
}

// Description returns the summary description.
func (s *PrometheusSummary) Description() string {
	return s.description
}

// Type returns the metric type.
func (s *PrometheusSummary) Type() metrics.MetricType {
	return metrics.SummaryType
}

// Tags returns the metric tags.
func (s *PrometheusSummary) Tags() metrics.Tags {
	return s.tags
}

// WithTags returns a new metric with the given tags.
func (s *PrometheusSummary) WithTags(tags metrics.Tags) metrics.Metric {
	// This is not directly supported by Prometheus
	return s
}

// Observe adds a single observation to the summary.
func (s *PrometheusSummary) Observe(value float64) {
	s.summary.Observe(value)
}

// Objectives returns the quantile objectives.
func (s *PrometheusSummary) Objectives() map[float64]float64 {
	return s.objectives
}

// Helper function to convert Prometheus labels to metrics.Tags
func convertLabelsToTags(labels prometheus.Labels) metrics.Tags {
	tags := make(metrics.Tags)
	for k, v := range labels {
		tags[k] = v
	}
	return tags
}

func main() {
	// Create a logger
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	// Create a Prometheus registry
	registry := NewPrometheusRegistry()

	// Create a router configuration with metrics enabled
	routerConfig := router.RouterConfig{
		ServiceName:       "prometheus-service", // Added ServiceName
		Logger:            logger,
		GlobalTimeout:     2 * time.Second,
		GlobalMaxBodySize: 1 << 20, // 1 MB
		EnableMetrics:     true,
		MetricsConfig: &router.MetricsConfig{
			Collector:        registry,
			Namespace:        "myapp",
			Subsystem:        "api",
			EnableLatency:    true,
			EnableThroughput: true,
			EnableQPS:        true,
			EnableErrors:     true,
		},
		SubRouters: []router.SubRouterConfig{
			{
				PathPrefix: "/api",
				Routes: []any{ // Changed to []any
					router.RouteConfigBase{
						Path:    "/hello",
						Methods: []router.HttpMethod{router.MethodGet},
						Handler: func(w http.ResponseWriter, r *http.Request) {
							w.Header().Set("Content-Type", "application/json")
							w.Write([]byte(`{"message":"Hello, World!"}`))
						},
					},
					router.RouteConfigBase{ // Add explicit type
						Path:    "/error",
						Methods: []router.HttpMethod{router.MethodGet},
						Handler: func(w http.ResponseWriter, r *http.Request) {
							http.Error(w, "Something went wrong", http.StatusInternalServerError)
						},
					},
				},
			},
		},
	}

	// Define the auth function that takes a context and token and returns a *string and a boolean
	authFunction := func(ctx context.Context, token string) (*string, bool) {
		// This is a simple example, so we'll just validate that the token is not empty
		if token != "" {
			// Return pointer to token as user object
			return &token, true
		}
		return nil, false // Return nil pointer for user
	}

	// Define the function to get the user ID from a *string
	userIdFromUserFunction := func(user *string) string {
		// In this example, we're using the string itself as the ID
		if user == nil {
			return "" // Handle nil pointer case
		}
		return *user // Dereference pointer
	}

	// Create a router with string as both the user ID and user type
	r := router.NewRouter[string, string](routerConfig, authFunction, userIdFromUserFunction)

	// Create a metrics handler
	metricsHandler := registry.Handler()

	// Create a mux to handle both the API and metrics endpoints
	mux := http.NewServeMux()
	mux.Handle("/metrics", metricsHandler)
	mux.Handle("/", r)

	// Start the server
	fmt.Println("Server listening on :8080")
	fmt.Println("API endpoints:")
	fmt.Println("  - GET /api/hello")
	fmt.Println("  - GET /api/error")
	fmt.Println("Metrics endpoint:")
	fmt.Println("  - GET /metrics")
	log.Fatal(http.ListenAndServe(":8080", mux))
}
