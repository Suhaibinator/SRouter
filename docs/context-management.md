# Context management

SRouter stores its request-scoped values in one `scontext.SRouterContext[T, U]`
attached to the standard `context.Context`. `T` is the router's user ID type and
`U` is its user object type. Middleware and handlers must use the same type
arguments that were passed to `router.NewRouter[T, U]`.

Use the helpers in `pkg/scontext` instead of reading or writing
`SRouterContext` fields directly. The wrapper is shared by pointer across the
middleware chain, and a handler that has timed out may briefly continue in a
goroutine while the router reads request state. The helpers synchronize access
with the wrapper's internal lock.

## Stored values

| Value | Write helper | Read helper |
| --- | --- | --- |
| User ID | `WithUserID` | `GetUserID` |
| User object (`*U`) | `WithUser` | `GetUser` |
| Client IP | `WithClientIP`, `WithClientInfo` | `GetClientIP` |
| User agent | `WithUserAgent`, `WithClientInfo` | `GetUserAgent` |
| Trace ID | `WithTraceID` | `GetTraceID` |
| Build identity | `WithBuildID` | `GetBuildID` |
| Configuration identity | `WithConfigID` | `GetConfigID` |
| Database transaction | `WithTransaction` | `GetTransaction` |
| Route template and path parameters | `WithRouteInfo`, `SetRouteInfo` | `GetRouteTemplate`, `GetPathParams` |
| Allowed CORS origin and credentials | `WithCORSInfo` | `GetCORSInfo` |
| Requested CORS headers | `WithCORSRequestedHeaders` | `GetCORSRequestedHeaders` |
| Generic-handler error | `WithHandlerError` | `GetHandlerError` |
| Application boolean flag | `WithFlag` | `GetFlag` |
| All correlation values at once | (see individual writers) | `GetCorrelation` |
| Request-scoped logger | `WithRequestLogger` | `GetLogger` |

Most getters return `(value, ok)` so an unset value can be distinguished from
its zero value. The trace-ID getters instead return an empty string when no
trace ID is set. `WithTraceID` preserves an existing ID rather than overwriting
one propagated by an upstream service.

Applications may configure `RouterDependencies.BuildID` and
`RouterDependencies.ConfigID` to install opaque, log-safe runtime identities.
SRouter samples each non-nil provider once at the beginning of every request,
before CORS, routing, and middleware. Empty results remain unset; a non-empty
local result replaces an inherited identity. Providers must be concurrency-safe,
fast, and non-panicking.

These identities are not propagated through request or response headers.
Non-HTTP work, such as background workers, can install already-sampled values
with `WithBuildID` and `WithConfigID`.

The router populates client information and, after a route match, its route
template and path parameters. When CORS is configured, CORS information is
stored even when the request has no `Origin` or the origin is denied; an empty
stored origin represents that outcome. When a typed handler completes through
the normal chain, its returned error is recorded before the remaining
middleware unwinds. A handler that continues after the timeout stage returns
may record its error later.

Values returned by the helpers can themselves be references. In particular,
the user is a `*U`, the transaction is an interface, and path parameters are a
slice. Treat those referenced values as shared unless your application makes
its own copy.

## Writing values in middleware

Each `With*` helper returns the context to propagate. This matters when the
request did not already contain an SRouter context and the helper had to create
one.

```go
func TagAdmin[UserID comparable, User any](
	isAdmin func(*http.Request) bool,
) common.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := scontext.WithFlag[UserID, User](
				r.Context(), "is_admin", isAdmin(r),
			)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
```

Middleware that needs to inspect state after the handler should retain the
derived request rather than reading the original request's context:

```go
ctx := scontext.WithFlag[string, User](r.Context(), "audited", true)
nextRequest := r.WithContext(ctx)
next.ServeHTTP(w, nextRequest)

handlerErr, failed := scontext.GetHandlerError[string, User](nextRequest.Context())
_ = handlerErr
_ = failed
```

## Reading values in a handler

```go
func accountHandler(w http.ResponseWriter, r *http.Request) {
	userID, authenticated := scontext.GetUserID[string, User](r.Context())
	if !authenticated {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	user, hasUser := scontext.GetUser[string, User](r.Context())
	clientIP, _ := scontext.GetClientIP[string, User](r.Context())
	routeTemplate, _ := scontext.GetRouteTemplate(r.Context())

	_, _, _ = userID, user, hasUser
	_, _ = clientIP, routeTemplate
}
```

## Reading correlation values together

Every individual getter walks the context chain and takes the wrapper's lock
once. Code that stamps correlation onto log entries or metrics needs the trace
ID, the build and configuration identities, and the user ID together, so it
pays both costs four times per entry. `GetCorrelation` reads them in one pass —
one chain walk, one read lock — and returns them as a plain value:

```go
func logFields(ctx context.Context) []zap.Field {
	c, ok := scontext.GetCorrelation[uint64, User](ctx)
	if !ok {
		return nil
	}

	fields := make([]zap.Field, 0, 4)
	if c.TraceIDSet {
		fields = append(fields, zap.String(logkeys.TraceID, c.TraceID))
	}
	if c.BuildIDSet {
		fields = append(fields, zap.String(logkeys.BuildID, c.BuildID))
	}
	if c.ConfigIDSet {
		fields = append(fields, zap.String(logkeys.ConfigID, c.ConfigID))
	}
	if c.UserIDSet {
		fields = append(fields, zap.Uint64("user_id", c.UserID))
	}
	return fields
}
```

`Correlation[T]` holds values rather than Zap fields, so it can also be used
with metrics and other logging implementations. Callers can choose their own
rendering for the user ID.

Each field carries a `Set` flag, so a value that was written empty on purpose
stays distinguishable from one that was never written. The result is a copy
taken at the moment of the call: a later write through a `With*` helper does
not change it, and two separate calls are not an atomic pair. Scalar values and
presence flags are copied; references inside a generic user ID retain their
normal Go sharing semantics.

Code that reads correlation only in order to log it should call `GetLogger`
instead. It returns a logger that already carries the same values as fields.

## Request-scoped logger

Configure logging once at application initialization. `NewRouter` creates a
`scontext.RequestLoggerSource[T]` from its resolved `RouterConfig.Logger` and
optional `RouterDependencies.UserIDField`, then attaches that source before
route dispatch. The source holds the application logger and user-ID encoder;
it contains no request values or per-request cache.

`GetLogger[T, U](ctx)` returns the shared request logger. Use `Named` with a
relative service name and reuse that child within the operation:

```go
logger, ok := scontext.GetLogger[uint64, User](ctx)
if ok {
	logger = logger.Named("common_service.admin")
} else {
	logger = adminFallbackLogger
}
logger.Info("operation started")
logger.Info("operation completed")
```

The application name is preserved: an application logger named `myapp` produces
`myapp.common_service.admin`. The [logging guide](./logging.md#request-scoped-logger)
explains component ownership and startup user-ID formatting.

Correlation is stamped in this order, with each field present only when its
corresponding `Set` flag is true:

| Field | Key | Rendering |
| --- | --- | --- |
| Trace ID | `logkeys.TraceID` | String |
| Build identity | `logkeys.BuildID` | String |
| Configuration identity | `logkeys.ConfigID` | String |
| User ID | `logkeys.UserID` | Startup encoder, or explicit `UserIDField` override |

Explicitly empty strings and zero user IDs remain present. `WithTraceID`
preserves an existing trace ID and leaves the cache current in that case.
`WithBuildID`, `WithConfigID`, and `WithUserID` invalidate the cache.

Derivation is lazy: multiple correlation writes before the first `GetLogger`
lead to one derivation during sequential use. Formatting and Zap core encoding
run outside the context lock. Concurrent first readers may duplicate this work;
they reuse the first published logger for the current correlation/source
version. If either changes during derivation, `GetLogger` discards the result
and tries again, at most three derivations per call. After that it returns the
last snapshot it built without caching it, so a call racing a sustained stream
of writes still returns promptly. Panics propagate without marking an obsolete
logger current.

A returned logger, including a named child, is an immutable snapshot. After a
correlation write, call `GetLogger` again and derive a new named child to see the
change. Copying an SRouter context shares the immutable source and any current
logger, but future correlation/source writes and cache updates are independent.
Use the `With*` helpers for writes; direct struct-field writes bypass cache
invalidation and synchronization.

Contexts created by `EnsureSRouterContext` or a correlation helper alone have no
logging source; `GetLogger` returns `nil, false`. Existing users of
`GetCorrelation` can continue applying their own fields in that case.

For background work, create a source once at worker initialization and reuse it
at each job boundary:

```go
// At startup, using the application logger before any job fields are added:
source := scontext.NewRequestLoggerSource[uint64](appLogger, nil)
worker := &Worker{logSource: source}
```

The worker holds `logSource *scontext.RequestLoggerSource[uint64]`:

```go
func (w *Worker) handle(ctx context.Context, msg Message) error {
	ctx = scontext.WithRequestLogger[uint64, User](ctx, w.logSource)
	ctx = scontext.WithBuildID[uint64, User](ctx, w.buildID)
	ctx = scontext.WithConfigID[uint64, User](ctx, w.configID)
	ctx = scontext.WithTraceID[uint64, User](ctx, msg.TraceID)
	return w.process(ctx, msg)
}
```

`WithRequestLogger(ctx, source)` replaces the source and invalidates the cached
logger. Passing nil removes it. A source's zero value disables logging, and
`NewRequestLoggerSource` returns nil when its base is nil. The router resolves a
nil configured logger to its production/no-op fallback before creating a source.
A base must not already carry request correlation fields, since Zap appends
fields instead of replacing them.

## Database transactions

Transactions stored in the context implement `scontext.DatabaseTransaction`:

```go
type DatabaseTransaction interface {
	Commit() error
	Rollback() error
	SavePoint(name string) error
	RollbackTo(name string) error
	GetDB() *gorm.DB
}
```

GORM's `*gorm.DB` does not implement this interface directly because its
transaction methods return `*gorm.DB`. Wrap it with
`middleware.NewGormTransactionWrapper` before storing it:

```go
tx := db.Begin()
if tx.Error != nil {
	return tx.Error
}

ctx := scontext.WithTransaction[string, User](
	r.Context(),
	middleware.NewGormTransactionWrapper(tx),
)
next.ServeHTTP(w, r.WithContext(ctx))
```

Use `Commit`, `Rollback`, `SavePoint`, and `RollbackTo` through the interface.
Call `GetDB()` when handler code needs the underlying GORM transaction.

## Copying SRouter context values

`CopySRouterContext[T, U](dst, src)` attaches a new wrapper containing the
source values to `dst`. It preserves `dst`'s cancellation and deadline chain.
If `src` has no SRouter context, it returns `dst` unchanged.

`CopySRouterContextOverlay[T, U](dst, src)` performs the same replacement only
when both contexts already contain an SRouter context. It is a no-op when either
wrapper is absent. It replaces the destination values; it does not merge them.

Both functions create an independent wrapper and copy the mutable `Flags` map
and `PathParams` slice. Other fields are assigned normally. Consequently,
reference-bearing values—including `User`, `Transaction`, `HandlerError`, and
any pointer-bearing user ID—still refer to the same underlying objects. These
functions are therefore not recursive deep-copy operations.
