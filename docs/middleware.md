# Custom Middleware

Middleware provides a powerful way to inject logic into the request/response cycle, handling concerns like logging, authentication, rate limiting, compression, CORS, and more, separate from your core request handlers.

SRouter uses the standard Go `http.Handler` interface for middleware, often defined using the `common.Middleware` type alias for clarity.

```go
// Likely defined in pkg/common/middleware.go or similar
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
	"github.com/Suhaibinator/SRouter/pkg/middleware" // For context helpers
	"go.uber.org/zap"                                // Example logger
)

// LogUserIDMiddleware logs the user ID if authentication was successful.
// Requires an authentication middleware to run first.
// This example shows accessing UserID, but other context values (TraceID, ClientIP, Transaction, Flags)
// can be accessed similarly using their respective GetXFromRequest functions.
func LogUserIDMiddleware(logger *zap.Logger) common.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Attempt to get User ID from context
			// Replace string, any with your router's UserIDType, UserObjectType
			userID, ok := middleware.GetUserIDFromRequest[string, any](r)
			// txInterface, txOK := middleware.GetTransactionFromRequest[string, any](r) // Example: Access transaction

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
        middleware.TraceMiddleware(),        // Global: Runs first
        middleware.Recovery(logger),         // Global: Recovers panics
        mymiddleware.AddHeaderMiddleware("X-Global", "true"), // Global
    },
    SubRouters: []router.SubRouterConfig{
        {
            PathPrefix: "/api/v1",
            Middlewares: []common.Middleware{
                MyAuthMiddleware(), // Sub-Router: Runs after global, before route-specific
                mymiddleware.AddHeaderMiddleware("X-API-Version", "v1"),
            },
            Routes: []any{
                router.RouteConfigBase{
                    Path: "/users",
                    Methods: []router.HttpMethod{router.MethodGet},
                    Middlewares: []common.Middleware{
                        mymiddleware.LogUserIDMiddleware(logger), // Route: Runs last before handler
                    },
                    Handler: GetUsersHandler,
                    AuthLevel: router.AuthRequired, // Example: Requires authentication
                },
                // ... other v1 routes
            },
        },
    },
    // ...
}

// Note: The router's internal authentication middleware (authRequiredMiddleware, authOptionalMiddleware)
// automatically bypass authentication checks for HTTP OPTIONS requests (preflight requests).
// This ensures CORS preflight requests succeed without needing explicit authentication.
```

## Middleware Execution Order

SRouter applies middleware in a specific order, generally wrapping handlers from the outside in:

`Timeout -> MaxBodySize -> Global Middleware -> Sub-Router Middleware -> Route Middleware -> Handler`

(Note: Internal middleware like Recovery, Authentication, Rate Limiting might be interleaved within this chain based on the router's internal implementation. Check the `router.wrapHandler` or similar internal functions for the precise order if needed.)

Middleware defined earlier in a slice generally runs *before* middleware defined later in the same slice (i.e., the outer layers of the onion).

## Middleware Reference

SRouter provides several built-in middleware functions and constructors in the `pkg/middleware` package.

-   **`Recovery(logger *zap.Logger) Middleware`**: Recovers from panics, logs them, and returns a 500 error. Exposed via `middleware.Recovery` variable. Note: SRouter applies its own recovery internally, so direct use might be unnecessary.
-   **Authentication Middleware**: Constructors for various schemes (see `pkg/middleware/auth.go`):
    -   `NewBearerTokenMiddleware[T, U](...)`
    -   `NewBearerTokenWithUserMiddleware[T, U](...)`
    -   `NewAPIKeyMiddleware[T, U](...)`
    -   `NewAPIKeyWithUserMiddleware[T, U](...)`
    -   Basic Auth is handled via `AuthenticationWithProvider` / `AuthenticationWithUserProvider` using a `BasicAuthProvider` / `BasicUserAuthProvider` (implementations likely in `pkg/middleware/auth_provider.go` or similar).
-   **`MaxBodySize(maxSize int64) Middleware`**: Limits request body size. Exposed via `middleware.MaxBodySize` variable. (Note: Usually configured via router config).
-   **`Timeout(timeout time.Duration) Middleware`**: Sets a request timeout. Exposed via `middleware.Timeout` variable. (Note: Usually configured via router config).
-   **`CORS(corsConfig CORSOptions) Middleware`**: Adds CORS headers. Exposed via `middleware.CORS` variable. Takes `middleware.CORSOptions` struct.
-   **`Chain(middlewares ...Middleware) Middleware`**: Chains multiple middlewares. (Located in `pkg/middleware/middleware.go`).
-   **`CreateTraceMiddleware(generator *IDGenerator) Middleware`**: Adds trace ID to context using an efficient generator. (Located in `pkg/middleware/trace.go`).
-   **Rate Limiting Middleware**: Constructed via `middleware.RateLimit[T, U](...)` or `middleware.CreateRateLimitMiddleware[T, U](...)`. (Located in `pkg/middleware/ratelimit.go`).
-   **Client IP Middleware**: `middleware.ClientIPMiddleware[T, U](config *IPConfig)`. (Located in `pkg/middleware/ip.go`). Added automatically by `NewRouter`.

**Note:** Logging is handled internally by SRouter based on the provided logger and configuration (like `EnableTraceLogging`), not typically via an exported `middleware.Logging` function.

Always check the specific package source code (`pkg/middleware/*.go`) for the most up-to-date list and usage details.

See the `examples/middleware` directory for runnable examples demonstrating custom and potentially built-in middleware usage.
