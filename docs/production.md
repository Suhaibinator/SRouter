# Production Considerations

Production configuration should make route errors fail during startup, bound request work, and match the application's actual proxy and deployment topology.

## Performance Considerations

### Build before serving

Call `Build` after registering routes and before starting the server:

```go
if err := r.Build(); err != nil {
	logger.Fatal("invalid router configuration", zap.Error(err))
}
```

This validates and freezes the route tree, including paths, handlers, methods,
middleware, authentication callback dependencies, and negative timeout or body
limits after inheritance is resolved. It does not validate every runtime
setting—for example rate-limit values, custom key extractors, proxy/CORS
topology, and all auth-token choices still require application review. Without
an explicit call, the first request builds the tree and a configuration error
becomes a 500 response.

SRouter uses `julienschmidt/httprouter`'s radix-tree matching. Treat performance claims as workload-dependent: benchmark complete handlers and middleware with representative paths and payloads rather than assuming a constant lookup cost.

### Bound request work

Configure limits at the narrowest useful scope:

- `GlobalTimeout`, `group.Timeout`, and route `Overrides.Timeout` set a deadline
  around the final body-limited handler. Custom global, group, and route
  middleware runs outside that deadline. A handler must observe
  `req.Context().Done()`; code that ignores cancellation can continue after
  SRouter sends a timeout response.
- `GlobalMaxBodySize`, `group.MaxBodySize`, and route `Overrides.MaxBodySize`
  install `http.MaxBytesReader` immediately before the handler. Earlier custom
  middleware sees the original body and must not read it without its own bound.
  Choose limits based on each endpoint's expected payload.
- Keep global middleware small because every matched request that passes
  built-in authentication and rate limiting pays its cost. Group and route
  middleware are additive.

Profile the complete service with Go benchmarks, `pprof`, and production-like codecs before introducing pools or other allocation optimizations. Reuse long-lived clients and database pools rather than creating them per request.

### Configure the HTTP server

SRouter is an `http.Handler`; connection-level limits remain the responsibility of `http.Server` or the fronting proxy:

```go
srv := &http.Server{
	Addr:              ":8080",
	Handler:           r,
	ReadHeaderTimeout: 5 * time.Second,
	ReadTimeout:       30 * time.Second,
	WriteTimeout:      30 * time.Second,
	IdleTimeout:       2 * time.Minute,
}
```

Tune these values to the service, especially for streaming or WebSocket routes where a general write timeout can be inappropriate.

## Security Considerations

### Client IPs and proxy trust

A nil `RouterConfig.IPConfig` uses the immediate peer address and ignores proxy headers. Enable a header source only when a trusted proxy overwrites or sanitizes it. SRouter's `X-Forwarded-For` policy selects the rightmost entry, not the client-controlled leftmost entry. See [IP Configuration](./ip-configuration.md).

This choice affects logs and IP-based rate limits, so verify it against the deployed proxy chain rather than only local tests.

### Authentication

Use `AuthRequired` for routes that must not run anonymously, and call `Build` at startup so missing built-in authentication callbacks are caught before traffic arrives. Protect credentials with TLS and avoid logging raw authorization headers, cookies, API keys, or tokens. See [Authentication](./authentication.md).

Reusable authentication providers are available in `pkg/middleware`, but validation, credential storage, rotation, and authorization policy remain application responsibilities.

### Rate limiting

The built-in limiter is a nonblocking, in-process sliding-window counter. Its
state is not shared between replicas or persisted across restarts. Stale
entries are swept lazily when a later new key is inserted, so a stable key set
can retain idle entries. A high rate of new identities can also create temporary
memory and CPU pressure.

Use it for per-instance protection. Put a shared limiter in a gateway or other external system when the limit must be global across replicas. For `StrategyIP`, first verify proxy trust. For `StrategyUser`, ensure authentication runs before rate limiting; router-scoped custom middleware runs too late to populate its user key. See [Rate Limiting](./rate-limiting.md).

### CORS

List exact origins for credentialed browser traffic. When `Origins` contains `"*"`, SRouter emits the wildcard and suppresses `Access-Control-Allow-Credentials`, even if `AllowCredentials` is true. CORS controls browser access to responses; it does not authenticate requests or constrain non-browser clients. See [CORS Configuration](./cors-configuration.md).

### Application input and output

SRouter limits and decodes input but cannot enforce application-specific meaning. Validate required fields, ranges, identifiers, query parameters, path parameters, and custom headers. Use parameterized database queries, avoid constructing commands or paths from unchecked input, and use context-appropriate output escaping such as `html/template` for HTML.

The generic route `Sanitizer` runs after decoding and before the handler and can return an error to reject invalid data. It complements, rather than replaces, validation in domain and persistence layers.

### Logs and metrics

Set an intentional production zap level and sampling policy. Request summaries can include path, client IP, and user agent; rate-limit warnings include the derived limiter key. Do not derive custom limiter keys from raw secrets when those logs are retained.

Validate metric names and labels in a staging environment and control high-cardinality labels in custom metrics.

## Graceful Shutdown

With an active context, `Router.Shutdown(ctx)` marks the router as shutting
down, returns 503 for new requests, and waits for tracked requests until they
drain or `ctx` expires. It stops the trace-ID generator when enabled and returns
`ctx.Err()` if the deadline or cancellation wins. If `ctx` is already done at
entry, it returns `ctx.Err()` without changing router state or stopping the
generator. It does not close the network listener; `http.Server.Shutdown` does
that.

Use an overall deadline and call both:

```go
stop := make(chan os.Signal, 1)
signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
<-stop

ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
defer cancel()

// Stop the router from accepting application work and drain its requests.
if err := r.Shutdown(ctx); err != nil {
	logger.Error("router shutdown did not complete", zap.Error(err))
}

// Close listeners and drain net/http connections within the remaining deadline.
if err := srv.Shutdown(ctx); err != nil {
	logger.Error("HTTP server shutdown did not complete", zap.Error(err))
}
```

Calling the router first means the listener can briefly accept connections that receive 503 while existing work drains. Calling the HTTP server first is also reasonable when immediately closing listeners is preferred, but always call `Router.Shutdown` afterward so router-owned components are stopped.

Handlers should propagate request contexts into database, RPC, and other downstream calls so request deadlines and client cancellation can be observed. See `examples/graceful-shutdown` for a runnable signal-handling example.
