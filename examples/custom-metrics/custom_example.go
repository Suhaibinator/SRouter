package main

import (
	"fmt"
	"net/http"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// CustomMetricsExample demonstrates how to use custom metrics with SRouter
func CustomMetricsExample() {
	// Create a new Prometheus registry
	registry := prometheus.NewRegistry()

	// Create a custom metrics registry that uses the Prometheus registry
	metricsRegistry := NewCustomMetricsRegistry(registry)

	// Create a metrics collector
	collector := NewPrometheusMetricsCollector(metricsRegistry)

	// Create a metrics middleware
	middleware := NewMetricsMiddleware(collector)

	// Create some example metrics
	requestCounter := metricsRegistry.Counter(
		"example_requests_total",
		"Total number of example requests",
		prometheus.Labels{"service": "example"},
	)

	requestGauge := metricsRegistry.Gauge(
		"example_active_requests",
		"Number of active example requests",
		prometheus.Labels{"service": "example"},
	)

	requestDuration := metricsRegistry.Histogram(
		"example_request_duration_seconds",
		"Example request duration in seconds",
		prometheus.Labels{"service": "example"},
		[]float64{0.01, 0.05, 0.1, 0.5, 1, 2, 5},
	)

	// Create a handler that uses the metrics
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		requestGauge.Inc()
		defer requestGauge.Dec()

		start := time.Now()
		defer func() {
			requestDuration.Observe(time.Since(start).Seconds())
		}()

		requestCounter.Inc()

		// Simulate some work
		time.Sleep(100 * time.Millisecond)

		_, _ = fmt.Fprintf(w, "Hello, World!")
	})

	// Wrap the handler with the metrics middleware
	wrappedHandler := middleware.Middleware("example", handler)

	// Register the handler
	http.Handle("/", wrappedHandler)

	// Register the metrics handler
	http.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))

	fmt.Println("Starting server on :8080")
	fmt.Println("Visit http://localhost:8080/ to generate metrics")
	fmt.Println("Visit http://localhost:8080/metrics to view metrics")
}
