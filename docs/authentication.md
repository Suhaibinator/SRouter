# Authentication

SRouter provides a flexible authentication system integrated with its routing configuration, allowing you to specify authentication requirements per route. Authentication is primarily handled via **middleware**.

## Authentication Levels

SRouter defines three authentication levels using the `router.AuthLevel` type (defined in `pkg/router`). You can specify the required level for a route in its `router.RouteConfigBase` or `router.RouteConfig[T, U]` using the `AuthLevel` field (which is a pointer, `*router.AuthLevel`). If `AuthLevel` is `nil`, the route inherits the default level from its parent sub-router, or ultimately defaults to `router.NoAuth` if no parent specifies it.

1.  **`router.NoAuth`**: No authentication is required. The request proceeds directly to the handler (after other middleware). This is the default if `AuthLevel` is not set.
2.  **`router.AuthOptional`**: Authentication is attempted by the configured authentication middleware.
    *   If authentication succeeds, the middleware should populate the user ID (and optionally the user object) into the request context using `middleware.WithUserID` and `middleware.WithUser`. The request then proceeds to the handler.
    *   If authentication fails (or no credentials are provided), the request *still proceeds* to the handler, but without user information in the context. The handler must check for the presence of user information if needed.
3.  **`router.AuthRequired`**: Authentication is required.
    *   If authentication succeeds, the middleware populates the context (as above) and proceeds to the handler.
    *   If authentication fails, the authentication middleware **must** reject the request, typically by sending an HTTP error response (e.g., `401 Unauthorized`) and stopping the middleware chain. The handler is not called.

```go
import "github.com/Suhaibinator/SRouter/pkg/router"
import "github.com/Suhaibinator/SRouter/pkg/middleware" // For context helpers if needed in handler

// Example route configurations:
routePublic := router.RouteConfigBase{
    Path: "/public/info",
    // AuthLevel: nil, // Defaults to router.NoAuth
    // Or explicitly:
    AuthLevel: router.Ptr(router.NoAuth),
    // ... handler, methods
}

routeOptional := router.RouteConfigBase{
    Path: "/user/profile", // Maybe shows generic profile if not logged in, specific if logged in
    AuthLevel: router.Ptr(router.AuthOptional),
    // ... handler, methods
}

routeProtected := router.RouteConfig[UpdateSettingsReq, UpdateSettingsResp]{ // Assume types exist
    Path: "/user/settings",
    AuthLevel: router.Ptr(router.AuthRequired), // Must be logged in
    // ... handler, methods, codec
}
```
*(Note: `router.Ptr()` is a helper function in `pkg/router` to get a pointer to an `AuthLevel` value, as the config fields expect pointers)*

## Authentication Middleware

SRouter itself **does not perform authentication**. You must provide your own authentication middleware or use pre-built ones (like those potentially offered in `pkg/middleware` for common schemes like Basic Auth, Bearer Token, API Key).

This middleware is responsible for:

1.  Extracting credentials from the request (e.g., `Authorization` header, cookies, API keys).
2.  Validating these credentials against your user store or authentication provider.
3.  Based on the validation result and the route's required `AuthLevel`:
    *   **On Success**: Populate the context using `middleware.WithUserID[UserIDType, UserObjectType](ctx, userID)` and optionally `middleware.WithUser[UserIDType, UserObjectType](ctx, userObject)`. Call `next.ServeHTTP(w, r.WithContext(populatedCtx))`.
    *   **On Failure (for `AuthRequired`)**: Write an error response (e.g., `http.Error(w, "Unauthorized", http.StatusUnauthorized)`) and **do not** call `next.ServeHTTP`.
    *   **On Failure (for `AuthOptional`)**: Simply call `next.ServeHTTP(w, r)` without populating the context.

**Important Note on OPTIONS Requests:** All built-in authentication middleware automatically bypasses authentication checks for HTTP `OPTIONS` requests. This is done to ensure that CORS preflight requests function correctly without requiring authentication headers. If you implement custom authentication middleware, you should generally include similar logic to skip checks for `OPTIONS` requests.

**Applying Middleware:** Authentication middleware should typically be added globally in `RouterConfig.Middlewares` or per-sub-router in `SubRouterConfig.Middlewares` so it runs for all relevant routes. Ensure it runs *before* other middleware that might depend on the user context (like user-based rate limiting).

```go
// Example: Applying a custom JWT validation middleware globally
// Assume MyJwtMiddleware validates a JWT and calls WithUserID/WithUser on success
routerConfig := router.RouterConfig{
    // ... logger, etc.
    Middlewares: []common.Middleware{
        middleware.TraceMiddleware(), // Trace ID first
        MyJwtMiddleware(jwtValidationService), // Your auth middleware
        middleware.RateLimiterMiddleware(/*...*/), // Rate limiter might depend on user ID
        middleware.Logging(logger),
    },
    // ...
}
// UserIDType and UserObjectType for NewRouter must match what MyJwtMiddleware puts in context
r := router.NewRouter[string, MyUserType](routerConfig, /* auth funcs */)
```

**Role of `authFunction` and `userIdFromUserFunction` in `NewRouter`:**

These functions passed to `NewRouter` were primarily used by older, built-in authentication mechanisms that are now discouraged in favor of explicit middleware. While they might still be used internally by some legacy or basic middleware provided in `pkg/middleware`, **the recommended approach is to implement authentication logic entirely within dedicated middleware.** Your middleware should handle credential validation and context population directly.

## Accessing User Information

In your handlers, you can access the authenticated user's information (if present) using helper functions from the `pkg/middleware` package:

```go
import "github.com/Suhaibinator/SRouter/pkg/middleware"

func GetUserSettingsHandler(w http.ResponseWriter, r *http.Request) {
    // Replace string, MyUserType with your router's UserIDType, UserObjectType
    userID, userOK := middleware.GetUserIDFromRequest[string, MyUserType](r)
    userObject, userObjOK := middleware.GetUserFromRequest[string, MyUserType](r) // Returns *MyUserType

    // For AuthRequired routes, userOK should always be true if the handler is reached.
    // For AuthOptional routes, you MUST check userOK.
    if !userOK {
         // Handle case where user is not authenticated (only possible for AuthOptional)
         http.Error(w, "Authentication required or failed", http.StatusUnauthorized)
         return
    }

    fmt.Fprintf(w, "Settings for User ID: %s\n", userID)
    if userObjOK && userObject != nil {
        fmt.Fprintf(w, "User Email: %s\n", userObject.Email) // Assuming MyUserType has Email
    }

    // ... fetch and return settings for userID ...
}
```

See the `examples/auth`, `examples/auth-levels`, and `examples/user-auth` directories for runnable examples.
