// Package router provides a flexible and feature-rich HTTP routing framework.
// It supports middleware, sub-routers, generic handlers, and various configuration options.
package router

import (
	"net/http"
	"time"

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

// PrometheusConfig is removed in favor of MetricsConfig with v2 metrics system.
// This type is kept for reference but should not be used.
// Deprecated: Use MetricsConfig instead.
type PrometheusConfig struct{}

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
type RouterConfig struct {
	ServiceName         string                            // Name of the service, used for metrics tagging etc.
	Logger              *zap.Logger                       // Logger for all router operations
	GlobalTimeout       time.Duration                     // Default response timeout for all routes
	GlobalMaxBodySize   int64                             // Default maximum request body size in bytes
	GlobalRateLimit     *common.RateLimitConfig[any, any] // Use common.RateLimitConfig // Default rate limit for all routes
	IPConfig            *IPConfig                         // Configuration for client IP extraction
	EnableTraceLogging  bool                              // Enable trace logging
	TraceLoggingUseInfo bool                              // Use Info level for trace logging
	TraceIDBufferSize   int                               // Buffer size for trace ID generator (0 disables trace ID)
	MetricsConfig       *MetricsConfig                    // Metrics configuration (optional)
	SubRouters          []SubRouterConfig                 // Sub-routers with their own configurations
	Middlewares         []common.Middleware               // Global middlewares applied to all routes
	AddUserObjectToCtx  bool                              // Add user object to context
	CORSConfig          *CORSConfig                       // CORS configuration (optional, if nil CORS is disabled)
}

// GenericRouteRegistrationFunc defines the function signature for registering a generic route declaratively.
// This function is stored in SubRouterConfig.Routes and called during router initialization.
// It captures the specific generic types and calls the appropriate registration logic.
type GenericRouteRegistrationFunc[T comparable, U any] func(r *Router[T, U], sr SubRouterConfig)

// SubRouterConfig defines configuration for a group of routes with a common path prefix.
// This allows for organizing routes into logical groups and applying shared configuration.
type SubRouterConfig struct {
	PathPrefix          string                            // Common path prefix for all routes in this sub-router
	TimeoutOverride     time.Duration                     // Override global timeout for all routes in this sub-router
	MaxBodySizeOverride int64                             // Override global max body size for all routes in this sub-router
	RateLimitOverride   *common.RateLimitConfig[any, any] // Use common.RateLimitConfig // Override global rate limit for all routes in this sub-router
	Routes              []any                             // Routes in this sub-router. Can contain RouteConfigBase or GenericRouteRegistrationFunc.
	Middlewares         []common.Middleware               // Middlewares applied to all routes in this sub-router
	// SubRouters is a slice of nested sub-routers
	// This allows for creating a hierarchy of sub-routers
	SubRouters []SubRouterConfig // Nested sub-routers
	AuthLevel  *AuthLevel        // Default authentication level for all routes in this sub-router (overridden by route-specific AuthLevel)
}

// RouteConfigBase defines the base configuration for a route without generics.
// It includes settings for path, HTTP methods, authentication, timeouts, and middleware.
type RouteConfigBase struct {
	Path        string                            // Route path (will be prefixed with sub-router path prefix if applicable)
	Methods     []HttpMethod                      // HTTP methods this route handles (use constants like MethodGet)
	AuthLevel   *AuthLevel                        // Authentication level for this route (NoAuth, AuthOptional, or AuthRequired). If nil, inherits from sub-router or defaults to NoAuth.
	Timeout     time.Duration                     // Override timeout for this specific route
	MaxBodySize int64                             // Override max body size for this specific route
	RateLimit   *common.RateLimitConfig[any, any] // Use common.RateLimitConfig // Rate limit for this specific route
	Handler     http.HandlerFunc                  // Standard HTTP handler function
	Middlewares []common.Middleware               // Middlewares applied to this specific route
}

// RouteConfig defines a route with generic request and response types.
// It extends RouteConfigBase with type parameters for request and response data,
// allowing for strongly-typed handlers and automatic marshaling/unmarshaling.
type RouteConfig[T any, U any] struct {
	Path        string                            // Route path (will be prefixed with sub-router path prefix if applicable)
	Methods     []HttpMethod                      // HTTP methods this route handles (use constants like MethodGet)
	AuthLevel   *AuthLevel                        // Authentication level for this route (NoAuth, AuthOptional, or AuthRequired). If nil, inherits from sub-router or defaults to NoAuth.
	Timeout     time.Duration                     // Override timeout for this specific route
	MaxBodySize int64                             // Override max body size for this specific route
	RateLimit   *common.RateLimitConfig[any, any] // Use common.RateLimitConfig // Rate limit for this specific route
	Codec       Codec[T, U]                       // Codec for marshaling/unmarshaling request and response
	Handler     GenericHandler[T, U]              // Generic handler function
	Middlewares []common.Middleware               // Middlewares applied to this specific route
	SourceType  SourceType                        // How to retrieve request data (defaults to Body)
	SourceKey   string                            // Query parameter name (only used for query parameters)
	Sanitizer   func(T) (T, error)                // Optional sanitizer function applied after decoding
}

// Middleware is an alias for common.Middleware.
// It represents a function that wraps an http.Handler to provide additional functionality.
type Middleware = common.Middleware

// GenericHandler defines a handler function with generic request and response types.
// It takes an http.Request and a typed request data object, and returns a typed response
// object and an error. This allows for strongly-typed request and response handling.
// The type parameters T and U represent the request and response data types respectively.
// When used with RegisterGenericRoute, the framework automatically handles decoding the
// request and encoding the response using the specified Codec.
type GenericHandler[T any, U any] func(r *http.Request, data T) (U, error)

// Codec defines an interface for marshaling and unmarshaling request and response data.
// It provides methods for creating new request objects, decoding request data, and encoding
// response data. This allows for different data formats (e.g., JSON, Protocol Buffers).
// The framework includes implementations for JSON and Protocol Buffers in the codec package.
type Codec[T any, U any] interface {
	// NewRequest creates a new zero-value instance of the request type T.
	// This is used by the framework to get an instance for decoding, avoiding reflection.
	NewRequest() T

	// Decode extracts and deserializes data from an HTTP request body into a value of type T.
	// The type T represents the request data type.
	Decode(r *http.Request) (T, error)

	// DecodeBytes extracts and deserializes data from a byte slice into a value of type T.
	// This is used for source types where the data is already extracted (e.g., query/path parameters).
	// The type T represents the request data type.
	DecodeBytes(data []byte) (T, error)

	// Encode serializes a value of type U and writes it to the HTTP response.
	// It converts the Go value to the wire format (e.g., JSON, Protocol Buffers) and
	// sets appropriate headers (e.g., Content-Type). If the serialization fails, it returns an error.
	// The type U represents the response data type.
	Encode(w http.ResponseWriter, resp U) error
}

// Ptr returns a pointer to the given AuthLevel value.
// Useful for setting AuthLevel fields in configurations.
func Ptr(level AuthLevel) *AuthLevel {
	return &level
}
