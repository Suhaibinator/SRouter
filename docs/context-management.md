# Context management

SRouter stores its request-scoped values in one `scontext.SRouterContext[T, U]`
attached to the standard `context.Context`. `T` is the router's user ID type and
`U` is its user object type. Middleware and handlers must use the same type
arguments that were passed to `router.NewRouter[T, U]`.

Write helpers take both type arguments because they may create the wrapper.
Read helpers are non-generic unless their result depends on a user type:
`GetUserID[T]` and `GetCorrelation[T]` return values containing `T`;
`GetUser[T, U]` returns `*U`; and `GetSRouterContext[T, U]` returns the
typed wrapper. These typed reads report mismatched types as absent.
All other getters read whichever SRouter carrier is present, without requiring
callers to know its user ID or user object types.

All `SRouterContext` fields are private. Use the helpers in `pkg/scontext`
to read and write request state. The wrapper is shared by pointer across the
middleware chain, and a handler that has timed out may briefly continue in a
goroutine while the router reads request state. The helpers synchronize access
with the wrapper's internal lock.

## Stored values

| Value | Write helper | Read helper | Clear helper |
| --- | --- | --- | --- |
| User ID | `WithUserID` | `GetUserID` | `ClearUserID`, `ClearIdentity` |
| User object (`*U`) | `WithUser` | `GetUser` | `ClearUser`, `ClearIdentity` |
| Client IP | `WithClientIP`, `WithClientInfo` | `GetClientIP` | `ClearClientIP`, `ClearClientInfo` |
| User agent | `WithUserAgent`, `WithClientInfo` | `GetUserAgent` | `ClearUserAgent`, `ClearClientInfo` |
| Trace ID | `WithTraceID` / `SetTraceID` | `GetTraceID` | `ClearTraceID` |
| Build identity | `WithBuildID` | `GetBuildID` | `ClearBuildID` |
| Configuration identity | `WithConfigID` | `GetConfigID` | `ClearConfigID` |
| Database transaction | `WithTransaction` | `GetTransaction` | `ClearTransaction` |
| Route template and path parameters | `WithRouteInfo`, `SetRouteInfo` | `GetRouteTemplate`, `GetPathParams` | `ClearRouteInfo` |
| Allowed CORS origin and credentials | `WithCORSInfo` | `GetCORSInfo` | `ClearCORSInfo` |
| Requested CORS headers | `WithCORSRequestedHeaders` | `GetCORSRequestedHeaders` | `ClearCORSRequestedHeaders` |
| Generic-handler error | `WithHandlerError` | `GetHandlerError` | `ClearHandlerError` |
| Application boolean flag | `WithFlag` | `GetFlag` | `ClearFlag` |
| All correlation values at once | (see individual writers) | `GetCorrelation` | (see individual clears) |
| Request-scoped logger | `WithRequestLogger` | `GetLogger` | `ClearRequestLogger` |

Most getters return `(value, ok)` so an unset value can be distinguished from
its zero value. The trace-ID getters instead return an empty string when no
trace ID is set. `SetTraceID` unconditionally replaces the ID under the context
lock, marks it set (even when empty), and invalidates the cached request logger.
It performs no validation. Automatic tracing uses it after validation at the
request boundary; see [Logging](./logging.md#trace-id-integration).
`WithTraceID` preserves an existing ID rather than overwriting
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

## Clearing values

Every clear helper takes `[T, U]` and returns the supplied context unchanged.
It mutates an existing matching wrapper under its lock, resetting both the
stored value and its presence. Missing wrappers and mismatched types are no-ops;
clearing never creates a wrapper. Repeated clears leave values absent.
`ClearFlag(ctx, name)` deletes only that named flag.

A zero or nil write still means present: for example, `WithUserID(ctx, 0)`
and `WithUser(ctx, nil)` do not remove identity. Use `ClearIdentity` to remove
both the user ID and user object atomically. `ClearClientInfo`, `ClearRouteInfo`,
and `ClearCORSInfo` likewise clear their related values under one lock.

Derived contexts share the wrapper unless explicitly copied. For follow-up work
that must run without the parent's actor, clone first:

```go
child := scontext.CopySRouterContext[T, U](ctx, ctx)
child = scontext.ClearIdentity[T, U](child)
// GetUserID[T](child) and GetUser[T, U](child) now report absent.
// The parent and its other children retain their identity.
```

This removes the stored identity only; application flags and other context
values remain. See the [identity context example](../examples/identity-context/main.go);
run it with `go run .` from `examples/identity-context`.

Clearing user ID, trace/build/config ID, or client IP invalidates the cached
request logger, including through grouped clears. Reacquire `GetLogger` and
any named child afterward: previously returned loggers are immutable snapshots
and still carry their original fields. `ClearRequestLogger` removes the source
and cache; `WithRequestLogger` can attach a source again. Other clears preserve
the cache. `ClearTraceID` also allows a subsequent `WithTraceID` to install an ID.

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

handlerErr, failed := scontext.GetHandlerError(nextRequest.Context())
_ = handlerErr
_ = failed
```

## Reading values in a handler

```go
func accountHandler(w http.ResponseWriter, r *http.Request) {
	userID, authenticated := scontext.GetUserID[string](r.Context())
	if !authenticated {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	user, hasUser := scontext.GetUser[string, User](r.Context())
	clientIP, _ := scontext.GetClientIP(r.Context())
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
	c, ok := scontext.GetCorrelation[uint64](ctx)
	if !ok {
		return nil
	}

	fields := make([]zap.Field, 0, 4)
	if c.HasTraceID() {
		fields = append(fields, zap.String(logkeys.TraceID, c.TraceID))
	}
	if c.HasBuildID() {
		fields = append(fields, zap.String(logkeys.BuildID, c.BuildID))
	}
	if c.HasConfigID() {
		fields = append(fields, zap.String(logkeys.ConfigID, c.ConfigID))
	}
	if c.HasUserID() {
		fields = append(fields, zap.Uint64("user_id", c.UserID))
	}
	return fields
}
```

`Correlation[T]` holds values rather than Zap fields, so it can also be used
with metrics and other logging implementations. Callers can choose their own
rendering for the user ID. Client IP remains available through `GetClientIP`;
it is part of `GetLogger` derivation but is not added to the public
`Correlation` value.

`HasTraceID()`, `HasBuildID()`, `HasConfigID()`, and `HasUserID()` read the
snapshot's private presence mask. An explicitly empty value stays distinguishable
from an absent or cleared one. The zero snapshot reports all values absent.
The result is a copy taken at the moment of the call: a later write or clear
does not change it, and two separate calls are not an atomic pair. Scalar values and
presence flags are copied; references inside a generic user ID retain their
normal Go sharing semantics.

Code that reads correlation only in order to log it should call `GetLogger`
instead. It returns a logger that already carries the same values as fields.

## Request-scoped logger

Configure logging once at application initialization. `NewRouter` creates a
`scontext.RequestLoggerSource[T]` from its resolved `RouterConfig.Logger` and
optional `RouterDependencies.UserIDField`, then attaches that source at the
beginning of request handling, before a lazy build. Client information and
runtime identities are also installed at that boundary. The source holds the
application logger and user-ID encoder; it contains no request values or
per-request cache.

`GetLogger(ctx)` returns the shared request logger. Use `Named` with a
relative service name and reuse that child within the operation:

```go
logger, ok := scontext.GetLogger(ctx)
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

Request fields are stamped in this order:

| Field | Key | Rendering |
| --- | --- | --- |
| Trace ID | `logkeys.TraceID` | Non-empty string |
| Build identity | `logkeys.BuildID` | String |
| Configuration identity | `logkeys.ConfigID` | String |
| Client IP | `logkeys.ClientIP` | Non-empty string |
| User ID | `logkeys.UserID` | Startup encoder, or explicit `UserIDField` override |

Client IP and trace ID are included only when non-empty. Build and configuration
identities retain their `Set` semantics, and a zero user ID remains present.
`WithTraceID` preserves an existing trace ID and leaves the cache current in
that case. `WithClientIP`, `WithClientInfo`, `WithBuildID`, `WithConfigID`, and
`WithUserID` invalidate the cache when they change a stamped value. Updating
only the user agent does not rebuild the logger because user agent is not part
of the shared logger.

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
stamped-field write, call `GetLogger` again and derive a new named child to see
the change. Copying an SRouter context shares the immutable source and any
current logger, but future request-field/source writes and cache updates are
independent.
Use the write helpers to preserve cache invalidation and synchronization.

Contexts created by `EnsureSRouterContext` or a request-field helper alone have
no logging source; `GetLogger` returns `nil, false`. Existing users of
`GetCorrelation` can continue applying their own fields in that case.

For background work, create a source once at initialization and reuse it at
each job boundary:

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
A base must not already carry request fields such as `client_ip`, `trace_id`,
or `user_id`, since Zap appends fields instead of replacing them.

SRouter installs the request logger before running middleware. Middleware
constructors do not require a logger argument or manual source installation.
See [Middleware logging](./logging.md#middleware-logging).

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

### Clearing or replacing a transaction in a child operation

`WithTransaction` and `ClearTransaction` mutate an existing matching wrapper.
Derived contexts normally share that wrapper: adding a deadline or calling
`context.WithValue` does not isolate SRouter state. Request middleware can
deliberately populate the shared wrapper with `WithTransaction`.

For post-commit callbacks or follow-up work that must not inherit the parent's
transaction, clone first:

```go
child := scontext.CopySRouterContext[T, U](ctx, ctx)
child = scontext.ClearTransaction[T, U](child)
// GetTransaction(child) returns (nil, false).
// The parent and its other children keep their transaction.
```

For a child operation that uses a different transaction, also clone first:

```go
child := scontext.CopySRouterContext[T, U](ctx, ctx)
child = scontext.WithTransaction[T, U](child, childTransaction)
```

`ClearTransaction[T, U]` clears the reference and presence flag under the
wrapper's lock. It returns the supplied context and does not create a wrapper
when none exists. A wrapper with a different `T` or `U` is left unchanged.
Clearing is idempotent and never commits, rolls back, or calls any other method
on the transaction. Other request state, including the cached logger, is
preserved. `WithTransaction(ctx, nil)` continues to mean explicitly present
nil, so `GetTransaction` returns `(nil, true)` until cleared.

See the [transaction context example](../examples/transaction-context/main.go);
run it with `go run .` from `examples/transaction-context`.

## Breaking change: correlation presence methods

Replace `c.TraceIDSet`, `c.BuildIDSet`, `c.ConfigIDSet`, and `c.UserIDSet` with
`c.HasTraceID()`, `c.HasBuildID()`, `c.HasConfigID()`, and `c.HasUserID()`.
The value fields (`TraceID`, `BuildID`, `ConfigID`, `UserID`) remain exported.
Presence is stored in a private bitmask; assigning a value field does not mark
it present. Obtain populated snapshots through `GetCorrelation` after using
the context's `With*` helpers instead of constructing literals with presence
booleans. Copies retain their presence independently of later context changes.
The removed exported presence fields are also no longer included by default
struct serialization, such as `encoding/json`; use an application-owned DTO
when a serialized presence representation is needed.

## Breaking change: non-generic metadata getters

Remove type arguments from `GetBuildID`, `GetConfigID`, `GetFlag`,
`GetClientIP`, `GetUserAgent`, `GetTransaction`, `GetTraceID`,
`GetCORSInfo`, `GetCORSRequestedHeaders`, `GetHandlerError`, and `GetLogger`.
For example, a transaction read is now `scontext.GetTransaction(ctx)`.

These getters no longer filter carriers by user ID type. Return types,
synchronization, unset-value behavior, and logger caching are unchanged.
`GetRouteTemplate` and `GetPathParams` were already non-generic.
Typed getters listed at the top of this guide retain their type parameters,
as do setters, copy helpers, and all `Clear*` helpers.

## Breaking change: private context fields

`SRouterContext` now has no exported fields. Code that reads or writes fields
directly, or uses populated struct literals, must migrate to the helpers in the
stored-values table above. For example, use `GetUserID[T]` to read both the
value and presence instead of reading `UserID` and `UserIDSet`; initialize
values through `WithUserID[T, U]` rather than a struct literal. Use
`WithFlag` and `GetFlag` for named flags instead of accessing the map.

Replace manual field and presence resets with the corresponding `Clear*`
helper, retaining any clone-first isolation. In particular, use
`ClearIdentity[T, U]` to remove both actor fields and `ClearTransaction[T, U]`
to remove the transaction. The internal presence mask is private;
`Correlation[T]` exposes presence through its `Has*` methods.

The type, its zero value, constructors, attachment helpers, and existing helper
signatures remain available. Do not copy a wrapper by value; it contains a
mutex. Use `CopySRouterContext` or `CopySRouterContextOverlay` instead.
`Correlation[T]` remains a public value snapshot with exported value fields.

Private fields do not change reference-sharing semantics: user objects,
transactions, and slices returned by getters still require caller coordination
when mutated. Encapsulation does not recursively copy those objects.

## Copying SRouter context values

`CopySRouterContext[T, U](dst, src)` attaches a new wrapper containing the
source values to `dst`. It preserves `dst`'s cancellation and deadline chain.
If `src` has no SRouter context, it returns `dst` unchanged.

`CopySRouterContextOverlay[T, U](dst, src)` performs the same replacement only
when both contexts already contain an SRouter context. It is a no-op when either
wrapper is absent. It replaces the destination values; it does not merge them.

Both functions create an independent wrapper and copy the mutable flags map
and path-parameter slice. Other fields are assigned normally. Consequently,
reference-bearing values—including the user, transaction, handler error, and
any pointer-bearing user ID—still refer to the same underlying objects. These
functions are therefore not recursive deep-copy operations.

### Client IP normalization

`WithClientIP` and `WithClientInfo` remove a valid socket port before storing
client information. This happens once per context write, not per log record.
The normalized value is shared by `GetClientIP`, rate limiting, and the cached
request logger. Writing another port for the same normalized IP does not
invalidate the cached logger. Existing IPv6 bracket and zone handling is
preserved; malformed addresses are retained without reinterpretation.

SRouter initializes client information before invoking middleware. An uninitialized client IP is omitted from logs; logging
never falls back to `RemoteAddr` or mutates request context.
