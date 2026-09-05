# Request-scoped logger in `SRouterContext`

Status: implemented (2026-09-05)

## Problem

Applications built on SRouter want every log line inside a request to carry
the request's correlation values: `trace_id`, `build_id`, `config_id`, and
`user_id`. SRouter stores those values in `SRouterContext` and exposes them
through `GetCorrelation`, but it offers no logger that already carries them.
The consumer is left with two bad options per log line:

1. Call `logger.With(correlationFields...)` at every log site. Measured in
   go-common, one such call costs about 92 ns and two allocations (384 B),
   before the level is checked, because `With` clones the core and pre-encodes
   the fields. go-common has roughly 540 of these call sites, and a hot
   permission check that should cost 2 ns costs 400 ns and six allocations
   almost entirely because of this pattern.
2. Pass the correlation values as ordinary fields on each line. That keeps
   the cost down only if every call site remembers to do it, and it still
   walks the context and takes the lock once per line.

Zap's intended pattern is a child logger stamped once at the boundary that
owns the request. SRouter owns that boundary, already holds the router
logger, and already writes every correlation value. It should also own the
request-scoped logger.

## Goals

- A `*zap.Logger` per request, stamped with the current correlation values,
  readable from any `context.Context` that carries an `SRouterContext`.
- Reading it costs one context walk and no allocations. Lines emitted through
  it cost nothing beyond zap's own level check when the level is disabled,
  and do not re-encode the correlation fields when it is enabled.
- The logger is never stale relative to the correlation values SRouter has
  written: a user ID stored by authentication is visible on the logger the
  handler reads.
- Contexts created outside the router keep working unchanged.

## Non-goals

- Replacing the router's own logging (`r.logger`, the request summary, error
  records). Those keep using their explicit field lists.
- Propagating any new value over HTTP headers.
- Changing the `Correlation` value type or `GetCorrelation`.

## Design

### Storage

`SRouterContext[T, U]` gains three unexported fields, guarded by the existing
`mu`:

```go
// Request-scoped logging. base is installed once by the boundary that owns
// the request; logger is derived from base plus the correlation fields and
// rebuilt on demand after any correlation write.
logBase     *zap.Logger
logger      *zap.Logger
loggerStale bool
userIDField func(T) zap.Field
```

The fields are unexported on purpose. Every other field on the struct is
exported for historical reasons, but the derived logger has an invariant
(it must match the correlation values) that direct writes would break.

### Installing the base

```go
// WithRequestLogger installs base as the logger from which the request's
// stamped logger is derived. base must be the application's logger, not one
// already stamped with request fields. Passing nil removes the request
// logger. userIDField renders the user ID; nil selects zap.Any.
func WithRequestLogger[T comparable, U any](ctx context.Context, base *zap.Logger, userIDField func(T) zap.Field) context.Context
```

The router calls it in `ServeHTTP`, immediately before `WithClientInfo`
creates the context (router.go, around line 675), using the resolved
application logger and a new `RouterDependencies.UserIDField`. `NewRouter`
resolves `RouterConfig.Logger` to a production or no-op logger when it is
nil and then names its own copy `"SRouter"` (router.go:105-124). The router
must keep the resolved, unnamed logger in a second field and derive request
loggers from that one, so application lines never inherit the `SRouter`
name and the base is never nil.

`RouterDependencies` gains one optional field:

```go
// UserIDField renders a user ID as a log field on the request logger.
// Optional; nil renders with zap.Any, which picks the typed zap constructor
// for builtin T (uint64, string, ...) and falls back to reflection for
// named types.
UserIDField func(T) zap.Field
```

Applications that run work outside the router (background jobs, message
consumers) call `WithRequestLogger` themselves at the job boundary, the same
way they already call `WithBuildID` and `WithConfigID` there.

### Derivation

The stamped logger is derived lazily. Every correlation writer marks it
stale instead of rebuilding it:

- `WithTraceID`, only on the branch that actually sets the ID (it preserves
  an existing one, and that branch must not mark anything).
- `WithBuildID`, `WithConfigID`, `WithUserID`.
- `WithRequestLogger` itself.

`GetLogger` rebuilds on read when stale:

```go
// GetLogger returns the request-scoped logger, stamped with the correlation
// values written so far. It returns nil and false when the context carries
// no SRouterContext or no request logger was installed. The returned logger
// reflects the correlation values at the moment of the call; a later
// correlation write is visible only through a fresh GetLogger call.
func GetLogger[T comparable, U any](ctx context.Context) (*zap.Logger, bool) {
	rc, ok := GetSRouterContext[T, U](ctx)
	if !ok {
		return nil, false
	}
	rc.mu.RLock()
	if !rc.loggerStale {
		l := rc.logger
		rc.mu.RUnlock()
		return l, l != nil
	}
	rc.mu.RUnlock()

	rc.mu.Lock()
	defer rc.mu.Unlock()
	if rc.loggerStale { // re-check: another reader may have rebuilt
		rc.rebuildLoggerLocked()
	}
	return rc.logger, rc.logger != nil
}
```

`rebuildLoggerLocked` clears the flag and sets `rc.logger` to
`rc.logBase.With(fields...)`, or to nil when there is no base. The fields,
in this fixed order, each present only when its `Set` flag is true:

| Field | Key | Constructor |
| --- | --- | --- |
| Trace ID | `logkeys.TraceID` (`trace_id`) | `zap.String` |
| Build ID | `logkeys.BuildID` (`build_id`) | `zap.String` |
| Config ID | `logkeys.ConfigID` (`config_id`) | `zap.String` |
| User ID | `logkeys.UserID` (`user_id`, new constant) | `userIDField`, else `zap.Any` |

Why lazy rather than rebuilding in each writer: the router writes build ID,
config ID, and trace ID before dispatch, and the user ID during
authentication, all before the handler emits its first line. Eager
derivation would clone the logger up to four times per request. Lazy
derivation clones it once, on the handler's first `GetLogger`, and again
only if something writes correlation afterwards, which nothing in the
router does. The double-checked lock is the standard idiom and the stale
branch is hit at most a couple of times per request.

The `Set` flags decide presence, matching `GetCorrelation`, so a trace ID
that was explicitly set to the empty string is still stamped as
`trace_id=""`. Consumers that want an always-present `trace_id` key can add
it themselves; SRouter does not invent values.

### Read path cost

`GetLogger` on the fast path is one `ctx.Value` walk, one `RLock`/`RUnlock`,
one pointer copy. No allocations. The logger it returns already has the
correlation fields encoded, so a line through it costs zap's level check
when disabled, and encodes only the line's own fields when enabled. Compared
with today's per-line `With`, that removes both allocations and the field
encoding from every line, and removes the correlation encoding from admitted
lines as well.

### Concurrency

The struct is shared by pointer across the middleware chain, and a
timed-out handler's goroutine may keep logging while the router goroutine
writes the handler error. Both paths go through `mu`, as today. The logger
pointer is swapped under the write lock and read under the read lock. A
goroutine holding a `*zap.Logger` obtained earlier keeps a valid, immutable
logger; it just may lack a value written later.

### Contexts without a request logger

`EnsureSRouterContext` creates a context with no base, so a context first
touched by `WithUserID` from application code (go-common's agentkit actor
context, for example) carries correlation but no logger. `GetLogger`
returns `nil, false` and the consumer falls back to whatever it did before.
The `Correlation` accessor is unchanged for that purpose.

## Changes by file

- `pkg/scontext/context.go`: fields, `WithRequestLogger`, `GetLogger`,
  `rebuildLoggerLocked`, stale marks in the four correlation writers. Add a
  `zap` import; `scontext` gains a dependency on `go.uber.org/zap`, which the
  module already requires.
- `pkg/logkeys/keys.go`: `UserID = "user_id"`, with the corresponding test
  row in `keys_test.go`.
- `pkg/router/router.go`: `RouterDependencies.UserIDField`; install the base
  logger in `ServeHTTP` before `WithClientInfo`.
- `docs/context-management.md`: table rows for `WithRequestLogger` /
  `GetLogger`, a paragraph on staleness and on job boundaries.
- `docs/logging.md`: a "Request-scoped logger" section after "Trace ID
  integration" showing the handler-side usage and the `UserIDField` hook.
- `CLAUDE.md` and `AGENTS.md`: add `logger, ok := scontext.GetLogger[T, U](r.Context())`
  to the context helper snippet.

## Tests

`pkg/scontext`:

- No base installed: `GetLogger` returns `nil, false` before and after
  correlation writes.
- Base installed, no correlation: logger is returned and carries no
  correlation fields (observer core).
- Each writer in turn (`WithTraceID`, `WithBuildID`, `WithConfigID`,
  `WithUserID`) makes the next `GetLogger` return a logger carrying the new
  field in the documented order; the previously returned logger is unchanged.
- `WithTraceID` on a context that already has a trace ID does not mark the
  logger stale (assert the same `*zap.Logger` pointer comes back).
- `UserIDField` is used when supplied; `zap.Any` otherwise, checked for a
  `uint64` T and a `string` T.
- `WithRequestLogger(ctx, nil, nil)` removes the logger.
- Race test: concurrent `GetLogger` readers with a writer calling
  `WithUserID`, run under `-race`.
- Benchmark: `GetLogger` on the fast path reports zero allocations.

`pkg/router`:

- A request through `ServeHTTP` with the built-in auth on an auth-required
  route: the handler's `GetLogger` carries `trace_id`, `build_id`,
  `config_id`, and `user_id`, and none of the lines carry `logger=SRouter`.
- The same with auth optional and no token: `user_id` is absent.
- `RouterConfig.Logger` nil: the request logger is derived from the
  router's fallback logger, not left absent, and carries no `SRouter` name.

Coverage must stay above the 80 % CI gate.

## Compatibility

Additive. No existing helper changes signature or behaviour; the only
observable change for an existing consumer is that `scontext` now imports
`zap`. Ship as a minor version.

## Consumer follow-up (go-common, out of scope here)

- Add `appLogger.For[U](ctx, fallback *zap.Logger) *zap.Logger` returning the
  request logger when present and `WithContextFields(ctx, fallback)` when
  not, so non-router contexts keep their trace fields until their boundary
  installs a base.
- Replace the ~540 `WithContextFields[U](ctx, h.logger)` call sites with
  `appLogger.For[U](ctx, h.logger)`.
- Install the base logger in the agentkit actor context and any job
  boundary that writes correlation.
- Pass the same `*zap.Logger` to `RouterConfig.Logger` and to the go-common
  handlers, and set `RouterDependencies.UserIDField` to `zap.Uint64` for
  `UserIdType`.
- Independently, convert `zap.String(k, enum.String())` fields to
  `zap.Stringer` so enum rendering is also deferred to encode time.

## Rejected alternatives

- **Store `[]zap.Field` instead of a logger.** Every line would then splice
  the slice into its own fields, which allocates before the level check.
  Strictly worse than the logger.
- **Rebuild eagerly in each writer.** Simpler, but up to four clones per
  request where one is needed. The lazy variant is a dozen lines more.
- **Lazy `zapcore.ObjectMarshaler` over the context.** Zero allocations when
  disabled but takes the lock and re-encodes the correlation on every
  admitted line, and does nothing for the per-line `With` habit. The stored
  logger is cheaper whenever logging is on, which Info and Warn always are.
- **Keep it in the consumer.** Only SRouter sees every correlation write, so
  only SRouter can keep the logger current without an invalidation protocol
  spread across two repositories.
