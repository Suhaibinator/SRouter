// Package middleware provides a collection of HTTP middleware components for the SRouter framework.
package middleware

import (
	"context"
	"net/http"

	"github.com/Suhaibinator/SRouter/pkg/common"
	"github.com/google/uuid"
)

// WithTraceID adds a trace ID to the SRouterContext in the provided context.
// If no SRouterContext exists, one will be created.
//
// This function is part of the SRouterContext approach for storing values in the context,
// which avoids deep nesting of context values by using a single wrapper structure.
func WithTraceID[T comparable, U any](ctx context.Context, traceID string) context.Context {
	// Get or create the router context
	rc, ok := GetSRouterContext[T, U](ctx)
	if !ok {
		rc = &SRouterContext[T, U]{
			Flags: make(map[string]bool),
		}
	}

	// Store the trace ID in the flags
	rc.TraceID = traceID
	rc.TraceIDSet = true

	// Update the context
	return context.WithValue(ctx, sRouterContextKey{}, rc)
}

// GetTraceIDFromContext extracts the trace ID from a context.
// It first tries to find the trace ID in the SRouterContext using the flags map,
// and falls back to the legacy context key approach for backward compatibility.
// Returns an empty string if no trace ID is found.
func GetTraceIDFromContext(ctx context.Context) string {
	// Try the new way first
	rc, ok := GetSRouterContext[string, any](ctx)
	if !ok {
		return ""
	}

	// Check if the trace ID is set in the SRouterContext
	if rc.TraceIDSet {
		return rc.TraceID
	}
	return ""
}

// GetTraceID extracts the trace ID from the request context.
// Returns an empty string if no trace ID is found.
func GetTraceID(r *http.Request) string {
	return GetTraceIDFromContext(r.Context())
}

// AddTraceIDToRequest adds a trace ID to the request context.
// This is useful for testing or for manually setting a trace ID.
func AddTraceIDToRequest(r *http.Request, traceID string) *http.Request {
	ctx := WithTraceID[string, any](r.Context(), traceID)
	return r.WithContext(ctx)
}

// TraceMiddleware creates a middleware that generates a unique trace ID for each request
// and adds it to the request context. This allows for request tracing across logs.
func TraceMiddleware() common.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Generate a unique trace ID
			traceID := uuid.New().String()

			// Add the trace ID to the request context using the new wrapper
			ctx := WithTraceID[string, any](r.Context(), traceID)

			// Call the next handler with the updated request
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
