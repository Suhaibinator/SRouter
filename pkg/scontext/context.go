// Package scontext provides centralized context management for the SRouter framework.
// It implements a single context wrapper (SRouterContext) that holds all request-scoped
// values such as user information, trace IDs, client IPs, database transactions, and
// route metadata. This approach avoids deep context nesting and provides type-safe
// access to context values through generic functions.
package scontext

import (
	"context"
	"maps"
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
// It provides a centralized storage for all request-scoped data, avoiding
// the need for multiple context.WithValue calls and deep context nesting.
// T is the User ID type (comparable), U is the User object type (any).
type SRouterContext[T comparable, U any] struct {
	UserID T
	User   *U

	TraceID string

	ClientIP string

	// UserAgent holds the user agent string from the request.
	UserAgent string

	Transaction DatabaseTransaction

	// Route information
	RouteTemplate string
	PathParams    httprouter.Params

	// CORS information determined by middleware
	AllowedOrigin      string
	CredentialsAllowed bool
	RequestedHeaders   string // Stores the requested headers from CORS preflight requests

	// HandlerError stores any error returned by the route handler
	HandlerError error

	UserIDSet             bool
	UserSet               bool
	ClientIPSet           bool
	UserAgentSet          bool
	TraceIDSet            bool
	TransactionSet        bool
	RouteTemplateSet      bool
	AllowedOriginSet      bool
	CredentialsAllowedSet bool
	RequestedHeadersSet   bool // Flag for RequestedHeaders
	// HandlerErrorSet is used to distinguish between an unset error and an explicitly set nil error.
	HandlerErrorSet bool

	Flags map[string]bool
}

// NewSRouterContext creates a new SRouterContext instance with initialized fields.
// It returns a pointer to a new context with an empty Flags map ready for use.
// T is the User ID type (comparable), U is the User object type (any).
func NewSRouterContext[T comparable, U any]() *SRouterContext[T, U] {
	return &SRouterContext[T, U]{
		Flags: make(map[string]bool),
	}
}

// GetSRouterContext retrieves the SRouterContext from a standard context.Context.
// It returns the context and a boolean indicating whether it was found.
// If no SRouterContext exists, it returns nil and false.
// T is the User ID type (comparable), U is the User object type (any).
func GetSRouterContext[T comparable, U any](ctx context.Context) (*SRouterContext[T, U], bool) {
	rc, ok := ctx.Value(sRouterContextKey{}).(*SRouterContext[T, U])
	return rc, ok
}

// WithSRouterContext adds or replaces the SRouterContext in a context.Context.
// It returns a new context containing the provided SRouterContext.
// This is typically used internally by the framework.
// T is the User ID type (comparable), U is the User object type (any).
func WithSRouterContext[T comparable, U any](ctx context.Context, rc *SRouterContext[T, U]) context.Context {
	return context.WithValue(ctx, sRouterContextKey{}, rc)
}

// EnsureSRouterContext retrieves an existing SRouterContext or creates a new one if none exists.
// It returns both the SRouterContext and the potentially updated context.
// This is used internally by With* functions to ensure a context exists before setting values.
// T is the User ID type (comparable), U is the User object type (any).
func EnsureSRouterContext[T comparable, U any](ctx context.Context) (*SRouterContext[T, U], context.Context) {
	rc, ok := GetSRouterContext[T, U](ctx)
	if !ok {
		rc = NewSRouterContext[T, U]()
		ctx = WithSRouterContext(ctx, rc)
	}
	return rc, ctx
}

// WithUserID adds a user ID to the context.
// The user ID is typically set by authentication middleware after validating credentials.
// T is the User ID type (comparable), U is the User object type (any).
func WithUserID[T comparable, U any](ctx context.Context, userID T) context.Context {
	rc, ctx := EnsureSRouterContext[T, U](ctx)
	rc.UserID = userID
	rc.UserIDSet = true
	return ctx
}

// GetUserID retrieves the user ID from the context.
// It returns the user ID and a boolean indicating whether it was found.
// If no user ID is set, it returns the zero value of T and false.
// T is the User ID type (comparable), U is the User object type (any).
func GetUserID[T comparable, U any](ctx context.Context) (T, bool) {
	var zero T
	rc, ok := GetSRouterContext[T, U](ctx)
	if !ok || !rc.UserIDSet {
		return zero, false
	}
	return rc.UserID, true
}

// GetUserIDFromRequest is a convenience function that extracts the user ID from an http.Request.
// It is equivalent to calling GetUserID with r.Context().
// T is the User ID type (comparable), U is the User object type (any).
func GetUserIDFromRequest[T comparable, U any](r *http.Request) (T, bool) {
	return GetUserID[T, U](r.Context())
}

// WithUser adds a user object to the context.
// The user object is typically set by authentication middleware that returns full user details.
// T is the User ID type (comparable), U is the User object type (any).
func WithUser[T comparable, U any](ctx context.Context, user *U) context.Context {
	rc, ctx := EnsureSRouterContext[T, U](ctx)
	rc.User = user
	rc.UserSet = true
	return ctx
}

// GetUser retrieves the user object from the context.
// It returns a pointer to the user object and a boolean indicating whether it was found.
// If no user is set, it returns nil and false.
// T is the User ID type (comparable), U is the User object type (any).
func GetUser[T comparable, U any](ctx context.Context) (*U, bool) {
	rc, ok := GetSRouterContext[T, U](ctx)
	if !ok || !rc.UserSet {
		return nil, false
	}
	return rc.User, true
}

// GetUserFromRequest is a convenience function that extracts the user object from an http.Request.
// It is equivalent to calling GetUser with r.Context().
// T is the User ID type (comparable), U is the User object type (any).
func GetUserFromRequest[T comparable, U any](r *http.Request) (*U, bool) {
	return GetUser[T, U](r.Context())
}

// WithFlag adds a boolean flag to the context.
// Flags are used to store custom boolean values that don't warrant their own field.
// The flag name should be descriptive and unique within the application.
// T is the User ID type (comparable), U is the User object type (any).
func WithFlag[T comparable, U any](ctx context.Context, name string, value bool) context.Context {
	rc, ctx := EnsureSRouterContext[T, U](ctx)
	if rc.Flags == nil {
		rc.Flags = make(map[string]bool)
	}
	rc.Flags[name] = value
	return ctx
}

// GetFlag retrieves a boolean flag from the context.
// It returns the flag value and a boolean indicating whether the flag exists.
// If the flag doesn't exist, it returns false, false.
// T is the User ID type (comparable), U is the User object type (any).
func GetFlag[T comparable, U any](ctx context.Context, name string) (bool, bool) {
	rc, ok := GetSRouterContext[T, U](ctx)
	if !ok || rc.Flags == nil {
		return false, false
	}
	value, exists := rc.Flags[name]
	return value, exists
}

// GetFlagFromRequest is a convenience function that extracts a flag from an http.Request.
// It is equivalent to calling GetFlag with r.Context() and the flag name.
// T is the User ID type (comparable), U is the User object type (any).
func GetFlagFromRequest[T comparable, U any](r *http.Request, name string) (bool, bool) {
	return GetFlag[T, U](r.Context(), name)
}

// WithClientIP adds the client IP address to the context.
// The IP is typically extracted by the router based on IPConfig settings,
// considering headers like X-Forwarded-For, X-Real-IP, or RemoteAddr.
// T is the User ID type (comparable), U is the User object type (any).
func WithClientIP[T comparable, U any](ctx context.Context, ip string) context.Context {
	rc, ctx := EnsureSRouterContext[T, U](ctx)
	rc.ClientIP = ip
	rc.ClientIPSet = true
	return ctx
}

// GetClientIP retrieves the client IP address from the context.
// It returns the IP address and a boolean indicating whether it was found.
// If no client IP is set, it returns an empty string and false.
// T is the User ID type (comparable), U is the User object type (any).
func GetClientIP[T comparable, U any](ctx context.Context) (string, bool) {
	rc, ok := GetSRouterContext[T, U](ctx)
	if !ok || !rc.ClientIPSet {
		return "", false
	}
	return rc.ClientIP, true
}

// GetClientIPFromRequest is a convenience function that extracts the client IP from an http.Request.
// It is equivalent to calling GetClientIP with r.Context().
// T is the User ID type (comparable), U is the User object type (any).
func GetClientIPFromRequest[T comparable, U any](r *http.Request) (string, bool) {
	return GetClientIP[T, U](r.Context())
}

// WithUserAgent adds the User-Agent string to the context.
// The User-Agent is typically extracted from the request headers by the router.
// T is the User ID type (comparable), U is the User object type (any).
func WithUserAgent[T comparable, U any](ctx context.Context, ua string) context.Context {
	rc, ctx := EnsureSRouterContext[T, U](ctx)
	rc.UserAgent = ua
	rc.UserAgentSet = true
	return ctx
}

// GetUserAgent retrieves the User-Agent string from the context.
// It returns the User-Agent and a boolean indicating whether it was found.
// If no User-Agent is set, it returns an empty string and false.
// T is the User ID type (comparable), U is the User object type (any).
func GetUserAgent[T comparable, U any](ctx context.Context) (string, bool) {
	rc, ok := GetSRouterContext[T, U](ctx)
	if !ok || !rc.UserAgentSet {
		return "", false
	}
	return rc.UserAgent, true
}

// GetUserAgentFromRequest is a convenience function that extracts the User-Agent from an http.Request.
// It is equivalent to calling GetUserAgent with r.Context().
// T is the User ID type (comparable), U is the User object type (any).
func GetUserAgentFromRequest[T comparable, U any](r *http.Request) (string, bool) {
	return GetUserAgent[T, U](r.Context())
}

// WithTransaction adds a database transaction to the context.
// This is typically used by database middleware to make a transaction available
// to handlers for transactional operations. The transaction should implement
// the DatabaseTransaction interface.
// T is the User ID type (comparable), U is the User object type (any).
func WithTransaction[T comparable, U any](ctx context.Context, tx DatabaseTransaction) context.Context {
	rc, ctx := EnsureSRouterContext[T, U](ctx)
	rc.Transaction = tx
	rc.TransactionSet = true
	return ctx
}

// GetTransaction retrieves a database transaction from the context.
// It returns the transaction and a boolean indicating whether it was found.
// If no transaction is set, it returns nil and false.
// T is the User ID type (comparable), U is the User object type (any).
func GetTransaction[T comparable, U any](ctx context.Context) (DatabaseTransaction, bool) {
	rc, ok := GetSRouterContext[T, U](ctx)
	if !ok || !rc.TransactionSet {
		return nil, false
	}
	return rc.Transaction, true
}

// GetTransactionFromRequest is a convenience function that extracts a database transaction from an http.Request.
// It is equivalent to calling GetTransaction with r.Context().
// T is the User ID type (comparable), U is the User object type (any).
func GetTransactionFromRequest[T comparable, U any](r *http.Request) (DatabaseTransaction, bool) {
	return GetTransaction[T, U](r.Context())
}

// WithTraceID adds a trace ID to the context.
// The trace ID is used for distributed tracing and request correlation.
// If a trace ID is already set, this function will not overwrite it,
// preserving trace IDs propagated from upstream services.
// T is the User ID type (comparable), U is the User object type (any).
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

// GetTraceIDFromContext retrieves the trace ID from the context.
// It returns the trace ID if set, or an empty string if not found.
// This function never returns an error; absence is indicated by an empty string.
// T is the User ID type (comparable), U is the User object type (any).
func GetTraceIDFromContext[T comparable, U any](ctx context.Context) string {
	rc, ok := GetSRouterContext[T, U](ctx)
	if !ok || !rc.TraceIDSet {
		return ""
	}
	return rc.TraceID
}

// GetTraceIDFromRequest is a convenience function that extracts the trace ID from an http.Request.
// It is equivalent to calling GetTraceIDFromContext with r.Context().
// Returns an empty string if no trace ID is set.
// T is the User ID type (comparable), U is the User object type (any).
func GetTraceIDFromRequest[T comparable, U any](r *http.Request) string {
	return GetTraceIDFromContext[T, U](r.Context())
}

// WithRouteInfo adds route information to the context.
// This includes path parameters extracted by httprouter and the route template string.
// This function is called internally by the router when a route is matched.
// The route template is the original path pattern (e.g., "/users/:id") used for metrics and logging.
// T is the User ID type (comparable), U is the User object type (any).
func WithRouteInfo[T comparable, U any](ctx context.Context, params httprouter.Params, routeTemplate string) context.Context {
	rc, ctx := EnsureSRouterContext[T, U](ctx)
	rc.PathParams = params
	rc.RouteTemplate = routeTemplate
	rc.RouteTemplateSet = true
	return ctx
}

// GetRouteTemplateFromContext retrieves the route template from the context.
// The route template is the original path pattern (e.g., "/users/:id") before parameter substitution.
// It returns the template and a boolean indicating whether it was found.
// This is useful for metrics and logging where you want consistent route identifiers.
// T is the User ID type (comparable), U is the User object type (any).
func GetRouteTemplateFromContext[T comparable, U any](ctx context.Context) (string, bool) {
	rc, ok := GetSRouterContext[T, U](ctx)
	if !ok || !rc.RouteTemplateSet {
		return "", false
	}
	return rc.RouteTemplate, true
}

// GetRouteTemplateFromRequest is a convenience function that extracts the route template from an http.Request.
// It is equivalent to calling GetRouteTemplateFromContext with r.Context().
// T is the User ID type (comparable), U is the User object type (any).
func GetRouteTemplateFromRequest[T comparable, U any](r *http.Request) (string, bool) {
	return GetRouteTemplateFromContext[T, U](r.Context())
}

// GetPathParamsFromContext retrieves the path parameters from the context.
// Path parameters are extracted by httprouter from the URL path (e.g., :id in "/users/:id").
// It returns the parameters and a boolean indicating whether they were found.
// T is the User ID type (comparable), U is the User object type (any).
func GetPathParamsFromContext[T comparable, U any](ctx context.Context) (httprouter.Params, bool) {
	rc, ok := GetSRouterContext[T, U](ctx)
	if !ok || !rc.RouteTemplateSet { // Use RouteTemplateSet as indicator that params are also set
		return nil, false
	}
	return rc.PathParams, true
}

// GetPathParamsFromRequest is a convenience function that extracts path parameters from an http.Request.
// It is equivalent to calling GetPathParamsFromContext with r.Context().
// T is the User ID type (comparable), U is the User object type (any).
func GetPathParamsFromRequest[T comparable, U any](r *http.Request) (httprouter.Params, bool) {
	return GetPathParamsFromContext[T, U](r.Context())
}

// WithCORSInfo adds CORS (Cross-Origin Resource Sharing) information to the context.
// This is used internally by the CORS middleware to store the allowed origin and
// whether credentials are allowed for the current request. These values are used
// when generating error responses to ensure CORS headers are properly set.
// T is the User ID type (comparable), U is the User object type (any).
func WithCORSInfo[T comparable, U any](ctx context.Context, allowedOrigin string, credentialsAllowed bool) context.Context {
	rc, ctx := EnsureSRouterContext[T, U](ctx)
	rc.AllowedOrigin = allowedOrigin
	rc.CredentialsAllowed = credentialsAllowed
	rc.AllowedOriginSet = true
	rc.CredentialsAllowedSet = true // Set both flags when info is added
	return ctx
}

// GetCORSInfo retrieves CORS (Cross-Origin Resource Sharing) details from the context.
// It returns:
// - allowedOrigin: The origin that should be set in Access-Control-Allow-Origin header
// - credentialsAllowed: Whether Access-Control-Allow-Credentials should be "true"
// - ok: Whether CORS information was found in the context
// T is the User ID type (comparable), U is the User object type (any).
func GetCORSInfo[T comparable, U any](ctx context.Context) (allowedOrigin string, credentialsAllowed bool, ok bool) {
	rc, found := GetSRouterContext[T, U](ctx)
	if !found || !rc.AllowedOriginSet { // Check if origin was set as the primary indicator
		return "", false, false
	}
	// Return the stored values. CredentialsAllowedSet is implicitly true if AllowedOriginSet is true based on WithCORSInfo logic.
	return rc.AllowedOrigin, rc.CredentialsAllowed, true
}

// GetCORSInfoFromRequest is a convenience function that extracts CORS details from an http.Request.
// It is equivalent to calling GetCORSInfo with r.Context().
// T is the User ID type (comparable), U is the User object type (any).
func GetCORSInfoFromRequest[T comparable, U any](r *http.Request) (allowedOrigin string, credentialsAllowed bool, ok bool) {
	return GetCORSInfo[T, U](r.Context())
}

// WithCORSRequestedHeaders stores the Access-Control-Request-Headers value from a CORS preflight request.
// This is used internally when the CORS configuration allows wildcard headers (*),
// so the exact requested headers can be echoed back in the Access-Control-Allow-Headers response.
// T is the User ID type (comparable), U is the User object type (any).
func WithCORSRequestedHeaders[T comparable, U any](ctx context.Context, requestedHeaders string) context.Context {
	rc, ctx := EnsureSRouterContext[T, U](ctx)
	rc.RequestedHeaders = requestedHeaders
	rc.RequestedHeadersSet = true
	return ctx
}

// GetCORSRequestedHeaders retrieves the Access-Control-Request-Headers value from the context.
// This is used internally by the CORS handler to echo back the requested headers
// when wildcard headers are allowed in the configuration.
// It returns the headers string and a boolean indicating whether it was found.
// T is the User ID type (comparable), U is the User object type (any).
func GetCORSRequestedHeaders[T comparable, U any](ctx context.Context) (string, bool) {
	rc, ok := GetSRouterContext[T, U](ctx)
	if !ok || !rc.RequestedHeadersSet {
		return "", false
	}
	return rc.RequestedHeaders, true
}

// GetCORSRequestedHeadersFromRequest is a convenience function that extracts CORS requested headers from an http.Request.
// It is equivalent to calling GetCORSRequestedHeaders with r.Context().
// T is the User ID type (comparable), U is the User object type (any).
func GetCORSRequestedHeadersFromRequest[T comparable, U any](r *http.Request) (string, bool) {
	return GetCORSRequestedHeaders[T, U](r.Context())
}

// WithHandlerError sets the handler error in the context. This is typically used by the framework
// to store errors returned by generic route handlers, making them available to middleware.
// T is the User ID type (comparable), U is the User object type (any).
func WithHandlerError[T comparable, U any](ctx context.Context, err error) context.Context {
	rc, ctx := EnsureSRouterContext[T, U](ctx)
	rc.HandlerError = err
	rc.HandlerErrorSet = true
	return ctx
}

// GetHandlerError retrieves the handler error from the context if one was set.
// This is useful for middleware that needs to react to errors returned by route handlers,
// such as transaction middleware that might rollback on errors.
// T is the User ID type (comparable), U is the User object type (any).
func GetHandlerError[T comparable, U any](ctx context.Context) (error, bool) {
	rc, ok := GetSRouterContext[T, U](ctx)
	if !ok || !rc.HandlerErrorSet {
		return nil, false
	}
	return rc.HandlerError, true
}

// GetHandlerErrorFromRequest is a convenience function that extracts the handler error from an http.Request.
// It is equivalent to calling GetHandlerError with r.Context().
// T is the User ID type (comparable), U is the User object type (any).
func GetHandlerErrorFromRequest[T comparable, U any](r *http.Request) (error, bool) {
	return GetHandlerError[T, U](r.Context())
}

// SRouter Context Copying Functions
//
// The scontext package provides two functions for copying SRouterContext between contexts,
// each with different behavior for handling destination contexts:
//
// 1. CopySRouterContext: Standard deep copy operation
//    - Copies source SRouterContext to destination
//    - Automatically creates SRouterContext in destination if needed
//    - Returns destination unchanged if source has no SRouterContext
//
// 2. CopySRouterContextOverlay: Conditional copy operation
//    - Only copies if destination already has an SRouterContext
//    - No-op if destination lacks SRouterContext (preserves original destination)
//    - Use when you want to update existing context without creating new structures
//
// Both functions perform deep copies to ensure independence between source and destination.

// cloneSRouterContext creates a deep copy of an existing SRouterContext.
// This is an internal helper function used by the various copy functions.
// T is the User ID type (comparable), U is the User object type (any).
func cloneSRouterContext[T comparable, U any](src *SRouterContext[T, U]) *SRouterContext[T, U] {
	dst := &SRouterContext[T, U]{
		UserID:                src.UserID,
		User:                  src.User,
		TraceID:               src.TraceID,
		ClientIP:              src.ClientIP,
		UserAgent:             src.UserAgent,
		Transaction:           src.Transaction,
		RouteTemplate:         src.RouteTemplate,
		PathParams:            src.PathParams, // Will be deep copied below
		AllowedOrigin:         src.AllowedOrigin,
		CredentialsAllowed:    src.CredentialsAllowed,
		RequestedHeaders:      src.RequestedHeaders,
		HandlerError:          src.HandlerError,
		UserIDSet:             src.UserIDSet,
		UserSet:               src.UserSet,
		ClientIPSet:           src.ClientIPSet,
		UserAgentSet:          src.UserAgentSet,
		TraceIDSet:            src.TraceIDSet,
		TransactionSet:        src.TransactionSet,
		RouteTemplateSet:      src.RouteTemplateSet,
		AllowedOriginSet:      src.AllowedOriginSet,
		CredentialsAllowedSet: src.CredentialsAllowedSet,
		RequestedHeadersSet:   src.RequestedHeadersSet,
		HandlerErrorSet:       src.HandlerErrorSet,
	}

	// Deep copy the Flags map
	if src.Flags != nil {
		dst.Flags = make(map[string]bool, len(src.Flags))
		maps.Copy(dst.Flags, src.Flags)
	} else {
		dst.Flags = make(map[string]bool)
	}

	// Deep copy PathParams slice
	if src.PathParams != nil {
		dst.PathParams = make(httprouter.Params, len(src.PathParams))
		copy(dst.PathParams, src.PathParams)
	}

	return dst
}

// CopySRouterContext creates a deep copy of the SRouterContext from the source context
// and adds it to the destination context. This is useful when you need to transfer
// all SRouter context values to a new context while preserving the destination's
// underlying context chain (cancellation, deadlines, etc.).
//
// If no SRouterContext exists in the source, the destination context is returned unchanged.
// If the source contains an SRouterContext, all values (including flags) are deep copied
// to ensure the destination has its own independent copy. The destination context will
// automatically have an SRouterContext added if none exists.
//
// T is the User ID type (comparable), U is the User object type (any).
func CopySRouterContext[T comparable, U any](dst, src context.Context) context.Context {
	srcRC, ok := GetSRouterContext[T, U](src)
	if !ok {
		return dst
	}

	dstRC := cloneSRouterContext(srcRC)
	return WithSRouterContext(dst, dstRC)
}

// CopySRouterContextOverlay creates a deep copy of the SRouterContext from the source context
// and overlays it onto the destination context only if the destination already has an
// SRouterContext. If the destination does not have an SRouterContext, this function
// performs no operation and returns the destination unchanged.
//
// If no SRouterContext exists in the source, the destination context is returned unchanged.
// If both source and destination contain SRouterContexts, the source values (including flags)
// are deep copied and completely replace the destination's SRouterContext.
//
// This function is useful when you want to update existing SRouter context values
// but avoid creating new context structures where none existed before.
//
// T is the User ID type (comparable), U is the User object type (any).
func CopySRouterContextOverlay[T comparable, U any](dst, src context.Context) context.Context {
	srcRC, srcOk := GetSRouterContext[T, U](src)
	if !srcOk {
		return dst
	}

	_, dstOk := GetSRouterContext[T, U](dst)
	if !dstOk {
		return dst // No-op if destination doesn't have SRouterContext
	}

	dstRC := cloneSRouterContext(srcRC)
	return WithSRouterContext(dst, dstRC)
}
