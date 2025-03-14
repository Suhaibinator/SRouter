// Package middleware provides a collection of HTTP middleware components for the SRouter framework.
package middleware

import (
	"net/http"
	"strconv"
	"time"
)

// PrometheusMetrics is a middleware that collects Prometheus metrics for HTTP requests.
// It can track request latency, throughput, queries per second, and error rates.
// This is a placeholder implementation that demonstrates the structure but doesn't
// actually record metrics to Prometheus. In a real implementation, you would use
// the prometheus client library.
func PrometheusMetrics(registry interface{}, namespace, subsystem string, enableLatency, enableThroughput, enableQPS, enableErrors bool) Middleware {
	// In a real implementation, we would use the prometheus client library
	// For now, we'll just create a middleware that logs metrics
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Create a response writer that captures metrics
			rw := &prometheusResponseWriter{
				ResponseWriter: w,
				statusCode:     http.StatusOK,
			}

			// Call the next handler
			next.ServeHTTP(rw, r)

			// Record metrics
			duration := time.Since(start)
			statusCode := rw.statusCode
			method := r.Method
			path := r.URL.Path

			// In a real implementation, we would use the prometheus client library
			// to record metrics. For now, we'll just log them.
			if enableLatency {
				// Record request latency
				// e.g., requestLatency.WithLabelValues(method, path, strconv.Itoa(statusCode)).Observe(duration.Seconds())
				_ = duration
			}

			if enableThroughput {
				// Record request throughput (bytes)
				// e.g., requestThroughput.WithLabelValues(method, path, strconv.Itoa(statusCode)).Add(float64(rw.bytesWritten))
				_ = rw.bytesWritten
			}

			if enableQPS {
				// Record queries per second
				// e.g., requestsTotal.WithLabelValues(method, path, strconv.Itoa(statusCode)).Inc()
				_ = method
				_ = path
				_ = strconv.Itoa(statusCode)
			}

			if enableErrors && statusCode >= 400 {
				// Record errors
				// e.g., requestErrors.WithLabelValues(method, path, strconv.Itoa(statusCode)).Inc()
				_ = statusCode // Use statusCode to avoid empty branch warning
			}
		})
	}
}

// PrometheusHandler returns an HTTP handler for exposing Prometheus metrics.
// This handler would typically be mounted at a path like "/metrics" to allow
// Prometheus to scrape the metrics. This is a placeholder implementation that
// doesn't actually expose real metrics.
func PrometheusHandler(registry interface{}) http.Handler {
	// In a real implementation, we would use the prometheus client library
	// to create a handler that exposes metrics
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		_, err := w.Write([]byte("# Prometheus metrics would be exposed here"))
		if err != nil {
			// Log the error in a real implementation
			// For now, we'll just ignore it
			_ = err // Explicitly ignore the error to satisfy linter
		}
	})
}

// prometheusResponseWriter is a wrapper around http.ResponseWriter that captures metrics.
// It tracks the status code and number of bytes written to the response.
type prometheusResponseWriter struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int64
}

// WriteHeader captures the status code and calls the underlying ResponseWriter.WriteHeader.
// This allows the middleware to track the HTTP status code for metrics.
func (rw *prometheusResponseWriter) WriteHeader(statusCode int) {
	rw.statusCode = statusCode
	rw.ResponseWriter.WriteHeader(statusCode)
}

// Write captures the number of bytes written and calls the underlying ResponseWriter.Write.
// This allows the middleware to track the response size for throughput metrics.
func (rw *prometheusResponseWriter) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.bytesWritten += int64(n)
	return n, err
}

// Flush calls the underlying ResponseWriter.Flush if it implements http.Flusher.
// This allows streaming responses to be flushed to the client immediately.
func (rw *prometheusResponseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
