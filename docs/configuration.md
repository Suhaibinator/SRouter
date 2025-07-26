# Configuration Reference

This section details the configuration structs used by SRouter.

## `RouterConfig`

This is the main configuration struct passed to `router.NewRouter`.

```go
package router

import (
	"time"
	"github.com/Suhaibinator/SRouter/pkg/common"
	"go.uber.org/zap"
)

type RouterConfig struct {
	// ServiceName identifies the service (e.g., "user-api", "auth-service").
	// This name is used for tagging metrics and potentially other identification purposes. Required.
	ServiceName string

	// Logger instance for all router operations. Required.
	Logger *zap.Logger

	// GlobalTimeout specifies the default response timeout for all routes.
	// Can be overridden by SubRouterConfig or RouteConfig. Zero means no timeout.
	GlobalTimeout time.Duration

	// GlobalMaxBodySize specifies the default maximum request body size in bytes.
	// Can be overridden. Zero or negative means no limit (use with caution).
	GlobalMaxBodySize int64

	// GlobalRateLimit specifies the default rate limit configuration for all routes.
	// Uses common.RateLimitConfig. Can be overridden. Nil means no global rate limit.
	GlobalRateLimit *common.RateLimitConfig[any, any]

	// IPConfig defines how client IP addresses are extracted.
	// See docs/ip-configuration.md. Nil uses default (IPSourceXForwardedFor, TrustProxy: true).
	IPConfig *IPConfig // Defined in router package

	// EnableTraceLogging enables detailed request/response logging including timing, status, etc.
	// Often used in conjunction with TraceIDBufferSize > 0.
	EnableTraceLogging bool

	// TraceLoggingUseInfo controls the log level for trace logging.
	// If true, logs at Info level; otherwise, logs at Debug level.
	TraceLoggingUseInfo bool

	// TraceIDBufferSize sets the buffer size for the background trace ID generator.
	// If > 0, trace IDs are automatically generated and added to request context and logs.
	// Setting to 0 disables automatic trace ID generation. See docs/logging.md#trace-id-integration.
	TraceIDBufferSize int

	// MetricsConfig holds detailed configuration for the v2 metrics system.
	// Metrics are enabled when this field is non-nil. See docs/metrics.md.
	MetricsConfig *MetricsConfig

	// SubRouters is a slice of SubRouterConfig structs defining sub-routers. Required.
	SubRouters []SubRouterConfig

	// Middlewares is a slice of global middlewares applied to all routes.
	// Executed after internal middleware like timeout/body limit but before
	// sub-router and route-specific middleware.
	Middlewares []common.Middleware

	// AddUserObjectToCtx controls whether the built-in authentication middleware
	// (if used) should attempt to add the full user object (U) to the context,
	// in addition to the user ID (T). Often true if U provides useful info.
	AddUserObjectToCtx bool

	// CORSConfig defines the CORS configuration for cross-origin requests.
	// If nil, CORS is disabled. See CORSConfig struct for details.
	CORSConfig *CORSConfig
}
```

## `MetricsConfig`

Used within `RouterConfig` to configure the metrics system.

```go
package router

// No import needed - MetricsConfig uses any type for flexibility

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
```

## `SubRouterConfig`

Defines a group of routes under a common path prefix with shared configuration overrides. Can be nested.

```go
package router

import (
	"time"
	"github.com/Suhaibinator/SRouter/pkg/common"
)

type SubRouterConfig struct {
        // PathPrefix is the common URL prefix for all routes and nested sub-routers
        // within this group (e.g., "/api/v1").
        PathPrefix string

        // Overrides allows this sub-router to specify timeout, body size, or rate limit
        // settings that override the global configuration. Zero values mean no override.
        Overrides common.RouteOverrides

        // Routes is a slice containing route definitions. Must contain RouteConfigBase
        // or GenericRouteRegistrationFunc values.
        Routes []RouteDefinition

        // Middlewares is a slice of middlewares applied only to routes within this
        // sub-router (and its children), executed before global/parent middleware.
        Middlewares []common.Middleware

        // SubRouters defines nested sub-routers within this group.
        SubRouters []SubRouterConfig

	// AuthLevel sets the default authentication level for routes in this sub-router
	// if the route itself doesn't specify one. Nil inherits from parent or defaults to NoAuth.
	AuthLevel *AuthLevel
}
```

## `RouteConfigBase`

Used for defining standard routes with `http.HandlerFunc`.

```go
package router

import (
	"net/http"
	"github.com/Suhaibinator/SRouter/pkg/common"
)

type RouteConfigBase struct {
	// Path is the route path relative to any parent sub-router prefixes (e.g., "/users/:id"). Required.
	Path string

	// Methods is a slice of HTTP methods this route handles (e.g., []HttpMethod{MethodGet, MethodPost}). Required.
	Methods []HttpMethod // Use router.HttpMethod type

	// AuthLevel specifies the authentication requirement for this route.
	// Nil inherits from parent sub-router or defaults to NoAuth.
	AuthLevel *AuthLevel

        // Overrides allows this route to specify timeout, body size, or rate limit
        // settings. Zero values mean inherit from the sub-router or global configuration.
        Overrides common.RouteOverrides

	// Handler is the standard Go HTTP handler function. Required.
	Handler http.HandlerFunc

        // Middlewares is a slice of middlewares applied only to this route, executed
        // before global middlewares.
	Middlewares []common.Middleware
}
```

## `RouteConfig[T, U]`

Used for defining generic routes with type-safe request (`T`) and response (`U`) handling.

```go
package router

import (
	"net/http"
	"github.com/Suhaibinator/SRouter/pkg/common"
	"github.com/Suhaibinator/SRouter/pkg/codec"
)

// GenericHandler is the function signature for generic route handlers.
type GenericHandler[T any, U any] func(r *http.Request, req T) (U, error)

type RouteConfig[T any, U any] struct {
	// Path is the route path relative to any parent sub-router prefixes. Required.
	Path string

	// Methods is a slice of HTTP methods this route handles. Required.
	Methods []HttpMethod // Use router.HttpMethod type

	// AuthLevel specifies the authentication requirement for this route.
	// Nil inherits.
	AuthLevel *AuthLevel

        // Overrides allows this route to specify timeout, body size, or rate limit
        // settings. Zero values mean inherit from the sub-router or global configuration.
        Overrides common.RouteOverrides

	// Codec is the encoder/decoder implementation for types T and U. Required.
	// Must implement the codec.Codec[T, U] interface.
	Codec codec.Codec[T, U]

	// Handler is the generic handler function. Required.
	Handler GenericHandler[T, U]

	// Middlewares is a slice of middlewares applied only to this route.
	Middlewares []common.Middleware

	// SourceType specifies where to retrieve the request data T from.
	// Defaults to Body. See docs/generic-routes.md#source-types.
	SourceType SourceType

	// SourceKey is used when SourceType is not Body (e.g., query or path parameter name).
	SourceKey string

	// Sanitizer is an optional function to validate/transform request data after decoding.
	// Called after successful decoding but before handler execution.
	Sanitizer func(T) (T, error)
}
```

## `AuthLevel` (Enum)

Defines the authentication requirement levels.

```go
package router

type AuthLevel int

const (
	// NoAuth indicates that no authentication is required. (Default)
	NoAuth AuthLevel = iota
	// AuthOptional indicates that authentication is optional.
	AuthOptional
	// AuthRequired indicates that authentication is required.
	AuthRequired
)

// Ptr returns a pointer to an AuthLevel value (helper for config).
func Ptr(level AuthLevel) *AuthLevel {
	return &level
}
```

## `SourceType` (Enum)

Defines where generic request data is retrieved from.

```go
package router

type SourceType int

const (
	// Body retrieves data from the request body (default).
	Body SourceType = iota
	// Base64QueryParameter retrieves data from a base64-encoded query parameter.
	Base64QueryParameter
	// Base62QueryParameter retrieves data from a base62-encoded query parameter.
	Base62QueryParameter
	// Base64PathParameter retrieves data from a base64-encoded path parameter.
	Base64PathParameter
	// Base62PathParameter retrieves data from a base62-encoded path parameter.
	Base62PathParameter
	// Empty does not decode anything. Acts as a no-op for decoding.
	Empty
)
```

## `CORSConfig`

**MISSING FROM DOCUMENTATION** - This struct exists in the implementation but is not documented.

```go
package router

import "time"

// CORSConfig defines the configuration for Cross-Origin Resource Sharing (CORS).
type CORSConfig struct {
	Origins          []string      // Allowed origins (e.g., "http://example.com", "*"). Required.
	Methods          []string      // Allowed methods (e.g., "GET", "POST"). Defaults to simple methods if empty.
	Headers          []string      // Allowed headers. Defaults to simple headers if empty.
	ExposeHeaders    []string      // Headers the browser is allowed to access.
	AllowCredentials bool          // Whether to allow credentials (cookies, authorization headers).
	MaxAge           time.Duration // How long the results of a preflight request can be cached.
}
```