# Logging

SRouter writes structured logs with `go.uber.org/zap`. Library request log sites use
Zap's `Check`/`Write` pattern, constructing event fields only for records
accepted by the logger, including its sampling decision. Set `RouterConfig.Logger`
to control encoding, destinations, and enabled levels. When it is nil,
`NewRouter` creates a production logger and falls back to a no-op logger only
if creation fails. SRouter names its library logger child `SRouter`.

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

SRouter emits one `"Request summary statistics"` record for every request when either of these settings is enabled, including build failures, shutdown rejection, and CORS responses.

- `TraceIDConfig != nil`, which enables automatic trace IDs and request summaries.
- `EnableTraceLogging`, which enables request summaries independently of trace IDs.

The summary contains `method`, `path`, `status`, `duration`, `bytes`,
`client_ip`, and `user_agent`. It also contains configured `build_id` and
`config_id` values when available. A non-empty `trace_id` already present in
the request context is included independently of the automatic trace setting.
Automatic tracing resolves the ID before build, shutdown, CORS, and routing, so
unmatched 404/405 responses and all early returns also carry it.

SRouter adds available runtime identities to its request summaries and
request-bound authentication, rate-limit, timeout, panic recovery, handled
HTTP error, lazy-build failure, and JSON-response write-failure logs. Startup
and route-registration warnings, such as a route without a sanitizer or an
insecure CORS configuration, belong to no request: they sample the identity
callbacks when the record is written and carry `build_id` and `config_id`, but
no trace or user fields. The callbacks are skipped when `Warn` is disabled.

Runtime identities are opaque, log-safe application values. SRouter samples
them once per request and does not propagate them through headers. Background
workers may install already-sampled values with `scontext.WithBuildID` and
`scontext.WithConfigID`.

All structured-log field names emitted by SRouter are exported from the
dependency-free `pkg/logkeys` package. Applications use constants such as
`logkeys.ClientIP`, `logkeys.TraceID`, `logkeys.BuildID`, `logkeys.ConfigID`,
and `logkeys.UserID` to keep their logs aligned with SRouter without depending
on Zap. Replace uses of the removed `logkeys.IP` constant with
`logkeys.ClientIP`; SRouter no longer emits the `ip` alias.

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
	TraceIDConfig:       nil,
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

Set a non-nil `TraceIDConfig` to enable tracing. The empty configuration
generates synchronously; a positive buffer starts a background generator.

```go
config := router.RouterConfig{
	Logger: logger,
	TraceIDConfig: &router.TraceIDConfig{
		BufferSize:     1000,
		Source:         traceid.FromHeader(traceid.HeaderXRequestID),
		ResponseHeader: traceid.HeaderXTraceID,
	},
}
```

At the start of every request, after context and logger initialization, SRouter:

1. Reuses an existing context ID if it passes safety checks and validation.
2. Otherwise calls the configured source once.
3. Generates a 32-character hexadecimal UUIDv7 if the source reports absence,
   returns an empty or unsafe value, or the validator rejects it.
4. Stores the final ID using `scontext.SetTraceID` and writes only the
   configured canonical response header.

A nil source reads `X-Trace-ID`. A nil validator accepts 1–64 ASCII letters,
digits, hyphens, and underscores. An empty response-header name uses
`X-Trace-ID`. Even a custom validator cannot bypass the mandatory non-empty,
64-byte maximum, valid UTF-8, and no whitespace/control-character checks.
Generated fallbacks bypass custom validation so resolution always finishes.

The resolved ID is shared by request loggers, summaries, router error logs,
response headers, and JSON error bodies (`error.trace_id`). This includes
unmatched routes/methods, CORS, shutdown rejection, and lazy-build failures;
their existing HTTP status codes and body formats remain unchanged.
Request headers are never modified. A source header is not automatically
written back; only `ResponseHeader` is set, even when it differs from the source.

### Upstream sources

Import `github.com/Suhaibinator/SRouter/pkg/traceid`. `FromHeader(name)`
reads the first raw header value without trimming or validating it. Constants
include `HeaderXTraceID`, `HeaderXRequestID`, `HeaderXCorrelationID`,
`HeaderCloudflareRayID`, `HeaderB3TraceID`, and `HeaderTraceparent`.

For W3C input, set `Source: traceid.FromTraceparent`. It validates the
[W3C traceparent format](https://www.w3.org/TR/trace-context/#traceparent-header),
including lowercase hex, nonzero trace and parent IDs, version 00's exact
length, and the forbidden ff version. Future versions accept the base fields
and an opaque hyphen-delimited suffix. It returns only the 32-character trace
ID; it does not create spans or emit a complete `traceparent`.

AWS, Google, and other structured formats can use a custom
`traceid.Source func(*http.Request) (string, bool)` that parses the provider
header and returns its ID component. Raw custom headers can use `FromHeader`.
A custom `traceid.Validator func(string) bool` can accept other ID alphabets
within the mandatory safety limits. Sources and validators must be fast,
concurrency-safe, and non-panicking; they run before route recovery.
Source implementations should only extract values; request logging should
happen after resolution so it receives the final ID.

Retrieve and propagate it with the `pkg/scontext` helpers:

```go
func callDownstream(r *http.Request) (*http.Response, error) {
	traceID := scontext.GetTraceID(r.Context())

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

`scontext.GetTraceID` provides the same value when only a `context.Context` is available.

Request logs include a non-empty trace ID already stored in the SRouter context,
even when automatic tracing is disabled. SRouter does not generate IDs solely
for logging, so a request without a context trace omits `trace_id`. Log
enrichment does not create or change response headers or JSON bodies. The
configured automatic trace stage controls response headers and JSON trace fields.
With `TraceIDConfig: nil`, existing context IDs are preserved without validation
or replacement and remain available to logs, but no response ID is added.

Startup and explicit build logs have no request context. Lazy-build failure
logs use the ID already resolved at the request boundary.

## Request-scoped logger

SRouter makes one shared request logger available at the beginning of request
handling. It lazily stamps non-empty `client_ip` and `trace_id`, plus available
`build_id`, `config_id`, and `user_id`. Services own their relative component
names and apply them with Zap's `Named` method:

```go
func (h *AdminHandler) handle(ctx context.Context) {
	logger, ok := scontext.GetLogger(ctx)
	if ok {
		logger = logger.Named("common_service.admin")
	} else {
		logger = h.fallbackLogger // Already named at service initialization.
	}
	logger.Info("operation started")
	// Reuse this logger throughout the operation.
	logger.Info("operation completed")
}
```

The base is the resolved `RouterConfig.Logger`, including its existing name,
static fields, sinks, levels, and options. SRouter appends `SRouter` only to its
own internal logger. Given a root named `myapp`, the example emits
`myapp.common_service.admin`; another service can independently derive
`myapp.common_service.permission` from the same request logger.

Keep the service name relative to the application root. Passing an existing
fully qualified `h.logger.Name()` to `Named` could duplicate the application
prefix. `Named` sets Zap's logger-name metadata, emitted under the encoder's
`NameKey`; adding `zap.String("logger", name)` would be an ordinary field instead.

A named child shares the request logger's core and already encoded correlation.
Naming still clones the small logger value and may allocate a joined name, so
derive once per service operation and reuse it across log lines. This model
uses application-wide logging configuration. Additional service fields or
options must be applied explicitly to the child; naming cannot transfer them
from another logger.

Configure user-ID formatting at router initialization:

```go
type UserID uint64

r := router.NewRouter(router.RouterConfig{
	Logger: appLogger,
}, router.RouterDependencies[UserID, User]{
	Authenticate: authenticate,
	UserID:       userIDFromUser,
	// Optional: provide an explicit typed conversion or custom representation.
	UserIDField: func(id UserID) zap.Field {
		return zap.Uint64(logkeys.UserID, uint64(id))
	},
})
```

Leaving `UserIDField` nil selects the encoder once from `T`'s static type.
Strings, bools, integers, and floats, including named types such as `UserID`,
use typed Zap fields. The implementation uses safe reflection kind accessors;
it does not use unsafe casts. Types implementing Zap object/array marshaling,
JSON/text marshaling, `fmt.Stringer`, or `error` retain `zap.Any` semantics.
Other types also use `zap.Any`; for interface-typed IDs it handles each value's
dynamic type.
`T` remains `comparable`, so built-in IDs require no added interface methods.

An explicit formatter overrides the default representation and field key. It
must be concurrency-safe and should be fast and free of side effects, since
concurrent derivations may call it more than once. It may read context values,
but must not recursively request the same logger or mutate correlation. Panics
propagate and leave the cache invalidated for a subsequent call.

A warmed `GetLogger` allocates nothing. A later stamped-field write invalidates
the cache; this includes `WithClientIP` and `WithClientInfo` when they change
the client IP. A user-agent-only change does not rebuild the logger. Previously
returned loggers remain unchanged. See
[Context management](./context-management.md#request-scoped-logger) for cache
semantics and configuring a reusable source for background jobs. Run the
[request logger example](../examples/request-logger/main.go) to see named service
logging with a named numeric user-ID type.

### Middleware logging

Middleware in `pkg/middleware` is intended to run through SRouter. Register it
on the router, a group, or a route. `Router.ServeHTTP` installs the shared
request logger and resolved client information before middleware executes;
applications do not need to attach a logger source or client-IP middleware.
Configure logging and IP selection through `RouterConfig.Logger` and
`RouterConfig.IPConfig`.

The context setters normalize client information when it is stored. Logging
reuses that value without reading or cleaning `RemoteAddr`, copying context,
or inventing a client IP. An unavailable `client_ip` is omitted. Rate limiting
retains its defensive `RemoteAddr` fallback when context client information
is missing.

The logger-accepting middleware forms were removed. Update direct calls as
follows:

| Previous form | Current form |
| --- | --- |
| `middleware.Recovery(logger)` | `middleware.Recovery[UserID, User]()` |
| `middleware.RateLimit(config, limiter, logger)` | `middleware.RateLimit(config, limiter)` |
| `middleware.AuthenticationWithProvider(provider, logger)` | `middleware.AuthenticationWithProvider[UserID, User](provider)` |
| `middleware.AuthenticationWithUserProvider(provider, logger)` | `middleware.AuthenticationWithUserProvider[UserID, User](provider)` |
| Bearer/API-key convenience constructor with a final `logger` argument | Remove the final `logger` argument |

Authentication logs use the shared `client_ip` and no longer emit `remote_addr`.
Rate-limit fallback logs report the selected identity as `key`; when context
client information is missing, the raw socket fallback is not relabeled as
`client_ip`. Update queries that previously used `remote_addr` accordingly.

## Generator lifecycle

Positive `BufferSize` starts a background worker; zero uses synchronous
generation without a worker. Negative values fail `Build`.
`Router.Shutdown` stops the worker. Call it during application shutdown even
when the surrounding `http.Server` is managed separately. Requests rejected
after shutdown can still generate an ID synchronously when the buffer is empty.

For use outside the router:

```go
generator, err := traceid.NewGenerator(1000)
if err != nil {
	return err
}
defer generator.Stop()
id := generator.Next()
```

`Next` never waits for the worker and remains usable after `Stop`.
Concurrent calls to `Next` and `Stop` are supported. `traceid.Generate()`
generates a single UUIDv7 synchronously.

## Breaking changes and migration after PR #129

[PR #129](https://github.com/Suhaibinator/SRouter/pull/129) unified request
logging and preserved the previous tracing API. This follow-up replaces that
API and moves automatic resolution to the request boundary. Its middleware
logging migrations above still apply.

| Removed API | Replacement |
| --- | --- |
| `RouterConfig.TraceIDBufferSize: 0` | `TraceIDConfig: nil` to keep tracing disabled |
| `RouterConfig.TraceIDBufferSize: n` for positive n | `TraceIDConfig: &router.TraceIDConfig{BufferSize: n}` |
| `middleware.CreateTraceMiddleware` | Configure `RouterConfig.TraceIDConfig`; remove the manual wrapper |
| `middleware.IDGenerator` / `middleware.NewIDGenerator(n)` | `traceid.Generator` / `traceid.NewGenerator(n)`, now returning an error |
| Generator `GetID()` / `GetIDNonBlocking()` | `Next()`, which never waits for the worker |
| `middleware.GenerateTraceID()` | `traceid.Generate()` |

Use `&router.TraceIDConfig{}` to enable synchronous generation. A valid context
ID now takes precedence over an upstream header; an invalid context ID is
replaced. Automatic tracing also covers early responses and unmatched routes.
Any non-nil trace configuration enables summaries; `EnableTraceLogging`
remains independent.

`scontext.WithTraceID` still preserves an already-set ID.
Use `scontext.SetTraceID` when unconditional replacement is intended; it
synchronizes the write and invalidates the cached request logger.

See the [trace logging example](../examples/trace-logging/main.go).

### Clearing request correlation

Use the `scontext.Clear*` helpers to remove stored correlation values, and
`ClearRequestLogger[T, U]` to remove the logging source and cache. Reacquire
`GetLogger` and any named child after clearing: previously returned loggers
retain their original fields. Clone first when the parent must keep its state;
see [Clearing values](./context-management.md#clearing-values).
