package metrics

import (
	"bufio"
	"net"
	"net/http"
	"strconv"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/scontext" // Keep scontext import
)

// cachedHistogram returns the histogram for the given cache key, building it at
// most once. Building (and registering with the backend) per request would
// allocate and contend the registry lock on every request.
func (m *MetricsMiddlewareImpl[T, U]) cachedHistogram(key string, build func() Histogram) Histogram {
	if v, ok := m.metricCache.Load(key); ok {
		return v.(Histogram)
	}
	actual, _ := m.metricCache.LoadOrStore(key, build())
	return actual.(Histogram)
}

// cachedCounter returns the counter for the given cache key, building it at most once.
func (m *MetricsMiddlewareImpl[T, U]) cachedCounter(key string, build func() Counter) Counter {
	if v, ok := m.metricCache.Load(key); ok {
		return v.(Counter)
	}
	actual, _ := m.metricCache.LoadOrStore(key, build())
	return actual.(Counter)
}

// taggedHistogram starts a histogram builder with the configured default tags applied.
func (m *MetricsMiddlewareImpl[T, U]) taggedHistogram(name, desc string) HistogramBuilder {
	b := m.registry.NewHistogram().Name(name).Description(desc)
	for k, v := range m.config.DefaultTags {
		if v != "" {
			b = b.Tag(k, v)
		}
	}
	return b
}

// taggedCounter starts a counter builder with the configured default tags applied.
func (m *MetricsMiddlewareImpl[T, U]) taggedCounter(name, desc string) CounterBuilder {
	b := m.registry.NewCounter().Name(name).Description(desc)
	for k, v := range m.config.DefaultTags {
		if v != "" {
			b = b.Tag(k, v)
		}
	}
	return b
}

// Handler wraps an HTTP handler with metrics collection. It captures metrics such as
// request latency, throughput, QPS, and errors based on the middleware configuration.
// The metrics are collected using the registry provided to the middleware, are tagged
// with the configured DefaultTags, and are built once per route and then cached.
// The 'name' parameter can be used as a fallback identifier if route template information is not available.
// This is now a method on the generic MetricsMiddlewareImpl[T, U].
func (m *MetricsMiddlewareImpl[T, U]) Handler(name string, handler http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check if we should collect metrics for this request
		if m.filter != nil && !m.filter.Filter(r) {
			handler.ServeHTTP(w, r)
			return
		}

		// Check if we should sample this request
		if m.sampler != nil && !m.sampler.Sample() {
			handler.ServeHTTP(w, r)
			return
		}

		// Create a response writer wrapper to capture the status code
		rw := &responseWriter{ResponseWriter: w, statusCode: http.StatusOK}

		// Start the timer
		start := time.Now()

		// Call the handler
		handler.ServeHTTP(rw, r)

		// Calculate the duration
		duration := time.Since(start)

		// Get the route template from the context if available, using the correct generic types T and U
		routeIdentifier := name // Default to the name parameter if no route template is found
		routeTemplate, hasTemplate := scontext.GetRouteTemplateFromRequest[T, U](r)
		if hasTemplate {
			routeIdentifier = routeTemplate
		}

		// Collect metrics
		if m.config.EnableLatency {
			// Route-specific histogram for request latency
			latency := m.cachedHistogram("latency|"+routeIdentifier, func() Histogram {
				return m.taggedHistogram("request_latency_seconds", "Request latency in seconds").
					Tag("route", routeIdentifier).
					Build()
			})
			latency.Observe(duration.Seconds())

			// Global histogram for request latency across all routes.
			// (Named all_* rather than *_total: the _total suffix is reserved
			// for counters in Prometheus naming conventions.)
			totalLatency := m.cachedHistogram("latency|all", func() Histogram {
				return m.taggedHistogram("all_request_latency_seconds", "Request latency in seconds across all routes").
					Build()
			})
			totalLatency.Observe(duration.Seconds())
		}

		if m.config.EnableThroughput && r.ContentLength > 0 {
			// Route-specific counter for request throughput
			throughput := m.cachedCounter("throughput|"+routeIdentifier, func() Counter {
				return m.taggedCounter("request_throughput_bytes", "Request throughput in bytes").
					Tag("route", routeIdentifier).
					Build()
			})
			throughput.Add(float64(r.ContentLength))

			// Also track total throughput across all routes
			totalThroughput := m.cachedCounter("throughput|all", func() Counter {
				return m.taggedCounter("request_throughput_bytes_total", "Total request throughput in bytes across all routes").
					Build()
			})
			totalThroughput.Add(float64(r.ContentLength))
		}

		if m.config.EnableQPS {
			// Route-specific counter for requests
			qps := m.cachedCounter("qps|"+routeIdentifier, func() Counter {
				return m.taggedCounter("requests_total", "Total number of requests").
					Tag("route", routeIdentifier).
					Build()
			})
			qps.Inc()

			// Global counter for total requests across all routes
			totalQps := m.cachedCounter("qps|all", func() Counter {
				return m.taggedCounter("all_requests_total", "Total number of requests across all routes").
					Build()
			})
			totalQps.Inc()
		}

		if m.config.EnableErrors && rw.statusCode >= 400 {
			// Record the numeric status code (e.g. "404"), not the status text.
			statusCode := strconv.Itoa(rw.statusCode)

			// Route-specific counter for errors
			errors := m.cachedCounter("errors|"+routeIdentifier+"|"+statusCode, func() Counter {
				return m.taggedCounter("request_errors_total", "Total number of request errors").
					Tag("route", routeIdentifier).
					Tag("status_code", statusCode).
					Build()
			})
			errors.Inc()

			// Global counter for errors across all routes
			totalErrors := m.cachedCounter("errors|all|"+statusCode, func() Counter {
				return m.taggedCounter("all_request_errors_total", "Total number of request errors across all routes").
					Tag("status_code", statusCode).
					Build()
			})
			totalErrors.Inc()
		}
	})
}

// Removed the standalone getRouteTemplateFromRequest function as it's now handled directly within Handler using generics.

// responseWriter is a wrapper around http.ResponseWriter that captures the HTTP status code.
// It intercepts calls to WriteHeader to record the status code for metrics collection.
// If WriteHeader is not called explicitly, it defaults to 200 (OK).
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

// WriteHeader captures the HTTP status code and forwards it to the underlying ResponseWriter.
// The captured status code is used for metrics collection, particularly for error rate tracking.
// This method ensures the status code is recorded even if the handler sets it explicitly.
func (rw *responseWriter) WriteHeader(statusCode int) {
	rw.statusCode = statusCode
	rw.ResponseWriter.WriteHeader(statusCode)
}

// Hijack delegates to the underlying ResponseWriter when it supports http.Hijacker.
// This is required for WebSocket upgrades to work through the metrics wrapper.
func (rw *responseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := rw.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	return h.Hijack()
}
