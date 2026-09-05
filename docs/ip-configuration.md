# IP Configuration

SRouter records a client IP in `SRouterContext` for logging and rate limiting. Configure its source with `RouterConfig.IPConfig`.

## Safe default

When `RouterConfig.IPConfig` is `nil`, the router ignores proxy headers and uses `http.Request.RemoteAddr`, with a valid host port removed:

```go
config := router.RouterConfig{
	IPConfig: nil, // use the immediate peer address
}
```

This is appropriate when the service is internet-facing or when the immediate peer address is the value you need.

## Trusted proxy configuration

Use a header source only when every request reaches the application through infrastructure that overwrites or sanitizes that header:

```go
config := router.RouterConfig{
	IPConfig: &router.IPConfig{
		Source:     router.IPSourceXForwardedFor,
		TrustProxy: true,
	},
}
```

The available sources are:

- `IPSourceRemoteAddr`: the immediate peer in `RemoteAddr`.
- `IPSourceXForwardedFor`: the **rightmost non-empty** entry in `X-Forwarded-For`.
- `IPSourceXRealIP`: the complete `X-Real-IP` value.
- `IPSourceCustomHeader`: the complete value of `CustomHeader`.

For example, given:

```text
X-Forwarded-For: client-supplied, client-seen-by-edge, edge-seen-by-app-proxy
```

SRouter selects `edge-seen-by-app-proxy`, the rightmost value appended by the
proxy nearest the application. That value describes the nearest proxy's
observed upstream peer; it is not the nearest proxy's own address. SRouter
deliberately does not select the commonly described leftmost "original client"
value, because a client can prepend arbitrary entries. If your topology has
multiple trusted proxy hops and you need a different address, normalize the
header at the last proxy before it reaches SRouter.

If `IPConfig` is non-nil but `Source` is empty or unknown, SRouter treats it as `IPSourceXForwardedFor`. Prefer setting `Source` explicitly.

## `TrustProxy` behavior

- With `TrustProxy: false`, SRouter ignores a configured header source and uses `RemoteAddr`.
- With `TrustProxy: true`, SRouter uses the configured header. It falls back to `RemoteAddr` only when the selected header is empty.

SRouter does not reject a malformed, non-empty proxy-header value. It removes a port when the value is a valid host-port pair; otherwise it preserves the value. Header validation and sanitization must therefore happen at the trusted proxy boundary.

`router.DefaultIPConfig()` and the standalone `router.ClientIPMiddleware(nil)` are different from a nil `RouterConfig.IPConfig`: they default to trusted `X-Forwarded-For`. Use that convenience default only behind a trusted proxy.

## Custom header

```go
config := router.RouterConfig{
	IPConfig: &router.IPConfig{
		Source:       router.IPSourceCustomHeader,
		CustomHeader: "CF-Connecting-IP",
		TrustProxy:   true,
	},
}
```

Make sure requests cannot bypass the proxy and reach the application with a client-controlled value for the custom header.

## Reading the client IP

Use the context helper in a handler or middleware:

```go
clientIP, ok := scontext.GetClientIP[string, User](req.Context())
if !ok {
	// No SRouter client information has been attached to this request.
}
```

The type arguments must match the router's user ID and user object types.
