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
cd examples/simple && go build
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

SRouter is a high-performance HTTP router framework built on `julienschmidt/httprouter` with Go generics support (requires Go 1.24.0+). The codebase follows a layered architecture with clear separation of concerns.

### Core Type Parameters
Throughout the codebase, `T` represents the UserID type (must be comparable) and `U` represents the User object type. These generic parameters enable type-safe authentication and context management.

### Package Structure
- **pkg/router/**: Core routing engine with generic route registration, middleware orchestration, and request handling
- **pkg/middleware/**: Authentication providers, rate limiting, tracing, and database transaction middleware
- **pkg/codec/**: Request/response marshaling interfaces and implementations (JSON, Protocol Buffers)
- **pkg/metrics/**: Interface-based metrics system for pluggable backends
- **pkg/scontext/**: Centralized context management with SRouterContext[T,U] wrapper
- **pkg/common/**: Shared types like Middleware, RateLimitConfig

### Request Flow
1. HTTP Request → Router.ServeHTTP
2. CORS handling (if configured)
3. Client IP extraction based on IPConfig
4. Metrics and trace ID injection
5. httprouter path matching
6. Middleware chain execution (Recovery → Auth → RateLimit → Route-specific → Global → Timeout → Handler)
7. Generic handler marshaling/unmarshaling (if applicable)

### Key Design Patterns
- **Middleware Chain**: Composable middleware with configurable execution order
- **Configuration Hierarchy**: Global → SubRouter → Route with cascading overrides
- **Generic Routes**: Type-safe handlers with automatic codec-based marshaling
- **Context Wrapper**: Single SRouterContext avoids deep context nesting

### Testing Approach
The codebase maintains >90% coverage with comprehensive unit tests. Tests often use generic test helpers and mock interfaces (e.g., in router/internal/mocks/).

## Important Concepts

### Authentication Levels
Routes support three authentication levels:
- `NoAuth`: No authentication required
- `AuthOptional`: Authentication attempted but not required
- `AuthRequired`: Authentication mandatory

Authentication is typically handled by middleware that populates the context using scontext helpers.

### Rate Limiting
Flexible rate limiting with strategies:
- `StrategyIP`: Based on client IP
- `StrategyUser`: Based on authenticated user ID
- `StrategyCustom`: Custom key extraction

Uses Uber's ratelimit library with leaky bucket algorithm.

### Generic Route Registration
Generic routes should be registered using `NewGenericRouteDefinition` within SubRouterConfig.Routes for declarative configuration:
```go
router.NewGenericRouteDefinition[ReqType, RespType, UserIDType, UserType](
    router.RouteConfig[ReqType, RespType]{...}
)
```

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