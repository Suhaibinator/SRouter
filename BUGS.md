# SRouter Bug Review — Progress Tracker

Review date: 2026-06-09. Source: full read of every non-test file in `pkg/` (~6,000 lines).
Items 1–4 were confirmed with throwaway repro tests (since deleted); items 5–13 were
high-confidence findings from inspection.

**Status legend:** `[ ]` open · `[x]` fixed · strike through items deemed won't-fix.

**Progress: 20/20 fixed** (all on branch `bugs`, 2026-06-09). Full suite, `go test -race`,
`go vet`, and all examples build clean. Regression tests added in
`pkg/router/bug_regression_test.go`, plus new cases in the codec, common, and middleware
test files.

---

## Confirmed bugs (reproduced with tests)

- [x] **1. `StrategyUser` rate limiting via overrides returns 500 on every request**
  `getEffectiveRateLimit` (`pkg/router/router.go`) converted `RateLimitConfig[any,any]`
  to `[T,U]` but set `UserIDFromUser`/`UserIDToString` to nil; `extractUserKey`
  (`pkg/middleware/ratelimit.go`) then hard-errored → 500 on every request.
  **Fixed:** the conversion now wraps both functions across the type change, and
  `extractUserKey` falls back to the default user-ID-to-string conversion when
  `UserIDToString` is nil (it is now documented as optional in `common.RateLimitConfig`).
  *Regression test:* `TestStrategyUserRateLimitViaOverrides`.

- [x] **2. `RegisterGenericRouteOnSubRouter` applies global middlewares twice**
  It folded `r.middlewares` into the route's middleware list and `wrapHandler`
  appended them again.
  **Fixed:** globals removed from the fold; `wrapHandler` remains the single place
  that applies them (matching `NewGenericRouteDefinition`).
  *Regression test:* `TestRegisterGenericRouteOnSubRouterAppliesGlobalsOnce`.

- [x] **3. Nested sub-routers do not inherit parent sub-router middlewares (contradicts docs)**
  `registerSubRouterWithResolvedOverrides` propagated prefix/overrides but not
  `Middlewares` or `AuthLevel`.
  **Fixed:** nested sub-routers now inherit parent middlewares additively (parent's run
  first) and the parent's `AuthLevel` when they don't set their own. The same inheritance
  was added to `resolveSubRouterConfigForPrefixMatch` so post-creation lookups agree.
  *Regression tests:* `TestNestedSubRouterInheritsParentMiddlewares`,
  `TestNestedSubRouterInheritsParentAuthLevel`.

- [x] **4. `UberRateLimiter.Allow` blocks the goroutine, then denies anyway; limit/window conversion inflates small limits**
  uber-go `ratelimit.Take()` slept until the next slot and the RPS conversion floored
  to ≥1 rps ("2 per minute" behaved as ~60/minute).
  **Fixed:** the internals were replaced with a non-blocking sliding-window counter
  (current + overlap-weighted previous window, O(1) per key) that honors limit/window
  exactly and denies immediately. The `UberRateLimiter` name is kept for API
  compatibility; the `go.uber.org/ratelimit` dependency was dropped (`go mod tidy`).
  *Regression test:* `TestUberRateLimiter_WindowSemanticsAndNonBlocking`.

## High-confidence bugs found by inspection

- [x] **5. Data race on `SRouterContext` after timeout**
  A timed-out request's handler goroutine could keep mutating the shared
  `*SRouterContext` (e.g. `WithHandlerError`, `Flags` map) while the router goroutine
  read it (deferred logger, post-timeout middleware reading `GetHandlerError`).
  **Fixed:** `SRouterContext` now carries an internal `sync.RWMutex`; every `With*`/`Get*`
  helper (and `cloneSRouterContext`) locks it. Uniform locking is deliberate: handlers
  may call any public `With*` setter after a timeout has returned, so no field can be
  assumed single-goroutine. Uncontended cost is ~tens of ns per access.

- [x] **6. Graceful-shutdown `WaitGroup` misuse**
  `wg.Add(1)` lived in the innermost handler, racing `wg.Wait()` from zero and leaving
  mid-chain requests untracked.
  **Fixed:** the shutdown check + `wg.Add(1)` moved to the top of `ServeHTTP`, performed
  under `shutdownMu.RLock` so it can never race `Shutdown`'s write-lock + `Wait`. The
  whole middleware chain is now tracked.

- [x] **7. `MetricsConfig` settings silently ignored**
  `DefaultTags`, `SamplingRate`, `Subsystem`, and `MiddlewareFactory` were unused.
  **Fixed:** `DefaultTags` are applied to every metric the middleware builds;
  `MetricsMiddlewareConfig.SamplingRate` in (0,1) now auto-installs a `RandomSampler`
  (in `NewMetricsMiddleware` and `Configure`); `MetricsConfig.Subsystem` is emitted as a
  `subsystem` tag (and `Namespace` as `service`, only when non-empty); a
  `MiddlewareFactory` implementing `metrics.MetricsMiddleware[T,U]` now takes precedence
  over `Collector`. Config docs updated.

- [x] **8. Per-request Prometheus collector churn + panic path**
  A new collector was constructed/registered on every request, non-`AlreadyRegistered`
  errors hit `MustRegister` (panic in the request path), the global latency histogram
  was named `*_total`, and `status_code` stored `http.StatusText`.
  **Fixed:** `MetricsMiddlewareImpl` caches built metrics in a `sync.Map` (one build per
  route/status); the Prometheus adapter logs registration errors and keeps the
  unregistered (still functional) metric instead of panicking; the global histogram is
  renamed `all_request_latency_seconds` (matching `all_requests_total`); `status_code`
  now records the numeric code (e.g. `"404"`).

- [x] **9. Body-too-large detection by string comparison**
  `err.Error() == "http: request body too large"` broke when codecs wrapped the error.
  **Fixed:** new `isMaxBytesError` helper using `errors.As` with `*http.MaxBytesError`,
  used in both `route.go` and `handleError`.

- [x] **10. `DecodeBase62` corrupts binary payloads with leading zero bytes**
  The `big.Int` round trip dropped leading `0x00` bytes.
  **Fixed:** base58-style convention — each leading `'0'` character decodes to one
  leading zero byte, and a new `EncodeBase62` emits them symmetrically so
  `DecodeBase62(EncodeBase62(b))` round-trips exactly.
  *Regression test:* `TestBase62RoundTrip`.

- [x] **11. Exported legacy `middleware.Timeout` is racy**
  It wrote the 408 to the raw writer without checking whether the handler had already
  written.
  **Fixed:** mirrors the router's hardened version — `wroteHeader`/`timedOut` atomics,
  the 408 is only written if the response hasn't started, late handler writes get
  `http.ErrHandlerTimeout`, and `Write` re-checks `timedOut` under the mutex (the same
  re-check was added to the router's writer).

- [x] **12. OPTIONS bypasses authentication**
  `Authentication` and `AuthenticationBool` skipped auth for OPTIONS, unlike the
  provider-based variants.
  **Fixed:** the bypass is removed — all methods are authenticated consistently. CORS
  preflight is handled by the router before middleware runs, so preflights never reach
  these middlewares; doc comments call out the behavior change.

- [x] **13. Spoofable client identity defaults**
  Leftmost `X-Forwarded-For` (attacker-controlled) was trusted, and any client-supplied
  `X-Trace-ID` was propagated verbatim.
  **Fixed:** `extractIPFromXForwardedFor` now uses the rightmost (nearest-proxy-appended)
  entry, with guidance in `DefaultIPConfig` docs for direct-exposure deployments; inbound
  `X-Trace-ID` values are validated (≤64 chars, `[A-Za-z0-9_-]` only) and replaced with a
  generated ID when invalid.

## Minor / polish

- [x] **14.** `wrapHandler` docstring corrected: recovery is the *outermost* middleware;
  the full chain order is now documented outermost→innermost.
- [x] **15.** `RegisterSubRouter` now appends to `r.config.SubRouters`, so
  `RegisterGenericRouteOnSubRouter` can find dynamically added sub-routers.
  *Regression test:* `TestRegisterGenericRouteOnDynamicallyAddedSubRouter`.
- [x] **16.** `NewRouter` logs a startup warning for `Origins: ["*"]` +
  `AllowCredentials: true`; disallowed-origin responses now include `Vary: Origin`.
- [x] **17.** Recovery middleware wraps the writer to track written state; after a panic
  that occurred mid-response it logs only, instead of appending a second status/body
  onto the partial response.
- [x] **18.** `IDGenerator`'s background goroutine now uses a blocking channel send —
  it parks for free when the buffer is full (no more 1 ms polling wakeups).
- [x] **19.** Request-summary logging is decoupled from trace IDs: the previously dead
  `RouterConfig.EnableTraceLogging` flag now enables the summary/status/bytes capture
  even when `TraceIDBufferSize` is 0.
- [x] **20.** `MiddlewareChain.Append` copies into a fresh backing array, so multiple
  Appends on the same parent chain can no longer clobber each other.
  *Regression test:* `TestMiddlewareChainAppendDoesNotAliasParent`.

---

## Behavior changes to call out in release notes

- OPTIONS requests are no longer exempt from `middleware.Authentication` /
  `middleware.AuthenticationBool` (#12).
- `X-Forwarded-For` extraction uses the rightmost entry instead of the leftmost (#13).
- Rate limits now honor limit/window exactly; previously sub-1/sec limits were inflated
  to 1/sec and over-limit requests were held before rejection (#4).
- The global latency histogram was renamed `request_latency_seconds_total` →
  `all_request_latency_seconds`, and error counters' `status_code` tag now holds the
  numeric code (#8).
- `EnableTraceLogging: true` now produces request-summary logs even without trace IDs (#19).
- `DecodeBase62` treats leading `'0'` characters as leading zero bytes (#10).
