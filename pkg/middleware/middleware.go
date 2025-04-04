// Package middleware provides a collection of HTTP middleware components for the SRouter framework.
// These middleware components can be used to add functionality such as logging, recovery from panics,
// authentication, request timeouts, and more to your HTTP handlers.
package middleware

import (
	"context"
	"net/http"
	"runtime/debug"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/common"
	"go.uber.org/zap"
)

// Middleware is an alias for the common.Middleware type.
// It represents a function that wraps an http.Handler to provide additional functionality.
type Middleware = common.Middleware

// Chain chains multiple middlewares together into a single middleware.
// The middlewares are applied in reverse order, so the first middleware in the list
// will be the outermost wrapper (the first to process the request and the last to process the response).
func Chain(middlewares ...Middleware) Middleware {
	return func(next http.Handler) http.Handler {
		for i := len(middlewares) - 1; i >= 0; i-- {
			next = middlewares[i](next)
		}
		return next
	}
}

// Recovery is a middleware that recovers from panics in HTTP handlers.
// It logs the panic and stack trace using the provided logger and returns a 500 Internal Server Error response.
// This prevents the server from crashing when a panic occurs in a handler.
func Recovery(logger *zap.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					// Log the panic
					logger.Error("Panic recovered",
						zap.Any("panic", rec),
						zap.String("stack", string(debug.Stack())),
						zap.String("method", r.Method),
						zap.String("path", r.URL.Path),
					)

					// Return a 500 Internal Server Error
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
				}
			}()

			next.ServeHTTP(w, r)
		})
	}
}

// Logging is a middleware that logs HTTP requests and responses.
// It captures the request method, path, status code, and duration.
// The log level is determined by the status code and duration:
// - 500+ status codes are logged at Error level
// - 400-499 status codes are logged at Warn level
// - Requests taking longer than 1 second are logged at Warn level
// - All other requests are logged at Debug level
func Logging(logger *zap.Logger) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()

			// Create a response writer that captures the status code
			rw := &responseWriter{
				ResponseWriter: w,
				statusCode:     http.StatusOK,
			}

			// Call the next handler
			next.ServeHTTP(rw, r)

			// Calculate duration
			duration := time.Since(start)

			// Use appropriate log level based on status code and duration
			if rw.statusCode >= 500 {
				// Server errors at Error level
				logger.Error("Server error",
					zap.String("method", r.Method),
					zap.String("path", r.URL.Path),
					zap.Int("status", rw.statusCode),
					zap.Duration("duration", duration),
					zap.String("remote_addr", r.RemoteAddr),
				)
			} else if rw.statusCode >= 400 {
				// Client errors at Warn level
				logger.Warn("Client error",
					zap.String("method", r.Method),
					zap.String("path", r.URL.Path),
					zap.Int("status", rw.statusCode),
					zap.Duration("duration", duration),
				)
			} else if duration > 1*time.Second {
				// Slow requests at Warn level
				logger.Warn("Slow request",
					zap.String("method", r.Method),
					zap.String("path", r.URL.Path),
					zap.Int("status", rw.statusCode),
					zap.Duration("duration", duration),
				)
			} else {
				// Normal requests at Debug level to avoid log spam
				logger.Debug("Request",
					zap.String("method", r.Method),
					zap.String("path", r.URL.Path),
					zap.Int("status", rw.statusCode),
					zap.Duration("duration", duration),
				)
			}
		})
	}
}

// Authentication function has been moved to auth.go

// MaxBodySize is a middleware that limits the size of the request body.
// It prevents clients from sending excessively large requests that could
// consume too much memory or cause denial of service.
func MaxBodySize(maxSize int64) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Limit the size of the request body
			r.Body = http.MaxBytesReader(w, r.Body, maxSize)

			// Call the next handler
			next.ServeHTTP(w, r)
		})
	}
}

// Timeout is a middleware that sets a timeout for the request processing.
// If the handler takes longer than the specified timeout to respond,
// the middleware will cancel the request context and return a 408 Request Timeout response.
// This prevents long-running requests from blocking server resources indefinitely.
func Timeout(timeout time.Duration) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Create a context with a timeout
			ctx, cancel := context.WithTimeout(r.Context(), timeout)
			defer cancel()

			// Create a new request with the timeout context
			r = r.WithContext(ctx)

			// Create a mutex to protect access to the response writer
			var wMutex sync.Mutex

			// Create a wrapped response writer that uses the mutex
			wrappedW := &mutexResponseWriter{
				ResponseWriter: w,
				mu:             &wMutex,
			}

			// Use a channel to signal when the handler is done
			done := make(chan struct{})
			go func() {
				next.ServeHTTP(wrappedW, r)
				close(done)
			}()

			select {
			case <-done:
				// Handler finished normally
				return
			case <-ctx.Done():
				// Timeout occurred
				wMutex.Lock()
				http.Error(w, "Request Timeout", http.StatusRequestTimeout)
				wMutex.Unlock()
				return
			}
		})
	}
}

// mutexResponseWriter is a wrapper around http.ResponseWriter that uses a mutex to protect access.
// This ensures thread-safety when writing to the response from multiple goroutines.
type mutexResponseWriter struct {
	http.ResponseWriter
	mu *sync.Mutex
}

// WriteHeader acquires the mutex and calls the underlying ResponseWriter.WriteHeader.
// This ensures thread-safety when setting the status code from multiple goroutines.
func (rw *mutexResponseWriter) WriteHeader(statusCode int) {
	rw.mu.Lock()
	defer rw.mu.Unlock()
	rw.ResponseWriter.WriteHeader(statusCode)
}

// Write acquires the mutex and calls the underlying ResponseWriter.Write.
// This ensures thread-safety when writing the response body from multiple goroutines.
func (rw *mutexResponseWriter) Write(b []byte) (int, error) {
	rw.mu.Lock()
	defer rw.mu.Unlock()
	return rw.ResponseWriter.Write(b)
}

// Flush acquires the mutex and calls the underlying ResponseWriter.Flush if it implements http.Flusher.
// This ensures thread-safety when flushing the response from multiple goroutines.
func (rw *mutexResponseWriter) Flush() {
	rw.mu.Lock()
	defer rw.mu.Unlock()
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

type CORSOptions struct {
	Origins          []string
	Methods          []string
	Headers          []string
	ExposeHeaders    []string // Headers the browser is allowed to access
	AllowCredentials bool     // Whether to allow credentials (cookies, authorization headers)
	MaxAge           time.Duration
}

// CORS is a middleware that adds Cross-Origin Resource Sharing (CORS) headers to the response.
// It allows you to specify which origins, methods, headers, and credentials are allowed for cross-origin requests,
// and which headers can be exposed to the client-side script.
// This middleware handles preflight OPTIONS requests automatically and optimizes header setting.
func CORS(corsConfig CORSOptions) Middleware {
	// Precompute header values for efficiency
	allowOrigin := ""
	if len(corsConfig.Origins) > 0 {
		allowOrigin = strings.Join(corsConfig.Origins, ", ")
	}

	allowMethods := ""
	if len(corsConfig.Methods) > 0 {
		allowMethods = strings.Join(corsConfig.Methods, ", ")
	}

	allowHeaders := ""
	if len(corsConfig.Headers) > 0 {
		allowHeaders = strings.Join(corsConfig.Headers, ", ")
	}

	exposeHeaders := ""
	if len(corsConfig.ExposeHeaders) > 0 {
		exposeHeaders = strings.Join(corsConfig.ExposeHeaders, ", ")
	}

	maxAge := ""
	if corsConfig.MaxAge > 0 {
		maxAge = strconv.Itoa(int(corsConfig.MaxAge.Seconds()))
	}

	allowCredentials := corsConfig.AllowCredentials

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Set headers common to both preflight and actual requests
			if allowOrigin != "" {
				w.Header().Set("Access-Control-Allow-Origin", allowOrigin)
			}
			// Allow-Credentials MUST be set on actual responses if needed,
			// and it's often helpful to mirror it on preflight for consistency,
			// although the spec primarily cares about it on the actual response.
			if allowCredentials {
				w.Header().Set("Access-Control-Allow-Credentials", "true")
			}

			// Handle preflight (OPTIONS) requests
			if r.Method == http.MethodOptions {
				// Set headers specific to preflight responses
				if allowMethods != "" {
					w.Header().Set("Access-Control-Allow-Methods", allowMethods)
				}
				if allowHeaders != "" {
					w.Header().Set("Access-Control-Allow-Headers", allowHeaders)
				}
				if maxAge != "" {
					w.Header().Set("Access-Control-Max-Age", maxAge)
				}

				// Preflight requests don't need to go further down the chain.
				// Respond with 200 OK (or 204 No Content is also common).
				w.WriteHeader(http.StatusOK)
				return
			}

			// Set headers specific to the actual response *before* calling the next handler
			// Expose-Headers tells the browser which headers the JS code is allowed to access.
			if exposeHeaders != "" {
				w.Header().Set("Access-Control-Expose-Headers", exposeHeaders)
			}

			// Call the next handler for actual requests (GET, POST, etc.)
			next.ServeHTTP(w, r)
		})
	}
}

// responseWriter is a wrapper around http.ResponseWriter that captures the status code.
// This allows middleware to inspect the status code after the handler has completed.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

// WriteHeader captures the status code and calls the underlying ResponseWriter.WriteHeader.
// This allows middleware to inspect the status code after the handler has completed.
func (rw *responseWriter) WriteHeader(statusCode int) {
	rw.statusCode = statusCode
	rw.ResponseWriter.WriteHeader(statusCode)
}

// Write calls the underlying ResponseWriter.Write.
// It passes through the write operation to the wrapped ResponseWriter.
func (rw *responseWriter) Write(b []byte) (int, error) {
	return rw.ResponseWriter.Write(b)
}

// Flush calls the underlying ResponseWriter.Flush if it implements http.Flusher.
// This allows streaming responses to be flushed to the client immediately.
func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
