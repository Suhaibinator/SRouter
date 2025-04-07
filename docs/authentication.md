# Authentication

SRouter provides a flexible authentication system integrated with its routing configuration. Setting the `AuthLevel` on a route activates **built-in authentication middleware** that relies on functions provided during router initialization. You can also implement **custom authentication middleware** for more complex or alternative authentication schemes.

## Authentication Levels and Built-in Middleware

SRouter defines three authentication levels using the `router.AuthLevel` type. You specify the required level for a route in its `RouteConfigBase` or `RouteConfig[T, U]` using the `AuthLevel` field (which is a pointer, `*router.AuthLevel`). If `AuthLevel` is `nil`, the route inherits the default level from its parent sub-router, or ultimately defaults to `NoAuth` if no parent specifies it.

Setting `AuthLevel` to `AuthOptional` or `AuthRequired` activates **built-in middleware** within the router. This middleware performs the following based on the level:

1.  **`router.NoAuth`**: No authentication is required or attempted by the built-in middleware. The request proceeds directly to the next middleware or handler. This is the default if `AuthLevel` is not set.
2.  **`router.AuthOptional`**: The built-in authentication middleware is activated. It attempts to validate credentials (currently expects a Bearer token in the `Authorization` header) using the `authFunction` provided to `NewRouter`.
    *   If authentication succeeds, the middleware populates the user ID (using the `userIdFromUserFunction` from `NewRouter`) and optionally the user object into the request context using `scontext.WithUserID` and `scontext.WithUser`. The request then proceeds to the next middleware or handler.
    *   If authentication fails (or no `Authorization` header is provided), the request *still proceeds* to the next middleware or handler, but without user information in the context. The handler must check for the presence of user information using `scontext.GetUserIDFromRequest` or `scontext.GetUserFromRequest`.
3.  **`router.AuthRequired`**: The built-in authentication middleware is activated and authentication is mandatory. It attempts validation as described for `AuthOptional`.
    *   If authentication succeeds, the middleware populates the context (as above) and proceeds to the next middleware or handler.
    *   If authentication fails, the built-in middleware **rejects** the request by sending an HTTP `401 Unauthorized` response and stops the middleware chain. The handler is not called.

```go
import (
	"github.com/Suhaibinator/SRouter/pkg/router"
	"github.com/Suhaibinator/SRouter/pkg/scontext" // For context functions
)

// Example route configurations:
routePublic := router.RouteConfigBase{
    Path: "/public/info",
    // AuthLevel: nil, // Defaults to NoAuth
    // Or explicitly:
    AuthLevel: router.Ptr(router.NoAuth),
    // ... handler, methods
}

routeOptional := router.RouteConfigBase{
    Path: "/user/profile", // Maybe shows generic profile if not logged in, specific if logged in
    AuthLevel: router.Ptr(router.AuthOptional),
    // ... handler, methods
}

routeProtected := router.RouteConfig[UpdateSettingsReq, UpdateSettingsResp]{
    Path: "/user/settings",
    AuthLevel: router.Ptr(router.AuthRequired), // Must be logged in
    // ... handler, methods, codec
}
```
*(Note: `router.Ptr()` is a simple helper function to get a pointer to an `AuthLevel` value, as the config fields expect pointers)*

## Authentication Functions (`NewRouter`)

The core of the built-in authentication mechanism relies on two functions you **must** provide when creating the router instance with `NewRouter`:

1.  **`authFunction func(ctx context.Context, token string) (UserObjectType, bool)`**:
    *   This function is called by the built-in middleware when `AuthLevel` is `AuthOptional` or `AuthRequired`.
    *   It receives the request context and the token string extracted from the `Authorization: Bearer <token>` header.
    *   It should validate the token (e.g., check a database, validate a JWT signature).
    *   It must return the corresponding `UserObjectType` (your application's user struct/type) and `true` if the token is valid, or a zero-value `UserObjectType` and `false` if invalid.

2.  **`userIdFromUserFunction func(user UserObjectType) UserIDType`**:
    *   This function is called *after* `authFunction` returns `true`.
    *   It receives the `UserObjectType` returned by `authFunction`.
    *   It must return the corresponding `UserIDType` (your application's user ID type, e.g., `string`, `int`) for that user. This ID is then stored in the request context.

```go
// Example dummy functions for NewRouter
func myAuthValidator(ctx context.Context, token string) (MyUserType, bool) {
    // TODO: Implement actual token validation (e.g., JWT check, DB lookup)
    if token == "valid-token-for-user-123" {
        return MyUserType{ID: "123", Email: "user@example.com"}, true
    }
    return MyUserType{}, false
}

func myGetIDFromUser(user MyUserType) string {
    return user.ID
}

// ... later, when creating the router:
r := router.NewRouter[string, MyUserType](routerConfig, myAuthValidator, myGetIDFromUser)
```

**If you do not intend to use the built-in `AuthLevel` mechanism** (e.g., you rely solely on custom authentication middleware), you must still provide non-nil functions to `NewRouter`. These can be simple dummy functions that always return `false` or zero values.

## Custom Authentication Middleware

While the `AuthLevel` setting provides convenient Bearer token authentication via the built-in mechanism, you can implement **custom authentication middleware** for other schemes (Cookies, API Keys, Basic Auth, etc.) or more complex logic.

Your custom middleware is responsible for:

1.  Extracting credentials from the request (e.g., cookies, different header formats).
2.  Validating these credentials.
3.  Populating the context on success using `scontext.WithUserID[UserIDType, UserObjectType](ctx, userID)` and optionally `scontext.WithUser[UserIDType, UserObjectType](ctx, userObject)`. **Crucially, use the `scontext` package functions** so that `scontext.GetUserIDFromRequest` works consistently.
4.  Calling `next.ServeHTTP(w, r.WithContext(populatedCtx))` on success.
5.  Handling failures appropriately:
    *   For logic equivalent to `AuthRequired`, write an error response (e.g., `http.Error(w, "Unauthorized", http.StatusUnauthorized)`) and **do not** call `next.ServeHTTP`.
    *   For logic equivalent to `AuthOptional`, simply call `next.ServeHTTP(w, r)` without populating the context.

**Important Note on OPTIONS Requests:** The built-in authentication middleware automatically bypasses authentication checks for HTTP `OPTIONS` requests to ensure CORS preflight requests function correctly. Your custom authentication middleware should generally include similar logic.

**Applying Custom Middleware:** Add your custom authentication middleware globally in `RouterConfig.Middlewares` or per-sub-router in `SubRouterConfig.Middlewares`. Ensure it runs *before* other middleware that might depend on the user context (like user-based rate limiting). If using custom middleware, you might set `AuthLevel` to `NoAuth` for relevant routes to prevent the built-in middleware from running unnecessarily.

```go
// Example: Applying a custom API Key validation middleware globally
// Assume MyApiKeyMiddleware validates an X-API-Key header and calls scontext.WithUserID/User on success
routerConfig := router.RouterConfig{
    // ... logger, etc.
    Middlewares: []common.Middleware{
        middleware.TraceMiddleware(), // Trace ID first (assuming pkg/middleware provides this)
        MyApiKeyMiddleware(apiKeyValidationService), // Your custom auth middleware
        // middleware.RateLimiterMiddleware(/*...*/), // Rate limiter might depend on user ID set by MyApiKeyMiddleware
        // middleware.Logging(logger),
    },
    // ...
}

// Define dummy functions since NewRouter requires them, even if unused by MyApiKeyMiddleware
dummyAuthFunc := func(ctx context.Context, token string) (MyUserType, bool) { return MyUserType{}, false }
dummyGetIDFunc := func(user MyUserType) string { return "" }

// UserIDType and UserObjectType for NewRouter must match what MyApiKeyMiddleware puts in context
r := router.NewRouter[string, MyUserType](routerConfig, dummyAuthFunc, dummyGetIDFunc)

// Routes using this custom middleware might set AuthLevel: router.Ptr(router.NoAuth)
// if MyApiKeyMiddleware handles all required/optional logic itself.
```

*(This section is replaced by the "Authentication Functions (`NewRouter`)" section above)*

## Accessing User Information

In your handlers, you can access the authenticated user's information (set by either the built-in or custom authentication middleware) using helper functions from the `pkg/scontext` package:

```go
import (
	"fmt"
	"net/http"
	"github.com/Suhaibinator/SRouter/pkg/scontext" // Use scontext for accessing user info
)

// Assume MyUserType has fields like ID (string) and Email (string)
type MyUserType struct {
	ID    string
	Email string
}

func GetUserSettingsHandler(w http.ResponseWriter, r *http.Request) {
    // Replace string, MyUserType with your router's actual UserIDType, UserObjectType
    userID, userOK := scontext.GetUserIDFromRequest[string, MyUserType](r)
    userObject, userObjOK := scontext.GetUserFromRequest[string, MyUserType](r) // Returns *MyUserType

    // For routes using AuthRequired (built-in or custom equivalent),
    // userOK should always be true if the handler is reached.
    // For routes using AuthOptional (built-in or custom equivalent),
    // you MUST check userOK.
    if !userOK {
         // Handle case where user is not authenticated (possible for AuthOptional)
         http.Error(w, "Authentication required or failed", http.StatusUnauthorized)
         return
    }

    fmt.Fprintf(w, "Settings for User ID: %s\n", userID)
    if userObjOK && userObject != nil {
        fmt.Fprintf(w, "User Email: %s\n", userObject.Email)
    }

    // ... fetch and return settings for userID ...
}
```

See the `examples/auth`, `examples/auth-levels`, and `examples/user-auth` directories for runnable examples.
