package main

import (
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// CustomMetricsRegistry is a simple metrics registry that allows for dependency injection
type CustomMetricsRegistry struct {
	registry     *prometheus.Registry
	counters     map[string]prometheus.Counter
	gauges       map[string]prometheus.Gauge
	histograms   map[string]prometheus.Histogram
	countersMu   sync.RWMutex
	gaugesMu     sync.RWMutex
	histogramsMu sync.RWMutex
}

// NewCustomMetricsRegistry creates a new CustomMetricsRegistry
func NewCustomMetricsRegistry(registry *prometheus.Registry) *CustomMetricsRegistry {
	return &CustomMetricsRegistry{
		registry:   registry,
		counters:   make(map[string]prometheus.Counter),
		gauges:     make(map[string]prometheus.Gauge),
		histograms: make(map[string]prometheus.Histogram),
	}
}

// Counter creates or retrieves a counter
func (r *CustomMetricsRegistry) Counter(name, help string, labels prometheus.Labels) prometheus.Counter {
	r.countersMu.Lock()
	defer r.countersMu.Unlock()

	var key strings.Builder
	key.WriteString(name)
	for k, v := range labels {
		key.WriteString(k)
		key.WriteString(":")
		key.WriteString(v)
	}

	if counter, ok := r.counters[key.String()]; ok {
		return counter
	}

	counter := prometheus.NewCounter(prometheus.CounterOpts{
		Name:        name,
		Help:        help,
		ConstLabels: labels,
	})

	r.registry.MustRegister(counter)
	r.counters[key.String()] = counter

	return counter
}

// Gauge creates or retrieves a gauge
func (r *CustomMetricsRegistry) Gauge(name, help string, labels prometheus.Labels) prometheus.Gauge {
	r.gaugesMu.Lock()
	defer r.gaugesMu.Unlock()

	var key strings.Builder
	key.WriteString(name)
	for k, v := range labels {
		key.WriteString(k)
		key.WriteString(":")
		key.WriteString(v)
	}

	if gauge, ok := r.gauges[key.String()]; ok {
		return gauge
	}

	gauge := prometheus.NewGauge(prometheus.GaugeOpts{
		Name:        name,
		Help:        help,
		ConstLabels: labels,
	})

	r.registry.MustRegister(gauge)
	r.gauges[key.String()] = gauge

	return gauge
}

// Histogram creates or retrieves a histogram
func (r *CustomMetricsRegistry) Histogram(name, help string, labels prometheus.Labels, buckets []float64) prometheus.Histogram {
	r.histogramsMu.Lock()
	defer r.histogramsMu.Unlock()

	var key strings.Builder
	key.WriteString(name)
	for k, v := range labels {
		key.WriteString(k)
		key.WriteString(":")
		key.WriteString(v)
	}

	if histogram, ok := r.histograms[key.String()]; ok {
		return histogram
	}

	if buckets == nil {
		buckets = prometheus.DefBuckets
	}

	histogram := prometheus.NewHistogram(prometheus.HistogramOpts{
		Name:        name,
		Help:        help,
		ConstLabels: labels,
		Buckets:     buckets,
	})

	r.registry.MustRegister(histogram)
	r.histograms[key.String()] = histogram

	return histogram
}

// MetricsCollector is an interface for collecting metrics
type MetricsCollector interface {
	CollectRequestMetrics(handler string, statusCode int, duration time.Duration, contentLength int64)
}

// PrometheusMetricsCollector implements MetricsCollector using Prometheus
type PrometheusMetricsCollector struct {
	registry *CustomMetricsRegistry
}

// NewPrometheusMetricsCollector creates a new PrometheusMetricsCollector
func NewPrometheusMetricsCollector(registry *CustomMetricsRegistry) *PrometheusMetricsCollector {
	return &PrometheusMetricsCollector{
		registry: registry,
	}
}

// CollectRequestMetrics collects metrics for an HTTP request
func (c *PrometheusMetricsCollector) CollectRequestMetrics(handler string, statusCode int, duration time.Duration, contentLength int64) {
	// Request count
	c.registry.Counter(
		"http_requests_total",
		"Total number of HTTP requests",
		prometheus.Labels{
			"handler": handler,
			"status":  http.StatusText(statusCode),
		},
	).Inc()

	// Request duration
	c.registry.Histogram(
		"http_request_duration_seconds",
		"HTTP request duration in seconds",
		prometheus.Labels{
			"handler": handler,
		},
		[]float64{0.01, 0.05, 0.1, 0.5, 1, 2, 5, 10},
	).Observe(duration.Seconds())

	// Request size
	if contentLength > 0 {
		c.registry.Counter(
			"http_request_size_bytes",
			"HTTP request size in bytes",
			prometheus.Labels{
				"handler": handler,
			},
		).Add(float64(contentLength))
	}
}

// MetricsMiddleware is middleware for collecting metrics
type MetricsMiddleware struct {
	collector MetricsCollector
}

// NewMetricsMiddleware creates a new MetricsMiddleware
func NewMetricsMiddleware(collector MetricsCollector) *MetricsMiddleware {
	return &MetricsMiddleware{
		collector: collector,
	}
}

// Middleware wraps an HTTP handler with metrics collection
func (m *MetricsMiddleware) Middleware(handler string, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}
		next.ServeHTTP(rw, r)
		duration := time.Since(start)
		m.collector.CollectRequestMetrics(handler, rw.statusCode, duration, r.ContentLength)
	})
}

// responseWriter is a wrapper around http.ResponseWriter that captures the status code
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

// WriteHeader captures the status code and calls the underlying ResponseWriter
func (rw *responseWriter) WriteHeader(statusCode int) {
	rw.statusCode = statusCode
	rw.ResponseWriter.WriteHeader(statusCode)
}
