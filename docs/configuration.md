# Configuration Reference

This section details the configuration structs used by SRouter.

## `RouterConfig`

This is the main configuration struct passed to `router.NewRouter`.

```go
package router

import (
	"time"
	"github.com/Suhaibinator/SRouter/pkg/common"
	"github.com/Suhaibinator/SRouter/pkg/middleware"
	"go.uber.org/zap"
)

type RouterConfig struct {
	// Logger instance for all router operations. Required.
	Logger *zap.Logger

	// GlobalTimeout specifies the default response timeout for all routes.
	// Can be overridden by SubRouterConfig or RouteConfig. Zero means no timeout.
	GlobalTimeout time.Duration

	// GlobalMaxBodySize specifies the default maximum request body size in bytes.
	// Can be overridden. Zero or negative means no limit (use with caution).
	GlobalMaxBodySize int64

	// GlobalRateLimit specifies the default rate limit configuration for all routes.
	// Can be overridden. Nil means no global rate limit.
	GlobalRateLimit *middleware.RateLimitConfig[any, any]

	// IPConfig defines how client IP addresses are extracted.
	// See docs/ip-configuration.md. Nil uses default (RemoteAddr, no proxy trust).
	IPConfig *middleware.IPConfig

	// EnableMetrics globally enables or disables the metrics collection system.
	EnableMetrics bool

	// EnableTracing enables distributed tracing support (Note: Actual implementation
	// might be via specific tracing middleware rather than just this flag).
	EnableTracing bool // Check if still relevant or handled by middleware

	// EnableTraceID enables automatic trace ID generation and injection into context
	// and logs. Can also be achieved using middleware.TraceMiddleware().
	EnableTraceID bool

	// MetricsConfig holds detailed configuration for the metrics system when EnableMetrics is true.
	// See MetricsConfig section below and docs/metrics.md.
	MetricsConfig *MetricsConfig

	// SubRouters is a slice of SubRouterConfig structs defining sub-routers.
	SubRouters []SubRouterConfig

	// Middlewares is a slice of global middlewares applied to all routes.
	// Executed after internal middleware like timeout/body limit but before
	// sub-router and route-specific middleware.
	Middlewares []common.Middleware

	// AddUserObjectToCtx controls whether the built-in authentication middleware
	// (if used) should attempt to add the full user object (U) to the context,
	// in addition to the user ID (T). Often true if U provides useful info.
	AddUserObjectToCtx bool

	// --- Deprecated/Removed Fields (Example - check current source) ---
	// CacheGet, CacheSet, CacheKeyPrefix (Caching should now be implemented via middleware)
}
```

## `MetricsConfig`

Used within `RouterConfig` to configure the metrics system.

```go
package router

import "github.com/Suhaibinator/SRouter/pkg/metrics" // Assuming interfaces are here

type MetricsConfig struct {
	// Collector provides the implementation for creating and managing metric instruments.
	// Must implement metrics.Collector. Required if EnableMetrics is true.
	Collector any // Typically metrics.Collector

	// Exporter provides the implementation for exposing metrics (e.g., HTTP handler).
	// Optional. Might implement metrics.Exporter or metrics.HTTPExporter.
	Exporter any

	// MiddlewareFactory creates the metrics middleware.
	// Optional. If nil, SRouter likely uses a default factory. Must implement metrics.MiddlewareFactory.
	MiddlewareFactory any

	// Namespace for metrics (e.g., "myapp"). Used as a prefix.
	Namespace string

	// Subsystem for metrics (e.g., "http", "api"). Used as a prefix after Namespace.
	Subsystem string

	// EnableLatency enables collection of request latency metrics (histogram/summary).
	EnableLatency bool

	// EnableThroughput enables collection of request/response size metrics (histogram/summary).
	EnableThroughput bool

	// EnableQPS enables collection of request count metrics (counter, often labeled 'requests_total').
	EnableQPS bool

	// EnableErrors enables collection of HTTP error count metrics (counter, often labeled 'http_errors_total').
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
	"github.com/Suhaibinator/SRouter/pkg/middleware"
)

type SubRouterConfig struct {
	// PathPrefix is the common URL prefix for all routes and nested sub-routers
	// within this group (e.g., "/api/v1").
	PathPrefix string

	// TimeoutOverride overrides the global or parent sub-router's timeout.
	// Zero inherits the parent's value.
	TimeoutOverride time.Duration

	// MaxBodySizeOverride overrides the global or parent's max body size.
	// Zero inherits. Negative means no limit for this sub-router.
	MaxBodySizeOverride int64

	// RateLimitOverride overrides the global or parent's rate limit config.
	// Nil inherits.
	RateLimitOverride *middleware.RateLimitConfig[any, any]

	// Routes is a slice containing route definitions. Can hold:
	// - RouteConfigBase (for standard http.HandlerFunc routes)
	// - GenericRouteDefinition (for generic routes, created via NewGenericRouteDefinition)
	Routes []any

	// Middlewares is a slice of middlewares applied only to routes within this
	// sub-router (and its children), executed after global/parent middleware.
	Middlewares []common.Middleware

	// SubRouters defines nested sub-routers within this group.
	SubRouters []SubRouterConfig

	// AuthLevel sets the default authentication level for routes in this sub-router
	// if the route itself doesn't specify one. Nil inherits from parent or defaults to NoAuth.
	AuthLevel *AuthLevel

	// --- Deprecated/Removed Fields (Example - check current source) ---
	// CacheResponse, CacheKeyPrefix
}
```

## `RouteConfigBase`

Used for defining standard routes with `http.HandlerFunc`.

```go
package router

import (
	"net/http"
	"time"
	"github.com/Suhaibinator/SRouter/pkg/common"
	"github.com/Suhaibinator/SRouter/pkg/middleware"
)

type RouteConfigBase struct {
	// Path is the route path relative to any parent sub-router prefixes (e.g., "/users/:id"). Required.
	Path string

	// Methods is a slice of HTTP methods this route handles (e.g., []string{"GET", "POST"}). Required.
	Methods []string

	// AuthLevel specifies the authentication requirement for this route.
	// Nil inherits from parent sub-router or defaults to NoAuth.
	AuthLevel *AuthLevel

	// Timeout overrides the timeout specifically for this route. Zero inherits.
	Timeout time.Duration

	// MaxBodySize overrides the max body size specifically for this route. Zero inherits. Negative means no limit.
	MaxBodySize int64

	// RateLimit overrides the rate limit specifically for this route. Nil inherits.
	RateLimit *middleware.RateLimitConfig[any, any]

	// Handler is the standard Go HTTP handler function. Required.
	Handler http.HandlerFunc

	// Middlewares is a slice of middlewares applied only to this route, executed
	// after global and sub-router middlewares.
	Middlewares []common.Middleware
}
```

## `RouteConfig[T, U]`

Used for defining generic routes with type-safe request (`T`) and response (`U`) handling.

```go
package router

import (
	"net/http"
	"time"
	"github.com/Suhaibinator/SRouter/pkg/common"
	"github.com/Suhaibinator/SRouter/pkg/middleware"
)

// GenericHandler is the function signature for generic route handlers.
type GenericHandler[T any, U any] func(r *http.Request, req T) (U, error)

type RouteConfig[T any, U any] struct {
	// Path is the route path relative to any parent sub-router prefixes. Required.
	Path string

	// Methods is a slice of HTTP methods this route handles. Required.
	Methods []string

	// AuthLevel specifies the authentication requirement for this route.
	// Nil inherits.
	AuthLevel *AuthLevel

	// Timeout overrides the timeout specifically for this route. Zero inherits.
	Timeout time.Duration

	// MaxBodySize overrides the max body size specifically for this route. Zero inherits. Negative means no limit.
	MaxBodySize int64

	// RateLimit overrides the rate limit specifically for this route. Nil inherits.
	RateLimit *middleware.RateLimitConfig[any, any]

	// Codec is the encoder/decoder implementation for types T and U. Required.
	// Must implement the Codec[T, U] interface.
	Codec Codec[T, U]

	// Handler is the generic handler function. Required.
	Handler GenericHandler[T, U]

	// Middlewares is a slice of middlewares applied only to this route.
	Middlewares []common.Middleware

	// SourceType specifies where to retrieve the request data T from.
	// Defaults to Body. See docs/source-types.md.
	SourceType SourceType

	// SourceKey is used when SourceType is not Body (e.g., query or path parameter name).
	SourceKey string

	// --- Deprecated/Removed Fields (Example - check current source) ---
	// CacheResponse, CacheKeyPrefix
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
)
