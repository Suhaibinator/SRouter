# Configuration reference

SRouter separates application-wide infrastructure in `RouterConfig` from the
recursive route tree built with `Router` and `RouteGroup` methods.

## `RouterConfig`

```go
type RouterConfig struct {
	ServiceName         string
	BuildIDProvider     func() string
	ConfigIDProvider    func() string
	Logger              *zap.Logger
	GlobalTimeout       time.Duration
	GlobalMaxBodySize   int64
	GlobalRateLimit     *common.RateLimitConfig[any, any]
	GlobalAuthToken     *common.AuthTokenConfig
	IPConfig            *IPConfig
	EnableTraceLogging  bool
	TraceLoggingUseInfo bool
	TraceIDBufferSize   int
	MetricsConfig       *MetricsConfig
	Middlewares         []common.Middleware
	AddUserObjectToCtx  bool
	CORSConfig          *CORSConfig
}
```

`BuildIDProvider` and `ConfigIDProvider` are optional. SRouter invokes each
configured provider once per request and stores a non-empty result in the shared
SRouter context. Returned strings are opaque, log-safe identifiers: SRouter does
not parse, normalize, cache, or propagate them through headers. Providers must
be concurrency-safe, fast, and non-panicking.

- Global timeout, body-size, rate-limit, and auth-token settings are defaults
  inherited by route groups and routes.
- Zero `GlobalTimeout` and `GlobalMaxBodySize` disable those policies. Negative
  values are rejected by `Build`.
- `Middlewares` run before group and route middleware for matched requests that
  pass the built-in authentication and rate-limit stages.
- A nil logger is replaced with a production logger, falling back to a no-op
  logger if creation fails.
- A nil `CORSConfig` disables CORS handling.

Routes do not live inside `RouterConfig`. Add them after `NewRouter` with
`Router.Route` and `Router.Group`.

The authentication callbacks passed to `NewRouter` may be nil when every route
resolves to `NoAuth`. If any route resolves to `AuthOptional` or `AuthRequired`,
`Build` requires both the token-validation callback and the user-ID extraction
callback.

The router itself is the root group. Its fluent `Use`, `Timeout`,
`MaxBodySize`, `RateLimit`, `AuthToken`, and `Auth` methods override global
defaults for the entire route tree; `RateLimit` is typed to the router's
`UserID` and `User` parameters.

## Route groups

```go
api := r.Group("/api").
	Timeout(3 * time.Second).
	MaxBodySize(2 << 20).
	RateLimit(apiRateLimit).
	AuthToken(apiTokenConfig).
	Auth(router.AuthRequired).
	Use(apiMiddleware)

v1 := api.Group("/v1")
v1.Route(routeA, routeB)
```

Every group can recursively call:

```go
Group(prefix string) *RouteGroup[UserID, User]
Route(routes ...RouteDefinition) *RouteGroup[UserID, User]
Use(middlewares ...common.Middleware) *RouteGroup[UserID, User]
Timeout(timeout time.Duration) *RouteGroup[UserID, User]
MaxBodySize(bytes int64) *RouteGroup[UserID, User]
RateLimit(config *common.RateLimitConfig[UserID, User]) *RouteGroup[UserID, User]
AuthToken(config *common.AuthTokenConfig) *RouteGroup[UserID, User]
Auth(level AuthLevel) *RouteGroup[UserID, User]
```

Policy inherits independently for each field. A zero timeout/body limit or nil
rate limit explicitly disables that inherited policy. A nil auth-token config
resets to the built-in `Authorization` header source. See
[Route groups](route-groups.md) for lifecycle and middleware order.

## `RouteConfigBase`

```go
type RouteConfigBase struct {
	Path           string
	Methods        []HttpMethod
	AuthLevel      *AuthLevel
	Overrides      common.RouteOverrides
	Handler        http.HandlerFunc
	Middlewares    []common.Middleware
	DisableTimeout bool
}
```

- `Path` is absolute when registered on the root router and relative to a group.
  An empty path matches a non-root group's exact prefix.
- `Methods` and `Handler` are required.
- A nil `AuthLevel` inherits from the innermost group, ultimately defaulting to
  `NoAuth`.
- `Overrides` wins over group and global policy for values it sets.
- `DisableTimeout` explicitly disables the effective timeout for long-lived
  handlers such as WebSockets and SSE.

## `RouteConfig[Req, Resp]`

```go
type RouteConfig[Req any, Resp any] struct {
	Path           string
	Methods        []HttpMethod
	AuthLevel      *AuthLevel
	Overrides      common.RouteOverrides
	Codec          codec.Codec[Req, Resp]
	Handler        GenericHandler[Req, Resp]
	Middlewares    []common.Middleware
	SourceType     SourceType
	SourceKey      string
	Sanitizer      func(context.Context, Req) (Req, error)
	DisableTimeout bool
}
```

Typed configs implement the same sealed `RouteDefinition` interface as
`RouteConfigBase`, so both can be mixed in one `Route` call. The codec and
handler retain their concrete request and response types through compilation.

## `common.RouteOverrides`

```go
type RouteOverrides struct {
	Timeout     time.Duration
	MaxBodySize int64
	RateLimit   *RateLimitConfig[any, any]
	AuthToken   *AuthTokenConfig
}
```

Route overrides use non-zero/non-nil values to replace inherited policy. Use
`DisableTimeout` to disable a route timeout; use the corresponding group policy
method with zero/nil to disable inherited group policy for a subtree.

## Authentication levels

```go
type AuthLevel int

const (
	NoAuth AuthLevel = iota
	AuthOptional
	AuthRequired
)
```

Use `new(router.AuthRequired)` when setting a route field. Groups take the value
directly: `group.Auth(router.AuthRequired)`.

## Generic request sources

```go
const (
	Body SourceType = iota
	Base64QueryParameter
	Base62QueryParameter
	Base64PathParameter
	Base62PathParameter
	Empty
)
```

`SourceKey` is required for query-parameter sources. For path-parameter sources
it selects a named path parameter; when it is empty, SRouter uses the first
matched path parameter. `Body` and `Empty` ignore it.

## Build lifecycle

```go
if err := r.Build(); err != nil {
	log.Fatal(err)
}
```

The first `Build` call validates and compiles a dispatcher, freezes the route
tree, and caches either success or failure. Later calls return that cached
result. `ServeHTTP` calls `Build` lazily if necessary. Register all routes and
groups before either event; mutation after the first build attempt panics.

Build errors include invalid paths, negative timeout/body-size values, missing
handlers or methods, nil middleware, missing authentication callbacks for
authenticated routes, duplicate routes, and underlying path conflicts. Rate
limit values such as `Limit`, `Window`, and a custom key extractor are not
validated by `Build`; validate them while constructing application config.
