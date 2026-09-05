# Logging

SRouter writes structured logs with `go.uber.org/zap`. Set `RouterConfig.Logger` to control encoding, destinations, and enabled levels. When it is nil, `NewRouter` creates a production logger and falls back to a no-op logger only if creation fails. The router names its child logger `SRouter`.

```go
logger, err := zap.NewProduction()
if err != nil {
	return err
}
defer logger.Sync()

r := router.NewRouter(router.RouterConfig{
	Logger: logger,
}, router.RouterDependencies[string, User]{
	Authenticate: authenticate,
	UserID:       userIDFromUser,
	BuildID:      func() string { return buildID },
	ConfigID:     func() string { return configID },
})
```

## Request summary logging

For requests that reach normal route dispatch, SRouter emits one `"Request summary statistics"` record after the request when either of these settings is enabled. Early responses such as build failures, shutdown rejection, and CORS requests handled before dispatch do not pass through this summary wrapper.

- `TraceIDBufferSize > 0`, which enables automatic trace IDs on matched routes and request summaries.
- `EnableTraceLogging`, which enables request summaries independently of trace IDs.

The summary contains `method`, `path`, `status`, `duration`, `bytes`, `ip`, and `user_agent`. It also contains configured `build_id` and `config_id` values when available. For a matched route, it contains `trace_id` when automatic trace generation is enabled. An unmatched 404 or 405 still receives a summary, but it never enters the per-route trace middleware and therefore has no automatically generated `trace_id`.

SRouter adds available runtime identities to its request-bound authentication,
timeout, panic recovery, handled HTTP error, lazy-build failure, and JSON-response
write-failure logs. Startup and route-registration logs have no request context
and remain unchanged.

Runtime identities are opaque, log-safe application values. SRouter samples
them once per request and does not propagate them through headers. Background
workers may install already-sampled values with `scontext.WithBuildID` and
`scontext.WithConfigID`.

All structured-log field names emitted by SRouter are exported from the
dependency-free `pkg/logkeys` package. Applications use constants such as
`logkeys.TraceID`, `logkeys.BuildID`, and `logkeys.ConfigID` to keep their logs
aligned with SRouter without depending on Zap.

Its level is chosen in this priority order:

1. `Error` for status codes `>= 500`.
2. `Warn` when duration is greater than 500 ms.
3. `Info` for status codes from 400 through 499.
4. `Info` for other responses when `TraceLoggingUseInfo` is true.
5. `Debug` otherwise.

Thus `TraceLoggingUseInfo` changes only otherwise-successful summaries. A slow 4xx request is `Warn`, while a fast 4xx request is `Info`.

To emit summaries without generating trace IDs:

```go
config := router.RouterConfig{
	Logger:              logger,
	EnableTraceLogging:  true,
	TraceLoggingUseInfo: false, // successful summaries are Debug
	TraceIDBufferSize:   0,
}
```

## Error log levels

Errors handled at the router boundary use these defaults:

- `Debug` for `context.Canceled`.
- `Warn` for `context.DeadlineExceeded`.
- `Info` for other 4xx responses.
- `Error` for 5xx responses and other unexpected errors.

An `HTTPError.WithLogLevel` setting takes precedence over those defaults. Route timeouts also produce a separate `"Request timed out"` warning. When the timeout middleware can still write its 408 response, an enabled request summary independently classifies that 408 by the rules above. If the handler already started a response, the timeout is logged but the summary retains the response's existing status.

Panic recovery logs `"Panic recovered"` at `Error`. If the handler already started the response, SRouter logs the panic but does not append a second error body.

See [Custom Error Handling](./error-handling.md) for causes, structured fields, and level overrides.

## Trace ID integration

Set `TraceIDBufferSize` above zero to create a buffered ID generator and install trace middleware on every route:

```go
config := router.RouterConfig{
	Logger:            logger,
	TraceIDBufferSize: 1000,
}
```

For each request that matches a configured route, the middleware:

1. Reuses a valid inbound `X-Trace-ID`, if present. Accepted values are 1–64 ASCII alphanumeric, hyphen, or underscore characters.
2. Otherwise generates a 32-character hexadecimal UUIDv7 trace ID.
3. Stores the ID in the SRouter request context.
4. Sets the response `X-Trace-ID` header.

In automatic mode, the same context ID is used by request summaries and router error logs for matched routes. JSON error responses include it as `error.trace_id`.

Retrieve and propagate it with the `pkg/scontext` helpers:

```go
func callDownstream(r *http.Request) (*http.Response, error) {
	traceID := scontext.GetTraceID[string, User](r.Context())

	req, err := http.NewRequestWithContext(
		r.Context(),
		http.MethodGet,
		"http://downstream.internal/data",
		nil,
	)
	if err != nil {
		return nil, err
	}
	if traceID != "" {
		req.Header.Set("X-Trace-ID", traceID)
	}
	return http.DefaultClient.Do(req)
}
```

`scontext.GetTraceID[T, U]` provides the same value when only a `context.Context` is available.

Even with automatic trace IDs disabled, request-boundary error records produced during route handling contain a `trace_id`: SRouter reuses an ID already in the request context or creates a log-only ID. A newly generated log-only ID is not injected into the request, response header, or JSON body. Setup and build logs emitted before route dispatch have no request trace ID.

## Generator lifecycle

An automatic generator starts a background goroutine. `Router.Shutdown` stops it, so applications that enable `TraceIDBufferSize` should call `Shutdown` as part of server shutdown even when the surrounding `http.Server` is managed separately.

If you install trace middleware manually, you own the generator and must stop it:

```go
idGenerator := middleware.NewIDGenerator(1000)
defer idGenerator.Stop()

traceMiddleware := middleware.CreateTraceMiddleware[string, User](idGenerator)
r := router.NewRouter(router.RouterConfig{
	Logger:             logger,
	EnableTraceLogging: true,
}, router.RouterDependencies[string, User]{
	Authenticate: authenticate,
	UserID:       userIDFromUser,
})

handler := traceMiddleware(r)
```

Wrapping the router places the trace ID in the context and response header before built-in authentication, rate limiting, CORS, and route dispatch, so router error logs can reuse it. Adding trace middleware through `RouterConfig.Middlewares` runs it later, after built-in authentication and rate limiting; failures in those earlier stages will not see its ID.

Request summaries include `trace_id` when automatic tracing is enabled with
`TraceIDBufferSize > 0`. The manual wrapper above still supplies an ID to
handlers and router error logs, but with a zero buffer setting the request
summary omits it. Serve `handler` rather than `r` in that example.

See `examples/trace-logging` for a runnable example.
