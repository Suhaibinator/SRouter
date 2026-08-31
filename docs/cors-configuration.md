# CORS Configuration

Inside the router, SRouter handles Cross-Origin Resource Sharing before route
matching, built-in authentication, and router-scoped middleware. Middleware
that wraps the entire router runs earlier and must allow preflights to reach
SRouter. Enable CORS with `RouterConfig.CORSConfig`:

```go
config := router.RouterConfig{
	CORSConfig: &router.CORSConfig{
		Origins:          []string{"https://app.example.com"},
		Methods:          []string{http.MethodGet, http.MethodPost},
		Headers:          []string{"Content-Type", "Authorization"},
		ExposeHeaders:    []string{"X-Request-ID"},
		AllowCredentials: true,
		MaxAge:           12 * time.Hour,
	},
}
```

When `CORSConfig` is nil, SRouter adds no CORS headers and does not intercept `OPTIONS` requests.

## Fields and defaults

- `Origins` lists exact origins. `"*"` allows any origin. An empty list allows none.
- `Methods` lists allowed preflight methods using case-sensitive HTTP method names. An empty list defaults to `GET`, `HEAD`, and `POST`.
- `Headers` lists allowed preflight request-header names. Matching is case-insensitive. An empty list defaults to `Accept`, `Accept-Language`, `Content-Language`, and `Content-Type`. Browsers use header values to decide whether a preflight is necessary, but SRouter validates only the names in that preflight. Consequently, allowing `Content-Type` also permits non-safelisted values such as `application/json` after a successful preflight.
- `ExposeHeaders` lists response headers browser JavaScript may read. SRouter sends it on allowed, non-`OPTIONS` requests.
- `AllowCredentials` emits `Access-Control-Allow-Credentials: true` only for an explicitly matched origin.
- `MaxAge` emits `Access-Control-Max-Age` in truncated whole seconds when it is
  at least one second. Zero, negative, and subsecond values are omitted.

Methods do not support a wildcard. `Headers: []string{"*"}` accepts every requested header and echoes the browser's `Access-Control-Request-Headers` value in the preflight response.

## Wildcard origins and credentials

The CORS protocol does not allow a credentialed response with `Access-Control-Allow-Origin: *`. If `Origins` contains `"*"`, SRouter always selects the wildcard—even if the same list also contains an exact origin—and never emits `Access-Control-Allow-Credentials`. The router logs a warning when this is combined with `AllowCredentials: true`.

Use explicit origins for credentialed requests:

```go
CORSConfig: &router.CORSConfig{
	Origins:          []string{"https://app.example.com"},
	Methods:          []string{http.MethodGet, http.MethodPost},
	Headers:          []string{"Content-Type", "Authorization"},
	AllowCredentials: true,
}
```

## Request behavior

For a request with an `Origin` header, SRouter:

1. Matches the origin against `Origins`.
2. Stores the selected origin and credential decision in `SRouterContext`.
3. Sets `Access-Control-Allow-Origin`, credential, exposure, and `Vary` headers as applicable before downstream middleware runs.
4. For `OPTIONS`, checks `Access-Control-Request-Method` and `Access-Control-Request-Headers`, writes a 204 response, and stops the chain.

An allowed preflight receives `Access-Control-Allow-Methods`, `Access-Control-Allow-Headers`, and positive `Access-Control-Max-Age` values as configured. If its requested method or headers are disallowed, SRouter still returns 204 and can retain the already-decided origin and credentials headers, but omits all three preflight-specific headers. The browser interprets that omission as a failed preflight.

A request from a disallowed origin continues to the application when it is not `OPTIONS`, but receives no CORS allowance headers. The browser prevents the calling page from reading the response. CORS is not authorization: non-browser clients are not constrained by it.

Preflight handling runs before built-in and router-scoped authentication, so
browsers can preflight protected routes without credentials. Authentication
middleware wrapped around the entire router runs first; it must pass CORS
`OPTIONS` requests through, or an external CORS layer must wrap it. The
subsequent actual request still goes through normal authentication and
middleware. Framework-generated errors after CORS processing carry the CORS
decision; earlier build failures and shutdown rejections do not.

## Cache behavior

Specific allowed origins and disallowed origins add `Vary: Origin`, preventing a shared cache from reusing one origin's CORS response for another. A wildcard response does not vary by origin.
