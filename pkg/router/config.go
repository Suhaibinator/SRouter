package router

import (
	"context"
	"net/http"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/codec"
	"github.com/Suhaibinator/SRouter/pkg/common"
	"go.uber.org/zap"
)

// CORSConfig defines the configuration for Cross-Origin Resource Sharing (CORS).
// It allows customization of which origins, methods, headers, and credentials are allowed
// for cross-origin requests, and which headers can be exposed to the client-side script.
type CORSConfig struct {
	Origins []string // Allowed origins (e.g., "http://example.com", "*"). Empty allows none.

	// Methods contains case-sensitive HTTP method names. Empty defaults to GET,
	// HEAD, and POST. Method wildcards are not supported.
	Methods []string

	// Headers contains case-insensitive request-header names. Empty defaults to
	// Accept, Accept-Language, Content-Language, and Content-Type. "*" accepts
	// every requested header and echoes the requested names in preflight responses.
	Headers          []string
	ExposeHeaders    []string      // Headers the browser is allowed to access.
	AllowCredentials bool          // Whether to allow credentials (cookies, authorization headers).
	MaxAge           time.Duration // Preflight cache duration; values below one second are omitted.
}

// HttpMethod defines the type for HTTP methods.
type HttpMethod string

// Constants for standard HTTP methods.
const (
	MethodGet     HttpMethod = http.MethodGet
	MethodHead    HttpMethod = http.MethodHead
	MethodPost    HttpMethod = http.MethodPost
	MethodPut     HttpMethod = http.MethodPut
	MethodPatch   HttpMethod = http.MethodPatch // RFC 5789
	MethodDelete  HttpMethod = http.MethodDelete
	MethodConnect HttpMethod = http.MethodConnect
	MethodOptions HttpMethod = http.MethodOptions
	MethodTrace   HttpMethod = http.MethodTrace
)

// AuthLevel defines the authentication level for a route.
// It determines how authentication is handled for the route.
type AuthLevel int

const (
	// NoAuth indicates that no authentication is required for the route.
	// The route will be accessible without any authentication.
	NoAuth AuthLevel = iota

	// AuthOptional indicates that authentication is optional for the route.
	// Valid credentials add the user ID to the request context; the user object
	// is also added when RouterConfig.AddUserObjectToCtx is true. Missing or
	// invalid credentials allow the request to proceed without a new identity.
	AuthOptional

	// AuthRequired indicates that authentication is required for the route.
	// If authentication fails, the request will be rejected with a 401 Unauthorized response.
	// Success adds the user ID and, when AddUserObjectToCtx is true, the user object.
	AuthRequired
)

// SourceType defines where to retrieve request data from.
// It determines how the request data is extracted and decoded.
type SourceType int

const (
	// Body retrieves data from the request body (default).
	// The request body is read and passed directly to the codec for decoding.
	Body SourceType = iota

	// Base64QueryParameter retrieves data from a base64-encoded query parameter.
	// The query parameter value is decoded from base64 before being passed to the codec.
	Base64QueryParameter

	// Base62QueryParameter retrieves data from a base62-encoded query parameter.
	// The query parameter value is decoded from base62 before being passed to the codec.
	Base62QueryParameter

	// Base64PathParameter retrieves data from a base64-encoded path parameter.
	// The path parameter value is decoded from base64 before being passed to the codec.
	Base64PathParameter

	// Base62PathParameter retrieves data from a base62-encoded path parameter.
	// The path parameter value is decoded from base62 before being passed to the codec.
	Base62PathParameter

	// Empty skips request decoding and passes the zero value of the request type
	// to the handler.
	Empty
)

// MetricsConfig defines the configuration for metrics collection.
// It allows customization of how metrics are collected and exposed.
type MetricsConfig struct {
	// Collector is the metrics collector to use. It must implement
	// metrics.MetricsRegistry for the router to build its default middleware.
	// If it is unusable, metrics remain disabled unless MiddlewareFactory supplies
	// a compatible custom middleware.
	Collector any // metrics.MetricsRegistry

	// MiddlewareFactory optionally supplies a custom metrics middleware. If it
	// implements metrics.MetricsMiddleware[T, U] (with the router's type
	// parameters), it takes precedence over Collector and is used on every
	// matched route that reaches the global-middleware stage. Otherwise the
	// router builds its own middleware from Collector.
	MiddlewareFactory any // metrics.MetricsMiddleware

	// Namespace for metrics. Applied as the "service" tag on all metrics
	// emitted by the built-in metrics middleware.
	Namespace string

	// Subsystem for metrics. Applied as the "subsystem" tag on all metrics
	// emitted by the built-in metrics middleware.
	Subsystem string

	// EnableLatency enables latency metrics.
	EnableLatency bool

	// EnableThroughput records positive request Content-Length values.
	EnableThroughput bool

	// EnableQPS enables cumulative request counters; derive a rate in the backend.
	EnableQPS bool

	// EnableErrors enables error metrics.
	EnableErrors bool
}

// RouterConfig defines the global configuration for the router.
// It includes settings for logging, timeouts, metrics, and middleware.
type RouterConfig struct {
	ServiceName         string                            // Fallback handler name passed to configured metrics middleware
	Logger              *zap.Logger                       // Logger for all router operations
	GlobalTimeout       time.Duration                     // Default response timeout for all routes
	GlobalMaxBodySize   int64                             // Default maximum request body size in bytes
	GlobalRateLimit     *common.RateLimitConfig[any, any] // Default rate limit for all routes
	GlobalAuthToken     *common.AuthTokenConfig           // Default auth token source for built-in auth middleware
	IPConfig            *IPConfig                         // Configuration for client IP extraction
	EnableTraceLogging  bool                              // Enable per-request summary logging even when TraceIDBufferSize is 0
	TraceLoggingUseInfo bool                              // Promote otherwise-successful request summaries from Debug to Info
	TraceIDBufferSize   int                               // Buffer size for trace ID generator (0 disables trace ID)
	MetricsConfig       *MetricsConfig                    // Metrics configuration (optional)
	Middlewares         []common.Middleware               // Global middleware for matched requests that pass built-in auth and rate limiting
	AddUserObjectToCtx  bool                              // Add user object to context
	CORSConfig          *CORSConfig                       // CORS configuration (optional, if nil CORS is disabled)
}

// routeRuntime is the non-generic bridge between route definitions and a Router.
// It keeps Router's user types out of RouteDefinition while preserving typed generic
// request and response handling inside RouteConfig.
type routeRuntime interface {
	handleError(http.ResponseWriter, *http.Request, error, int, string)
	recordHandlerError(*http.Request, error)
	warnMissingSanitizer(string, []HttpMethod)
}

// RouteDefinition is implemented by both RouteConfigBase and every
// RouteConfig[Req, Resp] instantiation. The unexported method intentionally
// seals the interface to route definitions provided by this package.
type RouteDefinition interface {
	baseConfig(routeRuntime, string) (RouteConfigBase, error)
}

// RouteConfigBase defines the base configuration for a standard (non-generic) route.
// It includes settings for path, HTTP methods, authentication, timeouts, and middleware.
// This is used for routes that work directly with http.ResponseWriter and *http.Request.
//
// Configuration precedence (when used within a route group):
// - Route settings override route-group settings
// - Route-group settings override global settings
// - Middlewares are additive (not replaced)
//
// Path is absolute for a root route and relative to its group. Methods and
// Handler are required. Build reports invalid paths and methods, negative
// timeout or body-size values, invalid authentication levels, and nil
// middleware before the router begins serving.
type RouteConfigBase struct {
	Path           string                // Route path, relative to its route group when grouped
	Methods        []HttpMethod          // HTTP methods this route handles (use constants like MethodGet)
	AuthLevel      *AuthLevel            // Authentication level for this route. If nil, inherits from its route group or defaults to NoAuth
	Overrides      common.RouteOverrides // Configuration overrides for this specific route
	Handler        http.HandlerFunc      // Standard HTTP handler function
	Middlewares    []common.Middleware   // Middlewares applied after global and route-group middlewares
	DisableTimeout bool                  // Indicates if the timeout should be disabled for this route (e.g., for WebSockets or long-lived connections).
}

func (route RouteConfigBase) baseConfig(routeRuntime, string) (RouteConfigBase, error) {
	return route, nil
}

// RouteConfig defines a route with generic request and response types.
// It provides type-safe request/response handling with automatic marshaling/unmarshaling.
// The framework handles decoding the request into type T and encoding the response of type U.
//
// Configuration precedence (when used within a route group):
// - Route settings override route-group settings
// - Route-group settings override global settings
// - Middlewares are additive (not replaced)
//
// RouteConfig values can be registered directly on Router or RouteGroup. Codec,
// Handler, and Methods are required. Query sources also require SourceKey; path
// sources use the first path parameter when SourceKey is empty.
type RouteConfig[T any, U any] struct {
	Path           string                              // Route path, relative to its route group when grouped
	Methods        []HttpMethod                        // HTTP methods this route handles (use constants like MethodGet)
	AuthLevel      *AuthLevel                          // Authentication level for this route. If nil, inherits from its route group or defaults to NoAuth
	Overrides      common.RouteOverrides               // Configuration overrides for this specific route
	Codec          codec.Codec[T, U]                   // Codec for marshaling/unmarshaling request and response (required)
	Handler        GenericHandler[T, U]                // Generic handler function (required)
	Middlewares    []common.Middleware                 // Middlewares applied after global and route-group middlewares
	SourceType     SourceType                          // Where to retrieve request data from (defaults to Body)
	SourceKey      string                              // Query parameter name (required) or path parameter name (optional; defaults to the first path parameter)
	Sanitizer      func(context.Context, T) (T, error) // Optional function to validate/transform request data after decoding
	DisableTimeout bool                                // Indicates if the timeout should be disabled for this route (e.g., for WebSockets or long-lived connections).
}

// GenericHandler defines a handler function with generic request and response types.
// It takes an http.Request and a typed request data object, and returns a typed response
// object and an error. This allows for strongly-typed request and response handling.
// The type parameters T and U represent the request and response data types respectively.
// When registered with Router.Route or RouteGroup.Route, the framework automatically handles decoding the
// request and encoding the response using the specified Codec.
type GenericHandler[T any, U any] func(r *http.Request, data T) (U, error)
