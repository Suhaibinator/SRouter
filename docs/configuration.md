# Configuration Reference

This section details the configuration structs used by SRouter.

## `RouterConfig`

This is the main configuration struct passed to `router.NewRouter`.

```go
package router

import (
    "time"
    "github.com/Suhaibinator/SRouter/pkg/common"
    // "github.com/Suhaibinator/SRouter/pkg/middleware" // No longer needed for core config structs
    "go.uber.org/zap"
)

type RouterConfig struct {
    // ServiceName identifies the service (e.g., "user-api", "auth-service").
    // This name is used for tagging metrics and other identification purposes. Required.
    ServiceName string

    // Logger instance for all router operations. Required.
    Logger *zap.Logger

    // GlobalTimeout specifies the default response timeout for all routes.
    // Can be overridden by SubRouterConfig or RouteConfig via RouteOverrides.
    // Zero means no timeout.
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
    // Setting to 0 disables automatic trace ID generation. See docs/trace-logging.md.
    TraceIDBufferSize int

    // MetricsConfig holds detailed configuration for the metrics system.
    // Metrics are enabled when this field is non-nil. See docs/metrics.md.
    MetricsConfig *MetricsConfig

    // SubRouters defines the sub-routers to register. Required.
    SubRouters []SubRouterConfig

    // Middlewares is a slice of global middlewares applied to all routes.
    // Executed after internal middleware like timeout/body limit but before
    // sub-router and route-specific middleware.
    Middlewares []common.Middleware

    // AddUserObjectToCtx controls whether the built-in authentication middleware
    // (if used) should attempt to add the full user object (U) to the context,
    // in addition to the user ID (T).
    AddUserObjectToCtx bool

    // CORSConfig enables integrated CORS handling. If nil, CORS is disabled.
    // See docs/cors-configuration.md for details.
    CORSConfig *CORSConfig
}
```

## `MetricsConfig`

Used within `RouterConfig` to configure the metrics system.

```go
package router

import "github.com/Suhaibinator/SRouter/pkg/metrics"

type MetricsConfig struct {
    // Collector provides the implementation for creating and managing metric instruments.
    // Must implement metrics.MetricsRegistry. Required when metrics are enabled.
    Collector any // Typically metrics.MetricsRegistry

    // MiddlewareFactory creates the metrics middleware.
    // Optional. If nil, SRouter uses a default factory. Must implement metrics.MetricsMiddleware.
    MiddlewareFactory any

    // Namespace for metrics (e.g., "myapp"). Used as a prefix.
    Namespace string

    // Subsystem for metrics (e.g., "http", "api"). Used as a prefix after Namespace.
    Subsystem string

    // EnableLatency enables collection of request latency metrics (histogram/summary).
    EnableLatency bool

    // EnableThroughput enables collection of request/response size metrics (histogram/summary).
    EnableThroughput bool

    // EnableQPS enables collection of request count metrics (counter, often labeled "requests_total").
    EnableQPS bool

    // EnableErrors enables collection of HTTP error count metrics (counter, often labeled "http_errors_total").
    EnableErrors bool
}
```

## `SubRouterConfig`

Defines a group of routes under a common path prefix with shared configuration overrides. Can be nested.

```go
package router

import (
    "github.com/Suhaibinator/SRouter/pkg/common"
)

type SubRouterConfig struct {
    // PathPrefix is the common URL prefix for all routes and nested sub-routers
    // within this group (e.g., "/api/v1").
    PathPrefix string

    // Overrides contains timeout, max body size, and rate limit overrides
    // that apply to routes in this sub-router. These overrides are NOT inherited
    // by nested sub-routers.
    Overrides common.RouteOverrides

    // Routes defines the routes in this sub-router. Each element must implement
    // the RouteDefinition interface (RouteConfigBase or GenericRouteRegistrationFunc).
    Routes []RouteDefinition

    // Middlewares is a slice of middlewares applied to all routes in this
    // sub-router (additive with global middlewares).
    Middlewares []common.Middleware

    // SubRouters defines nested sub-routers within this group. Nested sub-routers
    // inherit the parent's path prefix but not its Overrides.
    SubRouters []SubRouterConfig

    // AuthLevel sets the default authentication level for routes in this sub-router
    // if the route itself doesn't specify one. Nil inherits from parent or defaults to NoAuth.
    AuthLevel *AuthLevel
}
```

## `RouteOverrides`

Overrides for timeouts, body size limits, and rate limits. Used in `SubRouterConfig`,
`RouteConfigBase`, and `RouteConfig`.

```go
package common

import "time"

type RouteOverrides struct {
    // Timeout overrides the default timeout. Zero means no override.
    Timeout time.Duration

    // MaxBodySize overrides the default maximum request body size in bytes.
    // Zero means no override; negative disables the limit.
    MaxBodySize int64

    // RateLimit overrides the default rate limit configuration.
    // Nil means no override.
    RateLimit *RateLimitConfig[any, any]
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

    // Methods is a slice of HTTP methods this route handles. Required.
    Methods []HttpMethod

    // AuthLevel specifies the authentication requirement for this route.
    // Nil inherits from parent sub-router or defaults to NoAuth.
    AuthLevel *AuthLevel

    // Overrides allows this route to override timeout, body size, or rate limit settings.
    Overrides common.RouteOverrides

    // Handler is the standard Go HTTP handler function. Required.
    Handler http.HandlerFunc

    // Middlewares is a slice of middlewares applied only to this route,
    // executed before global middlewares.
    Middlewares []common.Middleware
}
```

## `RouteConfig[T, U]`

Used for defining generic routes with type-safe request (`T`) and response (`U`) handling.

```go
package router

import (
    "net/http"
    "github.com/Suhaibinator/SRouter/pkg/codec"
    "github.com/Suhaibinator/SRouter/pkg/common"
)

// GenericHandler is the function signature for generic route handlers.
type GenericHandler[T any, U any] func(r *http.Request, data T) (U, error)

type RouteConfig[T any, U any] struct {
    // Path is the route path relative to any parent sub-router prefixes. Required.
    Path string

    // Methods is a slice of HTTP methods this route handles. Required.
    Methods []HttpMethod

    // AuthLevel specifies the authentication requirement for this route.
    // Nil inherits from the parent sub-router.
    AuthLevel *AuthLevel

    // Overrides allows this route to override timeout, body size, or rate limit settings.
    Overrides common.RouteOverrides

    // Codec is the encoder/decoder implementation for types T and U. Required.
    Codec codec.Codec[T, U]

    // Handler is the generic handler function. Required.
    Handler GenericHandler[T, U]

    // Middlewares is a slice of middlewares applied only to this route.
    Middlewares []common.Middleware

    // SourceType specifies where to retrieve the request data T from.
    // Defaults to Body. See docs/source-types.md.
    SourceType SourceType

    // SourceKey is used when SourceType is not Body (e.g., query or path parameter name).
    SourceKey string

    // Sanitizer allows validation or transformation of the decoded request before
    // it is passed to the handler. Optional.
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
