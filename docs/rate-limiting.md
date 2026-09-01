# Rate Limiting

SRouter can apply a rate limit globally, to a route group, or to one route. The most specific configured value wins.

```go
config := router.RouterConfig{
	GlobalRateLimit: &common.RateLimitConfig[any, any]{
		BucketName: "public-api",
		Limit:      100,
		Window:     time.Minute,
		Strategy:   common.StrategyIP,
	},
}

r := router.NewRouter(config, router.RouterDependencies[string, User]{
	Authenticate: authenticate,
	UserID:       userID,
})

r.Group("/account").
	Auth(router.AuthRequired).
	RateLimit(&common.RateLimitConfig[string, User]{
	BucketName: "account",
	Limit:      20,
	Window:     time.Minute,
	Strategy:   common.StrategyUser,
})

r.Route(router.RouteConfigBase{
	Path:    "/login",
	Methods: []router.HttpMethod{router.MethodPost},
	Overrides: common.RouteOverrides{
		RateLimit: &common.RateLimitConfig[any, any]{
			BucketName: "login",
			Limit:      5,
			Window:     time.Minute,
			Strategy:   common.StrategyIP,
		},
	},
	Handler: loginHandler,
})
```

`RouterConfig` and `RouteOverrides` are non-generic, so their configurations use `[any, any]`. A route group's configuration uses the router's concrete user ID and user types.

## Algorithm and scope

Despite its compatibility name, `middleware.UberRateLimiter` no longer uses Uber's `ratelimit` package. It is an in-process, nonblocking sliding-window counter:

- The current and previous windows are retained per key; the previous count is weighted by its overlap with the sliding window.
- Requests beyond the limit are rejected immediately. They are never delayed or queued.
- State is local to one process and is not persisted or shared between replicas.
- Entries whose retained current/previous-window history can no longer affect a decision are eligible for eviction. An amortized sweep runs at most once per minute when a new key is created; stale entries can remain until another new key triggers a sweep.

This is suitable for per-instance protection. Use a gateway or shared backend when a limit must apply across replicas or survive restarts.

Always configure a positive `Limit` and `Window`. A non-positive limit denies every request; the underlying limiter treats a non-positive window as one second.

## Configuration fields

- `BucketName` namespaces the extracted client key. Use a stable, non-empty name.
- `Limit` is the maximum estimated request count in the sliding window.
- `Window` is the window duration.
- `Strategy` chooses how the client key is derived.
- `UserIDFromUser` optionally extracts an ID from a user object for `StrategyUser`.
- `UserIDToString` optionally controls how a user ID becomes a key.
- `KeyExtractor` is required for `StrategyCustom`.
- `ExceededHandler` takes over the denied response after rate-limit headers are set; it must write the desired status and body.

The limiter's internal key includes `BucketName`, the derived client key, `Limit`, and `Window`. Routes therefore share counters only when those values match. Reusing only a bucket name with a different limit or window creates separate counters.

## Strategies

### IP

`StrategyIP` uses the client IP placed in context by the router. Its security depends on [IP configuration](./ip-configuration.md): do not trust proxy headers unless a trusted proxy sanitizes them.

```go
RateLimit: &common.RateLimitConfig[any, any]{
	BucketName: "anonymous",
	Limit:      100,
	Window:     time.Minute,
	Strategy:   common.StrategyIP,
}
```

### User

`StrategyUser` first uses the stored user object when both it and `UserIDFromUser` are available. Otherwise it uses the user ID stored in context. `UserIDToString` is optional: the default handles `string`, `int`, `int64`, `fmt.Stringer`, and finally `fmt.Sprint`.

If no user key is available, the limiter does **not** reject the request as unauthenticated. It falls back to the context client IP, then to `RemoteAddr`. Use `AuthRequired` as well when the endpoint must be authenticated.

Built-in `AuthOptional` and `AuthRequired` authentication runs before the built-in rate limiter, so it can populate the user ID. Router, group, and route custom middleware run **after** the built-in rate limiter. A custom authentication middleware installed at those scopes therefore cannot populate a user key for an inherited `StrategyUser` limit. See [Custom authentication and rate limiting](./authentication.md#custom-authentication-and-rate-limiting) for supported arrangements.

### Custom

`StrategyCustom` calls `KeyExtractor` for each request:

```go
RateLimit: &common.RateLimitConfig[any, any]{
	BucketName: "api-key",
	Limit:      200,
	Window:     time.Hour,
	Strategy:   common.StrategyCustom,
	KeyExtractor: func(req *http.Request) (string, error) {
		key := req.Header.Get("X-API-Key")
		if key == "" {
			return "", errors.New("missing API key")
		}
		return key, nil
	},
}
```

A nil extractor, extractor error, or empty returned key produces a `500 Internal Server Error`. Do not include raw secrets in the key if rate-limit warning logs must not contain them.

## Shared buckets

Give multiple routes the same effective configuration to share a counter for each derived client key:

```go
shared := &common.RateLimitConfig[any, any]{
	BucketName: "authentication",
	Limit:      5,
	Window:     time.Minute,
	Strategy:   common.StrategyIP,
}

login.Overrides.RateLimit = shared
register.Overrides.RateLimit = shared
```

The five-request limit then applies to the combined login and registration traffic from each extracted IP.

## Responses and headers

Every request that reaches a rate-limit decision receives:

- `X-RateLimit-Limit`
- `X-RateLimit-Remaining`
- `X-RateLimit-Reset`

`X-RateLimit-Reset` is a Unix timestamp. For an allowed request it represents
the current time; for a denied request it conservatively identifies the end of
the limiter's current fixed window. Denied responses also receive
`Retry-After`, rounded down to seconds with a minimum of one second. The default
response uses status 429. `ExceededHandler`, when present, runs after these
headers have been set and is responsible for writing its own status and body:

```go
ExceededHandler: http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusTooManyRequests)
	_, _ = w.Write([]byte(`{"error":"rate limit exceeded"}`))
}),
```

See `examples/rate-limiting` for a runnable configuration.
