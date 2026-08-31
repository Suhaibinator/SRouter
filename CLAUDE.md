# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Commands

### Build and Development
```bash
# Run all tests
go test ./...

# Run tests with verbose output
go test -v ./...

# Run tests with coverage
go test -coverprofile=coverage.out ./...
go tool cover -html=coverage.out

# Run a single test
go test -run TestName ./pkg/router

# Run tests for a specific package
go test ./pkg/router/...

# Build examples
go build ./examples/...
```

### Code Quality
```bash
# Format code
go fmt ./...

# Run Go vet
go vet ./...

# Install and run golangci-lint (if needed)
go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
golangci-lint run
```

## Architecture Overview

SRouter is a high-performance HTTP router framework built on `julienschmidt/httprouter` with Go generics support (requires Go 1.27+). The codebase follows a layered architecture with clear separation of concerns.

### Core Type Parameters
For `Router[T, U]`, middleware, and `SRouterContext[T, U]`, `T` is the
comparable user-ID type and `U` is the user object type. Typed
`RouteConfig[Req, Resp]` and codecs use separate request/response parameters.

### Package Structure
- **pkg/router/**: Core routing engine with generic route registration, middleware orchestration, and request handling
- **pkg/middleware/**: Authentication providers, rate limiting, tracing, and database transaction middleware
- **pkg/codec/**: Request/response marshaling interfaces and implementations (JSON, Protocol Buffers)
- **pkg/metrics/**: Interface-based metrics system for pluggable backends
- **pkg/scontext/**: Centralized context management with SRouterContext[T,U] wrapper
- **pkg/common/**: Shared types like Middleware, RateLimitConfig

### Request Flow
1. `Router.ServeHTTP` builds lazily if needed and registers the request for shutdown tracking.
2. CORS handling may finish preflight requests before route matching.
3. Client IP and user-agent values are stored in the SRouter context.
4. An optional request-summary wrapper captures outcomes, including unmatched routes.
5. `httprouter` matches the request.
6. Matched routes execute Recovery → Trace ID → built-in Auth → RateLimit → Global/metrics → outer groups → inner groups → Route → Timeout → body limit → Handler.
7. Typed handlers decode, sanitize, invoke, and encode inside the final handler stage.

### Key Design Patterns
- **Middleware Chain**: Composable middleware with configurable execution order
- **Route Tree**: `Router.Group` creates recursive `RouteGroup` handles; policy inherits Global → outer groups → inner groups → Route
- **Generic Routes**: Type-safe handlers with automatic codec-based marshaling
- **Context Wrapper**: Single SRouterContext avoids deep context nesting

### Testing Approach
CI enforces at least 80% aggregate coverage across the library packages. Tests often use generic test helpers and mock interfaces (e.g., in pkg/router/internal/mocks/).

## Important Concepts

### Authentication Levels
Routes support three authentication levels:
- `NoAuth`: No authentication required
- `AuthOptional`: Authentication attempted but not required
- `AuthRequired`: Authentication mandatory

The router's built-in authentication stage uses the callbacks supplied to
`NewRouter` and populates the context before configured rate limiting. Those
callbacks are required only when an effective route auth level is not
`NoAuth`. Custom global or group authentication middleware runs after the
configured rate limiter.

### Rate Limiting
Flexible rate limiting with strategies:
- `StrategyIP`: Based on client IP
- `StrategyUser`: Based on authenticated user ID
- `StrategyCustom`: Custom key extraction

The built-in limiter is an in-memory, nonblocking sliding-window counter. It
keeps separate state by bucket, resolved client key, limit, and window, and
lazily evicts stale entries when a later new key triggers a sweep.

### Generic Route Registration
Generic `RouteConfig` values can be registered directly on a `Router` or `RouteGroup`:
```go
router.RouteConfig[ReqType, RespType]{...}
```

Call `r.Route(route)` for root routes or retain a group handle and call
`group.Route(route)`. Both methods accept standard and typed routes. Route trees
freeze at `Build` or the first request.

### Context Access
Always use scontext package helpers for type-safe context access:
```go
userID, ok := scontext.GetUserIDFromRequest[T, U](r)
user, ok := scontext.GetUserFromRequest[T, U](r)  // Returns *U
traceID := scontext.GetTraceIDFromRequest[T, U](r)
handlerErr, ok := scontext.GetHandlerErrorFromRequest[T, U](r)  // For generic routes
```

### Handler Error Context
Generic routes automatically store handler errors in the request context, allowing middleware to access them after handler execution. This is useful for:
- Transaction rollback decisions
- Custom error logging
- Circuit breaker patterns
- Error metrics collection

### Trace ID Generation
Enable trace ID generation by setting `TraceIDBufferSize > 0` in RouterConfig. This creates a background ID generator for efficient UUID generation and automatic request correlation.

## Documentation Maintenance

- Keep detailed behavior in the focused `docs/` guide and link to it from the
  README instead of copying API references into both places.
- Update documentation and runnable examples in the same change as public API,
  default, middleware-order, dependency, or algorithm changes.
- Run `go test ./internal/doccheck` for local Markdown links and
  `go build ./examples/...` for example packages.
- Use `go run .` in example instructions so multi-file programs work.
