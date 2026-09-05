# Request-scoped logger in `SRouterContext`

Status: implemented; revised 2026-09-05 for startup configuration and service-owned names.

## Purpose

Applications need trace, build, config, and user correlation on request logs.
Repeated `logger.With(correlationFields...)` calls clone and encode those fields
even before a log level is checked. Cache the stamped logger in SRouter's
existing context wrapper and let services attach their relative names.

The application root owns logging configuration. SRouter owns request
correlation. Each service owns its component name. The public usage and
formatter contract live in the [logging guide](../logging.md#request-scoped-logger);
context lifetime and job setup live in
[Context management](../context-management.md#request-scoped-logger).

## Startup configuration

`RequestLoggerSource[T]` holds a private application `*zap.Logger` and a private
`func(T) zap.Field`. It is immutable after construction and contains no request
state. `NewRequestLoggerSource(base, userIDField)` resolves a default encoder
once, or uses an explicit formatter. `NewRouter` constructs one source from its
resolved `RouterConfig.Logger` and optional `RouterDependencies.UserIDField`.
The configured application name remains intact; `SRouter` is appended only to
the router's internal logger.

`WithRequestLogger[T, U](ctx, source)` attaches the preconfigured source at a
request or job boundary. Formatter policy is not supplied on every request.
Background workers create a source once at startup and reuse it across jobs.
A nil source, a zero-valued source, or constructing with a nil base disables
request logging. A source base must not already carry request fields.

The ID type remains `T comparable`. A default encoder inspects
`reflect.TypeFor[T]()` at startup. Strings, bools, integers, and floats use typed
Zap fields through safe reflection accessors, including for named primitive
types. Types implementing Zap/JSON/text marshaling, `fmt.Stringer`, or `error` retain
`zap.Any` rendering; other types also fall back to `zap.Any`. For an interface
T, the fallback handles each value's dynamic type. An explicit formatter takes
precedence, including its field key.

There are no unsafe casts and no required `String` method. Applications can
provide a typed conversion at initialization when they want a specific format.

## Context storage and derivation

The context stores the shared source pointer, cached logger pointer, current
correlation/source version, and cached version under its existing `mu`.
`WithBuildID`, `WithConfigID`, `WithUserID`, and actual `WithTraceID` writes
advance the version. A preserved trace ID does not. Replacing or removing the
source also advances the version and releases the cached logger.

`GetLogger` has two paths:

1. Under a read lock, return the cached pointer if its version is current, or
   return `nil, false` when logging is disabled.
2. Otherwise snapshot correlation, source, and version under that lock, then
   release it. Build fields and call `base.With` outside the lock. Acquire the
   write lock to publish only if the version is still current. If it changed,
   discard the result and try again, up to three derivations per call; after
   that, return the last snapshot uncached. If another reader already
   published this version, reuse its logger.

Correlation fields are ordered trace, build, config, user. Presence follows the
existing `Set` flags, including explicitly empty strings or zero IDs.

Application formatters and Zap custom encoding can read context values without
deadlocking. They must be concurrency-safe and should be free of side effects;
concurrent first readers can derive independently. A failed derivation never
marks an obsolete logger current. Continuous correlation writes cause bounded
retries, never a spin; the normal request flow finishes these writes before
handler logging.
No extra derivation mutex or per-service map is stored in the context.

Previously returned loggers are immutable snapshots. After changing
correlation, consumers obtain a fresh request logger and named child. Context
copies share immutable configuration/current loggers but retain independent
correlation and cache updates. Direct writes to exported context fields bypass
the helpers' synchronization and invalidation contract.

## Service-owned names

A service reads the shared request logger and calls
`requestLogger.Named("common_service.admin")`. With an application root named
`myapp`, the result is `myapp.common_service.admin`. A sibling service can use
`common_service.permission` independently. The service retains a relative name;
using an old logger's fully qualified name could repeat the application prefix.

Zap's `Named` changes logger metadata and shares the stamped core. It does not
clone the core or encode correlation again. Derive one child per service
operation and reuse it across its log lines. Naming still allocates the logger
value and, when joining names, a string. Additional service fields or options
must be applied explicitly to the child.

The runnable [request logger example](../../examples/request-logger/main.go)
demonstrates this ownership, named numeric IDs, fallback loggers, and shutdown.
The go-common wrapper and call-site migration are subsequent work: it should
store a relative component name and fallback, get SRouter's request logger,
and add the component name during derivation. All participating services use
the application root's logging configuration.

## Performance evidence

Measured with Go 1.27.1, Zap 1.28.0, darwin/arm64 on an Apple M4 Max. Three local
runs; timings vary with workload and core implementation. Derivation uses a
JSON core writing to `io.Discard`, all four correlation fields, a named uint64
ID, and includes the invalidating user-ID write.

| Operation | Time per operation | Bytes | Allocations |
| --- | ---: | ---: | ---: |
| Warm `GetLogger` | ~4.7 ns | 0 | 0 |
| Warm lookup plus `Named` with an application prefix | 44–45 ns | 152 | 2 |
| Derivation with the default encoder | 382–431 ns | ~1,554 | 6 |
| Derivation with explicit uint64 conversion | 383–388 ns | ~1,554 | 6 |
| Derivation with `zap.Any` for the named ID | 700–703 ns | ~2,871 | 12 |

The logging state occupies four machine words on this 64-bit target, the same
space as the earlier PR's base/logger/stale/formatter fields. Compared with
main, requests carry that additional context state and attach the source under
the existing context lock. Requests that never obtain a logger do not derive
one. Empty correlation reuses the application logger without allocating.
Cached reads avoid field encoding; name derivation remains a per-operation
cost. Concurrent first readers may pay duplicate derivation costs.

Reproduce the microbenchmarks with:

```bash
go test -run '^$' -bench 'Benchmark(GetLogger|RequestLoggerNamed)' -benchmem -count=3 ./pkg/scontext
```

## Validation and scope

Tests cover correlation presence and order, lazy derivation, source reuse and
removal, copying before and after derivation, formatter overrides, named
primitive encoding, custom rendering, interface IDs, zero-allocation cached
reads, and independent named services sharing the request core.

Concurrency regressions cover formatter/core access to context, panic recovery,
in-flight derivation racing correlation/source changes, and concurrent first
readers choosing one published logger. Router integration covers authentication,
missing users, application name preservation, and nil configured logger fallback.
Run the race suite, lint, doc-link checks, and example builds before merging.

This remains additive relative to main. The new, unreleased PR API now takes a
source instead of `(base, userIDField)` at the request boundary. The existing
`Correlation` API and router error/summary logging remain supported. The
`scontext` package now uses Zap, already a module dependency.
