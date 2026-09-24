// Package scontext provides centralized context management for the SRouter framework.
// It implements a single context wrapper (SRouterContext) that holds all request-scoped
// values such as user information, trace IDs, client IPs, database transactions, and
// route metadata. This approach avoids deep context nesting and provides type-safe
// access to context values through generic functions.
package scontext

import (
	"context"
	"maps"
	"sync"

	"github.com/julienschmidt/httprouter" // Import for Params type
	"go.uber.org/zap"
	"gorm.io/gorm" // Needed for DatabaseTransaction
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

// presenceBits is guarded by the wrapper mutex, along with its values.
type presenceBits uint16

const (
	presentUserID presenceBits = 1 << iota
	presentUser
	presentBuildID
	presentConfigID
	presentClientIP
	presentUserAgent
	presentTraceID
	presentTransaction
	presentRouteTemplate
	presentAllowedOrigin
	presentCredentialsAllowed
	presentRequestedHeaders
	presentHandlerError
)

// SRouterContext holds all values that SRouter adds to request contexts.
// It provides a centralized storage for all request-scoped data, avoiding
// the need for multiple context.WithValue calls and deep context nesting.
// T is the User ID type (comparable), U is the User object type (any).
//
// The struct is shared by pointer across the request's middleware chain, and
// a timed-out request's handler goroutine may still be mutating it while the
// router goroutine reads it. All access through this package's read/write
// helpers is therefore synchronized by an internal lock. All fields are
// private; use the helpers to read and write request state. The zero value is
// ready to use. Do not copy a wrapper by value; use CopySRouterContext instead.
type SRouterContext[T comparable, U any] struct {
	// mu guards all fields below. The read/write helper functions in this
	// package take it automatically.
	mu sync.RWMutex

	userID T
	user   *U

	// buildID and configID are opaque runtime identities supplied by the
	// application. SRouter stores them without parsing or normalization.
	buildID  string
	configID string

	traceID string

	clientIP string

	// userAgent holds the user agent string from the request.
	userAgent string

	transaction DatabaseTransaction

	// Route information
	routeTemplate string
	pathParams    httprouter.Params

	// CORS information determined by middleware
	allowedOrigin      string
	credentialsAllowed bool
	// presence distinguishes absent values from explicitly stored zero values.
	// Keep it next to credentialsAllowed to share alignment padding.
	presence         presenceBits
	requestedHeaders string // Stores the requested headers from CORS preflight requests

	// handlerError stores any error returned by the route handler
	handlerError error

	flags map[string]bool

	// logSource is immutable application configuration shared across requests.
	// logger is this context's cached request-correlated snapshot. The versions
	// ensure that snapshot matches the source and current correlation values:
	// writers advance logVersion, and successful derivation publishes the same
	// value as loggerVersion.
	logSource     *RequestLoggerSource[T]
	logger        *zap.Logger
	logVersion    uint64
	loggerVersion uint64
}

// NewSRouterContext creates a new SRouterContext instance.
// The flags map is allocated lazily by WithFlag on first use, so contexts on
// requests that never set a flag (the common case) avoid the map allocation.
// T is the User ID type (comparable), U is the User object type (any).
func NewSRouterContext[T comparable, U any]() *SRouterContext[T, U] {
	return &SRouterContext[T, U]{}
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

// reader exposes request values whose types do not depend on T or U.
// Every *SRouterContext[T, U] implements it; each method takes the context lock.
type reader interface {
	getBuildID() (string, bool)
	getConfigID() (string, bool)
	flag(name string) (bool, bool)
	getClientIP() (string, bool)
	getUserAgent() (string, bool)
	getTransaction() (DatabaseTransaction, bool)
	getTraceID() string
	corsInfo() (string, bool, bool)
	corsRequestedHeaders() (string, bool)
	getHandlerError() (error, bool)
	requestLogger() (*zap.Logger, bool)
}

// getReader accepts a carrier regardless of its user ID and user object types.
func getReader(ctx context.Context) (reader, bool) {
	r, ok := ctx.Value(sRouterContextKey{}).(reader)
	return r, ok
}

// typedReader exposes values whose return types depend on the user ID type.
type typedReader[T comparable] interface {
	getUserID() (T, bool)
	correlation() Correlation[T]
}

// getTypedReader reports carriers with a different user ID type as absent.
func getTypedReader[T comparable](ctx context.Context) (typedReader[T], bool) {
	r, ok := ctx.Value(sRouterContextKey{}).(typedReader[T])
	return r, ok
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

// WithBuildID adds or replaces the opaque application build identity in the context.
// T is the User ID type (comparable), U is the User object type (any).
func WithBuildID[T comparable, U any](ctx context.Context, buildID string) context.Context {
	rc, ctx := EnsureSRouterContext[T, U](ctx)
	rc.mu.Lock()
	rc.buildID = buildID
	rc.presence |= presentBuildID
	rc.logVersion++
	rc.mu.Unlock()
	return ctx
}

// GetBuildID retrieves the opaque application build identity from the context.
// It returns an empty string and false when no build identity has been set.
func GetBuildID(ctx context.Context) (string, bool) {
	r, ok := getReader(ctx)
	if !ok {
		return "", false
	}
	return r.getBuildID()
}

func (rc *SRouterContext[T, U]) getBuildID() (string, bool) {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	if rc.presence&presentBuildID == 0 {
		return "", false
	}
	return rc.buildID, true
}

// WithConfigID adds or replaces the opaque configuration identity in the context.
// T is the User ID type (comparable), U is the User object type (any).
func WithConfigID[T comparable, U any](ctx context.Context, configID string) context.Context {
	rc, ctx := EnsureSRouterContext[T, U](ctx)
	rc.mu.Lock()
	rc.configID = configID
	rc.presence |= presentConfigID
	rc.logVersion++
	rc.mu.Unlock()
	return ctx
}

// GetConfigID retrieves the opaque configuration identity from the context.
// It returns an empty string and false when no configuration identity has been set.
func GetConfigID(ctx context.Context) (string, bool) {
	r, ok := getReader(ctx)
	if !ok {
		return "", false
	}
	return r.getConfigID()
}

func (rc *SRouterContext[T, U]) getConfigID() (string, bool) {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	if rc.presence&presentConfigID == 0 {
		return "", false
	}
	return rc.configID, true
}

// WithUserID adds a user ID to the context.
// The user ID is typically set by authentication middleware after validating credentials.
// T is the User ID type (comparable), U is the User object type (any).
func WithUserID[T comparable, U any](ctx context.Context, userID T) context.Context {
	rc, ctx := EnsureSRouterContext[T, U](ctx)
	rc.mu.Lock()
	rc.userID = userID
	rc.presence |= presentUserID
	rc.logVersion++
	rc.mu.Unlock()
	return ctx
}

// GetUserID retrieves the user ID from the context.
// It returns the user ID and a boolean indicating whether it was found.
// If no user ID is set, it returns the zero value of T and false.
// T is the User ID type (comparable).
func GetUserID[T comparable](ctx context.Context) (T, bool) {
	r, ok := getTypedReader[T](ctx)
	if !ok {
		var zero T
		return zero, false
	}
	return r.getUserID()
}

func (rc *SRouterContext[T, U]) getUserID() (T, bool) {
	var zero T
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	if rc.presence&presentUserID == 0 {
		return zero, false
	}
	return rc.userID, true
}

// WithUser adds a user object to the context.
// The user object is typically set by authentication middleware that returns full user details.
// T is the User ID type (comparable), U is the User object type (any).
func WithUser[T comparable, U any](ctx context.Context, user *U) context.Context {
	rc, ctx := EnsureSRouterContext[T, U](ctx)
	rc.mu.Lock()
	rc.user = user
	rc.presence |= presentUser
	rc.mu.Unlock()
	return ctx
}

// GetUser retrieves the user object from the context.
// It returns a pointer to the user object and a boolean indicating whether it was found.
// If no user is set, it returns nil and false.
// T is the User ID type (comparable), U is the User object type (any).
func GetUser[T comparable, U any](ctx context.Context) (*U, bool) {
	rc, ok := GetSRouterContext[T, U](ctx)
	if !ok {
		return nil, false
	}
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	if rc.presence&presentUser == 0 {
		return nil, false
	}
	return rc.user, true
}

// WithFlag adds a boolean flag to the context.
// Flags are used to store custom boolean values that don't warrant their own field.
// The flag name should be descriptive and unique within the application.
// T is the User ID type (comparable), U is the User object type (any).
func WithFlag[T comparable, U any](ctx context.Context, name string, value bool) context.Context {
	rc, ctx := EnsureSRouterContext[T, U](ctx)
	rc.mu.Lock()
	if rc.flags == nil {
		rc.flags = make(map[string]bool)
	}
	rc.flags[name] = value
	rc.mu.Unlock()
	return ctx
}

// GetFlag retrieves a boolean flag from the context.
// It returns the flag value and a boolean indicating whether the flag exists.
// If the flag doesn't exist, it returns false, false.
func GetFlag(ctx context.Context, name string) (bool, bool) {
	r, ok := getReader(ctx)
	if !ok {
		return false, false
	}
	return r.flag(name)
}

func (rc *SRouterContext[T, U]) flag(name string) (bool, bool) {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	if rc.flags == nil {
		return false, false
	}
	value, exists := rc.flags[name]
	return value, exists
}

// WithClientIP adds the client IP address to the context.
// The IP is typically extracted by the router based on IPConfig settings,
// considering headers like X-Forwarded-For, X-Real-IP, or RemoteAddr.
// Valid socket addresses are normalized before storage by removing their port.
// Bracketed IPv6 addresses retain brackets, while IPv6 zone addresses retain
// their zone without brackets. Values that are not valid IP socket addresses
// are stored unchanged.
// T is the User ID type (comparable), U is the User object type (any).
func WithClientIP[T comparable, U any](ctx context.Context, ip string) context.Context {
	ip = cleanClientIP(ip)
	rc, ctx := EnsureSRouterContext[T, U](ctx)
	rc.mu.Lock()
	if rc.presence&presentClientIP == 0 || rc.clientIP != ip {
		rc.logVersion++
	}
	rc.clientIP = ip
	rc.presence |= presentClientIP
	rc.mu.Unlock()
	return ctx
}

// WithClientInfo adds the client IP address and user agent to the context in a
// single initialization step. Routers should prefer this when both values are
// available so the shared request context is resolved and locked only once.
// The client IP is normalized using the same rules as WithClientIP.
func WithClientInfo[T comparable, U any](ctx context.Context, ip, userAgent string) context.Context {
	ip = cleanClientIP(ip)
	rc, ctx := EnsureSRouterContext[T, U](ctx)
	rc.mu.Lock()
	if rc.presence&presentClientIP == 0 || rc.clientIP != ip {
		rc.logVersion++
	}
	rc.clientIP = ip
	rc.userAgent = userAgent
	rc.presence |= presentClientIP | presentUserAgent
	rc.mu.Unlock()
	return ctx
}

// GetClientIP retrieves the client IP address from the context.
// It returns the IP address and a boolean indicating whether it was found.
// If no client IP is set, it returns an empty string and false.
// IP socket addresses written through this package have their port removed.
func GetClientIP(ctx context.Context) (string, bool) {
	r, ok := getReader(ctx)
	if !ok {
		return "", false
	}
	return r.getClientIP()
}

func (rc *SRouterContext[T, U]) getClientIP() (string, bool) {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	if rc.presence&presentClientIP == 0 {
		return "", false
	}
	return rc.clientIP, true
}

// WithUserAgent adds the User-Agent string to the context.
// The User-Agent is typically extracted from the request headers by the router.
// T is the User ID type (comparable), U is the User object type (any).
func WithUserAgent[T comparable, U any](ctx context.Context, ua string) context.Context {
	rc, ctx := EnsureSRouterContext[T, U](ctx)
	rc.mu.Lock()
	rc.userAgent = ua
	rc.presence |= presentUserAgent
	rc.mu.Unlock()
	return ctx
}

// GetUserAgent retrieves the User-Agent string from the context.
// It returns the User-Agent and a boolean indicating whether it was found.
// If no User-Agent is set, it returns an empty string and false.
func GetUserAgent(ctx context.Context) (string, bool) {
	r, ok := getReader(ctx)
	if !ok {
		return "", false
	}
	return r.getUserAgent()
}

func (rc *SRouterContext[T, U]) getUserAgent() (string, bool) {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	if rc.presence&presentUserAgent == 0 {
		return "", false
	}
	return rc.userAgent, true
}

// WithTransaction adds a database transaction to the context.
// This is typically used by database middleware to make a transaction available
// to handlers for transactional operations. The transaction should implement
// the DatabaseTransaction interface.
// This mutates an existing matching wrapper shared by the context chain. To
// replace a transaction only for a child operation, call CopySRouterContext
// first. A nil transaction is still present; use ClearTransaction to remove it.
// T is the User ID type (comparable), U is the User object type (any).
func WithTransaction[T comparable, U any](ctx context.Context, tx DatabaseTransaction) context.Context {
	rc, ctx := EnsureSRouterContext[T, U](ctx)
	rc.mu.Lock()
	rc.transaction = tx
	rc.presence |= presentTransaction
	rc.mu.Unlock()
	return ctx
}

// ClearTransaction removes the transaction reference and its presence flag.
// It mutates the shared wrapper only when its types match T and U, and returns
// ctx unchanged without creating a wrapper. Clone with CopySRouterContext first
// when parent or sibling operations must retain their transaction.
// It does not invoke transaction methods or change any other request state.
func ClearTransaction[T comparable, U any](ctx context.Context) context.Context {
	rc, ok := GetSRouterContext[T, U](ctx)
	if !ok {
		return ctx
	}
	rc.mu.Lock()
	rc.transaction = nil
	rc.presence &^= presentTransaction
	rc.mu.Unlock()
	return ctx
}

// GetTransaction retrieves a database transaction from the context.
// It returns the transaction and a boolean indicating whether it was found.
// If no transaction is set, it returns nil and false.
func GetTransaction(ctx context.Context) (DatabaseTransaction, bool) {
	r, ok := getReader(ctx)
	if !ok {
		return nil, false
	}
	return r.getTransaction()
}

func (rc *SRouterContext[T, U]) getTransaction() (DatabaseTransaction, bool) {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	if rc.presence&presentTransaction == 0 {
		return nil, false
	}
	return rc.transaction, true
}

// WithTraceID adds a trace ID to the context.
// The trace ID is used for distributed tracing and request correlation.
// If a trace ID is already set, this function will not overwrite it,
// preserving trace IDs propagated from upstream services.
// T is the User ID type (comparable), U is the User object type (any).
func WithTraceID[T comparable, U any](ctx context.Context, traceID string) context.Context {
	rc, ctx := EnsureSRouterContext[T, U](ctx)
	rc.mu.Lock()
	defer rc.mu.Unlock()
	// If TraceID is already set, do not overwrite it.
	if rc.presence&presentTraceID != 0 {
		return ctx
	}
	// Otherwise, set the trace ID and the flag.
	rc.traceID = traceID
	rc.presence |= presentTraceID
	rc.logVersion++
	return ctx
}

// SetTraceID unconditionally replaces the trace ID under the context lock and
// invalidates the cached request logger. It performs no validation. WithTraceID
// instead preserves an ID that is already set, including an empty ID.
func SetTraceID[T comparable, U any](ctx context.Context, traceID string) context.Context {
	rc, ctx := EnsureSRouterContext[T, U](ctx)
	rc.mu.Lock()
	defer rc.mu.Unlock()
	rc.traceID = traceID
	rc.presence |= presentTraceID
	rc.logVersion++
	return ctx
}

// GetTraceID retrieves the trace ID from the context.
// It returns the trace ID if set, or an empty string if not found.
// This function never returns an error; absence is indicated by an empty string.
func GetTraceID(ctx context.Context) string {
	r, ok := getReader(ctx)
	if !ok {
		return ""
	}
	return r.getTraceID()
}

func (rc *SRouterContext[T, U]) getTraceID() string {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	if rc.presence&presentTraceID == 0 {
		return ""
	}
	return rc.traceID
}

// Correlation carries the per-operation values used to correlate log entries
// and metrics: the trace ID, the opaque build and configuration identities,
// and the user ID. The Has* methods report presence, so a deliberately empty
// value stays distinguishable from an absent one. The zero value has no values
// present. Obtain populated snapshots with GetCorrelation; assigning exported
// value fields does not change their presence.
//
// It is a value snapshot independent of later wrapper mutations. References
// inside T retain their normal Go sharing semantics.
// T is the User ID type (comparable).
type Correlation[T comparable] struct {
	TraceID  string
	BuildID  string
	ConfigID string
	UserID   T

	presence presenceBits
}

const correlationPresence = presentTraceID | presentBuildID | presentConfigID | presentUserID

// HasTraceID reports whether the snapshot contains a trace ID, including its zero value.
func (c Correlation[T]) HasTraceID() bool {
	return c.presence&presentTraceID != 0
}

// HasBuildID reports whether the snapshot contains a build identity, including its zero value.
func (c Correlation[T]) HasBuildID() bool {
	return c.presence&presentBuildID != 0
}

// HasConfigID reports whether the snapshot contains a configuration identity, including its zero value.
func (c Correlation[T]) HasConfigID() bool {
	return c.presence&presentConfigID != 0
}

// HasUserID reports whether the snapshot contains a user ID, including its zero value.
func (c Correlation[T]) HasUserID() bool {
	return c.presence&presentUserID != 0
}

// GetCorrelation returns the correlation values carried by the context.
//
// It reads them after one walk of the context chain and under a single lock
// acquisition, where the individual accessors pay both costs per value. Code
// that stamps correlation onto log entries or metrics should prefer it, and
// unpack the result with the typed constructors its own logger wants:
//
//	if c, ok := scontext.GetCorrelation[uint64](ctx); ok {
//		fields := make([]zap.Field, 0, 4)
//		if c.HasTraceID() {
//			fields = append(fields, zap.String(logkeys.TraceID, c.TraceID))
//		}
//		if c.HasUserID() {
//			fields = append(fields, zap.Uint64("user_id", c.UserID))
//		}
//		// ...
//	}
//
// Returning values rather than built log fields lets callers use Correlation
// with metrics and other logging implementations, or choose their own fields.
//
// It returns the zero value and false when the context carries no
// SRouterContext. The result is a copy taken at the moment of the call: a
// later write through a With* helper does not change it, and two separate
// calls are not an atomic pair.
// T is the User ID type (comparable).
func GetCorrelation[T comparable](ctx context.Context) (Correlation[T], bool) {
	r, ok := getTypedReader[T](ctx)
	if !ok {
		return Correlation[T]{}, false
	}
	return r.correlation(), true
}

func (rc *SRouterContext[T, U]) correlation() Correlation[T] {
	// Hold the lock only long enough to copy the values out. Nothing between
	// the two calls can panic, so the unlock does not need to be deferred.
	rc.mu.RLock()
	c := rc.correlationLocked()
	rc.mu.RUnlock()
	return c
}

// correlationLocked copies correlation while the caller holds mu for reading
// or writing. It does not invoke application code.
func (rc *SRouterContext[T, U]) correlationLocked() Correlation[T] {
	return Correlation[T]{
		TraceID:  rc.traceID,
		BuildID:  rc.buildID,
		ConfigID: rc.configID,
		UserID:   rc.userID,

		// Exclude unrelated context state so equal correlation snapshots remain
		// equal even when client, route, or other metadata differs.
		presence: rc.presence & correlationPresence,
	}
}

// WithRouteInfo adds route information to the context.
// This includes path parameters extracted by httprouter and the route template string.
// This function is called internally by the router when a route is matched.
// The route template is the original path pattern (e.g., "/users/:id") used for metrics and logging.
// T is the User ID type (comparable), U is the User object type (any).
func WithRouteInfo[T comparable, U any](ctx context.Context, params httprouter.Params, routeTemplate string) context.Context {
	rc, ctx := EnsureSRouterContext[T, U](ctx)
	SetRouteInfo(rc, params, routeTemplate)
	return ctx
}

// SetRouteInfo updates route information on an existing SRouterContext without
// creating another context wrapper. Router dispatch uses this after request
// metadata has initialized the shared context.
func SetRouteInfo[T comparable, U any](rc *SRouterContext[T, U], params httprouter.Params, routeTemplate string) {
	rc.mu.Lock()
	rc.pathParams = params
	rc.routeTemplate = routeTemplate
	rc.presence |= presentRouteTemplate
	rc.mu.Unlock()
}

type routeInfoProvider interface {
	getPathParams() (httprouter.Params, bool)
	getRouteTemplate() (string, bool)
}

func (rc *SRouterContext[T, U]) getPathParams() (httprouter.Params, bool) {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	if rc.presence&presentRouteTemplate == 0 {
		return nil, false
	}
	return rc.pathParams, true
}

func (rc *SRouterContext[T, U]) getRouteTemplate() (string, bool) {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	if rc.presence&presentRouteTemplate == 0 {
		return "", false
	}
	return rc.routeTemplate, true
}

// GetRouteTemplate retrieves the route template from the context.
// The route template is the original path pattern (e.g., "/users/:id") before parameter substitution.
// It returns the template and a boolean indicating whether it was found.
// This is useful for metrics and logging where you want consistent route identifiers.
func GetRouteTemplate(ctx context.Context) (string, bool) {
	provider, ok := ctx.Value(sRouterContextKey{}).(routeInfoProvider)
	if !ok {
		return "", false
	}
	return provider.getRouteTemplate()
}

// GetPathParams retrieves the path parameters from the context.
// Path parameters are extracted by httprouter from the URL path (e.g., :id in "/users/:id").
// It returns the parameters and a boolean indicating whether they were found.
func GetPathParams(ctx context.Context) (httprouter.Params, bool) {
	provider, ok := ctx.Value(sRouterContextKey{}).(routeInfoProvider)
	if !ok {
		return nil, false
	}
	return provider.getPathParams()
}

// WithCORSInfo adds CORS (Cross-Origin Resource Sharing) information to the context.
// This is used internally by the CORS middleware to store the allowed origin and
// whether credentials are allowed for the current request. These values are used
// when generating error responses to ensure CORS headers are properly set.
// T is the User ID type (comparable), U is the User object type (any).
func WithCORSInfo[T comparable, U any](ctx context.Context, allowedOrigin string, credentialsAllowed bool) context.Context {
	rc, ctx := EnsureSRouterContext[T, U](ctx)
	rc.mu.Lock()
	rc.allowedOrigin = allowedOrigin
	rc.credentialsAllowed = credentialsAllowed
	rc.presence |= presentAllowedOrigin | presentCredentialsAllowed
	rc.mu.Unlock()
	return ctx
}

// GetCORSInfo retrieves CORS (Cross-Origin Resource Sharing) details from the context.
// It returns:
// - allowedOrigin: The origin that should be set in Access-Control-Allow-Origin header
// - credentialsAllowed: Whether Access-Control-Allow-Credentials should be "true"
// - ok: Whether CORS information was found in the context
func GetCORSInfo(ctx context.Context) (allowedOrigin string, credentialsAllowed bool, ok bool) {
	r, ok := getReader(ctx)
	if !ok {
		return "", false, false
	}
	return r.corsInfo()
}

func (rc *SRouterContext[T, U]) corsInfo() (string, bool, bool) {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	if rc.presence&presentAllowedOrigin == 0 { // Check if origin was set as the primary indicator
		return "", false, false
	}
	// Return the stored values. Credential presence is implied by origin presence based on WithCORSInfo logic.
	return rc.allowedOrigin, rc.credentialsAllowed, true
}

// WithCORSRequestedHeaders stores the Access-Control-Request-Headers value from a CORS preflight request.
// This is used internally when the CORS configuration allows wildcard headers (*),
// so the exact requested headers can be echoed back in the Access-Control-Allow-Headers response.
// T is the User ID type (comparable), U is the User object type (any).
func WithCORSRequestedHeaders[T comparable, U any](ctx context.Context, requestedHeaders string) context.Context {
	rc, ctx := EnsureSRouterContext[T, U](ctx)
	rc.mu.Lock()
	rc.requestedHeaders = requestedHeaders
	rc.presence |= presentRequestedHeaders
	rc.mu.Unlock()
	return ctx
}

// GetCORSRequestedHeaders retrieves the Access-Control-Request-Headers value from the context.
// This is used internally by the CORS handler to echo back the requested headers
// when wildcard headers are allowed in the configuration.
// It returns the headers string and a boolean indicating whether it was found.
func GetCORSRequestedHeaders(ctx context.Context) (string, bool) {
	r, ok := getReader(ctx)
	if !ok {
		return "", false
	}
	return r.corsRequestedHeaders()
}

func (rc *SRouterContext[T, U]) corsRequestedHeaders() (string, bool) {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	if rc.presence&presentRequestedHeaders == 0 {
		return "", false
	}
	return rc.requestedHeaders, true
}

// WithHandlerError sets the handler error in the context. This is typically used by the framework
// to store errors returned by generic route handlers, making them available to middleware.
// T is the User ID type (comparable), U is the User object type (any).
func WithHandlerError[T comparable, U any](ctx context.Context, err error) context.Context {
	rc, ctx := EnsureSRouterContext[T, U](ctx)
	rc.mu.Lock()
	rc.handlerError = err
	rc.presence |= presentHandlerError
	rc.mu.Unlock()
	return ctx
}

// GetHandlerError retrieves the handler error from the context if one was set.
// This is useful for middleware that needs to react to errors returned by route handlers,
// such as transaction middleware that might rollback on errors.
func GetHandlerError(ctx context.Context) (error, bool) {
	r, ok := getReader(ctx)
	if !ok {
		return nil, false
	}
	return r.getHandlerError()
}

func (rc *SRouterContext[T, U]) getHandlerError() (error, bool) {
	rc.mu.RLock()
	defer rc.mu.RUnlock()
	if rc.presence&presentHandlerError == 0 {
		return nil, false
	}
	return rc.handlerError, true
}

// SRouter context copying functions
//
// The scontext package provides two functions for copying SRouterContext between contexts,
// each with different behavior for handling destination contexts:
//
// 1. CopySRouterContext: Attach a copied wrapper to the destination
//    - Copies source SRouterContext to destination
//    - Automatically creates SRouterContext in destination if needed
//    - Returns destination unchanged if source has no SRouterContext
//
// 2. CopySRouterContextOverlay: Conditionally replace an existing wrapper
//    - Only copies if destination already has an SRouterContext
//    - No-op if destination lacks SRouterContext (preserves original destination)
//    - Use when you want to update existing context without creating new structures
//
// Both functions allocate an independent wrapper and clone flags and pathParams.
// Pointer- and interface-valued fields continue to refer to the same underlying objects.

// cloneSRouterContext creates a new wrapper containing a snapshot of src.
// flags and pathParams are cloned because they are mutable collections. Values
// such as user, transaction, and handlerError are assigned normally, so any
// objects referenced by those fields remain shared with src.
// T is the User ID type (comparable), U is the User object type (any).
func cloneSRouterContext[T comparable, U any](src *SRouterContext[T, U]) *SRouterContext[T, U] {
	src.mu.RLock()
	defer src.mu.RUnlock()
	dst := &SRouterContext[T, U]{
		presence:           src.presence,
		userID:             src.userID,
		user:               src.user,
		buildID:            src.buildID,
		configID:           src.configID,
		traceID:            src.traceID,
		clientIP:           src.clientIP,
		userAgent:          src.userAgent,
		transaction:        src.transaction,
		routeTemplate:      src.routeTemplate,
		pathParams:         src.pathParams, // Will be deep copied below
		allowedOrigin:      src.allowedOrigin,
		credentialsAllowed: src.credentialsAllowed,
		requestedHeaders:   src.requestedHeaders,
		handlerError:       src.handlerError,
		logSource:          src.logSource,
		logger:             src.logger,
		logVersion:         src.logVersion,
		loggerVersion:      src.loggerVersion,
	}

	// Deep copy the flags map
	if src.flags != nil {
		dst.flags = make(map[string]bool, len(src.flags))
		maps.Copy(dst.flags, src.flags)
	} else {
		dst.flags = make(map[string]bool)
	}

	// Deep copy pathParams slice
	if src.pathParams != nil {
		dst.pathParams = make(httprouter.Params, len(src.pathParams))
		copy(dst.pathParams, src.pathParams)
	}

	return dst
}

// CopySRouterContext copies the SRouterContext wrapper from src and attaches the
// copy to dst. The returned context retains dst's cancellation and deadline chain.
//
// If src has no SRouterContext, dst is returned unchanged. The new wrapper has
// independent flags and pathParams collections. Reference-bearing fields such
// as user, transaction, and handlerError still refer to the same underlying
// objects; this is not a recursive deep copy.
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

// CopySRouterContextOverlay copies the SRouterContext wrapper from src and
// replaces the destination wrapper only when dst already has one.
//
// If either context lacks an SRouterContext, dst is returned unchanged. This
// function replaces rather than merges destination values. The new wrapper has
// independent flags and pathParams collections, but reference-bearing fields
// still refer to the same underlying objects as src.
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
