# SRouter

SRouter is a high-performance HTTP router for Go that wraps [julienschmidt/httprouter](https://github.com/julienschmidt/httprouter) with advanced features including sub-router overrides, middleware support, generic-based marshaling/unmarshaling, configurable timeouts, body size limits, authentication levels, Prometheus integration, and intelligent logging.

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
- **Response Caching**: Built-in caching support for query and path parameter-based routes with customizable cache implementations

## Installation

```bash
go get github.com/Suhaibinator/SRouter
```

## Requirements

- Go 1.24.0 or higher
- [julienschmidt/httprouter](https://github.com/julienschmidt/httprouter) v1.3.0 or higher for high-performance routing
- [go.uber.org/zap](https://github.com/uber-go/zap) v1.27.0 or higher for structured logging
- [github.com/google/uuid](https://github.com/google/uuid) v1.6.0 or higher for trace ID generation
- [github.com/prometheus/client_golang](https://github.com/prometheus/client_golang) v1.21.1 or higher for Prometheus metrics (optional)
- [go.uber.org/ratelimit](https://github.com/uber-go/ratelimit) v0.3.1 or higher for rate limiting (optional)

All dependencies are properly documented with Go modules and will be automatically installed when you run `go get github.com/Suhaibinator/SRouter`.

## Getting Started

### Basic Usage

Here's a simple example of how to use SRouter:

```go
package main

import (
 "fmt"
 "log"
 "net/http"
 "time"

 "github.com/Suhaibinator/SRouter/pkg/router"
 "go.uber.org/zap"
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
 }

// Define the auth function that takes a context and token and returns a string and a boolean
// Note: The router's auth function includes context, but middleware auth functions don't
authFunction := func(ctx context.Context, token string) (string, bool) {
 // This is a simple example, so we'll just validate that the token is not empty
 if token != "" {
  return token, true
 }
 return "", false
}

 // Define the function to get the user ID from a string
 userIdFromUserFunction := func(user string) string {
  // In this example, we're using the string itself as the ID
  return user
 }

 // Create a router with string as both the user ID and user type
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

Sub-routers allow you to group routes with a common path prefix and apply shared configuration. They support both regular routes and generic routes:

```go
// Create a router with sub-routers
routerConfig := router.RouterConfig{
 Logger:            logger,
 GlobalTimeout:     2 * time.Second,
 GlobalMaxBodySize: 1 << 20, // 1 MB
 SubRouters: []router.SubRouterConfig{
  {
   PathPrefix:          "/api/v1",
   TimeoutOverride:     3 * time.Second,
   MaxBodySizeOverride: 2 << 20, // 2 MB
   Routes: []router.RouteConfigBase{
    {
     Path:    "/users",
     Methods: []string{"GET"},
     Handler: ListUsersHandler,
    },
    {
     Path:    "/users/:id",
     Methods: []string{"GET"},
     Handler: GetUserHandler,
    },
   },
  },
  {
   PathPrefix: "/api/v2",
   Routes: []router.RouteConfigBase{
    {
     Path:    "/users",
     Methods: []string{"GET"},
     Handler: ListUsersV2Handler,
    },
   },
  },
 },
}
```

#### Registering Generic Routes with SubRouters

You can register generic routes with sub-routers in two ways:

1. Using the `RegisterGenericRouteWithSubRouter` function:

```go
// Create a sub-router
subRouter := router.SubRouterConfig{
 PathPrefix: "/api/v1",
}

// Register a generic route with the sub-router
router.RegisterGenericRouteWithSubRouter[CreateUserReq, CreateUserResp, string, string](
 &subRouter,
 router.RouteConfig[CreateUserReq, CreateUserResp]{
  Path:      "/users",
  Methods:   []string{"POST"},
  AuthLevel: router.AuthRequired,
  Codec:     codec.NewJSONCodec[CreateUserReq, CreateUserResp](),
  Handler:   CreateUserHandler,
 },
)

// Register the sub-router with the router
r.RegisterSubRouter(subRouter)
```

2. Using the `CreateGenericRouteForSubRouter` function and adding it to the `GenericRoutes` field:

```go
// Create a sub-router
subRouter := router.SubRouterConfig{
 PathPrefix: "/api/v1",
}

// Create a GenericRouteConfigs to hold multiple generic routes
var routes router.GenericRouteConfigs

// Create a generic route for the sub-router
userRoute := router.CreateGenericRouteForSubRouter[CreateUserReq, CreateUserResp, string, string](
 router.RouteConfig[CreateUserReq, CreateUserResp]{
  Path:      "/users",
  Methods:   []string{"POST"},
  AuthLevel: router.AuthRequired,
  Codec:     codec.NewJSONCodec[CreateUserReq, CreateUserResp](),
  Handler:   CreateUserHandler,
 },
)

// Add the generic route to the GenericRouteConfigs
routes = append(routes, userRoute)

// Set the GenericRoutes field of the sub-router
subRouter.GenericRoutes = routes

// Register the sub-router with the router
r.RegisterSubRouter(subRouter)
```

#### Nested SubRouters

You can also nest sub-routers to create a hierarchical routing structure:

```go
// Create a main API sub-router
apiSubRouter := router.SubRouterConfig{
 PathPrefix: "/api",
}

// Create a v1 sub-router
apiV1SubRouter := router.SubRouterConfig{
 PathPrefix: "/v1",
}

// Create a users sub-router under v1
usersV1SubRouter := router.SubRouterConfig{
 PathPrefix: "/users",
 Routes: []router.RouteConfigBase{
  {
   Path:      "",
   Methods:   []string{"GET"},
   AuthLevel: router.NoAuth,
   Handler:   ListUsersHandler,
  },
 },
}

// Add the users sub-router to the v1 sub-router
router.RegisterSubRouterWithSubRouter(&apiV1SubRouter, usersV1SubRouter)

// Add the v1 sub-router to the main API sub-router
router.RegisterSubRouterWithSubRouter(&apiSubRouter, apiV1SubRouter)

// Register the main API sub-router with the router
r.RegisterSubRouter(apiSubRouter)
```

This creates routes like `/api/v1/users` with proper path prefixing.

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

// Register the generic route
router.RegisterGenericRoute[CreateUserReq, CreateUserResp, string](r, router.RouteConfig[CreateUserReq, CreateUserResp]{
 Path:        "/api/users",
 Methods:     []string{"POST"},
 AuthLevel:   router.AuthRequired,
 Codec:       codec.NewJSONCodec[CreateUserReq, CreateUserResp](),
 Handler:     CreateUserHandler,
})
```

Note that the `RegisterGenericRoute` function takes three type parameters: the request type, the response type, and the user type. The user type should match the second type parameter of your router.

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

r := router.NewRouter[string, User](config, authFunction, getUserIDFunction)
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

r := router.NewRouter[string, User](config, authFunction, getUserIDFunction)
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
    GlobalRateLimit: &middleware.RateLimitConfig{
        BucketName: "global",
        Limit:      100,
        Window:     time.Minute,
        Strategy:   "ip",
    },
}

// Create a sub-router with rate limiting
subRouter := router.SubRouterConfig{
    PathPrefix: "/api/v1",
    RateLimitOverride: &middleware.RateLimitConfig{
        BucketName: "api-v1",
        Limit:      50,
        Window:     time.Minute,
        Strategy:   "ip",
    },
    // ... other config
}

// Create a route with rate limiting
route := router.RouteConfigBase{
    Path:    "/users",
    Methods: []string{"POST"},
    RateLimit: &middleware.RateLimitConfig{
        BucketName: "create-user",
        Limit:      10,
        Window:     time.Minute,
        Strategy:   "ip",
    },
    // ... other config
}
```

#### Rate Limiting Strategies

SRouter supports several rate limiting strategies:

1. **IP-based Rate Limiting**: Limits requests based on the client's IP address (extracted according to the IP configuration). The rate limiting is smooth, distributing requests evenly across the time window rather than allowing bursts.

```go
RateLimit: &middleware.RateLimitConfig{
    BucketName: "ip-based",
    Limit:      100,
    Window:     time.Minute,
    Strategy:   "ip",
}
```

2. **User-based Rate Limiting**: Limits requests based on the authenticated user.

```go
RateLimit: &middleware.RateLimitConfig{
    BucketName: "user-based",
    Limit:      50,
    Window:     time.Minute,
    Strategy:   "user",
}
```

3. **Custom Rate Limiting**: Limits requests based on custom criteria.

```go
RateLimit: &middleware.RateLimitConfig{
    BucketName: "custom",
    Limit:      20,
    Window:     time.Minute,
    Strategy:   "custom",
    KeyExtractor: func(r *http.Request) (string, error) {
        // Extract API key from query parameter
        apiKey := r.URL.Query().Get("api_key")
        if apiKey == "" {
            // Fall back to IP if no API key is provided
            return r.RemoteAddr, nil
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
    RateLimit: &middleware.RateLimitConfig{
        BucketName: "auth-endpoints", // Shared bucket name
        Limit:      5,
        Window:     time.Minute,
        Strategy:   "ip",
    },
    // ... other config
}

// Register endpoint
registerRoute := router.RouteConfigBase{
    Path:    "/register",
    Methods: []string{"POST"},
    RateLimit: &middleware.RateLimitConfig{
        BucketName: "auth-endpoints", // Same bucket name as login
        Limit:      5,
        Window:     time.Minute,
        Strategy:   "ip",
    },
    // ... other config
}
```

#### Custom Rate Limit Responses

You can customize the response sent when a rate limit is exceeded:

```go
RateLimit: &middleware.RateLimitConfig{
    BucketName: "custom-response",
    Limit:      10,
    Window:     time.Minute,
    Strategy:   "ip",
    ExceededHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        w.WriteHeader(http.StatusTooManyRequests)
        w.Write([]byte(`{"error":"Rate limit exceeded","message":"Please try again later"}`))
    }),
}
```

See the `examples/rate-limiting` directory for a complete example of rate limiting.

### Authentication

SRouter provides a flexible authentication system with three authentication levels and two authentication approaches.

#### Authentication Levels

SRouter supports three authentication levels:

1. **NoAuth**: No authentication is required. The route is accessible without any authentication.
2. **AuthOptional**: Authentication is optional. If authentication credentials are provided, they will be validated and the user will be added to the request context if valid. If no credentials are provided or they are invalid, the request will still proceed without a user in the context.
3. **AuthRequired**: Authentication is required. If authentication fails, the request will be rejected with a 401 Unauthorized response. If authentication succeeds, the user will be added to the request context.

You can specify the authentication level for a route using the `AuthLevel` field in the route configuration:

```go
// Create a router configuration with different authentication levels
routerConfig := router.RouterConfig{
    // ...
    SubRouters: []router.SubRouterConfig{
        {
            PathPrefix: "/api",
            Routes: []router.RouteConfigBase{
                {
                    Path:      "/public",
                    Methods:   []string{"GET"},
                    AuthLevel: router.NoAuth,
                    Handler:   PublicHandler,
                },
                {
                    Path:      "/optional",
                    Methods:   []string{"GET"},
                    AuthLevel: router.AuthOptional,
                    Handler:   OptionalAuthHandler,
                },
                {
                    Path:      "/protected",
                    Methods:   []string{"GET"},
                    AuthLevel: router.AuthRequired,
                    Handler:   ProtectedHandler,
                },
            },
        },
    },
}
```

See the `examples/auth-levels` directory for a complete example of authentication levels.

### Context Management

SRouter uses a structured approach to context management using the `SRouterContext` wrapper. This approach avoids deep nesting of context values by storing all values in a single wrapper structure.

#### SRouterContext Wrapper

The `SRouterContext` wrapper is a generic type that holds all values that SRouter adds to request contexts:

```go
type SRouterContext[T comparable, U any] struct {
    // User ID and User object storage
    UserID T
    User   *U
    
    // Client IP address
    ClientIP string
    
    // Track which fields are set
    UserIDSet   bool
    UserSet     bool
    ClientIPSet bool
    
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
   ctx := WithUserID[T, U](r.Context(), userID)
   ctx = WithUser[T, U](ctx, user)
   ```

2. **Type Safety**: The generic type parameters ensure proper type handling without the need for type assertions.

3. **Extensibility**: New fields can be added to the `SRouterContext` struct without creating more nested contexts.

4. **Organization**: Related context values are grouped logically, making the code more maintainable.

#### Accessing Context Values

SRouter provides several helper functions for accessing context values:

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
traceID := middleware.GetTraceID(r)
```

These functions automatically handle the type parameters and provide a clean, consistent interface for accessing context values.

#### Authentication Approaches

SRouter provides two approaches to authentication:

##### Boolean Authentication

The simplest approach is to use a function that returns a boolean indicating whether authentication was successful:

```go
// Create a custom authentication function
func customAuth(r *http.Request) bool {
 // Get the token from the Authorization header
 token := r.Header.Get("Authorization")
 if token == "" {
  return false
 }
 
 // Remove the "Bearer " prefix if present
 token = strings.TrimPrefix(token, "Bearer ")
 
 // Validate the token (e.g., verify JWT, check against database, etc.)
 return validateToken(token)
}

// Create a middleware that uses the custom authentication function
authMiddleware := middleware.Authentication(customAuth)

// Apply the middleware to a route
r.RegisterRoute(router.RouteConfigBase{
 Path:        "/protected",
 Methods:     []string{"GET"},
 AuthLevel:   router.AuthRequired,
 Handler:     ProtectedHandler,
 Middlewares: []common.Middleware{
  authMiddleware,
 },
})
```

##### User-Returning Authentication

For more advanced use cases, you can use a function that returns a user object and an error. This allows you to:

1. Get detailed user information during authentication
2. Store the user in the request context for use in handlers
3. Implement fine-grained authorization based on user roles or permissions

```go
// Define your User type
type User struct {
 ID    string
 Name  string
 Email string
 Roles []string
}

// Create a custom authentication function that returns a User
func customUserAuth(r *http.Request) (*User, error) {
 // Get the token from the Authorization header
 token := r.Header.Get("Authorization")
 if token == "" {
  return nil, errors.New("no authorization header")
 }
 
 // Remove the "Bearer " prefix if present
 token = strings.TrimPrefix(token, "Bearer ")
 
 // Validate the token and retrieve the user
 user, err := validateTokenAndGetUser(token)
 if err != nil {
  return nil, err
 }
 
 return user, nil
}

// Create a middleware that uses the custom authentication function
authMiddleware := middleware.AuthenticationWithUser[User](customUserAuth)

// Apply the middleware to a route
r.RegisterRoute(router.RouteConfigBase{
 Path:        "/protected",
 Methods:     []string{"GET"},
 AuthLevel:   router.AuthRequired,
 Middlewares: []common.Middleware{
  authMiddleware,
 },
 Handler: func(w http.ResponseWriter, r *http.Request) {
  // Get the user from the context
  user := middleware.GetUser[User](r)
  if user == nil {
   http.Error(w, "User not found in context", http.StatusInternalServerError)
   return
  }
  
  // Use the user object
  fmt.Fprintf(w, "Hello, %s!", user.Name)
 },
})
```

SRouter provides several pre-built user authentication providers:

```go
// Basic Authentication with User
middleware.NewBasicAuthWithUserMiddleware[User](
 func(username, password string) (*User, error) {
  // Validate credentials and return user
  if username == "user1" && password == "password1" {
   return &User{
    ID:    "1",
    Name:  "User One",
    Email: "user1@example.com",
    Roles: []string{"user"},
   }, nil
  }
  return nil, errors.New("invalid credentials")
 },
 logger,
)

// Bearer Token Authentication with User
middleware.NewBearerTokenWithUserMiddleware[User](
 func(token string) (*User, error) {
  // Validate token and return user
  if token == "valid-token" {
   return &User{
    ID:    "1",
    Name:  "User One",
    Email: "user1@example.com",
    Roles: []string{"user"},
   }, nil
  }
  return nil, errors.New("invalid token")
 },
 logger,
)

// API Key Authentication with User
middleware.NewAPIKeyWithUserMiddleware[User](
 func(key string) (*User, error) {
  // Validate API key and return user
  if key == "valid-key" {
   return &User{
    ID:    "1",
    Name:  "User One",
    Email: "user1@example.com",
    Roles: []string{"user"},
   }, nil
  }
  return nil, errors.New("invalid API key")
 },
 "X-API-Key",
 "api_key",
 logger,
)
```

See the `examples/user-auth` directory for a complete example of user-returning authentication and the `examples/auth` directory for a complete example of basic authentication.

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
func RequestID() common.Middleware {
 return func(next http.Handler) http.Handler {
  return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
   // Generate a request ID
   requestID := uuid.New().String()
   
   // Add it to the context
   ctx := context.WithValue(r.Context(), "request_id", requestID)
   
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
  RequestID(),
  middleware.Logging(logger),
 },
 // ...
}
```

See the `examples/middleware` directory for examples of custom middleware.

### Source Types

SRouter provides flexible ways to retrieve and decode request data beyond just the request body. This is particularly useful for scenarios where you need to pass data in URLs or when working with clients that have limitations on request body usage.

#### Available Source Types

SRouter supports the following source types:

1. **Body** (default): Retrieves data from the request body.

   ```go
   router.RegisterGenericRoute[UserRequest, UserResponse, string](r, router.RouteConfig[UserRequest, UserResponse]{
       // ...
       // SourceType defaults to Body if not specified
   })
   ```

2. **Base64QueryParameter**: Retrieves data from a base64-encoded query parameter.

   ```go
   router.RegisterGenericRoute[UserRequest, UserResponse, string](r, router.RouteConfig[UserRequest, UserResponse]{
       // ...
       SourceType: router.Base64QueryParameter,
       SourceKey:  "data", // Will look for ?data=base64encodedstring
   })
   ```

3. **Base62QueryParameter**: Retrieves data from a base62-encoded query parameter.

   ```go
   router.RegisterGenericRoute[UserRequest, UserResponse, string](r, router.RouteConfig[UserRequest, UserResponse]{
       // ...
       SourceType: router.Base62QueryParameter,
       SourceKey:  "data", // Will look for ?data=base62encodedstring
   })
   ```

4. **Base64PathParameter**: Retrieves data from a base64-encoded path parameter.

   ```go
   router.RegisterGenericRoute[UserRequest, UserResponse, string](r, router.RouteConfig[UserRequest, UserResponse]{
       Path:       "/users/:data",
       // ...
       SourceType: router.Base64PathParameter,
       SourceKey:  "data", // Will use the :data path parameter
   })
   ```

5. **Base62PathParameter**: Retrieves data from a base62-encoded path parameter.

   ```go
   router.RegisterGenericRoute[UserRequest, UserResponse, string](r, router.RouteConfig[UserRequest, UserResponse]{
       Path:       "/users/:data",
       // ...
       SourceType: router.Base62PathParameter,
       SourceKey:  "data", // Will use the :data path parameter
   })
   ```

#### Example Usage

Here's a complete example of using different source types:

```go
package main

import (
 "context"
 "encoding/base64"
 "encoding/json"
 "fmt"
 "log"
 "net/http"
 "time"

 "github.com/Suhaibinator/SRouter/pkg/codec"
 "github.com/Suhaibinator/SRouter/pkg/router"
 "go.uber.org/zap"
)

// Define request and response types
type UserRequest struct {
 ID   string `json:"id"`
 Name string `json:"name"`
}

type UserResponse struct {
 Message string `json:"message"`
 ID      string `json:"id"`
 Name    string `json:"name"`
}

// Define a handler
func UserHandler(r *http.Request, req UserRequest) (UserResponse, error) {
 return UserResponse{
  Message: "Hello, " + req.Name + "!",
  ID:      req.ID,
  Name:    req.Name,
 }, nil
}

func main() {
 // Create a logger
 logger, _ := zap.NewProduction()
 defer logger.Sync()

 // Create a router configuration
 routerConfig := router.RouterConfig{
  Logger:            logger,
  GlobalTimeout:     2 * time.Second,
  GlobalMaxBodySize: 1 << 20, // 1 MB
 }

 // Define auth and user ID functions
 authFunction := func(ctx context.Context, token string) (string, bool) {
  return token, token != ""
 }

 userIdFromUserFunction := func(user string) string {
  return user
 }

 // Create a router
 r := router.NewRouter[string, string](routerConfig, authFunction, userIdFromUserFunction)

 // Register routes with different source types

 // 1. Standard body-based route (default)
 router.RegisterGenericRoute[UserRequest, UserResponse, string](r, router.RouteConfig[UserRequest, UserResponse]{
  Path:    "/users/body/:id",
  Methods: []string{"POST"},
  Codec:   codec.NewJSONCodec[UserRequest, UserResponse](),
  Handler: UserHandler,
  // SourceType defaults to Body
 })

 // 2. Base64 query parameter route
 router.RegisterGenericRoute[UserRequest, UserResponse, string](r, router.RouteConfig[UserRequest, UserResponse]{
  Path:       "/users/query/:id",
  Methods:    []string{"GET"},
  Codec:      codec.NewJSONCodec[UserRequest, UserResponse](),
  Handler:    UserHandler,
  SourceType: router.Base64QueryParameter,
  SourceKey:  "data", // Will look for ?data=base64encodedstring
 })

 // 3. Base64 path parameter route
 router.RegisterGenericRoute[UserRequest, UserResponse, string](r, router.RouteConfig[UserRequest, UserResponse]{
  Path:       "/users/path/:data",
  Methods:    []string{"GET"},
  Codec:      codec.NewJSONCodec[UserRequest, UserResponse](),
  Handler:    UserHandler,
  SourceType: router.Base64PathParameter,
  SourceKey:  "data", // Will use the :data path parameter
 })

 // Start the server
 fmt.Println("Server listening on :8080")
 
 // Example of how to create a base64-encoded request
 reqData := UserRequest{ID: "123", Name: "John"}
 jsonBytes, _ := json.Marshal(reqData)
 base64Data := base64.StdEncoding.EncodeToString(jsonBytes)
 
 fmt.Println("Example curl commands:")
 fmt.Println("  curl -X POST -H \"Content-Type: application/json\" -d '{\"id\":\"123\",\"name\":\"John\"}' http://localhost:8080/users/body/123")
 fmt.Printf("  curl -X GET \"http://localhost:8080/users/query/123?data=%s\"\n", base64Data)
 fmt.Printf("  curl -X GET http://localhost:8080/users/path/%s\n", base64Data)
 
 log.Fatal(http.ListenAndServe(":8080", r))
}
```

#### When to Use Different Source Types

- **Body** (default): Use for standard API requests where the client can send a request body.
- **Base64QueryParameter**: Use when you need to include structured data in a GET request, or when working with clients that can't easily send request bodies.
- **Base62QueryParameter**: Similar to Base64QueryParameter, but uses only alphanumeric characters, making it safer for URLs.
- **Base64PathParameter**: Use when you want to include structured data as part of the URL path.
- **Base62PathParameter**: Similar to Base64PathParameter, but uses only alphanumeric characters, making it safer for URLs.

#### Error Handling

SRouter provides appropriate error handling for each source type:

- Missing parameters return 400 Bad Request with a descriptive message
- Decoding errors return 400 Bad Request with details about the failure
- Unmarshaling errors return 400 Bad Request with information about the issue

### Response Caching

SRouter provides built-in caching support for query and path parameter-based routes. This is particularly useful for improving performance of read-heavy APIs or when working with expensive operations.

#### Configuring Caching

To enable caching, you need to:

1. Provide cache functions in the router configuration
2. Enable caching for specific routes

```go
// Create a simple in-memory cache
type InMemoryCache struct {
 cache map[string][]byte
 mu    sync.RWMutex
}

func NewInMemoryCache() *InMemoryCache {
 return &InMemoryCache{
  cache: make(map[string][]byte),
 }
}

func (c *InMemoryCache) Get(key string) ([]byte, bool) {
 c.mu.RLock()
 defer c.mu.RUnlock()
 value, found := c.cache[key]
 return value, found
}

func (c *InMemoryCache) Set(key string, value []byte) error {
 c.mu.Lock()
 defer c.mu.Unlock()
 c.cache[key] = value
 return nil
}

// Create a cache instance
cache := NewInMemoryCache()

// Configure the router with cache functions
routerConfig := router.RouterConfig{
 // ... other config
 CacheGet: cache.Get,
 CacheSet: cache.Set,
 CacheKeyPrefix: "global:", // Global prefix for all cache keys
}

// Create a router
r := router.NewRouter[string, string](routerConfig, authFunction, userIdFromUserFunction)

// Register routes with caching enabled
router.RegisterGenericRoute[UserRequest, UserResponse, string](r, router.RouteConfig[UserRequest, UserResponse]{
 Path:          "/users/query",
 Methods:       []string{"GET"},
 Codec:         jsonCodec,
 Handler:       getUserHandler,
 SourceType:    router.Base64QueryParameter,
 SourceKey:     "data",
 CacheResponse: true, // Enable caching for this route
 CacheKeyPrefix: "users:", // Route-specific prefix (will override global prefix)
})

// Create a sub-router with caching enabled for all routes
subRouter := router.SubRouterConfig{
 PathPrefix:     "/api/v1",
 CacheResponse:  true, // Enable caching for all routes in this sub-router
 CacheKeyPrefix: "api-v1:", // Sub-router specific prefix
 Routes: []router.RouteConfigBase{
  // Routes will inherit caching settings from the sub-router
 },
}
```

#### Cache Key Prefix Hierarchy

SRouter uses a hierarchical approach to cache key prefixes:

1. **Route-specific prefix**: If a route has a `CacheKeyPrefix` set, it will be used.
2. **Sub-router prefix**: If a route is part of a sub-router with a `CacheKeyPrefix` set, and the route doesn't have its own prefix, the sub-router's prefix will be used.
3. **Global prefix**: If neither the route nor its sub-router has a prefix, the global `CacheKeyPrefix` from the router configuration will be used.

This hierarchy allows you to:

- Set a global prefix for all cached responses
- Override the global prefix for a group of related routes using a sub-router
- Override both the global and sub-router prefixes for specific routes

For example:

```go
// Global prefix: "global:"
routerConfig := router.RouterConfig{
 // ... other config
 CacheGet: cache.Get,
 CacheSet: cache.Set,
 CacheKeyPrefix: "global:",
 SubRouters: []router.SubRouterConfig{
  {
   PathPrefix: "/api/v1",
   CacheResponse: true,
   CacheKeyPrefix: "api-v1:", // Override global prefix for this sub-router
   Routes: []router.RouteConfigBase{
    // These routes will use "api-v1:" as their cache key prefix
   },
  },
  {
   PathPrefix: "/api/v2",
   CacheResponse: true,
   // No CacheKeyPrefix, will use global prefix "global:"
   Routes: []router.RouteConfigBase{
    // These routes will use "global:" as their cache key prefix
   },
  },
 },
}

// Route-specific prefix: "users:"
router.RegisterGenericRoute[UserRequest, UserResponse, string](r, router.RouteConfig[UserRequest, UserResponse]{
 Path: "/api/v1/users/query",
 // ... other config
 CacheResponse: true,
 CacheKeyPrefix: "users:", // Override sub-router prefix for this route
})

// No route-specific prefix, will use sub-router prefix "api-v1:"
router.RegisterGenericRoute[UserRequest, UserResponse, string](r, router.RouteConfig[UserRequest, UserResponse]{
 Path: "/api/v1/products/query",
 // ... other config
 CacheResponse: true,
 // No CacheKeyPrefix, will use sub-router prefix "api-v1:"
})
```

This approach helps you organize your cache keys and avoid collisions between different parts of your application.

#### Caching Behavior

- Caching is only supported for routes that use query or path parameters as input sources (not for body-based routes)
- The cache key is the raw encoded value from the query or path parameter, without any decoding
- You can configure a CacheKeyPrefix at the router, sub-router, or route level to avoid cache collisions
- Caching happens before the codec level, so if there's a cache hit, the request doesn't even reach your handler
- If there's a cache miss, the request is processed normally and the response is cached before being sent

#### Cache Metrics

When metrics are enabled, SRouter automatically collects the following cache-related metrics:

- **Cache Hits**: Number of cache hits
- **Cache Misses**: Number of cache misses
- **Cache Hit Ratio**: Ratio of cache hits to total cache lookups

These metrics can be accessed through your metrics system (e.g., Prometheus) to monitor cache performance.

#### Using Different Cache Implementations

You can use any cache implementation by providing functions that match the following signatures:

```go
CacheGet: func(string) ([]byte, bool)
CacheSet: func(string, []byte) error
```

This allows you to use various caching solutions:

- In-memory caches like the one shown above
- Distributed caches like Redis or Memcached
- Persistent caches backed by a database
- Tiered caching systems with multiple layers

For example, to use Redis:

```go
import "github.com/go-redis/redis/v8"

// Create a Redis client
rdb := redis.NewClient(&redis.Options{
 Addr: "localhost:6379",
})

// Configure the router with Redis cache functions
routerConfig := router.RouterConfig{
 // ... other config
 CacheGet: func(key string) ([]byte, bool) {
  val, err := rdb.Get(context.Background(), key).Bytes()
  if err != nil {
   return nil, false
  }
  return val, true
 },
 CacheSet: func(key string, value []byte) error {
  return rdb.Set(context.Background(), key, value, time.Hour).Err()
 },
}
```

See the `examples/caching` directory for a complete example of response caching.

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
router.RegisterGenericRoute[CreateUserReq, CreateUserResp, string](r, router.RouteConfig[CreateUserReq, CreateUserResp]{
 Path:        "/api/users",
 Methods:     []string{"POST"},
 AuthLevel:   router.NoAuth, // No authentication required
 Codec:       NewXMLCodec[CreateUserReq, CreateUserResp](),
 Handler:     CreateUserHandler,
})
```

### Metrics

SRouter provides a flexible, interface-based metrics system that allows you to use your preferred metrics implementation while the framework handles the collection and aggregation of metrics.

#### Interface-Based Metrics System

The metrics system is built around a set of interfaces that define the contract between the framework and the metrics implementation:

- `MetricsRegistry`: Registry for creating and managing metrics
- `MetricsExporter`: Exporter for exposing metrics to monitoring systems
- `MetricsMiddleware`: Middleware for collecting metrics from HTTP requests
- `Metric`: Base interface for all metrics (Counter, Gauge, Histogram, Summary)

This approach allows you to:

1. **Use any metrics implementation**: Implement the interfaces with your preferred metrics library
2. **Customize metrics collection**: Control how metrics are collected and what metrics are exposed
3. **Integrate with existing systems**: Easily integrate with your existing monitoring infrastructure

#### Using Prometheus Metrics

Here's an example of using Prometheus metrics with SRouter:

```go
package main

import (
 "context"
 "fmt"
 "log"
 "net/http"
 "time"

 "github.com/Suhaibinator/SRouter/pkg/metrics"
 "github.com/Suhaibinator/SRouter/pkg/router"
 "github.com/prometheus/client_golang/prometheus"
 "github.com/prometheus/client_golang/prometheus/promhttp"
 "go.uber.org/zap"
)

// PrometheusRegistry implements the metrics.MetricsRegistry interface
type PrometheusRegistry struct {
 registry *prometheus.Registry
}

func NewPrometheusRegistry() *PrometheusRegistry {
 return &PrometheusRegistry{
  registry: prometheus.NewRegistry(),
 }
}

// Implement the metrics.MetricsRegistry interface methods...

// Create a router configuration with metrics enabled
routerConfig := router.RouterConfig{
 Logger:            logger,
 GlobalTimeout:     2 * time.Second,
 GlobalMaxBodySize: 1 << 20, // 1 MB
 EnableMetrics:     true,
 MetricsConfig: &router.MetricsConfig{
  Collector:        registry, // Your implementation of metrics.MetricsRegistry
  Namespace:        "myapp",
  Subsystem:        "api",
  EnableLatency:    true,
  EnableThroughput: true,
  EnableQPS:        true,
  EnableErrors:     true,
 },
 // ...
}

// Create a router
r := router.NewRouter[string, string](routerConfig, authFunction, userIdFromUserFunction)

// Create a metrics handler
metricsHandler := registry.Handler() // Your implementation of metrics.MetricsExporter.Handler()

// Create a mux to handle both the API and metrics endpoints
mux := http.NewServeMux()
mux.Handle("/metrics", metricsHandler)
mux.Handle("/", r)

// Start the server
http.ListenAndServe(":8080", mux)
```

See the `examples/prometheus` directory for a complete example of Prometheus metrics.

#### Creating Your Own Metrics Implementation

You can create your own metrics implementation by implementing the interfaces defined in the `metrics` package:

```go
// Create a custom metrics registry
type CustomRegistry struct {
 // Your implementation details
}

// Implement the metrics.MetricsRegistry interface
func (r *CustomRegistry) Register(metric metrics.Metric) error {
 // Your implementation
}

func (r *CustomRegistry) Get(name string) (metrics.Metric, bool) {
 // Your implementation
}

func (r *CustomRegistry) Unregister(name string) bool {
 // Your implementation
}

func (r *CustomRegistry) Clear() {
 // Your implementation
}

func (r *CustomRegistry) Snapshot() metrics.MetricsSnapshot {
 // Your implementation
}

func (r *CustomRegistry) WithTags(tags metrics.Tags) metrics.MetricsRegistry {
 // Your implementation
}

func (r *CustomRegistry) NewCounter() metrics.CounterBuilder {
 // Your implementation
}

func (r *CustomRegistry) NewGauge() metrics.GaugeBuilder {
 // Your implementation
}

func (r *CustomRegistry) NewHistogram() metrics.HistogramBuilder {
 // Your implementation
}

func (r *CustomRegistry) NewSummary() metrics.SummaryBuilder {
 // Your implementation
}

// Create a custom metrics exporter
type CustomExporter struct {
 // Your implementation details
}

// Implement the metrics.MetricsExporter interface
func (e *CustomExporter) Export(snapshot metrics.MetricsSnapshot) error {
 // Your implementation
}

func (e *CustomExporter) Start() error {
 // Your implementation
}

func (e *CustomExporter) Stop() error {
 // Your implementation
}

func (e *CustomExporter) Handler() http.Handler {
 // Your implementation
}
```

#### Using the Metrics Middleware

The metrics middleware is automatically added to the router when metrics are enabled. It collects the following metrics:

- **Latency**: Request latency in seconds
- **Throughput**: Request throughput in bytes
- **QPS**: Queries per second
- **Errors**: Request errors by status code

You can configure which metrics are collected using the `MetricsConfig`:

```go
routerConfig := router.RouterConfig{
 // ...
 EnableMetrics: true,
 MetricsConfig: &router.MetricsConfig{
  Collector:        registry,
  Namespace:        "myapp",
  Subsystem:        "api",
  EnableLatency:    true,  // Enable latency metrics
  EnableThroughput: true,  // Enable throughput metrics
  EnableQPS:        true,  // Enable QPS metrics
  EnableErrors:     true,  // Enable error metrics
 },
 // ...
}
```

#### Customizing Metrics Collection

You can customize metrics collection by implementing the `MetricsFilter` and `MetricsSampler` interfaces:

```go
// Create a custom metrics filter
type CustomFilter struct {
 // Your implementation details
}

// Implement the metrics.MetricsFilter interface
func (f *CustomFilter) Filter(r *http.Request) bool {
 // Return true if metrics should be collected for this request
 return r.URL.Path != "/health" // Don't collect metrics for health checks
}

// Create a custom metrics sampler
type CustomSampler struct {
 // Your implementation details
}

// Implement the metrics.MetricsSampler interface
func (s *CustomSampler) Sample() bool {
 // Return true if this request should be sampled
 return rand.Float64() < 0.1 // Sample 10% of requests
}

// Create a metrics middleware with the custom filter and sampler
middleware := metrics.NewMetricsMiddleware(registry, metrics.MetricsMiddlewareConfig{
 EnableLatency:    true,
 EnableThroughput: true,
 EnableQPS:        true,
 EnableErrors:     true,
}).WithFilter(&CustomFilter{}).WithSampler(&CustomSampler{})
```

See the `examples/custom-metrics` directory for a complete example of using custom metrics.

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
- **examples/subrouter-generic-routes**: An example of using generic routes with sub-routers
- **examples/nested-subrouters**: An example of nesting sub-routers for hierarchical routing
- **examples/trace-logging**: An example of using trace ID logging with SRouter
- **examples/caching**: An example of using response caching with SRouter

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
 EnableTracing      bool                                  // Enable distributed tracing
 EnableTraceID      bool                                  // Enable trace ID logging
 PrometheusConfig   *PrometheusConfig                     // Prometheus metrics configuration (optional, deprecated)
 MetricsConfig      *MetricsConfig                        // Metrics configuration (optional)
 SubRouters         []SubRouterConfig                     // Sub-routers with their own configurations
 Middlewares        []common.Middleware                   // Global middlewares applied to all routes
 AddUserObjectToCtx bool                                  // Add user object to context
 CacheGet           func(string) ([]byte, bool)           // Function to retrieve cached responses
 CacheSet           func(string, []byte) error            // Function to store responses in the cache
 CacheKeyPrefix     string                                // Prefix for cache keys to avoid collisions
}
```

### PrometheusConfig

```go
type PrometheusConfig struct {
 Registry         interface{} // Prometheus registry (prometheus.Registerer)
 Namespace        string      // Namespace for metrics
 Subsystem        string      // Subsystem for metrics
 EnableLatency    bool        // Enable latency metrics
 EnableThroughput bool        // Enable throughput metrics
 EnableQPS        bool        // Enable queries per second metrics
 EnableErrors     bool        // Enable error metrics
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
 Routes              []RouteConfigBase                     // Regular routes in this sub-router
 GenericRoutes       interface{}                           // Generic routes in this sub-router (can be a single GenericRouteRegistrar or GenericRouteConfigs)
 Middlewares         []common.Middleware                   // Middlewares applied to all routes in this sub-router
 CacheResponse       bool                                  // Enable caching for all routes in this sub-router
 CacheKeyPrefix      string                                // Prefix for cache keys to avoid collisions
}
```

### RouteConfigBase

```go
type RouteConfigBase struct {
 Path        string                                // Route path (will be prefixed with sub-router path prefix if applicable)
 Methods     []string                              // HTTP methods this route handles
 AuthLevel   AuthLevel                             // Authentication level for this route (NoAuth, AuthOptional, or AuthRequired)
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
 Path          string                                // Route path (will be prefixed with sub-router path prefix if applicable)
 Methods       []string                              // HTTP methods this route handles
 AuthLevel     AuthLevel                             // Authentication level for this route (NoAuth, AuthOptional, or AuthRequired)
 Timeout       time.Duration                         // Override timeout for this specific route
 MaxBodySize   int64                                 // Override max body size for this specific route
 RateLimit     *middleware.RateLimitConfig[any, any] // Rate limit for this specific route
 Codec         Codec[T, U]                           // Codec for marshaling/unmarshaling request and response
 Handler       GenericHandler[T, U]                  // Generic handler function
 Middlewares   []common.Middleware                   // Middlewares applied to this specific route
 SourceType    SourceType                            // How to retrieve request data (defaults to Body)
 SourceKey     string                                // Parameter name for query or path parameters
 CacheResponse bool                                  // Enable caching for this route (only works with query/path parameter source types)
 CacheKeyPrefix string                               // Prefix for cache keys to avoid collisions
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

SRouter provides several built-in middleware functions:

### Logging

Logs requests with method, path, status code, and duration:

```go
middleware.Logging(logger *zap.Logger) Middleware
```

### Recovery

Recovers from panics and returns a 500 Internal Server Error:

```go
middleware.Recovery(logger *zap.Logger) Middleware
```

### Authentication

SRouter provides several authentication middleware options:

#### Basic Authentication

```go
middleware.NewBasicAuthMiddleware(credentials map[string]string, logger *zap.Logger) Middleware
```

Example:

```go
// Create a middleware that uses basic authentication
authMiddleware := middleware.NewBasicAuthMiddleware(
    map[string]string{
        "user1": "password1",
        "user2": "password2",
    },
    logger,
)
```

#### Bearer Token Authentication

```go
middleware.NewBearerTokenMiddleware(validTokens map[string]bool, logger *zap.Logger) Middleware
```

Example:

```go
// Create a middleware that uses bearer token authentication
authMiddleware := middleware.NewBearerTokenMiddleware(
    map[string]bool{
        "token1": true,
        "token2": true,
    },
    logger,
)
```

#### Bearer Token with Validator

```go
middleware.NewBearerTokenValidatorMiddleware[T comparable](validator func(string) (T, bool), logger *zap.Logger) Middleware
```

Example:

```go
// Create a middleware that uses bearer token authentication with a validator function
authMiddleware := middleware.NewBearerTokenValidatorMiddleware(
    func(token string) (string, bool) {
        // Validate the token (e.g., verify JWT, check against database, etc.)
        if validateToken(token) {
            return token, true // Return the token as the user ID if valid
        }
        return "", false
    },
    logger,
)
```

#### API Key Authentication

```go
middleware.NewAPIKeyMiddleware(validKeys map[string]bool, header, query string, logger *zap.Logger) Middleware
```

Example:

```go
// Create a middleware that uses API key authentication
authMiddleware := middleware.NewAPIKeyMiddleware(
    map[string]bool{
        "key1": true,
        "key2": true,
    },
    "X-API-Key",  // Header name
    "api_key",    // Query parameter name
    logger,
)
```

#### Custom Authentication

```go
middleware.Authentication(authFunc func(*http.Request) bool) Middleware
```

Example:

```go
// Create a middleware that uses custom authentication
authMiddleware := middleware.Authentication(
    func(r *http.Request) bool {
        // Custom authentication logic
        return r.Header.Get("X-Custom-Auth") == "valid"
    },
)
```

### MaxBodySize

Limits the size of the request body:

```go
middleware.MaxBodySize(maxSize int64) Middleware
```

### Timeout

Sets a timeout for the request:

```go
middleware.Timeout(timeout time.Duration) Middleware
```

### CORS

Adds CORS headers to the response:

```go
middleware.CORS(origins []string, methods []string, headers []string) Middleware
```

### Chain

Chains multiple middlewares together:

```go
middleware.Chain(middlewares ...Middleware) Middleware
```

### Metrics Middleware

#### PrometheusMetrics (Deprecated)

Adds Prometheus metrics collection:

```go
middleware.PrometheusMetrics(
 registry interface{},
 namespace string,
 subsystem string,
 enableLatency bool,
 enableThroughput bool,
 enableQPS bool,
 enableErrors bool,
) Middleware
```

#### PrometheusHandler (Deprecated)

Creates a handler for exposing Prometheus metrics:

```go
middleware.PrometheusHandler(registry interface{}) http.Handler
```

#### Metrics Collector Interface

```go
type Collector interface {
 // Counter creates or retrieves a counter metric.
 Counter(name, help string, labelNames ...string) Counter

 // Gauge creates or retrieves a gauge metric.
 Gauge(name, help string, labelNames ...string) Gauge

 // Histogram creates or retrieves a histogram metric.
 Histogram(name, help string, buckets []float64, labelNames ...string) Histogram

 // Summary creates or retrieves a summary metric.
 Summary(name, help string, objectives map[float64]float64, maxAge, ageBuckets int, labelNames ...string) Summary
}
```

#### Metrics Exporter Interface

```go
type Exporter interface {
 // Handler returns an HTTP handler for exposing metrics.
 Handler() http.Handler
}
```

#### Metrics Middleware Factory Interface

```go
type MiddlewareFactory interface {
 // CreateMiddleware creates a middleware for collecting metrics.
 CreateMiddleware() func(http.Handler) http.Handler
}
```

### TraceMiddleware

Adds trace ID to the request context:

```go
middleware.TraceMiddleware() Middleware
```

## Codec Reference

SRouter provides two built-in codecs:

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

You can create your own codecs by implementing the `Codec` interface:

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

The SRouter framework uses a structured approach for retrieving information from request contexts. You should use these middleware package functions for context-related operations:

```go
// Get the user ID from the request
userID, ok := middleware.GetUserIDFromRequest[T, U](r)

// Get the user object from the request
user, ok := middleware.GetUserFromRequest[T, U](r)

// Get the client IP from the request
ip, ok := middleware.GetClientIPFromRequest[T, U](r)

// Get a flag from the request
flagValue, ok := middleware.GetFlagFromRequest[T, U](r, "flagName")

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

The order of middlewares matters. Middlewares are applied in reverse order, so the first middleware in the list is the outermost (last to execute before the request, first to execute after the response).

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
