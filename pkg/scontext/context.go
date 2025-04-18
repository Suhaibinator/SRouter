package scontext

import (
	"context"
	"net/http"

	"github.com/julienschmidt/httprouter" // Import for Params type
	"gorm.io/gorm"                        // Needed for DatabaseTransaction
)

// sRouterContextKey is a private type for the context key to avoid collisions
type sRouterContextKey struct{}

// DatabaseTransaction defines an interface for essential transaction control methods.
// This allows mocking transaction behavior for testing purposes.
// Note: Moved from middleware/db.go to avoid import cycle if db needed context.
// Consider if this interface truly belongs here or in a db-specific package. For now, placing here to resolve cycle.
type DatabaseTransaction interface {
	Commit() error
	Rollback() error
	SavePoint(name string) error
	RollbackTo(name string) error
	GetDB() *gorm.DB
}

// SRouterContext holds all values that SRouter adds to request contexts.
// T is the User ID type (comparable), U is the User object type (any).
type SRouterContext[T comparable, U any] struct {
	UserID T
	User   *U

	TraceID string

	ClientIP string

	Transaction DatabaseTransaction

	// Route information
	RouteTemplate string
	PathParams    httprouter.Params

	UserIDSet        bool
	UserSet          bool
	ClientIPSet      bool
	TraceIDSet       bool
	TransactionSet   bool
	RouteTemplateSet bool

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

// WithTraceID adds a trace ID to the SRouterContext in the provided context.
func WithTraceID[T comparable, U any](ctx context.Context, traceID string) context.Context {
	rc, ctx := EnsureSRouterContext[T, U](ctx)
	// If TraceID is already set, do not overwrite it.
	if rc.TraceIDSet {
		return ctx
	}
	// Otherwise, set the trace ID and the flag.
	rc.TraceID = traceID
	rc.TraceIDSet = true
	return ctx
}

// GetTraceIDFromContext extracts the trace ID from the SRouterContext within a context.
func GetTraceIDFromContext[T comparable, U any](ctx context.Context) string {
	rc, ok := GetSRouterContext[T, U](ctx)
	if !ok || !rc.TraceIDSet {
		return ""
	}
	return rc.TraceID
}

// GetTraceIDFromRequest is a convenience function to get the trace ID from a request.
func GetTraceIDFromRequest[T comparable, U any](r *http.Request) string {
	return GetTraceIDFromContext[T, U](r.Context())
}

// WithRouteInfo adds route information (path parameters and route template) to the context.
// This is called by the router when a route is matched.
func WithRouteInfo[T comparable, U any](ctx context.Context, params httprouter.Params, routeTemplate string) context.Context {
	rc, ctx := EnsureSRouterContext[T, U](ctx)
	rc.PathParams = params
	rc.RouteTemplate = routeTemplate
	rc.RouteTemplateSet = true
	return ctx
}

// GetRouteTemplateFromContext extracts the route template from the SRouterContext within a context.
func GetRouteTemplateFromContext[T comparable, U any](ctx context.Context) (string, bool) {
	rc, ok := GetSRouterContext[T, U](ctx)
	if !ok || !rc.RouteTemplateSet {
		return "", false
	}
	return rc.RouteTemplate, true
}

// GetRouteTemplateFromRequest is a convenience function to get the route template from a request.
func GetRouteTemplateFromRequest[T comparable, U any](r *http.Request) (string, bool) {
	return GetRouteTemplateFromContext[T, U](r.Context())
}

// GetPathParamsFromContext extracts the path parameters from the SRouterContext within a context.
func GetPathParamsFromContext[T comparable, U any](ctx context.Context) (httprouter.Params, bool) {
	rc, ok := GetSRouterContext[T, U](ctx)
	if !ok || !rc.RouteTemplateSet { // Use RouteTemplateSet as indicator that params are also set
		return nil, false
	}
	return rc.PathParams, true
}

// GetPathParamsFromRequest is a convenience function to get the path parameters from a request.
func GetPathParamsFromRequest[T comparable, U any](r *http.Request) (httprouter.Params, bool) {
	return GetPathParamsFromContext[T, U](r.Context())
}
