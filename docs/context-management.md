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

`Correlation[T]` holds values, not built log fields, so this package stays
independent of any logging implementation and the caller renders the user ID
with whatever constructor matches its own `T` — `zap.Uint64`, `zap.String`, or
its own encoder. A helper that built the field here would have to funnel every
user ID through the logger's any-typed fallback, which boxes the value and
allocates.

Each field carries a `Set` flag, so a value that was written empty on purpose
stays distinguishable from one that was never written. The result is a copy
taken at the moment of the call: a later write through a `With*` helper does
not change it, and two separate calls are not an atomic pair. The struct has no
reference-typed members, so it cannot alias the wrapper.

Code that reads correlation only in order to log it should call `GetLogger`
instead. It returns a logger that already carries the same values as fields.

## Request-scoped logger

`WithRequestLogger(ctx, base, userIDField)` installs `base` as the logger from
which the request's stamped logger is derived. `base` must be the application's
own logger, not one that already carries request fields. Passing a nil `base`
removes the request logger. `userIDField` renders the user ID as a field; a nil
`userIDField` selects `zap.Any`, which picks the typed constructor for a builtin
`T` and falls back to reflection for a named type.

The router calls `WithRequestLogger` on every request with its resolved
application logger and `RouterDependencies.UserIDField`, so handlers and
middleware only need `GetLogger`:

```go
logger, ok := scontext.GetLogger[uint64, User](r.Context())
```

`GetLogger` returns `(nil, false)` when the context carries no SRouter context,
and also when it carries one with no base installed. A context first touched by
`EnsureSRouterContext`, or by a bare `WithUserID` call from application code,
therefore carries correlation but no logger, and the caller falls back to
whatever logger it used before. `GetCorrelation` is unchanged for that case.

The stamped logger carries these fields, in this fixed order, each present only
when the corresponding value has been set:

| Field | Key | Constructor |
| --- | --- | --- |
| Trace ID | `logkeys.TraceID` | `zap.String` |
| Build identity | `logkeys.BuildID` | `zap.String` |
| Configuration identity | `logkeys.ConfigID` | `zap.String` |
| User ID | `logkeys.UserID` | `userIDField`, else `zap.Any` |

Presence follows the same `Set` flags as `GetCorrelation`, so a trace ID that
was set to the empty string on purpose is still stamped as `trace_id=""`.

The returned logger reflects the correlation values at the moment of the call. A
later `WithUserID`, `WithBuildID`, or `WithConfigID` does not change a logger
already in hand; it is visible only through a fresh `GetLogger`. In a normal
request that ordering is already correct, because the router writes every
correlation value before the handler runs. `WithTraceID` on a context that
already has a trace ID preserves the existing ID and therefore does not change
the logger at all.

Deriving the logger is lazy. Each correlation writer only marks it stale, and
the next `GetLogger` rebuilds it once, so a request that writes four correlation
values before its first log line pays for a single clone rather than four.

Work that runs outside the router owns its own boundary. Background jobs and
message consumers call `WithRequestLogger` themselves, alongside the
`WithBuildID` and `WithConfigID` calls they already make there:

```go
func (w *Worker) handle(ctx context.Context, msg Message) error {
	ctx = scontext.WithBuildID[uint64, User](ctx, w.buildID)
	ctx = scontext.WithConfigID[uint64, User](ctx, w.configID)
	ctx = scontext.WithTraceID[uint64, User](ctx, msg.TraceID)
	ctx = scontext.WithRequestLogger[uint64, User](ctx, w.logger, nil)

	return w.process(ctx, msg)
}
```

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
