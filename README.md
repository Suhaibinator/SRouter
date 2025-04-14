# SRouter

SRouter is a high-performance HTTP router for Go that wraps [julienschmidt/httprouter](https://github.com/julienschmidt/httprouter) with advanced features including sub-router overrides, middleware support, generic-based marshaling/unmarshaling, configurable timeouts, body size limits, authentication levels, a flexible metrics system, and intelligent logging.

[![Go Report Card](https://goreportcard.com/badge/github.com/Suhaibinator/SRouter)](https://goreportcard.com/report/github.com/Suhaibinator/SRouter)
[![GoDoc](https://godoc.org/github.com/Suhaibinator/SRouter?status.svg)](https://godoc.org/github.com/Suhaibinator/SRouter)
[![Tests](https://github.com/Suhaibinator/SRouter/actions/workflows/tests.yml/badge.svg)](https://github.com/Suhaibinator/SRouter/actions/workflows/tests.yml)
[![codecov](https://codecov.io/gh/Suhaibinator/SRouter/graph/badge.svg?token=NNIYO5HKX7)](https://codecov.io/gh/Suhaibinator/SRouter)

## Features

- **High Performance**: Built on top of julienschmidt/httprouter for blazing-fast O(1) path matching
- **Comprehensive Test Coverage**: Maintained at over 90% code coverage to ensure reliability
- **Sub-Router Overrides**: Configure timeouts, body size limits, and rate limits at the global, sub-router, or route level
- **Middleware Support**: Apply middleware at the global, sub-router, or route level with proper chaining
- **Generic-Based Marshaling/Unmarshaling**: Use Go 1.18+ generics for type-safe request and response handling
- **Configurable Timeouts**: Set timeouts at the global, sub-router, or route level with cascading defaults
- **Body Size Limits**: Configure maximum request body size at different levels to prevent DoS attacks
- **Rate Limiting**: Flexible rate limiting with support for IP-based, user-based, and custom strategies
- **Path Parameters**: Easy access to path parameters via request context
- **Graceful Shutdown**: Properly handle in-flight requests during shutdown
- **Flexible Metrics System**: Support for multiple metric formats, custom collectors, and dependency injection
- **Intelligent Logging**: Structured logging using `zap` with appropriate log levels for different types of events. Requires a logger instance in config.
- **Trace ID Logging**: Automatically generate and include a unique trace ID for each request in context and log entries.
- **Flexible Request Data Sources**: Support for retrieving request data from various sources including request body, query parameters, and path parameters with automatic decoding
- **Nested SubRouters**: Create hierarchical routing structures.
- **Declarative Generic Routes**: Define generic routes directly within `SubRouterConfig` for improved clarity.

## Installation

```bash
go get github.com/Suhaibinator/SRouter
```

## Requirements

- Go 1.24.0 or higher
- [julienschmidt/httprouter](https://github.com/julienschmidt/httprouter) v1.3.0 or higher for high-performance routing
- [go.uber.org/zap](https://github.com/uber-go/zap) v1.27.0 or higher for structured logging
- [github.com/google/uuid](https://github.com/google/uuid) v1.6.0 or higher for trace ID generation
- [go.uber.org/ratelimit](https://github.com/uber-go/ratelimit) v0.3.1 or higher for rate limiting (optional)
- Metrics dependencies (e.g., [github.com/prometheus/client_golang](https://github.com/prometheus/client_golang)) if using metrics.

All dependencies are properly documented with Go modules and will be automatically installed when you run `go get github.com/Suhaibinator/SRouter`.

## Getting Started

### Basic Usage

Here's a simple example of how to use SRouter:

```go
package main

import (
	"context" // Added context import
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/router"
	"github.com/Suhaibinator/SRouter/pkg/common" // Import common types
	"github.com/Suhaibinator/SRouter/pkg/middleware" // Import middleware package
	"go.uber.org/zap"
)

func main() {
	// Create a logger (Must provide one)
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	// Create a router configuration
	routerConfig := router.RouterConfig{
		Logger:            logger, // Required
		GlobalTimeout:     2 * time.Second,
		GlobalMaxBodySize: 1 << 20, // 1 MB
		EnableTraceID:     true,    // Enable trace ID logging (recommended)
		Middlewares: []common.Middleware{
		 // Add logging middleware if desired (uses the configured logger)
		 middleware.Logging(logger, false),
		},
	}

	// Define a simple auth function (replace with your actual logic or use auth middleware)
	authFunction := func(ctx context.Context, token string) (string, bool) {
		if token == "valid-token" {
			return "user-id-from-token", true
		}
		return "", false
	}

	// Define a function to extract a comparable UserID from the User object
	userIdFromUserFunction := func(user string) string {
		return user
	}

	// Create a router.
	r := router.NewRouter[string, string](routerConfig, authFunction, userIdFromUserFunction)

	// Register a simple route
	r.RegisterRoute(router.RouteConfigBase{
		Path:    "/hello",
		Methods: []router.HttpMethod{router.MethodGet},
		Handler: func(w http.ResponseWriter, r *http.Request) {
			// Access trace ID if needed
			traceID := scontext.GetTraceIDFromRequest[string, string](r)
			logger.Info("Handling /hello", zap.String("trace_id", traceID))

			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"message":"Hello, World!"}`))
		},
	})

	// Start the server
	fmt.Println("Server listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", r))
}
```

### Using Sub-Routers

Sub-routers allow you to group routes with a common path prefix and apply shared configuration. You can define both standard (`RouteConfigBase`) and generic routes declaratively within the `Routes` field, which now accepts `[]any`.

```go
// Define sub-router configurations
apiV1SubRouter := router.SubRouterConfig{
	PathPrefix:          "/api/v1",
	TimeoutOverride:     3 * time.Second,
	MaxBodySizeOverride: 2 << 20, // 2 MB
	Routes: []any{ // Use []any to hold different route types
		// Standard route
		router.RouteConfigBase{
			Path:    "/users", // Becomes /api/v1/users
			Methods: []router.HttpMethod{router.MethodGet},
			Handler: ListUsersHandler,
		},
		router.RouteConfigBase{
			Path:    "/users/:id", // Becomes /api/v1/users/:id
			Methods: []router.HttpMethod{router.MethodGet},
			Handler: GetUserHandler,
		},
		// Declarative generic route using the helper
		router.NewGenericRouteDefinition[CreateUserReq, CreateUserResp, string, string](
			router.RouteConfig[CreateUserReq, CreateUserResp]{
				Path:      "/users", // Path relative to the sub-router prefix (/api/v1/users)
				Methods:   []router.HttpMethod{router.MethodPost},
				AuthLevel: router.Ptr(router.AuthRequired),
				Codec:     codec.NewJSONCodec[CreateUserReq, CreateUserResp](),
				Handler:   CreateUserHandler,
				// Middlewares, Timeout, MaxBodySize, RateLimit can be set here too
			},
		),
	},
}

apiV2SubRouter := router.SubRouterConfig{
	PathPrefix: "/api/v2",
	Routes: []any{ // Use []any
		router.RouteConfigBase{
			Path:    "/users", // Becomes /api/v2/users
			Methods: []router.HttpMethod{router.MethodGet},
			Handler: ListUsersV2Handler,
		},
	},
}

// Create a router with sub-routers
routerConfig := router.RouterConfig{
	Logger:            logger, // Required
	GlobalTimeout:     2 * time.Second,
	GlobalMaxBodySize: 1 << 20, // 1 MB
	SubRouters: []router.SubRouterConfig{apiV1SubRouter, apiV2SubRouter},
}

r := router.NewRouter[string, string](routerConfig, authFunction, userIdFromUserFunction)

// Generic routes are now defined declaratively within SubRouterConfig.Routes
```

#### Nested SubRouters

You can nest `SubRouterConfig` structs within each other to create a hierarchical routing structure. Path prefixes are combined, and configuration overrides cascade down the hierarchy.

```go
// Define nested structure
usersV1SubRouter := router.SubRouterConfig{
	PathPrefix: "/users", // Relative to /api/v1
	Routes: []any{
		router.RouteConfigBase{ Path: "/:id", Methods: []router.HttpMethod{router.MethodGet}, Handler: GetUserHandler },
		router.NewGenericRouteDefinition[UserReq, UserResp, string, string](
			router.RouteConfig[UserReq, UserResp]{ Path: "/info", Methods: []router.HttpMethod{router.MethodPost}, Codec: userCodec, Handler: UserInfoHandler },
		),
	},
}
apiV1SubRouter := router.SubRouterConfig{
	PathPrefix: "/v1", // Relative to /api
	SubRouters: []router.SubRouterConfig{usersV1SubRouter},
	Routes: []any{
		router.RouteConfigBase{ Path: "/status", Methods: []router.HttpMethod{router.MethodGet}, Handler: V1StatusHandler },
	},
}
apiSubRouter := router.SubRouterConfig{
	PathPrefix: "/api", // Root prefix for this group
	SubRouters: []router.SubRouterConfig{apiV1SubRouter},
	Routes: []any{
		router.RouteConfigBase{ Path: "/health", Methods: []router.HttpMethod{router.MethodGet}, Handler: HealthHandler },
	},
}

// Register top-level sub-router during NewRouter
routerConfig := router.RouterConfig{ Logger: logger, SubRouters: []router.SubRouterConfig{apiSubRouter}, ... }
r := router.NewRouter[string, string](routerConfig, authFunction, userIdFromUserFunction)

// Routes are now registered:
// GET /api/health
// GET /api/v1/status
// GET /api/v1/users/:id
// POST /api/v1/users/info
```

### Using Generic Routes

SRouter supports generic routes for type-safe request and response handling:

```go
// Define request and response types
type CreateUserReq struct {
 Name  string `json:"name"`
 Email string `json:"email"`
}

type CreateUserResp struct {
 ID    string `json:"id"`
 Name  string `json:"name"`
 Email string `json:"email"`
}

// Define a generic handler
func CreateUserHandler(r *http.Request, req CreateUserReq) (CreateUserResp, error) {
 // In a real application, you would create a user in a database
 return CreateUserResp{
  ID:    "123",
  Name:  req.Name,
  Email: req.Email,
 }, nil
}

// Register the generic route directly on the router (not part of a sub-router)
// Note the extra arguments for effective settings (usually 0/nil for direct registration)
router.RegisterGenericRoute[CreateUserReq, CreateUserResp, string, string](r, router.RouteConfig[CreateUserReq, CreateUserResp]{
 Path:        "/standalone/users",
 Methods:     []router.HttpMethod{router.MethodPost},
 AuthLevel:   router.Ptr(router.AuthRequired), // Use Ptr helper
 Codec:       codec.NewJSONCodec[CreateUserReq, CreateUserResp](),
 Handler:     CreateUserHandler,
}, time.Duration(0), int64(0), nil) // Pass zero/nil for effective settings
```

Note that the `RegisterGenericRoute` function takes five type parameters: the request type, the response type, the user ID type, and the user object type. The last two should match the type parameters of your router. It also requires effective timeout, max body size, and rate limit values, which are typically zero/nil when registering directly on the root router. Use `NewGenericRouteDefinition` within `SubRouterConfig.Routes` for routes belonging to sub-routers.

### Using Path Parameters

SRouter makes it easy to access path parameters:

```go
func GetUserHandler(w http.ResponseWriter, r *http.Request) {
 // Get the user ID from the path parameters
 id := router.GetParam(r, "id")

 // Use the ID to fetch the user
 // user := fetchUser(id)

 // Return the user as JSON
 w.Header().Set("Content-Type", "application/json")
 // json.NewEncoder(w).Encode(user)
 fmt.Fprintf(w, `{"id": "%s"}`, id) // Example response
}
```

### Trace ID Logging

SRouter provides built-in support for trace ID logging, which allows you to correlate log entries across different parts of your application for a single request. Each request is assigned a unique trace ID (UUID) that is automatically included in all log entries related to that request if logging middleware is used.

#### Enabling Trace ID Logging

Trace ID generation and injection into the context can be enabled in one of two ways (using both is redundant):

1. **Via `RouterConfig` (Recommended):** Set `EnableTraceID: true`.

```go
// Create a router with trace ID logging enabled
routerConfig := router.RouterConfig{
    Logger:        logger, // Required
    EnableTraceID: true,   // Enable trace ID generation and context injection
    // Other configuration...
}
r := router.NewRouter[string, string](routerConfig, authFunction, userIdFromUserFunction)
```

2. **Via `TraceMiddleware` (Explicit):** Add `middleware.TraceMiddleware()` to your global middleware chain (usually first).

```go
// Create a router with trace middleware
routerConfig := router.RouterConfig{
    Logger: logger, // Required
    Middlewares: []common.Middleware{
        middleware.TraceMiddleware(), // Add this as the first middleware
        middleware.Logging(logger, false), // Logging middleware will pick up the trace ID
        // Other middleware...
    },
    // Other configuration...
}
r := router.NewRouter[string, string](routerConfig, authFunction, userIdFromUserFunction)
```

#### Accessing the Trace ID

You can access the trace ID in your handlers and middleware using helpers from `pkg/middleware`:

```go
func myHandler(w http.ResponseWriter, r *http.Request) {
    // Get the trace ID
    traceID := scontext.GetTraceIDFromRequest[string, string](r)

    // Use the trace ID in logs or downstream requests
    logger.Info("Processing request", zap.String("trace_id", traceID))
    fmt.Printf("Processing request with trace ID: %s\n", traceID)

    // ...
}
```

#### Propagating the Trace ID to Downstream Services

If your application calls other services, you should propagate the trace ID to maintain the request trace across service boundaries:

```go
func callDownstreamService(r *http.Request) {
    // Get the trace ID
    traceID := scontext.GetTraceIDFromRequest[string, string](r)

    // Create a new request to the downstream service
    req, _ := http.NewRequestWithContext(r.Context(), "GET", "https://api.example.com/data", nil)

    // Add the trace ID to the request headers (e.g., X-Trace-ID)
    if traceID != "" {
        req.Header.Set("X-Trace-ID", traceID)
    }

    // Make the request
    client := &http.Client{}
    resp, err := client.Do(req)

    // ...
}
```

#### Using the Trace ID in Context-Based Functions

For functions that receive a context but not an HTTP request, you can extract the trace ID from the context:

```go
func processData(ctx context.Context) {
    // Get the trace ID from the context
    traceID := scontext.GetTraceIDFromContext[string, string](ctx)

    // Use the trace ID
    log.Printf("[trace_id=%s] Processing data\n", traceID)

    // ...
}
```

See the `examples/trace-logging` directory for a complete example of trace ID logging.

### Graceful Shutdown

SRouter provides a `Shutdown` method for graceful shutdown:

```go
// Assume 'r' is your configured SRouter instance
// Create a server
srv := &http.Server{
 Addr:    ":8080",
 Handler: r,
}

// Start the server in a goroutine
go func() {
 if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
  log.Fatalf("listen: %s\n", err)
 }
}()

// Wait for interrupt signal (Ctrl+C or SIGTERM)
quit := make(chan os.Signal, 1)
signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
<-quit
log.Println("Shutting down server...")

// Create a deadline to wait for
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

// Shut down the router first (signals internal components like rate limiter)
if err := r.Shutdown(ctx); err != nil {
 log.Printf("SRouter shutdown failed: %v\n", err)
}

// Shut down the HTTP server (stops accepting new connections, waits for active requests)
if err := srv.Shutdown(ctx); err != nil {
 log.Fatalf("Server shutdown failed: %v", err)
}
log.Println("Server exited gracefully")
```

See the `examples/graceful-shutdown` directory for a complete example of graceful shutdown.

## Advanced Usage

### IP Configuration

SRouter provides a flexible way to extract client IP addresses using `router.IPConfig` (defined in `pkg/router/ip.go`), which is particularly important when your application is behind a reverse proxy or load balancer. The IP configuration allows you to specify where to extract the client IP from and whether to trust proxy headers. The extracted IP is added to the request context via the internal `ClientIPMiddleware`.

```go
// Configure IP extraction to use X-Forwarded-For header
routerConfig := router.RouterConfig{
    // ... other config
    IPConfig: &router.IPConfig{ // Use router.IPConfig
        Source:     router.IPSourceXForwardedFor, // Use router.IPSourceXForwardedFor
        TrustProxy: true, // Only if your proxy reliably sets this header
    },
}
```

#### IP Source Types

SRouter supports several IP source types (defined as `router.IPSourceType`):

1. **`router.IPSourceRemoteAddr`**: Uses the request's RemoteAddr field (default if no source is specified)
2. **`router.IPSourceXForwardedFor`**: Uses the X-Forwarded-For header (common for most reverse proxies)
3. **`router.IPSourceXRealIP`**: Uses the X-Real-IP header (used by Nginx and some other proxies)
4. **`router.IPSourceCustomHeader`**: Uses a custom header specified in the configuration

```go
// Configure IP extraction to use a custom header
routerConfig := router.RouterConfig{
    // ... other config
    IPConfig: &router.IPConfig{ // Use router.IPConfig
        Source:       router.IPSourceCustomHeader, // Use router.IPSourceCustomHeader
        CustomHeader: "CF-Connecting-IP", // Example: Cloudflare header
        TrustProxy:   true,
    },
}
```

#### Trust Proxy Setting

The `TrustProxy` setting determines whether to trust proxy headers:

- If `true`, the specified source will be used to extract the client IP.
- If `false` or if the specified source doesn't contain an IP, the request's RemoteAddr will be used as a fallback.

This is important for security, as malicious clients could potentially spoof headers if your application blindly trusts them.

See the `examples/middleware` directory for examples of using IP configuration.

### Rate Limiting

SRouter provides a flexible rate limiting system using `common.RateLimitConfig` (defined in `pkg/common/types.go`) that can be configured at the global, sub-router, or route level. Rate limits can be based on IP address, authenticated user, or custom criteria. Under the hood, SRouter uses [Uber's ratelimit library](https://github.com/uber-go/ratelimit) via the `middleware.RateLimit` function for efficient and smooth rate limiting with a leaky bucket algorithm.

#### Rate Limiting Configuration

```go
// Create a router with global rate limiting
routerConfig := router.RouterConfig{
    // ... other config
    GlobalRateLimit: &common.RateLimitConfig[any, any]{ // Use common.RateLimitConfig
        BucketName: "global_ip_limit",
        Limit:      100, // requests
        Window:     time.Minute, // per minute
        Strategy:   common.StrategyIP, // Use common constants
    },
}

// Create a sub-router with rate limiting override
subRouter := router.SubRouterConfig{
    PathPrefix: "/api/v1",
    RateLimitOverride: &common.RateLimitConfig[any, any]{ // Use common.RateLimitConfig
        BucketName: "api_v1_user_limit",
        Limit:      50,
        Window:     time.Hour,
        Strategy:   common.StrategyUser, // Requires auth middleware first
    },
    // ... other config
}

// Create a route with rate limiting
route := router.RouteConfig[MyReq, MyResp]{ // Use specific types for route config
    Path:    "/users",
    Methods: []router.HttpMethod{router.MethodPost},
    RateLimit: &common.RateLimitConfig[any, any]{ // Use common.RateLimitConfig
        BucketName: "create_user_ip_limit",
        Limit:      10,
        Window:     time.Minute,
        Strategy:   common.StrategyIP,
    },
    // ... other config
}
```

#### Rate Limiting Strategies

SRouter supports several rate limiting strategies using constants defined in the `common` package (`common.RateLimitStrategy`):

1. **IP-based Rate Limiting (`common.StrategyIP`)**: Limits requests based on the client's IP address (extracted according to the IP configuration). The rate limiting is smooth, distributing requests evenly across the time window rather than allowing bursts.

```go
RateLimit: &common.RateLimitConfig[any, any]{
    BucketName: "ip-based",
    Limit:      100,
    Window:     time.Minute,
    Strategy:   common.StrategyIP,
}
```

2. **User-based Rate Limiting (`common.StrategyUser`)**: Limits requests based on the authenticated user ID (requires authentication middleware to run first and populate the user ID in the context). Requires `UserIDToString` function in config.

```go
RateLimit: &common.RateLimitConfig[string, MyUser]{ // Example with specific types
    BucketName:     "user-based",
    Limit:          50,
    Window:         time.Minute,
    Strategy:       common.StrategyUser,
    UserIDToString: func(id string) string { return id }, // Required
    // UserIDFromUser: func(u MyUser) string { return u.ID }, // Optional if user object is needed
}
```

3. **Custom Rate Limiting (`common.StrategyCustom`)**: Limits requests based on custom criteria defined by a `KeyExtractor` function.

```go
RateLimit: &common.RateLimitConfig[any, any]{
    BucketName: "custom_api_key",
    Limit:      20,
    Window:     time.Minute,
    Strategy:   common.StrategyCustom,
    KeyExtractor: func(r *http.Request) (string, error) {
        // Extract API key from header
        apiKey := r.Header.Get("X-API-Key")
        if apiKey == "" {
            // Fall back to IP if no API key is provided
            // Note: Adjust T, U types based on your router's configuration
            ip, _ := scontext.GetClientIPFromRequest[string, any](r)
            return "ip:"+ip, nil // Prefix to avoid collisions
        }
        return "key:"+apiKey, nil // Prefix to avoid collisions
    },
}
```

#### Shared Rate Limit Buckets

You can share rate limit buckets between different endpoints by using the same bucket name:

```go
// Login endpoint
loginRoute := router.RouteConfigBase{
    Path:    "/login",
    Methods: []router.HttpMethod{router.MethodPost},
    RateLimit: &common.RateLimitConfig[any, any]{ // Use common.RateLimitConfig
        BucketName: "auth_ip_limit", // Shared bucket name
        Limit:      5,
        Window:     time.Minute,
        Strategy:   common.StrategyIP,
    },
    // ... other config
}

// Register endpoint
registerRoute := router.RouteConfigBase{
    Path:    "/register",
    Methods: []router.HttpMethod{router.MethodPost},
    RateLimit: &common.RateLimitConfig[any, any]{ // Use common.RateLimitConfig
        BucketName: "auth_ip_limit", // Same bucket name as login
        Limit:      5, // Limit applies to combined requests for this bucket
        Window:     time.Minute,
        Strategy:   common.StrategyIP,
    },
    // ... other config
}
```

#### Custom Rate Limit Responses

You can customize the response sent when a rate limit is exceeded:

```go
RateLimit: &common.RateLimitConfig[any, any]{ // Use common.RateLimitConfig
    BucketName: "custom_response_limit",
    Limit:      10,
    Window:     time.Minute,
    Strategy:   common.StrategyIP,
    ExceededHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        w.Header().Set("X-Retry-After", "60") // Example custom header
        w.WriteHeader(http.StatusTooManyRequests) // 429
        w.Write([]byte(`{"error":"Rate limit exceeded","message":"Please try again later"}`))
    }),
}
```

See the `examples/rate-limiting` directory for a complete example of rate limiting.

### Authentication

SRouter provides a flexible authentication system with three authentication levels. Authentication is typically handled via **middleware**, which should run before handlers that require authentication.

#### Authentication Levels

SRouter supports three authentication levels, specified in `RouteConfig` or `RouteConfigBase`:

1. **NoAuth**: No authentication is required.
2. **AuthOptional**: Authentication is attempted (e.g., by middleware). If successful, user info is added to the context. The request proceeds regardless.
3. **AuthRequired**: Authentication is required (e.g., by middleware). If authentication fails, the middleware should reject the request (e.g., with 401 Unauthorized). If successful, user info is added to the context.

```go
// Example route configurations
routePublic := router.RouteConfigBase{ AuthLevel: router.Ptr(router.NoAuth), ... }
routeOptional := router.RouteConfigBase{ AuthLevel: router.Ptr(router.AuthOptional), ... }
routeProtected := router.RouteConfigBase{ AuthLevel: router.Ptr(router.AuthRequired), ... }
```

#### Authentication Middleware

You need to provide your own authentication middleware or use pre-built ones from the `pkg/middleware` package. The middleware is responsible for:

1.  Extracting credentials (e.g., from headers).
2.  Validating credentials.
3.  If validation succeeds:
    *   Populating the request context with the user ID and optionally the user object using `scontext.WithUserID` and `scontext.WithUser`.
    *   Calling the next handler.
4.  If validation fails:
    *   For `AuthRequired` routes, rejecting the request (e.g., `http.Error(w, "Unauthorized", http.StatusUnauthorized)`).
    *   For `AuthOptional` routes, calling the next handler without populating user info.

**Important:** The `authFunction` and `userIdFromUserFunction` parameters passed to `NewRouter` are primarily used by the router's built-in `authRequiredMiddleware` and `authOptionalMiddleware`. While functional, relying solely on these for complex authentication scenarios is discouraged. **Using dedicated authentication middleware (like those in `pkg/middleware` or custom ones) applied globally or per-route is the recommended approach for better separation of concerns and flexibility.**

```go
// Example using a custom auth middleware
func MyAuthMiddleware(authService MyAuthService) common.Middleware {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            token := r.Header.Get("Authorization")
            token = strings.TrimPrefix(token, "Bearer ")

            user, userID, err := authService.ValidateToken(r.Context(), token)

            if err == nil { // Authentication successful
                ctx := scontext.WithUserID[string, MyUserType](r.Context(), userID) // Use actual types
                ctx = scontext.WithUser[string, MyUserType](ctx, user) // Use scontext.WithUser with both type params
                next.ServeHTTP(w, r.WithContext(ctx))
            } else { // Authentication failed
                // Check route's AuthLevel (This requires accessing route config, which middleware typically doesn't do directly)
                // A simpler approach is to apply this middleware only to routes where it's needed.
                // Or, the router's internal auth middleware (if used) handles the AuthLevel check.
                // If implementing purely custom middleware, you might apply it conditionally or handle levels internally.
                // For this example, assume it's applied only where required or optional handling is needed.
                // If AuthRequired was intended, reject here:
                // http.Error(w, "Unauthorized", http.StatusUnauthorized)
                // return
                // If AuthOptional, just proceed without user context:
                next.ServeHTTP(w, r)
            }
        })
    }
}

// Apply middleware globally or per-route/sub-router
routerConfig := router.RouterConfig{
    Logger: logger, // Required
    Middlewares: []common.Middleware{ MyAuthMiddleware(myService) },
    // ...
}
r := router.NewRouter[string, MyUserType](routerConfig, ...) // Match types
```

See the `examples/auth-levels`, `examples/user-auth`, and `examples/auth` directories for examples.

### Context Management

SRouter uses a structured approach to context management using the `SRouterContext` wrapper located in the `pkg/scontext` package. This approach avoids deep nesting of context values by storing all values in a single wrapper structure.

#### SRouterContext Wrapper

The `SRouterContext` wrapper is a generic type that holds all values that SRouter adds to request contexts:

```go
// Located in pkg/scontext/context.go
package scontext

type SRouterContext[T comparable, U any] struct {
    // User ID and User object storage
    UserID T
    User   *U // Pointer to user object

    // Client IP address
    ClientIP string

    // Trace ID
    TraceID string

    // Database transaction (using an interface for abstraction)
    Transaction DatabaseTransaction // See pkg/scontext/db.go

    // Track which fields are set
    UserIDSet      bool
    UserSet        bool
    ClientIPSet    bool
    TraceIDSet     bool
    TransactionSet bool

    // Additional flags (flexible storage)
    Flags map[string]any
}

// DatabaseTransaction interface (pkg/scontext/db.go)
type DatabaseTransaction interface {
    Commit() error
    Rollback() error
    SavePoint(name string) error
    RollbackTo(name string) error
    GetDB() *gorm.DB // Returns underlying *gorm.DB
}

// GormTransactionWrapper (pkg/middleware/db.go)
// Wraps *gorm.DB to implement DatabaseTransaction
type GormTransactionWrapper struct { DB *gorm.DB }
func NewGormTransactionWrapper(tx *gorm.DB) *GormTransactionWrapper { /* ... */ }
```

The type parameters `T` and `U` represent the user ID type and user object type, respectively. The `DatabaseTransaction` interface and `GormTransactionWrapper` facilitate testable transaction management (see `pkg/middleware/db.go` and `docs/context-management.md` for details on usage). Note that `GormTransactionWrapper` remains in `pkg/middleware` as it's a specific implementation detail related to GORM, while the core context logic is in `pkg/scontext`.

#### Benefits of SRouterContext

The SRouterContext approach offers several advantages:

1. **Reduced Context Nesting**: Instead of wrapping contexts multiple times like:
   ```go
   ctx = context.WithValue(context.WithValue(r.Context(), userIDKey, userID), userKey, user)
   ```
   SRouter uses a single context wrapper, updated via helper functions:
   ```go
   ctx := scontext.WithUserID[T, U](r.Context(), userID)
   ctx = scontext.WithUser[T](ctx, user)
   ```

2. **Type Safety**: The generic type parameters ensure proper type handling without the need for type assertions.

3. **Extensibility**: New fields can be added to the `SRouterContext` struct without creating more nested contexts. The `Flags` map allows custom middleware to add values.

4. **Organization**: Related context values are grouped logically, making the code more maintainable.

#### Accessing Context Values

SRouter provides several helper functions in the `pkg/middleware` package for accessing context values:

```go
// Get the user ID from the request
userID, ok := scontext.GetUserIDFromRequest[string, User](r) // Replace T, U

// Get the user object (pointer) from the request
user, ok := scontext.GetUserFromRequest[string, User](r) // Replace T, U

// Get the client IP from the request
ip, ok := scontext.GetClientIPFromRequest[string, User](r) // Replace T, U

// Get a flag from the request
flagValue, ok := scontext.GetFlagFromRequest[string, User](r, "flagName") // Replace T, U

// Get the trace ID from the request
traceID := scontext.GetTraceIDFromRequest[string, string](r) // Use with appropriate types

// Get trace ID from context directly
traceIDFromCtx := scontext.GetTraceIDFromContext[string, string](ctx)

// Get the database transaction interface from the request
txInterface, ok := scontext.GetTransactionFromRequest[T, U](r) // Replace T, U
if ok {
    // Use txInterface.Commit(), txInterface.Rollback(), or txInterface.GetDB()
}
```

These functions automatically handle the type parameters and provide a clean, consistent interface for accessing context values. **Note:** The user object returned by `scontext.GetUserFromRequest` is a pointer (`*U`). For database transactions, you retrieve the `DatabaseTransaction` interface and can use `GetDB()` to access the underlying `*gorm.DB` for operations. See `docs/context-management.md` for detailed usage.

### Custom Error Handling

You can create custom HTTP errors with specific status codes and messages:

```go
// Create a custom HTTP error
func NotFoundError(resourceType, id string) *router.HTTPError {
 return router.NewHTTPError(
  http.StatusNotFound,
  fmt.Sprintf("%s with ID %s not found", resourceType, id),
 )
}

// Use the custom error in a handler
func GetUserHandler(r *http.Request, req GetUserReq) (GetUserResp, error) {
 // Get the user ID from the request
 id := req.ID

 // Try to find the user
 // user, found := findUser(id)
 found := false // Example
 if !found {
  // Return a custom error
  return GetUserResp{}, NotFoundError("User", id)
 }

 // Return the user
 // return GetUserResp{ ID: user.ID, Name: user.Name, Email: user.Email, }, nil
 return GetUserResp{}, nil // Placeholder
}
```

### Custom Middleware

You can create custom middleware to add functionality to your routes:

```go
// Create a custom middleware that adds a request ID to the context
func RequestIDMiddleware() common.Middleware {
 return func(next http.Handler) http.Handler {
  return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
   // Generate a request ID
   requestID := uuid.New().String()

   // Add it to the context using the SRouterContext flags
   // Replace string, string with your router's T, U types
   ctx := scontext.WithFlag[string, string](r.Context(), "request_id", requestID)

   // Add it to the response headers
   w.Header().Set("X-Request-ID", requestID)

   // Call the next handler with the updated request
   next.ServeHTTP(w, r.WithContext(ctx))
  })
 }
}

// Apply the middleware to the router
routerConfig := router.RouterConfig{
 Logger: logger, // Required
 // ...
 Middlewares: []common.Middleware{
  RequestIDMiddleware(),
  middleware.Logging(logger, false), // Example logging middleware
 },
 // ...
}
```

See the `examples/middleware` directory for examples of custom middleware.

### Source Types

SRouter provides flexible ways to retrieve and decode request data beyond just the request body. This is particularly useful for scenarios where you need to pass data in URLs or when working with clients that have limitations on request body usage.

#### Available Source Types

SRouter supports the following source types (defined as constants in the `router` package):

1. **Body** (default): Retrieves data from the request body.

   ```go
   router.NewGenericRouteDefinition[UserRequest, UserResponse, string, string](
       router.RouteConfig[UserRequest, UserResponse]{
           // ...
           // SourceType defaults to Body if not specified
       },
   )
   ```

2. **Base64QueryParameter**: Retrieves data from a base64-encoded query parameter.

   ```go
   router.NewGenericRouteDefinition[UserRequest, UserResponse, string, string](
       router.RouteConfig[UserRequest, UserResponse]{
           // ...
           SourceType: router.Base64QueryParameter,
           SourceKey:  "data", // Will look for ?data=base64encodedstring
       },
   )
   ```

3. **Base62QueryParameter**: Retrieves data from a base62-encoded query parameter.

   ```go
   router.NewGenericRouteDefinition[UserRequest, UserResponse, string, string](
       router.RouteConfig[UserRequest, UserResponse]{
           // ...
           SourceType: router.Base62QueryParameter,
           SourceKey:  "data", // Will look for ?data=base62encodedstring
       },
   )
   ```

4. **Base64PathParameter**: Retrieves data from a base64-encoded path parameter.

   ```go
   router.NewGenericRouteDefinition[UserRequest, UserResponse, string, string](
       router.RouteConfig[UserRequest, UserResponse]{
           Path:       "/users/:data",
           // ...
           SourceType: router.Base64PathParameter,
           SourceKey:  "data", // Will use the :data path parameter
       },
   )
   ```

5. **Base62PathParameter**: Retrieves data from a base62-encoded path parameter.

   ```go
   router.NewGenericRouteDefinition[UserRequest, UserResponse, string, string](
       router.RouteConfig[UserRequest, UserResponse]{
           Path:       "/users/:data",
           // ...
           SourceType: router.Base62PathParameter,
           SourceKey:  "data", // Will use the :data path parameter
       },
   )
   ```

See the `examples/source-types` directory for a complete example.

### Custom Codec

You can create custom codecs for different data formats:

```go
// Create a custom XML codec
type XMLCodec[T any, U any] struct{}

// NewRequest creates a zero-value instance of the request type T.
func (c *XMLCodec[T, U]) NewRequest() T {
	var data T
	return data
}

// Decode reads from the request body and unmarshals XML.
func (c *XMLCodec[T, U]) Decode(r *http.Request) (T, error) {
 var data T
 body, err := io.ReadAll(r.Body)
 if err != nil { return data, err }
 defer r.Body.Close()
 err = xml.Unmarshal(body, &data)
 return data, err
}

// DecodeBytes unmarshals XML from a byte slice.
func (c *XMLCodec[T, U]) DecodeBytes(dataBytes []byte) (T, error) {
	var data T
	err := xml.Unmarshal(dataBytes, &data)
	return data, err
}

// Encode marshals the response to XML and writes it to the response writer.
func (c *XMLCodec[T, U]) Encode(w http.ResponseWriter, resp U) error {
 w.Header().Set("Content-Type", "application/xml")
 body, err := xml.Marshal(resp)
 if err != nil { return err }
 _, err = w.Write(body)
 return err
}

// Create a new XML codec instance
func NewXMLCodec[T any, U any]() *XMLCodec[T, U] {
 return &XMLCodec[T, U]{}
}

// Use the XML codec with a generic route definition within SubRouterConfig.Routes
xmlRouteDef := router.NewGenericRouteDefinition[CreateUserReq, CreateUserResp, string, string](
    router.RouteConfig[CreateUserReq, CreateUserResp]{
        Path:        "/api/users",
        Methods:     []router.HttpMethod{router.MethodPost},
        AuthLevel:   router.Ptr(router.NoAuth), // Use Ptr helper
        Codec:       NewXMLCodec[CreateUserReq, CreateUserResp](),
        Handler:     CreateUserHandler, // Assume handler exists
    },
)
// Add xmlRouteDef to SubRouterConfig.Routes
```

### Metrics

SRouter features a flexible, interface-based metrics system located in the `pkg/metrics` package. This allows integration with various metrics backends (like Prometheus, OpenTelemetry, etc.) by providing implementations for key interfaces.

#### Enabling and Configuring Metrics

To enable metrics, set `EnableMetrics: true` in your `RouterConfig`. You can further customize behavior using the `MetricsConfig` field:

```go
// Example: Configure metrics using MetricsConfig
routerConfig := router.RouterConfig{
    Logger:            logger, // Required
    GlobalTimeout:     2 * time.Second,
    GlobalMaxBodySize: 1 << 20, // 1 MB
    EnableMetrics:     true,      // Enable metrics collection
    MetricsConfig: &router.MetricsConfig{
        // Provide your implementations of metrics interfaces (Collector, MiddlewareFactory)
        // If nil, SRouter might use default implementations if available.
        Collector:        myMetricsCollector, // Must implement metrics.MetricsRegistry
        MiddlewareFactory: myMiddlewareFactory, // Optional: Must implement metrics.MetricsMiddleware

        // Configure which default metrics the built-in middleware should collect
        // if MiddlewareFactory is nil. These flags are used by metrics.NewMetricsMiddleware.
        // Note: Namespace and Subsystem are typically configured within your MetricsRegistry implementation.
        Namespace:        "myapp",
        Subsystem:        "api",
        EnableLatency:    true,  // Collect request latency
        EnableThroughput: true,  // Collect request/response size
        EnableQPS:        true,  // Collect requests per second
        EnableErrors:     true,  // Collect error counts by status code
    },
    // ... other config
}

r := router.NewRouter[string, string](routerConfig, authFunction, userIdFromUserFunction)

// Serving a /metrics endpoint is the application's responsibility.
// Get the handler from your MetricsRegistry implementation if it provides one.
var metricsHandler http.Handler
// Type assert myMetricsCollector (which should implement metrics.MetricsRegistry)
if registryWithHandler, ok := myMetricsCollector.(interface{ Handler() http.Handler }); ok {
	metricsHandler = registryWithHandler.Handler()
} else {
	metricsHandler = http.NotFoundHandler() // Or handle appropriately
}

// Serve metrics endpoint alongside your API (Example)
mux := http.NewServeMux()
mux.Handle("/metrics", metricsHandler)
mux.Handle("/", r)

// Start the server
// log.Fatal(http.ListenAndServe(":8080", mux))

```

#### Core Metrics Interfaces (`pkg/metrics`)

The system revolves around these key interfaces (you'll need to provide implementations):

1.  **`Collector`** (implements `metrics.MetricsRegistry`): Responsible for creating and managing individual metric types (Counters, Gauges, Histograms, Summaries). Your implementation will interact with your chosen metrics library (e.g., `prometheus.NewCounterVec`).
2.  **`MiddlewareFactory`** (Optional, implements `metrics.MetricsMiddleware`): Creates the actual `http.Handler` middleware that intercepts requests, records metrics using the `Collector`, and passes the request down the chain. SRouter likely provides a default factory if this is nil in the config.

#### Collected Metrics

When enabled via `MetricsConfig`, the default middleware typically collects:

-   **Latency**: Request duration.
-   **Throughput**: Request and response sizes.
-   **QPS**: Request rate.
-   **Errors**: Count of requests resulting in different HTTP error status codes.

#### Implementing Your Own Metrics Backend

1.  Choose your metrics library (e.g., `prometheus/client_golang`).
2.  Create structs that implement the `metrics.MetricsRegistry` and potentially `metrics.MetricsMiddleware` interfaces from `pkg/metrics`.
3.  Instantiate your implementations and pass them into the `MetricsConfig` when creating the router (`Collector` and optionally `MiddlewareFactory`).

See the `examples/prometheus` and `examples/custom-metrics` directories for potentially more detailed examples of implementing and using the metrics system. *Note: Ensure these examples reflect the latest interface-based approach.*

## Examples

SRouter includes several examples to help you get started:

- **examples/simple**: A simple example of using SRouter with basic routes
- **examples/auth**: An example of using authentication with SRouter
- **examples/auth-levels**: An example of using different authentication levels with SRouter
- **examples/user-auth**: An example of using user-returning authentication with SRouter
- **examples/generic**: An example of using generic routes with SRouter
- **examples/graceful-shutdown**: An example of graceful shutdown with SRouter
- **examples/middleware**: An example of using middleware with SRouter
- **examples/prometheus**: An example of using Prometheus metrics with SRouter
- **examples/custom-metrics**: An example of using custom metrics with SRouter
- **examples/rate-limiting**: An example of using rate limiting with SRouter
- **examples/source-types**: An example of using different source types for request data
- **examples/subrouters**: An example of using sub-routers with SRouter
- **examples/subrouter-generic-routes**: An example of using generic routes with sub-routers (declarative registration)
- **examples/nested-subrouters**: An example of nesting sub-routers for hierarchical routing (declarative registration)
- **examples/trace-logging**: An example of using trace ID logging with SRouter
- **examples/caching**: An example of implementing response caching using middleware (Note: Built-in config removed)

Each example includes a complete, runnable application that demonstrates a specific feature of SRouter.

## Configuration Reference

### RouterConfig

```go
type RouterConfig struct {
 Logger             *zap.Logger                       // Logger for all router operations. Required.
 GlobalTimeout      time.Duration                     // Default response timeout for all routes
 GlobalMaxBodySize  int64                             // Default maximum request body size in bytes
 GlobalRateLimit    *common.RateLimitConfig[any, any] // Default rate limit for all routes (uses common.RateLimitConfig)
 IPConfig           *router.IPConfig                  // Configuration for client IP extraction (uses router.IPConfig)
 EnableMetrics      bool                              // Enable metrics collection
 TraceIDBufferSize  int                               // Buffer size for trace ID generator (0 disables trace ID generation)
 MetricsConfig      *router.MetricsConfig             // Metrics configuration (optional, uses router.MetricsConfig)
 SubRouters         []SubRouterConfig                 // Sub-routers with their own configurations
 Middlewares        []common.Middleware               // Global middlewares applied to all routes (uses common.Middleware)
 AddUserObjectToCtx bool                              // Add user object to context (used by built-in auth middleware)
}
```

### MetricsConfig

```go
type MetricsConfig struct {
 // Collector is the metrics collector to use.
 // Must implement metrics.MetricsRegistry. Required if EnableMetrics is true.
 Collector any // metrics.MetricsRegistry

 // MiddlewareFactory is the factory for creating metrics middleware.
 // Optional. If nil, SRouter likely uses a default factory. Must implement metrics.MiddlewareFactory.
 MiddlewareFactory any // metrics.MiddlewareFactory

 // Namespace for metrics.
 Namespace string

 // Subsystem for metrics.
 Subsystem string

 // EnableLatency enables latency metrics.
 EnableLatency bool

 // EnableThroughput enables throughput metrics.
 EnableThroughput bool

 // EnableQPS enables queries per second metrics.
 EnableQPS bool

 // EnableErrors enables error metrics.
 EnableErrors bool
}
```

### SubRouterConfig

```go
type SubRouterConfig struct {
 PathPrefix          string                            // Common path prefix for all routes in this sub-router
 TimeoutOverride     time.Duration                     // Override global timeout for all routes in this sub-router
 MaxBodySizeOverride int64                             // Override global max body size for all routes in this sub-router
 RateLimitOverride   *common.RateLimitConfig[any, any] // Override global rate limit (uses common.RateLimitConfig)
 Routes              []any                             // Routes (can contain RouteConfigBase or GenericRouteRegistrationFunc)
 Middlewares         []common.Middleware               // Middlewares applied to all routes in this sub-router
 SubRouters          []SubRouterConfig                 // Nested sub-routers
 AuthLevel           *AuthLevel                        // Default auth level for routes in this sub-router (nil inherits)
 // CacheResponse, CacheKeyPrefix removed - implement caching via middleware if needed
}
```

### RouteConfigBase

```go
type RouteConfigBase struct {
 Path        string                            // Route path (will be prefixed with sub-router path prefix if applicable)
 Methods     []HttpMethod                      // HTTP methods this route handles (use constants like MethodGet)
 AuthLevel   *AuthLevel                        // Authentication level (nil uses sub-router default)
 Timeout     time.Duration                     // Override timeout for this specific route (0 inherits)
 MaxBodySize int64                             // Override max body size for this specific route (0 inherits, negative = no limit)
 RateLimit   *common.RateLimitConfig[any, any] // Rate limit for this specific route (nil inherits, uses common.RateLimitConfig)
 Handler     http.HandlerFunc                  // Standard HTTP handler function
 Middlewares []common.Middleware               // Middlewares applied to this specific route
}
```

### RouteConfig (Generic)

```go
type RouteConfig[T any, U any] struct {
 Path        string                            // Route path (will be prefixed with sub-router path prefix if applicable)
 Methods     []HttpMethod                      // HTTP methods this route handles (use constants like MethodGet)
 AuthLevel   *AuthLevel                        // Authentication level (nil uses sub-router default)
 Timeout     time.Duration                     // Override timeout for this specific route (0 inherits)
 MaxBodySize int64                             // Override max body size for this specific route (0 inherits, negative = no limit)
 RateLimit   *common.RateLimitConfig[any, any] // Rate limit for this specific route (nil inherits, uses common.RateLimitConfig)
 Codec       Codec[T, U]                       // Codec for marshaling/unmarshaling request and response. Required.
 Handler     GenericHandler[T, U]              // Generic handler function. Required.
 Middlewares []common.Middleware               // Middlewares applied to this specific route
 SourceType  SourceType                        // How to retrieve request data (defaults to Body)
 SourceKey   string                            // Parameter name for query or path parameters (if SourceType != Body)
 // CacheResponse, CacheKeyPrefix removed - implement caching via middleware if needed
}
```

### AuthLevel

```go
type AuthLevel int

const (
 // NoAuth indicates that no authentication is required for the route. (Default)
 NoAuth AuthLevel = iota

 // AuthOptional indicates that authentication is optional for the route.
 AuthOptional

 // AuthRequired indicates that authentication is required for the route.
 AuthRequired
)
```

### SourceType

```go
type SourceType int

const (
 // Body retrieves data from the request body (default).
 Body SourceType = iota

 // Base64QueryParameter retrieves data from a base64-encoded query parameter.
 Base64QueryParameter

 // Base62QueryParameter retrieves data from a base62-encoded query parameter.
 Base62QueryParameter

 // Base64PathParameter retrieves data from a base64-encoded path parameter.
 Base64PathParameter

 // Base62PathParameter retrieves data from a base62-encoded path parameter.
 Base62PathParameter
)
```

## Middleware Reference

SRouter provides several built-in middleware functions in the `pkg/middleware` package:

### Logging

Logs request details (method, path, status, duration, IP, trace ID). Requires the `zap.Logger` provided in `RouterConfig`. Automatically picks up trace ID if enabled.

```go
middleware.Logging(logger *zap.Logger, logInfoLevelForSuccess bool) Middleware
```
(See `docs/middleware.md` for log level details)

### Recovery

Recovers from panics and returns a 500 Internal Server Error. Logs the panic using the configured logger. (Applied internally by the router).

```go
// Applied internally, no need to add manually unless customizing recovery.
```

### Authentication

SRouter provides several authentication middleware constructors in `pkg/middleware`. See `docs/authentication.md`. These middlewares integrate with the `scontext` package to add user information to the request context.

#### Basic Authentication

```go
// T = UserID type, U = User object type
middleware.NewBasicAuthMiddleware[T comparable, U any](validCredentials map[string]T, logger *zap.Logger) common.Middleware
middleware.NewBasicUserAuthProvider[U any](getUserFunc func(username, password string) (*U, error)) // Provider for user objects
middleware.AuthenticationWithUserProvider[T comparable, U any](provider middleware.UserAuthProvider[U], logger *zap.Logger) common.Middleware // Middleware using the provider
```

#### Bearer Token Authentication

```go
// T = UserID type, U = User object type
middleware.NewBearerTokenMiddleware[T comparable, U any](validTokens map[string]T, logger *zap.Logger) common.Middleware
middleware.NewBearerTokenValidatorMiddleware[T comparable, U any](validator func(string) (T, bool), logger *zap.Logger) common.Middleware
middleware.NewBearerTokenWithUserMiddleware[T comparable, U any](getUserFunc func(token string) (*U, error), logger *zap.Logger) common.Middleware
```

#### API Key Authentication

```go
// T = UserID type, U = User object type
middleware.NewAPIKeyMiddleware[T comparable, U any](validKeys map[string]T, header, query string, logger *zap.Logger) common.Middleware
middleware.NewAPIKeyWithUserMiddleware[T comparable, U any](getUserFunc func(key string) (*U, error), header, query string, logger *zap.Logger) common.Middleware
```

### MaxBodySize

Limits the size of the request body (Applied internally based on config).

```go
// Configured via RouterConfig/SubRouterConfig/RouteConfig
```

### Timeout

Sets a timeout for the request (Applied internally based on config).

```go
// Configured via RouterConfig/SubRouterConfig/RouteConfig
```

### CORS

Adds CORS headers to the response.

```go
middleware.CORS(options middleware.CORSOptions) common.Middleware // Takes CORSOptions struct
```

### Chain

Chains multiple middlewares together (Used internally by the router).

```go
// Use common.NewMiddlewareChain().Append(...).Then(...) if needed manually
```

### Metrics Middleware

See Metrics section and `docs/metrics.md`.

### TraceMiddleware

Adds trace ID to the request context using `scontext`. Essential for log correlation. Recommended to enable via `RouterConfig.TraceIDBufferSize > 0` or add this middleware explicitly early in the chain. See `docs/trace-logging.md`.

```go
middleware.CreateTraceMiddleware(generator *middleware.IDGenerator) common.Middleware
// Note: The router creates the generator and middleware internally if TraceIDBufferSize > 0.
// You typically don't need to call this directly unless customizing the generator.
```

## Codec Reference

SRouter provides two built-in codecs in the `pkg/codec` package:

### JSONCodec

Uses JSON for marshaling and unmarshaling:

```go
codec.NewJSONCodec[T, U]() *codec.JSONCodec[T, U]
```

### ProtoCodec

Uses Protocol Buffers for marshaling and unmarshaling. Requires a factory function to create new request message instances without reflection.

```go
// Define a factory function for your specific proto message type (e.g., *MyProto)
myProtoFactory := func() *pb.MyProto { return &pb.MyProto{} } // Use generated type

// Pass the factory to the constructor
codec.NewProtoCodec[T, U](myProtoFactory) *codec.ProtoCodec[T, U] // T must match factory return type
```

### Codec Interface

You can create your own codecs by implementing the `Codec` interface (defined in `pkg/router/config.go`):

```go
type Codec[T any, U any] interface {
	// NewRequest creates a new zero-value instance of the request type T.
	NewRequest() T

	// Decode extracts and deserializes data from an HTTP request body into a value of type T.
	Decode(r *http.Request) (T, error)

	// DecodeBytes extracts and deserializes data from a byte slice into a value of type T.
	// Used for source types like query/path parameters.
	DecodeBytes(data []byte) (T, error)

	// Encode serializes a value of type U and writes it to the HTTP response.
	Encode(w http.ResponseWriter, resp U) error
}
```

## Path Parameter Reference

Path parameters defined in route paths (e.g., `/users/:id`) are extracted by the underlying `julienschmidt/httprouter` and made available via helper functions in the `router` package.

### GetParam

Retrieves a specific parameter from the request context:

```go
// Located in pkg/router/context_keys.go
router.GetParam(r *http.Request, name string) string
```

### GetParams

Retrieves all parameters from the request context as `httprouter.Params`:

```go
// Located in pkg/router/context_keys.go
router.GetParams(r *http.Request) httprouter.Params
```

### Retrieving Information from Request Context

The SRouter framework uses a structured approach for retrieving information added by its middleware (like user ID, client IP, trace ID) from request contexts. You should use the helper functions from the `pkg/scontext` package for these operations:

```go
// Get the user ID from the request
userID, ok := scontext.GetUserIDFromRequest[T, U](r) // Replace T, U with router's types

// Get the user object (pointer) from the request
user, ok := scontext.GetUserFromRequest[T, U](r) // Replace T, U with router's types

// Get the client IP from the request
ip, ok := scontext.GetClientIPFromRequest[T, U](r) // Replace T, U with router's types

// Get a flag from the request
flagValue, ok := scontext.GetFlagFromRequest[T, U](r, "flagName") // Replace T, U with router's types

// Get the trace ID from the request
traceID := scontext.GetTraceIDFromRequest[T, U](r)

// Get the database transaction interface from the request
txInterface, ok := scontext.GetTransactionFromRequest[T, U](r) // Replace T, U
```

These functions access values through the SRouterContext wrapper, providing a consistent and type-safe way to retrieve context values.

**Note:** The user object returned by `scontext.GetUserFromRequest` is a pointer (`*U`). For database transactions, retrieve the `DatabaseTransaction` interface and use `GetDB()` for GORM operations. See `docs/context-management.md` for details.

## Error Handling Reference

### NewHTTPError

Creates a new HTTPError:

```go
router.NewHTTPError(statusCode int, message string) *router.HTTPError
```

### HTTPError

Represents an HTTP error with a status code and message:

```go
type HTTPError struct {
 StatusCode int
 Message    string
}
```

## Performance Considerations

SRouter is designed to be highly performant. Here are some tips to get the best performance:

### Path Matching

SRouter uses julienschmidt/httprouter's O(1) or O(log n) path matching algorithm, which is much faster than regular expression-based routers.

### Middleware Ordering

The order of middlewares matters. Middlewares applied via `RouterConfig` or `SubRouterConfig` are executed before route-specific middleware. The internal `wrapHandler` applies middleware in a specific order (see source code for details, generally: timeout, body limit, global, sub-router, route, handler, with internal middleware like recovery, auth, rate limiting interleaved).

### Memory Allocation

SRouter minimizes allocations in the hot path. However, you can further reduce allocations by:

- Reusing request and response objects when possible
- Using sync.Pool for frequently allocated objects
- Avoiding unnecessary string concatenation

### Timeouts

Setting appropriate timeouts is crucial for performance and stability:

- Global timeouts protect against slow clients and DoS attacks
- Route-specific timeouts allow for different timeout values based on the expected response time of each route

### Body Size Limits

Setting appropriate body size limits is important for security and performance:

- Global body size limits protect against DoS attacks
- Route-specific body size limits allow for different limits based on the expected request size of each route

## Logging

SRouter uses structured logging via the `zap` library. A `*zap.Logger` instance **must** be provided in `RouterConfig`. The built-in `middleware.Logging` uses this logger and automatically includes contextual information like `trace_id` (if enabled). Appropriate log levels are used internally:

- **Error**: For server errors (status code 500+), timeouts, panics, and other exceptional conditions.
- **Warn**: For client errors (status code 400-499), slow requests (>1s), and potentially harmful situations.
- **Info**: For important operational information (used sparingly).
- **Debug**: For detailed request metrics, tracing information, and other development-related data (including successful request logs by default via `middleware.Logging`).

This approach ensures that your logs contain the right information at the right level. You can configure the log level of your `zap.Logger` instance passed to `RouterConfig` to control the verbosity.

```go
// Production: only show Info and above (adjust as needed)
config := zap.NewProductionConfig()
config.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
logger, _ := config.Build()
defer logger.Sync()

// Development: show Debug and above
devLogger, _ := zap.NewDevelopment()
defer devLogger.Sync()

// Pass the chosen logger to SRouter
routerConfig := router.RouterConfig{ Logger: logger, ... }
```
See `docs/logging.md` for more details.

## License

This project is licensed under the MIT License - see the LICENSE file for details.
