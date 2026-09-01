# Middleware

SRouter middleware uses the usual Go shape:

```go
type Middleware func(http.Handler) http.Handler
```

The first middleware in a slice is the outermost wrapper: it sees the request
first and resumes after all later middleware and the handler have returned.

## Writing middleware

```go
func AddHeader(key, value string) common.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set(key, value)
			next.ServeHTTP(w, req)
		})
	}
}
```

To pass values downstream, derive a request context:

```go
func WithRequestFlag(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		ctx := scontext.WithFlag[string, User](req.Context(), "audited", true)
		next.ServeHTTP(w, req.WithContext(ctx))
	})
}
```

Use the same `T` and `U` type arguments as the router when calling `scontext`
helpers. See [Context management](context-management.md) for the available
values and concurrency rules.

## Applying middleware

Middleware can be attached at four scopes:

```go
r := router.NewRouter(router.RouterConfig{
	Middlewares: []common.Middleware{globalAudit},
}, router.RouterDependencies[string, User]{
	Authenticate: authenticate,
	UserID:       userIDFromUser,
})

r.Use(rootHeaders)

api := r.Group("/api").Use(apiHeaders)
api.Route(router.RouteConfigBase{
	Path:        "/users",
	Methods:     []router.HttpMethod{router.MethodGet},
	Middlewares: []common.Middleware{routeAudit},
	Handler:     listUsers,
})
```

`RouterConfig.Middlewares` applies to matched routes. `Router.Use` applies to
root routes and every descendant. Group middleware is inherited from outer to
inner groups, and route middleware is the most local. Middleware is additive;
setting route policy does not replace an inherited middleware slice.

## Execution order

For a matched route, the effective order from outermost to innermost is:

1. panic recovery;
2. automatic trace-ID injection, when enabled;
3. built-in authentication, for optional or required routes;
4. configured rate limiting;
5. `RouterConfig.Middlewares`, followed by configured metrics;
6. `Router.Use`, then outer-to-inner group middleware;
7. route middleware;
8. timeout handling, when enabled;
9. request-body limiting and the handler.

CORS processing, client-IP extraction, route matching, shutdown rejection, and
request-summary logging live in `Router.ServeHTTP` outside this per-route
chain. A CORS preflight may finish before route middleware runs. Unmatched 404
and 405 responses do not enter the per-route chain.

Any middleware that returns without calling `next` short-circuits everything
inside it. In particular, built-in authentication and rate-limit rejections do
not reach global, metrics, group, or route middleware.

Custom authentication placed in a global or group middleware runs after the
configured rate limiter. A configured user-based limit therefore needs the
built-in authentication stage to establish the user identity first. An outer
handler around the router is the appropriate place when an application needs a
different top-level order.

## Observing typed-handler errors

When a typed handler completes through the normal chain, SRouter stores its
non-nil error in the pointer-backed context before outer middleware resumes:

```go
func ObserveResult(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		next.ServeHTTP(w, req)

		if err, ok := scontext.GetHandlerErrorFromRequest[string, User](req); ok {
			log.Printf("typed handler failed: %v", err)
		}
	})
}
```

This signal covers errors returned by a typed handler. It does not turn status
codes from standard handlers, decoding failures, short circuits, or panics into
handler errors. A timed-out handler that ignores cancellation can continue
after the timeout stage returns, so its eventual error may not exist when outer
middleware first resumes. If middleware must make transactional decisions for
all outcomes, it should also capture the response status, account for timeouts,
and handle panics deliberately.

## Built-in helpers

The `pkg/middleware` package exports:

- `Chain` for composing middleware;
- `Recovery` and `MaxBodySize` for direct use outside the router;
- trace-ID generation and propagation helpers;
- authentication providers and middleware;
- `RateLimit` and the built-in rate limiter; and
- `NewGormTransactionWrapper` for the transaction context interface.

The router installs its own recovery, authentication, trace, rate-limit,
timeout, and body-limit stages from configuration. Do not install duplicates
unless the extra layer is intentional. CORS, IP extraction, and request-summary
logging are router behavior rather than exported middleware.

See [Authentication](authentication.md), [Rate limiting](rate-limiting.md),
[Logging](logging.md), and [`examples/middleware`](../examples/middleware).
