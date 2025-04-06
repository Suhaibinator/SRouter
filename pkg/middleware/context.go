// Package middleware provides a collection of HTTP middleware components for the SRouter framework.
package middleware

import (
	"context"
	"net/http"
)

// sRouterContextKey is a private type for the context key to avoid collisions
type sRouterContextKey struct{}

// SRouterContext holds all values that SRouter adds to request contexts.
// It allows storing multiple types of values with only a single level of context nesting,
// solving the problem of deep context nesting which occurs when multiple middleware
// components each add their own values to the context.
//
// T is the User ID type (comparable), U is the User object type (any).
// This structure centralizes all context values that middleware components need to store
// or access, providing a cleaner and more efficient approach than using multiple separate
// context keys.
type SRouterContext[T comparable, U any] struct {
	// User ID and User object storage
	UserID T
	User   *U

	// Trace ID for tracing
	TraceID string

	// Client IP address
	ClientIP string

	// Database transaction
	Transaction DatabaseTransaction

	// Track which fields are set
	UserIDSet      bool
	UserSet        bool
	ClientIPSet    bool
	TraceIDSet     bool
	TransactionSet bool

	// Additional flags
	Flags map[string]bool
}

// NewSRouterContext creates a new router context
func NewSRouterContext[T comparable, U any]() *SRouterContext[T, U] {
	return &SRouterContext[T, U]{
		Flags: make(map[string]bool),
	}
}

// GetSRouterContext retrieves the router context from a request context
func GetSRouterContext[T comparable, U any](ctx context.Context) (*SRouterContext[T, U], bool) {
	rc, ok := ctx.Value(sRouterContextKey{}).(*SRouterContext[T, U])
	return rc, ok
}

// WithSRouterContext adds or updates the router context in the request context
func WithSRouterContext[T comparable, U any](ctx context.Context, rc *SRouterContext[T, U]) context.Context {
	return context.WithValue(ctx, sRouterContextKey{}, rc)
}

// EnsureSRouterContext retrieves or creates a router context
func EnsureSRouterContext[T comparable, U any](ctx context.Context) (*SRouterContext[T, U], context.Context) {
	rc, ok := GetSRouterContext[T, U](ctx)
	if !ok {
		rc = NewSRouterContext[T, U]()
		ctx = WithSRouterContext(ctx, rc)
	}
	return rc, ctx
}

// WithUserID adds a user ID to the context
func WithUserID[T comparable, U any](ctx context.Context, userID T) context.Context {
	rc, ctx := EnsureSRouterContext[T, U](ctx)
	rc.UserID = userID
	rc.UserIDSet = true
	return ctx
}

// GetUserID retrieves a user ID from the router context
func GetUserID[T comparable, U any](ctx context.Context) (T, bool) {
	var zero T
	rc, ok := GetSRouterContext[T, U](ctx)
	if !ok || !rc.UserIDSet {
		return zero, false
	}
	return rc.UserID, true
}

// GetUserIDFromRequest is a convenience function to get the user ID from a request
func GetUserIDFromRequest[T comparable, U any](r *http.Request) (T, bool) {
	return GetUserID[T, U](r.Context())
}

// WithUser adds a user to the context
func WithUser[T comparable, U any](ctx context.Context, user *U) context.Context {
	rc, ctx := EnsureSRouterContext[T, U](ctx)
	rc.User = user
	rc.UserSet = true
	return ctx
}

// GetUser retrieves a user from the router context
func GetUser[T comparable, U any](ctx context.Context) (*U, bool) {
	rc, ok := GetSRouterContext[T, U](ctx)
	if !ok || !rc.UserSet {
		return nil, false
	}
	return rc.User, true
}

// GetUserFromRequest is a convenience function to get the user from a request
func GetUserFromRequest[T comparable, U any](r *http.Request) (*U, bool) {
	return GetUser[T, U](r.Context())
}

// WithFlag adds a flag to the context
func WithFlag[T comparable, U any](ctx context.Context, name string, value bool) context.Context {
	rc, ctx := EnsureSRouterContext[T, U](ctx)
	if rc.Flags == nil {
		rc.Flags = make(map[string]bool)
	}
	rc.Flags[name] = value
	return ctx
}

// GetFlag retrieves a flag from the router context
func GetFlag[T comparable, U any](ctx context.Context, name string) (bool, bool) {
	rc, ok := GetSRouterContext[T, U](ctx)
	if !ok || rc.Flags == nil {
		return false, false
	}
	value, exists := rc.Flags[name]
	return value, exists
}

// GetFlagFromRequest is a convenience function to get a flag from a request
func GetFlagFromRequest[T comparable, U any](r *http.Request, name string) (bool, bool) {
	return GetFlag[T, U](r.Context(), name)
}

// WithClientIP adds a client IP to the context
func WithClientIP[T comparable, U any](ctx context.Context, ip string) context.Context {
	rc, ctx := EnsureSRouterContext[T, U](ctx)
	rc.ClientIP = ip
	rc.ClientIPSet = true
	return ctx
}

// GetClientIP retrieves a client IP from the router context
func GetClientIP[T comparable, U any](ctx context.Context) (string, bool) {
	rc, ok := GetSRouterContext[T, U](ctx)
	if !ok || !rc.ClientIPSet {
		return "", false
	}
	return rc.ClientIP, true
}

// GetClientIPFromRequest is a convenience function to get the client IP from a request
func GetClientIPFromRequest[T comparable, U any](r *http.Request) (string, bool) {
	return GetClientIP[T, U](r.Context())
}

// WithTransaction adds a database transaction to the context
func WithTransaction[T comparable, U any](ctx context.Context, tx DatabaseTransaction) context.Context {
	rc, ctx := EnsureSRouterContext[T, U](ctx)
	rc.Transaction = tx
	rc.TransactionSet = true
	return ctx
}

// GetTransaction retrieves a database transaction from the router context
func GetTransaction[T comparable, U any](ctx context.Context) (DatabaseTransaction, bool) {
	rc, ok := GetSRouterContext[T, U](ctx)
	if !ok || !rc.TransactionSet {
		return nil, false
	}
	return rc.Transaction, true
}

// GetTransactionFromRequest is a convenience function to get the transaction from a request
func GetTransactionFromRequest[T comparable, U any](r *http.Request) (DatabaseTransaction, bool) {
	return GetTransaction[T, U](r.Context())
}
