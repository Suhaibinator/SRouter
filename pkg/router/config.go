// Package router provides a flexible and feature-rich HTTP routing framework.
// It supports middleware, recursive route groups, generic handlers, and various configuration options.
package router

import (
	"context"
	"net/http"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/codec"
	"github.com/Suhaibinator/SRouter/pkg/common"

	// Removed: "github.com/Suhaibinator/SRouter/pkg/middleware"
	"go.uber.org/zap"
)

// CORSConfig defines the configuration for Cross-Origin Resource Sharing (CORS).
// It allows customization of which origins, methods, headers, and credentials are allowed
// for cross-origin requests, and which headers can be exposed to the client-side script.
type CORSConfig struct {
	Origins          []string      // Allowed origins (e.g., "http://example.com", "*"). Required.
	Methods          []string      // Allowed methods (e.g., "GET", "POST"). Defaults to simple methods if empty.
	Headers          []string      // Allowed headers. Defaults to simple headers if empty.
	ExposeHeaders    []string      // Headers the browser is allowed to access.
	AllowCredentials bool          // Whether to allow credentials (cookies, authorization headers).
	MaxAge           time.Duration // How long the results of a preflight request can be cached.
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
	// If authentication credentials are provided, they will be validated and the user
	// will be added to the request context if valid. If no credentials are provided
	// or they are invalid, the request will still proceed without a user in the context.
	AuthOptional

	// AuthRequired indicates that authentication is required for the route.
	// If authentication fails, the request will be rejected with a 401 Unauthorized response.
	// If authentication succeeds, the user will be added to the request context.
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

	// Empty does not decode anything. It acts as a noop for decoding.
	Empty
)

// MetricsConfig defines the configuration for metrics collection.
// It allows customization of how metrics are collected and exposed.
type MetricsConfig struct {
	// Collector is the metrics collector to use. It must implement
	// metrics.MetricsRegistry for metrics to be collected. If nil (or it does
	// not implement metrics.MetricsRegistry), no metrics middleware is installed.
	Collector any // metrics.MetricsRegistry

	// MiddlewareFactory optionally supplies a custom metrics middleware. If it
	// implements metrics.MetricsMiddleware[T, U] (with the router's type
	// parameters), it takes precedence over Collector and is used to wrap all
	// requests. Otherwise the router builds its own middleware from Collector.
	MiddlewareFactory any // metrics.MetricsMiddleware

	// Namespace for metrics. Applied as the "service" tag on all metrics
	// emitted by the built-in metrics middleware.
	Namespace string

	// Subsystem for metrics. Applied as the "subsystem" tag on all metrics
	// emitted by the built-in metrics middleware.
	Subsystem string

	// EnableLatency enables latency metrics.
	EnableLatency bool

	// EnableThroughput enables throughput metrics.
	EnableThroughput bool

	// EnableQPS enables queries per second metrics.
	EnableQPS bool

	// EnableErrors enables error metrics.
	EnableErrors bool
}

// RouterConfig defines the global configuration for the router.
// It includes settings for logging, timeouts, metrics, and middleware.
type RouterConfig struct {
	ServiceName         string                            // Name of the service, used for metrics tagging etc.
	Logger              *zap.Logger                       // Logger for all router operations
	GlobalTimeout       time.Duration                     // Default response timeout for all routes
	GlobalMaxBodySize   int64                             // Default maximum request body size in bytes
	GlobalRateLimit     *common.RateLimitConfig[any, any] // Use common.RateLimitConfig // Default rate limit for all routes
	GlobalAuthToken     *common.AuthTokenConfig           // Default auth token source for built-in auth middleware
	IPConfig            *IPConfig                         // Configuration for client IP extraction
	EnableTraceLogging  bool                              // Enable per-request summary logging even when TraceIDBufferSize is 0
	TraceLoggingUseInfo bool                              // Use Info level for trace logging
	TraceIDBufferSize   int                               // Buffer size for trace ID generator (0 disables trace ID)
	MetricsConfig       *MetricsConfig                    // Metrics configuration (optional)
	Middlewares         []common.Middleware               // Global middlewares applied to all routes
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
// RouteConfig values can be registered directly on Router or RouteGroup.
type RouteConfig[T any, U any] struct {
	Path           string                              // Route path, relative to its route group when grouped
	Methods        []HttpMethod                        // HTTP methods this route handles (use constants like MethodGet)
	AuthLevel      *AuthLevel                          // Authentication level for this route. If nil, inherits from its route group or defaults to NoAuth
	Overrides      common.RouteOverrides               // Configuration overrides for this specific route
	Codec          codec.Codec[T, U]                   // Codec for marshaling/unmarshaling request and response (required)
	Handler        GenericHandler[T, U]                // Generic handler function (required)
	Middlewares    []common.Middleware                 // Middlewares applied after global and route-group middlewares
	SourceType     SourceType                          // Where to retrieve request data from (defaults to Body)
	SourceKey      string                              // Parameter name for query/path parameters (required when SourceType is not Body/Empty)
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
