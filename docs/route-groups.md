# Route groups

SRouter has one runtime `Router` and one underlying `httprouter` dispatcher.
`RouteGroup` values organize that router into a recursive route tree; they are
not independent HTTP handlers.

```go
r := router.NewRouter[string, User](config, authenticate, userID)

api := r.Group("/api").
	Timeout(3 * time.Second).
	MaxBodySize(2 << 20).
	Use(apiMiddleware)

v1 := api.Group("/v1").Auth(router.AuthRequired)
users := v1.Group("/users")

users.Route(
	router.RouteConfigBase{
		Path:    "/:id",
		Methods: []router.HttpMethod{router.MethodGet},
		Handler: getUser,
	},
	router.RouteConfig[CreateUserRequest, CreateUserResponse]{
		Path:    "",
		Methods: []router.HttpMethod{router.MethodPost},
		Codec:   codec.NewJSONCodec[CreateUserRequest, CreateUserResponse](),
		Handler: createUser,
	},
)
```

The registered paths are `GET /api/v1/users/:id` and
`POST /api/v1/users`. A route path may be empty when it should match its
group's exact prefix.

## Recursive grouping

Every group can create children with `Group`, add routes with `Route`, and add
middleware with `Use`. Keep the returned handle when routes are assembled by
different packages:

```go
admin := r.Group("/admin").Auth(router.AuthRequired)
registerUsers(admin.Group("/users"))
registerAudit(admin.Group("/audit"))
```

There is no string lookup API. Code adds a route to the exact group handle it
owns, so repeated relative prefixes cannot be ambiguous.

## Inheritance

Group policy inherits one setting at a time from its parent. A child only
changes the settings whose methods it calls:

- `Timeout(duration)`; zero explicitly disables the inherited timeout.
- `MaxBodySize(bytes)`; zero explicitly disables the inherited body limit.
- `RateLimit(config)`; nil explicitly disables inherited rate limiting.
- `AuthToken(config)`; nil resets to the built-in `Authorization` header.
- `Auth(level)` sets the default authentication level.

Middleware is additive. Public middleware executes in this order:

1. `RouterConfig.Middlewares`
2. root-group middleware added with `Router.Use`
3. outermost group middleware
4. nested group middleware, from outer to inner
5. route middleware
6. route handler

Route-specific auth, timeout, body-size, rate-limit, and auth-token settings
still take precedence over inherited group policy.

## Build and freeze

Call `Build` during startup to validate and compile the complete tree:

```go
if err := r.Build(); err != nil {
	log.Fatal(err)
}
```

`ServeHTTP` calls `Build` automatically when it has not already run. The first
build freezes the route tree. Calling `Route`, `Group`, `Use`, or a group policy
method afterward panics because the underlying dispatcher does not support
concurrent mutation.

Build validates paths, methods, handlers, middleware, policy values, duplicate
routes, and `httprouter` path conflicts. Groups compile away after startup;
steady-state dispatch uses the single underlying router with no group traversal.
