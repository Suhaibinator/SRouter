# Authentication

SRouter offers route-aware built-in token authentication and also supports ordinary HTTP authentication middleware.

## Authentication levels

Set an `AuthLevel` on a route or inherit one from `Router.Auth` or `RouteGroup.Auth`:

```go
r.Auth(router.NoAuth)

r.Group("/account").Auth(router.AuthRequired).Route(router.RouteConfigBase{
	Path:    "/profile",
	Methods: []router.HttpMethod{router.MethodGet},
	Handler: profileHandler,
})

r.Route(router.RouteConfigBase{
	Path:      "/welcome",
	Methods:   []router.HttpMethod{router.MethodGet},
	AuthLevel: new(router.AuthOptional),
	Handler:   welcomeHandler,
})
```

- `NoAuth` does not run built-in authentication. This is the default.
- `AuthOptional` attempts authentication. The handler still runs without user context when the token is missing or invalid.
- `AuthRequired` requires successful authentication; otherwise SRouter returns a JSON 401 response and does not call later middleware or the handler.

A route-level value wins over its innermost group, and inner groups win over outer groups and the router root.

## Router authentication functions

Built-in authentication uses `RouterDependencies` passed to `NewRouter`:

```go
func authenticate(ctx context.Context, token string) (*User, bool) {
	user, err := users.FindByToken(ctx, token)
	if err != nil {
		return nil, false
	}
	return user, true
}

func userID(user *User) string {
	return user.ID
}

r := router.NewRouter(config, router.RouterDependencies[string, User]{
	Authenticate: authenticate,
	UserID:       userID,
})
```

On success, `authenticate` must return a usable `*User` and `true`. SRouter passes that pointer to `userID`, stores the resulting ID in `SRouterContext`, and stores the user pointer as well when `RouterConfig.AddUserObjectToCtx` is true.

The authentication dependencies are required only if at least one compiled route resolves to `AuthOptional` or `AuthRequired`. `Build` fails with a descriptive error when such a route exists and either dependency is nil. A router containing only `NoAuth` routes can use:

```go
r := router.NewRouter[string, User](config, router.RouterDependencies[string, User]{})
```

Calling `Build` during startup is recommended so callback and route configuration errors are reported before serving traffic. Otherwise the first request triggers the build.

## Token source

The default source is the `Authorization` header. SRouter removes an exact, case-sensitive `Bearer ` prefix when present and passes the remaining value to `authenticate`; a value without that prefix is passed unchanged.

Configure a header or cookie globally, on a group, or on one route:

```go
config.GlobalAuthToken = &common.AuthTokenConfig{
	Source:     common.AuthTokenSourceCookie,
	CookieName: "session",
}

api := r.Group("/api").AuthToken(&common.AuthTokenConfig{
	Source:     common.AuthTokenSourceHeader,
	HeaderName: "X-API-Token",
})

api.Route(router.RouteConfigBase{
	Path:    "/admin",
	Methods: []router.HttpMethod{router.MethodGet},
	Overrides: common.RouteOverrides{
		AuthToken: &common.AuthTokenConfig{
			Source:     common.AuthTokenSourceCookie,
			CookieName: "admin_session",
		},
	},
	Handler: adminHandler,
})
```

Precedence is route, innermost group, outer groups, global configuration, then the built-in `Authorization` default. `group.AuthToken(nil)` deliberately resets that subtree to the built-in header source instead of inheriting its parent.

The resolved source is the only source checked for a request. SRouter does not
fall back from a missing configured cookie to a header, or from a missing
configured header to a cookie. Missing or invalid credentials therefore leave
an `AuthOptional` request unauthenticated and cause an `AuthRequired` request
to return 401.

For a header source, an empty `HeaderName` becomes `Authorization`. A cookie
source with an empty `CookieName` cannot extract a token and logs a build-time
warning. An `AuthRequired` route that falls back to the completely implicit
`Authorization` default also logs a build-time warning so the source is visible
during deployment.

## Reading authenticated users

Use the typed context helpers:

```go
userID, authenticated := scontext.GetUserIDFromRequest[string, User](req)
user, userStored := scontext.GetUserFromRequest[string, User](req)
```

`userID` is available after successful built-in authentication. For built-in
authentication, `user` is stored only when `AddUserObjectToCtx` is enabled.
Custom middleware may store a user explicitly with `scontext.WithUser`. Always
check the booleans on `AuthOptional` routes.

## Custom authentication middleware

Custom middleware must populate the same typed context when it authenticates a request:

```go
func apiKeyAuth(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		user, ok := validateAPIKey(req.Header.Get("X-API-Key"))
		if !ok {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		ctx := scontext.WithUserID[string, User](req.Context(), user.ID)
		ctx = scontext.WithUser[string](ctx, user)
		next.ServeHTTP(w, req.WithContext(ctx))
	})
}
```

Middleware added through `RouterConfig.Middlewares`, `group.Use`, or route middleware runs after built-in authentication and rate limiting. Set the route's built-in level to `NoAuth` when the custom middleware owns the authentication decision.

### Custom authentication and rate limiting

Because the built-in rate limiter runs before router, group, and route custom middleware, a `StrategyUser` limit cannot see a user ID created by custom authentication at those scopes. It falls back to client IP instead.

Choose one of these arrangements when user-based limiting must use custom authentication:

- Wrap the whole router with authentication before assigning it to `http.Server.Handler`:

  ```go
  r := router.NewRouter[string, User](config, router.RouterDependencies[string, User]{})
  srv := &http.Server{Handler: corsAwareAPIKeyAuth(r)}
  ```

- Use built-in authentication so the user ID is populated before the built-in limiter.
- Use `StrategyCustom` with a key extractor that derives the required stable identity directly from the request.
- Implement authentication and rate limiting together in an external middleware chain where you control their order.

An external authentication wrapper runs before SRouter's CORS handling. When
`CORSConfig` is definitely enabled, a wrapper can pass only recognizable
preflights through to the router:

```go
func corsAwareAPIKeyAuth(next http.Handler) http.Handler {
	secured := apiKeyAuth(next)
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		isPreflight := req.Method == http.MethodOptions &&
			req.Header.Get("Origin") != "" &&
			req.Header.Get("Access-Control-Request-Method") != ""
		if isPreflight {
			next.ServeHTTP(w, req) // configured SRouter CORS will terminate it
			return
		}
		secured.ServeHTTP(w, req)
	})
}
```

Do not use this bypass when CORS is disabled or when the downstream handler
does not guarantee preflight interception. An alternative is an external CORS
layer placed outside authentication.

CORS preflight requests are handled before either built-in or router-scoped
custom authentication. Other `OPTIONS` requests are delegated to `httprouter`.
They enter the normal route middleware chain only when `OPTIONS` is explicitly
registered for the route; otherwise `httprouter` may generate its automatic
`OPTIONS`/`Allow` response or return 404.

The `pkg/middleware` package also contains reusable bearer-token, API-key, basic-user, and user-provider middleware building blocks. See [Custom Middleware](./middleware.md) for the complete middleware order.
