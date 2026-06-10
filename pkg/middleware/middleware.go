// Package middleware provides a collection of HTTP middleware components for the SRouter framework.
// These middleware components can be used to add functionality such as logging, recovery from panics,
// authentication, request timeouts, and more to your HTTP handlers.
package middleware

import (
	"context"
	"net/http"
	"runtime/debug"
	"sync"
	"sync/atomic"
	"time"

	"go.uber.org/zap"
)

// Chain combines multiple middleware functions into a single middleware.
// The middlewares are applied in the order they appear in the chain:
// the first middleware in the list will be the outermost wrapper.
// This means it will be the first to process the request and the last
// to process the response, following the "onion" model of middleware.
//
// Example:
//
//	chain := Chain(logging, auth, rateLimit)
//	// Results in: logging(auth(rateLimit(handler)))
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
func recovery(logger *zap.Logger) Middleware {
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

// Authentication function has been moved to auth.go

// MaxBodySize is a middleware that limits the size of the request body.
// It prevents clients from sending excessively large requests that could
// consume too much memory or cause denial of service.
func maxBodySize(maxSize int64) Middleware {
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
// the middleware will cancel the request context and return a 408 Request Timeout response,
// but only if the handler has not already started writing a response.
// Once the timeout fires, any further handler writes are rejected with
// http.ErrHandlerTimeout instead of racing with the timeout response.
// This prevents long-running requests from blocking server resources indefinitely.
func timeout(timeout time.Duration) Middleware {
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
				// Timeout occurred. If the handler already started writing
				// (or claims the response concurrently with the timeout),
				// don't write a second response on top of it.
				if wrappedW.wroteHeader.Load() || !wrappedW.markTimedOut() {
					return
				}

				// Serialize with any handler write currently in progress.
				wMutex.Lock()
				http.Error(w, "Request Timeout", http.StatusRequestTimeout)
				wMutex.Unlock()
				return
			}
		})
	}
}

// mutexResponseWriter is a wrapper around http.ResponseWriter that uses a mutex to protect access
// and tracks whether the response has been started. Once timedOut is set, all writes are rejected
// so a late handler can never touch the underlying writer after the timeout response was sent.
type mutexResponseWriter struct {
	http.ResponseWriter
	mu          *sync.Mutex
	wroteHeader atomic.Bool // Tracks if WriteHeader or Write has been called
	timedOut    atomic.Bool // When true, reject all writes to the underlying writer
}

// markTimedOut transitions the writer into the timed-out state, rejecting all
// further handler writes, and attempts to claim the response for the timeout
// handler. It returns false if a handler write claimed the response first (in
// the window between the caller's last check and this transition), in which
// case the timeout response must be suppressed.
func (rw *mutexResponseWriter) markTimedOut() bool {
	rw.timedOut.Store(true)
	return rw.wroteHeader.CompareAndSwap(false, true)
}

// WriteHeader acquires the mutex and calls the underlying ResponseWriter.WriteHeader.
// This ensures thread-safety when setting the status code from multiple goroutines.
func (rw *mutexResponseWriter) WriteHeader(statusCode int) {
	if rw.timedOut.Load() {
		return
	}
	rw.mu.Lock()
	defer rw.mu.Unlock()
	if !rw.wroteHeader.Swap(true) {
		rw.ResponseWriter.WriteHeader(statusCode)
	}
}

// Write acquires the mutex and calls the underlying ResponseWriter.Write.
// This ensures thread-safety when writing the response body from multiple goroutines.
func (rw *mutexResponseWriter) Write(b []byte) (int, error) {
	if rw.timedOut.Load() {
		return 0, http.ErrHandlerTimeout
	}
	rw.mu.Lock()
	defer rw.mu.Unlock()
	// Re-check under the lock: the timeout response may have been written
	// while this write was waiting for the mutex.
	if rw.timedOut.Load() {
		return 0, http.ErrHandlerTimeout
	}
	rw.wroteHeader.Store(true)
	return rw.ResponseWriter.Write(b)
}

// Flush acquires the mutex and calls the underlying ResponseWriter.Flush if it implements http.Flusher.
// This ensures thread-safety when flushing the response from multiple goroutines.
func (rw *mutexResponseWriter) Flush() {
	if rw.timedOut.Load() {
		return
	}
	rw.mu.Lock()
	defer rw.mu.Unlock()
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// CORS middleware has been removed. CORS is now handled directly within the router
// based on the CORSConfig provided in the main RouterConfig.
// See pkg/router/router.go and pkg/router/config.go.
