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
- **Intelligent Logging**: Appropriate log levels for different types of events
- **Trace ID Logging**: Automatically generate and include a unique trace ID for each request in all log entries
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
	"go.uber.org/zap"
	// "github.com/Suhaibinator/SRouter/pkg/common" // Import if adding specific middleware
	// "github.com/Suhaibinator/SRouter/pkg/middleware" // Import if adding specific middleware
)

func main() {
	// Create a logger
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	// Create a router configuration
	routerConfig := router.RouterConfig{
		Logger:            logger,
		GlobalTimeout:     2 * time.Second,
		GlobalMaxBodySize: 1 << 20, // 1 MB
		// Middlewares can be added here globally
		// Middlewares: []common.Middleware{
		//  middleware.Logging(logger), // Example: Add logging middleware
		// },
	}

	// Define a simple auth function (replace with your actual logic)
	// This function is passed to NewRouter.
	authFunction := func(ctx context.Context, token string) (string, bool) {
		// Example: Check if token is "valid-token"
		if token == "valid-token" {
			return "user-id-from-token", true // Return user ID and true if valid
		}
		return "", false // Return empty string and false if invalid
	}

	// Define a function to extract a comparable UserID from the User object (returned by authFunction)
	// In this case, the User object is just a string (the user ID itself).
	userIdFromUserFunction := func(user string) string {
		return user
	}

	// Create a router. Authentication logic is provided via functions.
	// The type parameters define the UserID type (string) and User object type (string).
	r := router.NewRouter[string, string](routerConfig, authFunction, userIdFromUserFunction)

	// Register a simple route
	r.RegisterRoute(router.RouteConfigBase{
		Path:    "/hello",
		Methods: []string{"GET"},
		Handler: func(w http.ResponseWriter, r *http.Request) {
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
			Methods: []string{"GET"},
			Handler: ListUsersHandler,
		},
		router.RouteConfigBase{
			Path:    "/users/:id", // Becomes /api/v1/users/:id
			Methods: []string{"GET"},
			Handler: GetUserHandler,
		},
		// Declarative generic route using the helper
		router.NewGenericRouteDefinition[CreateUserReq, CreateUserResp, string, string](
			router.RouteConfig[CreateUserReq, CreateUserResp]{
				Path:      "/users", // Path relative to the sub-router prefix (/api/v1/users)
				Methods:   []string{"POST"},
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
			Methods: []string{"GET"},
			Handler: ListUsersV2Handler,
		},
	},
}

// Create a router with sub-routers
routerConfig := router.RouterConfig{
	Logger:            logger,
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
		router.RouteConfigBase{ Path: "/:id", Methods: []string{"GET"}, Handler: GetUserHandler },
		router.NewGenericRouteDefinition[UserReq, UserResp, string, string](
			router.RouteConfig[UserReq, UserResp]{ Path: "/info", Methods: []string{"POST"}, Codec: userCodec, Handler: UserInfoHandler },
		),
	},
}
apiV1SubRouter := router.SubRouterConfig{
	PathPrefix: "/v1", // Relative to /api
	SubRouters: []router.SubRouterConfig{usersV1SubRouter},
	Routes: []any{
		router.RouteConfigBase{ Path: "/status", Methods: []string{"GET"}, Handler: V1StatusHandler },
	},
}
apiSubRouter := router.SubRouterConfig{
	PathPrefix: "/api", // Root prefix for this group
	SubRouters: []router.SubRouterConfig{apiV1SubRouter},
	Routes: []any{
		router.RouteConfigBase{ Path: "/health", Methods: []string{"GET"}, Handler: HealthHandler },
	},
}

// Register top-level sub-router during NewRouter
routerConfig := router.RouterConfig{ SubRouters: []router.SubRouterConfig{apiSubRouter}, ... }
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
 Methods:     []string{"POST"},
 AuthLevel:   router.AuthRequired,
 Codec:       codec.NewJSONCodec[CreateUserReq, CreateUserResp](),
 Handler:     CreateUserHandler,
}, time.Duration(0), int64(0), nil) // Pass zero/nil for effective settings
```

Note that the `RegisterGenericRoute` function takes five type parameters: the request type, the response type, the user ID type, and the user object type. The last two should match the type parameters of your router. It also requires effective timeout, max body size, and rate limit values, which are typically zero/nil when registering directly on the root router. Use `RegisterGenericRouteOnSubRouter` for routes belonging to sub-routers.

### Using Path Parameters

SRouter makes it easy to access path parameters:

```go
func GetUserHandler(w http.ResponseWriter, r *http.Request) {
 // Get the user ID from the path parameters
 id := router.GetParam(r, "id")

 // Use the ID to fetch the user
 user := fetchUser(id)

 // Return the user as JSON
 w.Header().Set("Content-Type", "application/json")
 json.NewEncoder(w).Encode(user)
}
```

### Trace ID Logging

SRouter provides built-in support for trace ID logging, which allows you to correlate log entries across different parts of your application for a single request. Each request is assigned a unique trace ID (UUID) that is automatically included in all log entries related to that request.

#### Enabling Trace ID Logging

There are two ways to enable trace ID logging:

1. Using the `EnableTraceID` configuration option:

```go
// Create a router with trace ID logging enabled
routerConfig := router.RouterConfig{
    Logger:        logger,
    EnableTraceID: true, // Enable trace ID logging
    // Other configuration...
}

r := router.NewRouter[string, string](routerConfig, authFunction, userIdFromUserFunction)
```

2. Using the `TraceMiddleware`:

```go
// Create a router with trace middleware
routerConfig := router.RouterConfig{
    Middlewares: []common.Middleware{
        middleware.TraceMiddleware(), // Add this as the first middleware
        // Other middleware...
    },
    // Other configuration...
}

r := router.NewRouter[string, string](routerConfig, authFunction, userIdFromUserFunction)
```

#### Accessing the Trace ID

You can access the trace ID in your handlers and middleware:

```go
func myHandler(w http.ResponseWriter, r *http.Request) {
    // Get the trace ID
    traceID := middleware.GetTraceID(r)

    // Use the trace ID
    fmt.Printf("Processing request with trace ID: %s\n", traceID)

    // ...
}
```

#### Propagating the Trace ID to Downstream Services

If your application calls other services, you can propagate the trace ID to maintain the request trace across service boundaries:

```go
func callDownstreamService(r *http.Request) {
    // Get the trace ID
    traceID := middleware.GetTraceID(r)

    // Create a new request to the downstream service
    req, _ := http.NewRequest("GET", "https://api.example.com/data", nil)

    // Add the trace ID to the request headers
    req.Header.Set("X-Trace-ID", traceID)

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
    traceID := middleware.GetTraceIDFromContext(ctx)

    // Use the trace ID
    log.Printf("[trace_id=%s] Processing data\n", traceID)

    // ...
}
```

See the `examples/trace-logging` directory for a complete example of trace ID logging.

### Graceful Shutdown

SRouter provides a `Shutdown` method for graceful shutdown:

```go
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

// Wait for interrupt signal
quit := make(chan os.Signal, 1)
signal.Notify(quit, os.Interrupt)
<-quit

// Create a deadline to wait for
ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
defer cancel()

// Shut down the router
if err := r.Shutdown(ctx); err != nil {
 log.Fatalf("Router shutdown failed: %v", err)
}

// Shut down the server
if err := srv.Shutdown(ctx); err != nil {
 log.Fatalf("Server shutdown failed: %v", err)
}
```

See the `examples/graceful-shutdown` directory for a complete example of graceful shutdown.

## Advanced Usage

### IP Configuration

SRouter provides a flexible way to extract client IP addresses, which is particularly important when your application is behind a reverse proxy or load balancer. The IP configuration allows you to specify where to extract the client IP from and whether to trust proxy headers.

```go
// Configure IP extraction to use X-Forwarded-For header
routerConfig := router.RouterConfig{
    // ... other config
    IPConfig: &middleware.IPConfig{
        Source:     middleware.IPSourceXForwardedFor,
        TrustProxy: true,
    },
}
```

#### IP Source Types

SRouter supports several IP source types:

1. **IPSourceRemoteAddr**: Uses the request's RemoteAddr field (default if no source is specified)
2. **IPSourceXForwardedFor**: Uses the X-Forwarded-For header (common for most reverse proxies)
3. **IPSourceXRealIP**: Uses the X-Real-IP header (used by Nginx and some other proxies)
4. **IPSourceCustomHeader**: Uses a custom header specified in the configuration

```go
// Configure IP extraction to use a custom header
routerConfig := router.RouterConfig{
    // ... other config
    IPConfig: &middleware.IPConfig{
        Source:       middleware.IPSourceCustomHeader,
        CustomHeader: "X-Client-IP",
        TrustProxy:   true,
    },
}
```

#### Trust Proxy Setting

The `TrustProxy` setting determines whether to trust proxy headers:

- If `true`, the specified source will be used to extract the client IP
- If `false` or if the specified source doesn't contain an IP, the request's RemoteAddr will be used as a fallback

This is important for security, as malicious clients could potentially spoof headers if your application blindly trusts them.

See the `examples/middleware` directory for examples of using IP configuration.

### Rate Limiting

SRouter provides a flexible rate limiting system that can be configured at the global, sub-router, or route level. Rate limits can be based on IP address, authenticated user, or custom criteria. Under the hood, SRouter uses [Uber's ratelimit library](https://github.com/uber-go/ratelimit) for efficient and smooth rate limiting with a leaky bucket algorithm.

#### Rate Limiting Configuration

```go
// Create a router with global rate limiting
routerConfig := router.RouterConfig{
    // ... other config
    GlobalRateLimit: &middleware.RateLimitConfig[any, any]{ // Use [any, any] for global/sub-router config
        BucketName: "global",
        Limit:      100,
        Window:     time.Minute,
        Strategy:   middleware.RateLimitStrategyIP, // Use constants
    },
}

// Create a sub-router with rate limiting
subRouter := router.SubRouterConfig{
    PathPrefix: "/api/v1",
    RateLimitOverride: &middleware.RateLimitConfig[any, any]{ // Use [any, any]
        BucketName: "api-v1",
        Limit:      50,
        Window:     time.Minute,
        Strategy:   middleware.RateLimitStrategyIP,
    },
    // ... other config
}

// Create a route with rate limiting
route := router.RouteConfig[MyReq, MyResp]{ // Use specific types for route config
    Path:    "/users",
    Methods: []string{"POST"},
    RateLimit: &middleware.RateLimitConfig[any, any]{ // Use [any, any] here too
        BucketName: "create-user",
        Limit:      10,
        Window:     time.Minute,
        Strategy:   middleware.RateLimitStrategyIP,
    },
    // ... other config
}
```

#### Rate Limiting Strategies

SRouter supports several rate limiting strategies using constants defined in the `middleware` package:

1. **IP-based Rate Limiting (`middleware.RateLimitStrategyIP`)**: Limits requests based on the client's IP address (extracted according to the IP configuration). The rate limiting is smooth, distributing requests evenly across the time window rather than allowing bursts.

```go
RateLimit: &middleware.RateLimitConfig[any, any]{
    BucketName: "ip-based",
    Limit:      100,
    Window:     time.Minute,
    Strategy:   middleware.RateLimitStrategyIP,
}
```

2. **User-based Rate Limiting (`middleware.RateLimitStrategyUser`)**: Limits requests based on the authenticated user ID (requires authentication middleware to run first and populate the user ID in the context).

```go
RateLimit: &middleware.RateLimitConfig[any, any]{
    BucketName: "user-based",
    Limit:      50,
    Window:     time.Minute,
    Strategy:   middleware.RateLimitStrategyUser,
}
```

3. **Custom Rate Limiting (`middleware.RateLimitStrategyCustom`)**: Limits requests based on custom criteria defined by a `KeyExtractor` function.

```go
RateLimit: &middleware.RateLimitConfig[any, any]{
    BucketName: "custom",
    Limit:      20,
    Window:     time.Minute,
    Strategy:   middleware.RateLimitStrategyCustom,
    KeyExtractor: func(r *http.Request) (string, error) {
        // Extract API key from query parameter
        apiKey := r.URL.Query().Get("api_key")
        if apiKey == "" {
            // Fall back to IP if no API key is provided
            ip, _ := middleware.GetClientIPFromRequest[string, string](r) // Adjust types as needed
            return ip, nil
        }
        return apiKey, nil
    },
}
```

#### Shared Rate Limit Buckets

You can share rate limit buckets between different endpoints by using the same bucket name:

```go
// Login endpoint
loginRoute := router.RouteConfigBase{
    Path:    "/login",
    Methods: []string{"POST"},
    RateLimit: &middleware.RateLimitConfig[any, any]{
        BucketName: "auth-endpoints", // Shared bucket name
        Limit:      5,
        Window:     time.Minute,
        Strategy:   middleware.RateLimitStrategyIP,
    },
    // ... other config
}

// Register endpoint
registerRoute := router.RouteConfigBase{
    Path:    "/register",
    Methods: []string{"POST"},
    RateLimit: &middleware.RateLimitConfig[any, any]{
        BucketName: "auth-endpoints", // Same bucket name as login
        Limit:      5,
        Window:     time.Minute,
        Strategy:   middleware.RateLimitStrategyIP,
    },
    // ... other config
}
```

#### Custom Rate Limit Responses

You can customize the response sent when a rate limit is exceeded:

```go
RateLimit: &middleware.RateLimitConfig[any, any]{
    BucketName: "custom-response",
    Limit:      10,
    Window:     time.Minute,
    Strategy:   middleware.RateLimitStrategyIP,
    ExceededHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusTooManyRequests)
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
routePublic := router.RouteConfigBase{ AuthLevel: router.NoAuth, ... }
routeOptional := router.RouteConfigBase{ AuthLevel: router.AuthOptional, ... }
routeProtected := router.RouteConfigBase{ AuthLevel: router.AuthRequired, ... }
```

#### Authentication Middleware

You need to provide your own authentication middleware or use pre-built ones from the `pkg/middleware` package. The middleware is responsible for:

1.  Extracting credentials (e.g., from headers).
2.  Validating credentials.
3.  If validation succeeds:
    *   Populating the request context with the user ID and optionally the user object using `middleware.WithUserID` and `middleware.WithUser`.
    *   Calling the next handler.
4.  If validation fails:
    *   For `AuthRequired` routes, rejecting the request (e.g., `http.Error(w, "Unauthorized", http.StatusUnauthorized)`).
    *   For `AuthOptional` routes, calling the next handler without populating user info.

**Important:** The `authFunction` and `userIdFromUserFunction` parameters in `NewRouter` are still present for internal use by the built-in `authRequiredMiddleware` and `authOptionalMiddleware`, but relying solely on these for complex authentication is discouraged. **Using dedicated authentication middleware is the recommended approach.**

```go
// Example using a custom auth middleware
func MyAuthMiddleware(authService MyAuthService) common.Middleware {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            token := r.Header.Get("Authorization")
            token = strings.TrimPrefix(token, "Bearer ")

            user, userID, err := authService.ValidateToken(r.Context(), token)

            if err == nil { // Authentication successful
                ctx := middleware.WithUserID[string, MyUserType](r.Context(), userID) // Use actual types
                ctx = middleware.WithUser[string, MyUserType](ctx, user)
                next.ServeHTTP(w, r.WithContext(ctx))
            } else { // Authentication failed
                // Check route's AuthLevel (This requires accessing route config, which middleware typically doesn't do directly)
                // A simpler approach is to apply this middleware only to routes where it's needed.
                // Or, the router's internal auth middleware (if used) handles the AuthLevel check.
                // If implementing purely custom middleware, you might apply it conditionally or handle levels internally.
                http.Error(w, "Unauthorized", http.StatusUnauthorized)
                return
            }
        })
    }
}

// Apply middleware globally or per-route/sub-router
routerConfig := router.RouterConfig{
    Middlewares: []common.Middleware{ MyAuthMiddleware(myService) },
    // ...
}
r := router.NewRouter[string, MyUserType](routerConfig, ...) // Match types
```

See the `examples/auth-levels`, `examples/user-auth`, and `examples/auth` directories for examples.

### Context Management

SRouter uses a structured approach to context management using the `SRouterContext` wrapper. This approach avoids deep nesting of context values by storing all values in a single wrapper structure.

#### SRouterContext Wrapper

The `SRouterContext` wrapper is a generic type that holds all values that SRouter adds to request contexts:

```go
// Located in pkg/middleware/context.go
type SRouterContext[T comparable, U any] struct {
    // User ID and User object storage
    UserID T
    User   *U

    // Client IP address
    ClientIP string

    // Trace ID
    TraceID string

    // Track which fields are set
    UserIDSet   bool
    UserSet     bool
    ClientIPSet bool
    TraceIDSet  bool

    // Additional flags
    Flags map[string]bool
}
```

The type parameters `T` and `U` represent the user ID type and user object type, respectively. This generic approach provides type safety while allowing flexibility in the types used for user IDs and objects.

#### Benefits of SRouterContext

The SRouterContext approach offers several advantages:

1. **Reduced Context Nesting**: Instead of wrapping contexts multiple times like:
   ```go
   ctx = context.WithValue(context.WithValue(r.Context(), userIDKey, userID), userKey, user)
   ```
   SRouter uses a single context wrapper:
   ```go
   ctx := middleware.WithUserID[T, U](r.Context(), userID)
   ctx = middleware.WithUser[T, U](ctx, user)
   ```

2. **Type Safety**: The generic type parameters ensure proper type handling without the need for type assertions.

3. **Extensibility**: New fields can be added to the `SRouterContext` struct without creating more nested contexts.

4. **Organization**: Related context values are grouped logically, making the code more maintainable.

#### Accessing Context Values

SRouter provides several helper functions in the `pkg/middleware` package for accessing context values:

```go
// Get the user ID from the request
userID, ok := middleware.GetUserIDFromRequest[string, User](r)

// Get the user object from the request
user, ok := middleware.GetUserFromRequest[string, User](r)

// Get the client IP from the request
ip, ok := middleware.GetClientIPFromRequest[string, User](r)

// Get a flag from the request
flagValue, ok := middleware.GetFlagFromRequest[string, User](r, "flagName")

// Get the trace ID from the request
traceID := middleware.GetTraceID(r) // Shortcut function

// Get trace ID from context directly
traceIDFromCtx := middleware.GetTraceIDFromContext(ctx)
```

These functions automatically handle the type parameters and provide a clean, consistent interface for accessing context values. **Note:** The user object returned by `middleware.GetUserFromRequest` is a pointer (`*U`).

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
 user, found := findUser(id)
 if !found {
  // Return a custom error
  return GetUserResp{}, NotFoundError("User", id)
 }

 // Return the user
 return GetUserResp{
  ID:    user.ID,
  Name:  user.Name,
  Email: user.Email,
 }, nil
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
   ctx := middleware.WithFlag[string, string](r.Context(), "request_id", requestID) // Assuming string/string router types

   // Add it to the response headers
   w.Header().Set("X-Request-ID", requestID)

   // Call the next handler with the updated request
   next.ServeHTTP(w, r.WithContext(ctx))
  })
 }
}

// Apply the middleware to the router
routerConfig := router.RouterConfig{
 // ...
 Middlewares: []common.Middleware{
  RequestIDMiddleware(),
  middleware.Logging(logger),
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
   router.RegisterGenericRoute[UserRequest, UserResponse, string, string](r, router.RouteConfig[UserRequest, UserResponse]{
       // ...
       // SourceType defaults to Body if not specified
   }, ...)
   ```

2. **Base64QueryParameter**: Retrieves data from a base64-encoded query parameter.

   ```go
   router.RegisterGenericRoute[UserRequest, UserResponse, string, string](r, router.RouteConfig[UserRequest, UserResponse]{
       // ...
       SourceType: router.Base64QueryParameter,
       SourceKey:  "data", // Will look for ?data=base64encodedstring
   }, ...)
   ```

3. **Base62QueryParameter**: Retrieves data from a base62-encoded query parameter.

   ```go
   router.RegisterGenericRoute[UserRequest, UserResponse, string, string](r, router.RouteConfig[UserRequest, UserResponse]{
       // ...
       SourceType: router.Base62QueryParameter,
       SourceKey:  "data", // Will look for ?data=base62encodedstring
   }, ...)
   ```

4. **Base64PathParameter**: Retrieves data from a base64-encoded path parameter.

   ```go
   router.RegisterGenericRoute[UserRequest, UserResponse, string, string](r, router.RouteConfig[UserRequest, UserResponse]{
       Path:       "/users/:data",
       // ...
       SourceType: router.Base64PathParameter,
       SourceKey:  "data", // Will use the :data path parameter
   }, ...)
   ```

5. **Base62PathParameter**: Retrieves data from a base62-encoded path parameter.

   ```go
   router.RegisterGenericRoute[UserRequest, UserResponse, string, string](r, router.RouteConfig[UserRequest, UserResponse]{
       Path:       "/users/:data",
       // ...
       SourceType: router.Base62PathParameter,
       SourceKey:  "data", // Will use the :data path parameter
   }, ...)
   ```

See the `examples/source-types` directory for a complete example.

### Custom Codec

You can create custom codecs for different data formats:

```go
// Create a custom XML codec
type XMLCodec[T any, U any] struct{}

func (c *XMLCodec[T, U]) Decode(r *http.Request) (T, error) {
 var data T

 // Read the request body
 body, err := io.ReadAll(r.Body)
 if err != nil {
  return data, err
 }
 defer r.Body.Close()

 // Unmarshal the XML
 err = xml.Unmarshal(body, &data)
 if err != nil {
  return data, err
 }

 return data, nil
}

func (c *XMLCodec[T, U]) Encode(w http.ResponseWriter, resp U) error {
 // Set the content type
 w.Header().Set("Content-Type", "application/xml")

 // Marshal the response
 body, err := xml.Marshal(resp)
 if err != nil {
  return err
 }

 // Write the response
 _, err = w.Write(body)
 return err
}

// Create a new XML codec
func NewXMLCodec[T any, U any]() *XMLCodec[T, U] {
 return &XMLCodec[T, U]{}
}

// Use the XML codec with a generic route
router.RegisterGenericRoute[CreateUserReq, CreateUserResp, string, string](r, router.RouteConfig[CreateUserReq, CreateUserResp]{
 Path:        "/api/users",
 Methods:     []string{"POST"},
 AuthLevel:   router.NoAuth, // No authentication required
 Codec:       NewXMLCodec[CreateUserReq, CreateUserResp](),
 Handler:     CreateUserHandler,
}, time.Duration(0), int64(0), nil) // Added effective settings
```

### Metrics

SRouter features a flexible, interface-based metrics system located in the `pkg/metrics` package. This allows integration with various metrics backends (like Prometheus, OpenTelemetry, etc.) by providing implementations for key interfaces.

#### Enabling and Configuring Metrics

To enable metrics, set `EnableMetrics: true` in your `RouterConfig`. You can further customize behavior using the `MetricsConfig` field:

```go
// Example: Configure metrics using MetricsConfig
routerConfig := router.RouterConfig{
    Logger:            logger,
    GlobalTimeout:     2 * time.Second,
    GlobalMaxBodySize: 1 << 20, // 1 MB
    EnableMetrics:     true,      // Enable metrics collection
    MetricsConfig: &router.MetricsConfig{
        // Provide your implementations of metrics interfaces (Collector, Exporter, MiddlewareFactory)
        // If nil, SRouter might use default implementations if available.
        Collector:        myMetricsCollector, // Must implement metrics.Collector
        Exporter:         myMetricsExporter,  // Optional: Must implement metrics.Exporter if needed (e.g., for /metrics endpoint)
        MiddlewareFactory: myMiddlewareFactory, // Optional: Must implement metrics.MiddlewareFactory

        // Configure metric details
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

// If your Exporter provides an HTTP handler (e.g., for Prometheus /metrics)
var metricsHandler http.Handler
if exporter, ok := myMetricsExporter.(metrics.Exporter); ok {
    metricsHandler = exporter.Handler()
} else {
    metricsHandler = http.NotFoundHandler() // Or handle appropriately
}

// Serve metrics endpoint alongside your API
mux := http.NewServeMux()
mux.Handle("/metrics", metricsHandler)
mux.Handle("/", r)

// Start the server
http.ListenAndServe(":8080", mux)

```

#### Core Metrics Interfaces (`pkg/metrics`)

The system revolves around these key interfaces (you'll need to provide implementations):

1.  **`Collector`**: Responsible for creating and managing individual metric types (Counters, Gauges, Histograms, Summaries). Your implementation will interact with your chosen metrics library (e.g., `prometheus.NewCounterVec`).
2.  **`Exporter`** (Optional): Responsible for exposing collected metrics. A common use case is providing an `http.Handler` for a `/metrics` endpoint (like `promhttp.Handler()`).
3.  **`MiddlewareFactory`** (Optional): Creates the actual `http.Handler` middleware that intercepts requests, records metrics using the `Collector`, and passes the request down the chain. SRouter likely provides a default factory if this is nil in the config.

#### Collected Metrics

When enabled via `MetricsConfig`, the default middleware typically collects:

-   **Latency**: Request duration.
-   **Throughput**: Request and response sizes.
-   **QPS**: Request rate.
-   **Errors**: Count of requests resulting in different HTTP error status codes.

#### Implementing Your Own Metrics Backend

1.  Choose your metrics library (e.g., `prometheus/client_golang`).
2.  Create structs that implement the `metrics.Collector`, `metrics.Exporter` (if needed), and potentially `metrics.MiddlewareFactory` interfaces from `pkg/metrics`.
3.  Instantiate your implementations and pass them into the `MetricsConfig` when creating the router.

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
 Logger             *zap.Logger                           // Logger for all router operations
 GlobalTimeout      time.Duration                         // Default response timeout for all routes
 GlobalMaxBodySize  int64                                 // Default maximum request body size in bytes
 GlobalRateLimit    *middleware.RateLimitConfig[any, any] // Default rate limit for all routes
 IPConfig           *middleware.IPConfig                  // Configuration for client IP extraction
 EnableMetrics      bool                                  // Enable metrics collection
 EnableTracing      bool                                  // Enable distributed tracing (Note: Implementation might be via middleware)
 EnableTraceID      bool                                  // Enable trace ID logging (can also be done via TraceMiddleware)
 MetricsConfig      *MetricsConfig                        // Metrics configuration (optional)
 SubRouters         []SubRouterConfig                     // Sub-routers with their own configurations
 Middlewares        []common.Middleware                   // Global middlewares applied to all routes
 AddUserObjectToCtx bool                                  // Add user object to context (used by built-in auth middleware)
 // CacheGet, CacheSet, CacheKeyPrefix removed - implement caching via middleware if needed
}
```

### MetricsConfig

```go
type MetricsConfig struct {
 // Collector is the metrics collector to use.
 // If nil, a default collector will be used if metrics are enabled.
 Collector interface{} // metrics.Collector

 // Exporter is the metrics exporter to use.
 // If nil, a default exporter will be used if metrics are enabled.
 Exporter interface{} // metrics.Exporter

 // MiddlewareFactory is the factory for creating metrics middleware.
 // If nil, a default middleware factory will be used if metrics are enabled.
 MiddlewareFactory interface{} // metrics.MiddlewareFactory

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
 PathPrefix          string                                // Common path prefix for all routes in this sub-router
 TimeoutOverride     time.Duration                         // Override global timeout for all routes in this sub-router
 MaxBodySizeOverride int64                                 // Override global max body size for all routes in this sub-router
 RateLimitOverride   *middleware.RateLimitConfig[any, any] // Override global rate limit for all routes in this sub-router
 Routes              []any                                 // Routes (RouteConfigBase or GenericRouteRegistrationFunc)
 Middlewares         []common.Middleware                   // Middlewares applied to all routes in this sub-router
 SubRouters          []SubRouterConfig                     // Nested sub-routers
 AuthLevel           *AuthLevel                            // Default auth level for routes in this sub-router
 // CacheResponse, CacheKeyPrefix removed - implement caching via middleware if needed
}
```

### RouteConfigBase

```go
type RouteConfigBase struct {
 Path        string                                // Route path (will be prefixed with sub-router path prefix if applicable)
 Methods     []string                              // HTTP methods this route handles
 AuthLevel   *AuthLevel                            // Authentication level (nil uses sub-router default)
 Timeout     time.Duration                         // Override timeout for this specific route
 MaxBodySize int64                                 // Override max body size for this specific route
 RateLimit   *middleware.RateLimitConfig[any, any] // Rate limit for this specific route
 Handler     http.HandlerFunc                      // Standard HTTP handler function
 Middlewares []common.Middleware                   // Middlewares applied to this specific route
}
```

### RouteConfig (Generic)

```go
type RouteConfig[T any, U any] struct {
 Path        string                                // Route path (will be prefixed with sub-router path prefix if applicable)
 Methods     []string                              // HTTP methods this route handles
 AuthLevel   *AuthLevel                            // Authentication level (nil uses sub-router default)
 Timeout     time.Duration                         // Override timeout for this specific route
 MaxBodySize int64                                 // Override max body size for this specific route
 RateLimit   *middleware.RateLimitConfig[any, any] // Rate limit for this specific route
 Codec       Codec[T, U]                           // Codec for marshaling/unmarshaling request and response
 Handler     GenericHandler[T, U]                  // Generic handler function
 Middlewares []common.Middleware                   // Middlewares applied to this specific route
 SourceType  SourceType                            // How to retrieve request data (defaults to Body)
 SourceKey   string                                // Parameter name for query or path parameters
 // CacheResponse, CacheKeyPrefix removed - implement caching via middleware if needed
}
```

### AuthLevel

```go
type AuthLevel int

const (
 // NoAuth indicates that no authentication is required for the route.
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

Logs requests with method, path, status code, and duration:

```go
middleware.Logging(logger *zap.Logger) Middleware
```

### Recovery

Recovers from panics and returns a 500 Internal Server Error:

```go
middleware.Recovery(logger *zap.Logger) Middleware // Note: Router applies its own recovery internally
```

### Authentication

SRouter provides several authentication middleware options in `pkg/middleware`:

#### Basic Authentication

```go
middleware.NewBasicAuthMiddleware(credentials map[string]string, logger *zap.Logger) Middleware
middleware.NewBasicAuthWithUserMiddleware[U any](validator func(string, string) (*U, error), logger *zap.Logger) Middleware
```

#### Bearer Token Authentication

```go
middleware.NewBearerTokenMiddleware(validTokens map[string]bool, logger *zap.Logger) Middleware
middleware.NewBearerTokenWithUserMiddleware[U any](validator func(string) (*U, error), logger *zap.Logger) Middleware
```

#### API Key Authentication

```go
middleware.NewAPIKeyMiddleware(validKeys map[string]bool, header, query string, logger *zap.Logger) Middleware
middleware.NewAPIKeyWithUserMiddleware[U any](validator func(string) (*U, error), header, query string, logger *zap.Logger) Middleware
```

### MaxBodySize

Limits the size of the request body (Note: Router applies this internally based on config):

```go
// Typically configured via RouterConfig/SubRouterConfig/RouteConfig
```

### Timeout

Sets a timeout for the request (Note: Router applies this internally based on config):

```go
// Typically configured via RouterConfig/SubRouterConfig/RouteConfig
```

### CORS

Adds CORS headers to the response:

```go
middleware.CORS(origins []string, methods []string, headers []string) Middleware
```

### Chain

Chains multiple middlewares together (Note: Router uses `pkg/common.MiddlewareChain` internally):

```go
// Use common.NewMiddlewareChain().Append(...).Then(...)
```

### Metrics Middleware

See Metrics section.

### TraceMiddleware

Adds trace ID to the request context:

```go
middleware.TraceMiddleware() Middleware
middleware.TraceMiddlewareWithConfig(bufferSize int) Middleware
```

## Codec Reference

SRouter provides two built-in codecs in the `pkg/codec` package:

### JSONCodec

Uses JSON for marshaling and unmarshaling:

```go
codec.NewJSONCodec[T, U]() *codec.JSONCodec[T, U]
```

### ProtoCodec

Uses Protocol Buffers for marshaling and unmarshaling:

```go
codec.NewProtoCodec[T, U]() *codec.ProtoCodec[T, U]
```

### Codec Interface

You can create your own codecs by implementing the `Codec` interface (defined in `pkg/router/config.go`):

```go
type Codec[T any, U any] interface {
 Decode(r *http.Request) (T, error)
 Encode(w http.ResponseWriter, resp U) error
}
```

## Path Parameter Reference

### GetParam

Retrieves a specific parameter from the request context:

```go
router.GetParam(r *http.Request, name string) string
```

### GetParams

Retrieves all parameters from the request context:

```go
router.GetParams(r *http.Request) httprouter.Params
```

### Retrieving Information from Request Context

The SRouter framework uses a structured approach for retrieving information from request contexts. You should use these middleware package functions (from `pkg/middleware`) for context-related operations:

```go
// Get the user ID from the request
userID, ok := middleware.GetUserIDFromRequest[T, U](r) // Replace T, U with router's types

// Get the user object from the request
user, ok := middleware.GetUserFromRequest[T, U](r) // Replace T, U with router's types

// Get the client IP from the request
ip, ok := middleware.GetClientIPFromRequest[T, U](r) // Replace T, U with router's types

// Get a flag from the request
flagValue, ok := middleware.GetFlagFromRequest[T, U](r, "flagName") // Replace T, U with router's types

// Get the trace ID from the request
traceID := middleware.GetTraceID(r)
```

These functions access values through the SRouterContext wrapper, providing a consistent and type-safe way to retrieve context values.

**Note:** The user object returned by `middleware.GetUserFromRequest` is a pointer (`*U`). This matches the internal storage format and avoids unnecessary copying.

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

The order of middlewares matters. Middlewares applied via `RouterConfig` or `SubRouterConfig` are executed before route-specific middleware. The internal `wrapHandler` applies middleware in a specific order (see source code for details, generally: recovery, auth, rate limit, route, sub-router, global, timeout, body limit, shutdown).

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

SRouter uses intelligent logging with appropriate log levels to provide useful information without creating log spam:

- **Error**: For server errors (status code 500+), timeouts, panics, and other exceptional conditions
- **Warn**: For client errors (status code 400-499), slow requests (>1s), and potentially harmful situations
- **Info**: For important operational information (used sparingly)
- **Debug**: For detailed request metrics, tracing information, and other development-related data

This approach ensures that your logs contain the right information at the right level:

- Critical issues are immediately visible at the Error level
- Potential problems are highlighted at the Warn level
- Normal operations are logged at the Debug level to avoid log spam

You can configure the log level of your zap.Logger to control the verbosity of the logs:

```go
// Production: only show Error and above
logger, _ := zap.NewProduction()

// Development: show Debug and above
logger, _ := zap.NewDevelopment()

// Custom: show Info and above
config := zap.NewProductionConfig()
config.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
logger, _ := config.Build()
```

## License

This project is licensed under the MIT License - see the LICENSE file for details.
