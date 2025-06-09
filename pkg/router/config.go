// Package router provides a flexible and feature-rich HTTP routing framework.
// It supports middleware, sub-routers, generic handlers, and various configuration options.
package router

import (
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
	// Collector is the metrics collector to use.
	// If nil, a default collector will be used if metrics are enabled.
	Collector any // metrics.Collector

	// MiddlewareFactory is the factory for creating metrics middleware.
	// If nil, a default middleware factory will be used if metrics are enabled.
	MiddlewareFactory any // metrics.MiddlewareFactory

	// Namespace for metrics.
	Namespace string

	// Subsystem for metrics.
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
// 
// Transaction Configuration:
// If any transaction is enabled (at global, sub-router, or route level),
// a TransactionFactory must be provided. The router will panic at startup
// if transactions are enabled without a factory.
type RouterConfig struct {
	ServiceName         string                            // Name of the service, used for metrics tagging etc.
	Logger              *zap.Logger                       // Logger for all router operations
	GlobalTimeout       time.Duration                     // Default response timeout for all routes
	GlobalMaxBodySize   int64                             // Default maximum request body size in bytes
	GlobalRateLimit     *common.RateLimitConfig[any, any] // Use common.RateLimitConfig // Default rate limit for all routes
	GlobalTransaction   *common.TransactionConfig         // Default transaction configuration for all routes
	IPConfig            *IPConfig                         // Configuration for client IP extraction
	EnableTraceLogging  bool                              // Enable trace logging
	TraceLoggingUseInfo bool                              // Use Info level for trace logging
	TraceIDBufferSize   int                               // Buffer size for trace ID generator (0 disables trace ID)
	MetricsConfig       *MetricsConfig                    // Metrics configuration (optional)
	SubRouters          []SubRouterConfig                 // Sub-routers with their own configurations
	Middlewares         []common.Middleware               // Global middlewares applied to all routes
	AddUserObjectToCtx  bool                              // Add user object to context
	CORSConfig          *CORSConfig                       // CORS configuration (optional, if nil CORS is disabled)
	TransactionFactory  common.TransactionFactory         // Factory for creating database transactions (required if any transaction is enabled)
}

// RouteDefinition is an interface that all route types must implement.
// This allows SubRouterConfig.Routes to store both standard and generic routes in a type-safe manner.
type RouteDefinition interface {
	isRouteDefinition()
}

// GenericRouteRegistrationFunc defines the function signature for registering a generic route declaratively.
// This function is stored in SubRouterConfig.Routes and called during router initialization.
// It receives the router instance and the SubRouterConfig it belongs to, allowing it to:
// - Calculate effective settings based on sub-router configuration
// - Apply the sub-router's path prefix
// - Combine middlewares appropriately
// - Register the route with all settings applied
//
// This type is used internally by NewGenericRouteDefinition.
type GenericRouteRegistrationFunc[T comparable, U any] func(r *Router[T, U], sr SubRouterConfig)

// Implement RouteDefinition for GenericRouteRegistrationFunc
func (GenericRouteRegistrationFunc[T, U]) isRouteDefinition() {}

// SubRouterConfig defines configuration for a group of routes with a common path prefix.
// This allows for organizing routes into logical groups and applying shared configuration.
// Sub-routers can be nested to create hierarchical routing structures with concatenated path prefixes.
//
// Important behaviors:
// - Path prefixes are concatenated when nesting (e.g., "/api" + "/v1" = "/api/v1")
// - Configuration overrides (timeout, max body size, rate limit) are NOT inherited by nested sub-routers
// - Middlewares are additive - they combine with parent and global middlewares
// - Routes can be added declaratively via the Routes field or imperatively after router creation
type SubRouterConfig struct {
	PathPrefix  string              // Common path prefix for all routes in this sub-router
	Overrides   common.RouteOverrides // Configuration overrides for routes in this sub-router (not inherited by nested sub-routers)
	Routes      []RouteDefinition   // Routes in this sub-router. Can contain RouteConfigBase or GenericRouteRegistrationFunc
	Middlewares []common.Middleware // Middlewares applied to all routes in this sub-router (additive with global middlewares)
	// SubRouters is a slice of nested sub-routers.
	// Nested sub-routers inherit the parent's path prefix (concatenated) but NOT configuration overrides.
	// Each nested sub-router must explicitly set its own overrides if needed.
	SubRouters []SubRouterConfig // Nested sub-routers with concatenated path prefixes
	AuthLevel  *AuthLevel        // Default authentication level for all routes in this sub-router (overridden by route-specific AuthLevel)
}

// RouteConfigBase defines the base configuration for a standard (non-generic) route.
// It includes settings for path, HTTP methods, authentication, timeouts, and middleware.
// This is used for routes that work directly with http.ResponseWriter and *http.Request.
//
// Configuration precedence (when used within a sub-router):
// - Route settings override sub-router settings
// - Sub-router settings override global settings
// - Middlewares are additive (not replaced)
type RouteConfigBase struct {
	Path        string              // Route path (will be prefixed with sub-router path prefix if applicable)
	Methods     []HttpMethod        // HTTP methods this route handles (use constants like MethodGet)
	AuthLevel   *AuthLevel          // Authentication level for this route. If nil, inherits from sub-router or defaults to NoAuth
	Overrides   common.RouteOverrides // Configuration overrides for this specific route
	Handler     http.HandlerFunc    // Standard HTTP handler function
	Middlewares []common.Middleware // Middlewares applied to this specific route (combined with sub-router and global middlewares)
}

// Implement RouteDefinition for RouteConfigBase
func (RouteConfigBase) isRouteDefinition() {}

// RouteConfig defines a route with generic request and response types.
// It provides type-safe request/response handling with automatic marshaling/unmarshaling.
// The framework handles decoding the request into type T and encoding the response of type U.
//
// Configuration precedence (when used within a sub-router):
// - Route settings override sub-router settings
// - Sub-router settings override global settings
// - Middlewares are additive (not replaced)
//
// Use NewGenericRouteDefinition to register these routes within SubRouterConfig.Routes.
type RouteConfig[T any, U any] struct {
	Path        string                // Route path (will be prefixed with sub-router path prefix if applicable)
	Methods     []HttpMethod          // HTTP methods this route handles (use constants like MethodGet)
	AuthLevel   *AuthLevel            // Authentication level for this route. If nil, inherits from sub-router or defaults to NoAuth
	Overrides   common.RouteOverrides // Configuration overrides for this specific route
	Codec       codec.Codec[T, U]     // Codec for marshaling/unmarshaling request and response (required)
	Handler     GenericHandler[T, U]  // Generic handler function (required)
	Middlewares []common.Middleware   // Middlewares applied to this specific route (combined with sub-router and global middlewares)
	SourceType  SourceType            // Where to retrieve request data from (defaults to Body)
	SourceKey   string                // Parameter name for query/path parameters (required when SourceType is not Body/Empty)
	Sanitizer   func(T) (T, error)    // Optional function to validate/transform request data after decoding
}


// GenericHandler defines a handler function with generic request and response types.
// It takes an http.Request and a typed request data object, and returns a typed response
// object and an error. This allows for strongly-typed request and response handling.
// The type parameters T and U represent the request and response data types respectively.
// When used with RegisterGenericRoute, the framework automatically handles decoding the
// request and encoding the response using the specified Codec.
type GenericHandler[T any, U any] func(r *http.Request, data T) (U, error)


// Ptr returns a pointer to the given AuthLevel value.
// Useful for setting AuthLevel fields in configurations.
func Ptr(level AuthLevel) *AuthLevel {
	return &level
}
