package metrics

import (
	"net/http"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/scontext" // Keep scontext import
)

// Handler wraps an HTTP handler with metrics collection. It captures metrics such as
// request latency, throughput, QPS, and errors based on the middleware configuration.
// The metrics are collected using the registry provided to the middleware.
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
			// Create a route-specific histogram for request latency
			latency := m.registry.NewHistogram().
				Name("request_latency_seconds").
				Description("Request latency in seconds").
				Tag("route", routeIdentifier).
				Build()

			// Observe the request latency
			latency.Observe(duration.Seconds())

			// Create a global histogram for total request latency across all routes
			totalLatency := m.registry.NewHistogram().
				Name("request_latency_seconds_total").
				Description("Total request latency in seconds across all routes").
				Build()

			// Observe the total latency
			totalLatency.Observe(duration.Seconds())
		}

		if m.config.EnableThroughput {
			// Create a route-specific counter for request throughput
			throughput := m.registry.NewCounter().
				Name("request_throughput_bytes").
				Description("Request throughput in bytes").
				Tag("route", routeIdentifier).
				Build()

			// Add the request size
			if r.ContentLength > 0 {
				throughput.Add(float64(r.ContentLength))

				// Also track total throughput across all routes
				totalThroughput := m.registry.NewCounter().
					Name("request_throughput_bytes_total").
					Description("Total request throughput in bytes across all routes").
					Build()

				totalThroughput.Add(float64(r.ContentLength))
			}
		}

		if m.config.EnableQPS {
			// Create a route-specific counter for requests per second
			qps := m.registry.NewCounter().
				Name("requests_total").
				Description("Total number of requests").
				Tag("route", routeIdentifier).
				Build()

			// Increment the counter
			qps.Inc()

			// Create a global counter for total requests across all routes
			totalQps := m.registry.NewCounter().
				Name("all_requests_total").
				Description("Total number of requests across all routes").
				Build()

			// Increment the total counter
			totalQps.Inc()
		}

		if m.config.EnableErrors && rw.statusCode >= 400 {
			// Create a route-specific counter for errors
			errors := m.registry.NewCounter().
				Name("request_errors_total").
				Description("Total number of request errors").
				Tag("route", routeIdentifier).
				Tag("status_code", http.StatusText(rw.statusCode)).
				Build()

			// Increment the counter
			errors.Inc()

			// Create a global counter for errors across all routes
			totalErrors := m.registry.NewCounter().
				Name("all_request_errors_total").
				Description("Total number of request errors across all routes").
				Tag("status_code", http.StatusText(rw.statusCode)).
				Build()

			// Increment the total counter
			totalErrors.Inc()
		}
	})
}

// Removed the standalone getRouteTemplateFromRequest function as it's now handled directly within Handler using generics.

// responseWriter is a wrapper around http.ResponseWriter that captures the status code.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

// WriteHeader captures the status code and calls the underlying ResponseWriter.
func (rw *responseWriter) WriteHeader(statusCode int) {
	rw.statusCode = statusCode
	rw.ResponseWriter.WriteHeader(statusCode)
}
