# Custom Middleware

Middleware provides a powerful way to inject logic into the request/response cycle, handling concerns like logging, authentication, rate limiting, compression, CORS, and more, separate from your core request handlers.

SRouter uses the standard Go `http.Handler` interface for middleware, often defined using the `common.Middleware` type alias for clarity.

```go
// Defined in pkg/common/types.go
package common

import "net/http"

// Middleware is a function that takes an http.Handler and returns an http.Handler.
type Middleware func(http.Handler) http.Handler
```

A middleware function wraps an existing `http.Handler` (the `next` handler in the chain) and returns a new `http.Handler` that performs some action before or after calling the `next` handler.

## Creating Custom Middleware

Here's an example of a simple custom middleware that adds a custom header to the response:

```go
package mymiddleware

import (
	"net/http"
	"github.com/Suhaibinator/SRouter/pkg/common" // Assuming common types are here
)

// AddHeaderMiddleware adds a static header to every response.
func AddHeaderMiddleware(key, value string) common.Middleware {
	// Return the actual middleware function
	return func(next http.Handler) http.Handler {
		// Return the handler that performs the action
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Action before calling the next handler (optional)
			// fmt.Println("Adding header...")

			// Add the header to the response writer
			w.Header().Set(key, value)

			// Call the next handler in the chain
			next.ServeHTTP(w, r)

			// Action after calling the next handler (optional)
			// fmt.Println("Header added.")
		})
	}
}
```

Another example: A middleware that logs the User ID if present in the context.

```go
package mymiddleware

import (
	"fmt"
	"net/http"
	"github.com/Suhaibinator/SRouter/pkg/common"
	"github.com/Suhaibinator/SRouter/pkg/scontext" // Use scontext for context helpers
	"go.uber.org/zap"                              // Example logger
)

// LogUserIDMiddleware logs the user ID if authentication was successful.
// Requires an authentication middleware to run first.
// This example shows accessing UserID, but other context values (TraceID, ClientIP, Transaction, Flags)
// can be accessed similarly using their respective GetXFromRequest functions.
func LogUserIDMiddleware(logger *zap.Logger) common.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Attempt to get User ID from context
			// Replace string, any with your router's actual UserIDType, UserObjectType
			userID, ok := scontext.GetUserIDFromRequest[string, any](r)
			// txInterface, txOK := scontext.GetTransactionFromRequest[string, any](r) // Example: Access transaction

			if ok {
				// Log if user ID was found
				logger.Debug("Authenticated user ID found in context", zap.String("userID", userID))
				fmt.Printf("[Debug] Authenticated User ID: %s for path %s\n", userID, r.URL.Path)
			} else {
				logger.Debug("No authenticated user ID found in context")
				fmt.Printf("[Debug] No User ID for path %s\n", r.URL.Path)
			}

			// Call the next handler regardless
			next.ServeHTTP(w, r)
		})
	}
}
```

## Applying Middleware

Middleware can be applied at three levels:

1.  **Global**: Added to `RouterConfig.Middlewares`. Applied to *all* routes handled by the router.
2.  **Sub-Router**: Added to `SubRouterConfig.Middlewares`. Applied to all routes within that sub-router (and its nested sub-routers), *after* any global middleware.
3.  **Route-Specific**: Added to `RouteConfigBase.Middlewares` or `RouteConfig.Middlewares`. Applied only to that specific route, *after* any global and sub-router middleware.

```go
// Example applying middleware at different levels
routerConfig := router.RouterConfig{
    // ... logger, etc.
    Middlewares: []common.Middleware{
        // middleware.CreateTraceMiddleware[T, U](idGen), // Global: Added automatically if TraceIDBufferSize > 0
        // middleware.Recovery, // Global: Applied internally by SRouter
        mymiddleware.AddHeaderMiddleware("X-Global", "true"), // Global: Custom middleware
        // Note: Request logging is handled internally if EnableTraceLogging is true
    },
    SubRouters: []router.SubRouterConfig{
        {
            PathPrefix: "/api/v1",
            Middlewares: []common.Middleware{
                MyCustomAuthMiddleware(), // Sub-Router: Runs before global middleware
                mymiddleware.AddHeaderMiddleware("X-API-Version", "v1"), // Sub-Router: Custom middleware
            },
            Routes: []router.RouteDefinition{
                router.RouteConfigBase{
                    Path: "/users",
                    Methods: []router.HttpMethod{router.MethodGet},
                    Middlewares: []common.Middleware{
                        mymiddleware.LogUserIDMiddleware(logger), // Route: Runs last before handler
                    },
                    Handler: GetUsersHandler,
                    AuthLevel: router.Ptr(router.AuthRequired), // Example: Requires authentication
                },
                // ... other v1 routes
            },
        },
    },
    // ...
}

// Note: CORS preflight requests (OPTIONS with Origin header and CORS-specific headers)
// are handled at the CORS layer before reaching authentication middleware.
// Other OPTIONS requests are subject to normal authentication requirements.
```

## Middleware Execution Order

SRouter applies middleware in a specific order by wrapping the final handler. The effective order, from outermost (runs first) to innermost (runs last before handler), based on the internal `wrapHandler` and `registerSubRouter` logic, is:

1.  **Recovery Middleware** (Applied internally)
2.  **Authentication Middleware** (Applied internally if `AuthLevel` is `AuthRequired` or `AuthOptional`)
3.  **Rate Limiting Middleware** (Applied internally if `rateLimit` config exists)
4.  **Route-Specific and Sub-Router Middlewares** (`SubRouterConfig.Middlewares` and `RouteConfig.Middlewares`)
5.  **Global Middlewares** (`RouterConfig.Middlewares`, including Trace if enabled)
6.  **Timeout Middleware** (Applied if `timeout > 0`)
7.  **Your Actual Handler** (`http.HandlerFunc` or `GenericHandler`)

Note: The `registerSubRouter` function combines sub-router and route-specific middleware before passing the combined list to `wrapHandler`. `wrapHandler` then applies global middleware before this combined list.

Middleware within the *same slice* (e.g., `RouterConfig.Middlewares`) are applied in the order they appear in the slice; the first one in the slice becomes the outermost wrapper.

## Middleware Reference

SRouter provides several built-in middleware functions and applies others internally. Refer to the source code or specific examples for exact signatures and usage.

**Exported from `pkg/middleware`:**

-   **`Recovery`**: Recovers from panics. Applied internally by SRouter, usually no need to add manually.
-   **`MaxBodySize(limit int64)`**: Limits request body size. Applied internally based on config, usually no need to add manually.
-   **`Timeout(timeout time.Duration)`**: Applies a request timeout using context. Applied internally based on config, usually no need to add manually.
    -   **`CreateTraceMiddleware[T, U](idGen *IDGenerator)`**: Creates the trace ID middleware. Added automatically to global middleware if `RouterConfig.TraceIDBufferSize > 0`. See [Trace ID Logging](./trace-logging.md).
-   **`RateLimit(config *common.RateLimitConfig[T, U], limiter common.RateLimiter, logger *zap.Logger)`**: Applies rate limiting. Applied internally based on config, usually no need to add manually. See [Rate Limiting](./rate-limiting.md).
-   **`NewGormTransactionWrapper`**: Wrapper for GORM transactions (used with `scontext`). See [Context Management](./context-management.md).

**Internal / Not Exported for Direct Use:**

-   **Request Logging**: Handled internally by `Router.ServeHTTP` if `RouterConfig.EnableTraceLogging` is true. There is no separate `Logging` middleware to add. See [Logging](./logging.md).
-   **Authentication**: The built-in Bearer token authentication (`authRequiredMiddleware`, `authOptionalMiddleware`) is applied internally based on `AuthLevel` config. For other schemes (Basic, API Key, etc.), you need to implement custom middleware. See [Authentication](./authentication.md).
-   **IP Extraction**: Handled internally by `Router.ServeHTTP` based on `RouterConfig.IPConfig`. See [IP Configuration](./ip-configuration.md).
-   **CORS**: Handled internally by `Router.ServeHTTP` based on `RouterConfig.CORSConfig`. See [CORS Configuration](./cors-configuration.md).

Always check the specific package documentation or source code for the most up-to-date list and usage details of built-in middleware.

See the `examples/middleware` directory for runnable examples.
