// Package middleware provides recovery, request-body limiting, authentication,
// trace IDs, rate limiting, and transaction helpers for SRouter HTTP handlers.
package middleware

import (
	"net/http"
	"runtime/debug"
	"slices"

	"github.com/Suhaibinator/SRouter/internal/requestlog"
	"github.com/Suhaibinator/SRouter/pkg/logkeys"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
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
		for _, middleware := range slices.Backward(middlewares) {
			next = middleware(next)
		}
		return next
	}
}

// Recovery is a middleware that recovers from panics in HTTP handlers.
// It logs the panic and stack trace through the request logger, when configured,
// and returns a 500 Internal Server Error response.
// This prevents the server from crashing when a panic occurs in a handler.
func Recovery[T comparable, U any]() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			defer func() {
				if rec := recover(); rec != nil {
					if ce := requestlog.Check[T, U](r.Context(), zapcore.ErrorLevel, "Panic recovered"); ce != nil {
						ce.Write(
							zap.Any(logkeys.Panic, rec),
							zap.String(logkeys.Stack, string(debug.Stack())),
							zap.String(logkeys.Method, r.Method),
							zap.String(logkeys.Path, r.URL.Path),
						)
					}

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

// CORS middleware has been removed. CORS is now handled directly within the router
// based on the CORSConfig provided in the main RouterConfig.
// See pkg/router/router.go and pkg/router/config.go.
