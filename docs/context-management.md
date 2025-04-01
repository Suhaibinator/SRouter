# Context Management

SRouter employs a structured approach to manage values added to the `http.Request` context by its middleware and internal components. Instead of nesting multiple `context.WithValue` calls, it uses a single wrapper struct, `middleware.SRouterContext`, stored under a specific key.

## The `SRouterContext` Wrapper

This generic struct, defined in `pkg/middleware/context.go`, consolidates common context values:

```go
package middleware

// SRouterContext holds values added to the request context by SRouter middleware.
// T is the UserID type (comparable), U is the User object type (any).
type SRouterContext[T comparable, U any] struct {
	// UserID holds the authenticated user's ID.
	UserID T
	// User holds a pointer to the authenticated user object.
	User *U // Pointer to avoid copying potentially large structs

	// ClientIP holds the determined client IP address.
	ClientIP string

	// TraceID holds the unique identifier for the request trace.
	TraceID string

	// --- Internal tracking flags ---

	// UserIDSet indicates if the UserID field has been explicitly set.
	UserIDSet bool
	// UserSet indicates if the User field has been explicitly set.
	UserSet bool
	// ClientIPSet indicates if the ClientIP field has been explicitly set.
	ClientIPSet bool
	// TraceIDSet indicates if the TraceID field has been explicitly set.
	TraceIDSet bool

	// Flags allow storing arbitrary boolean flags or simple string values.
	Flags map[string]any // Changed to map[string]any for more flexibility (e.g., request ID string)
}

// contextKey is an unexported type used as the key for storing SRouterContext in context.Context.
type contextKey struct{}

// srouterCtxKey is the specific key used.
var srouterCtxKey = contextKey{}
```

The type parameters `T` (UserID type) and `U` (User object type) must match the types used when creating the `router.NewRouter[T, U]` instance.

## Benefits

This approach offers several advantages over traditional `context.WithValue` nesting:

1.  **Reduced Nesting**: Avoids deeply nested contexts, potentially improving lookup performance slightly and simplifying context propagation.
2.  **Type Safety**: Generics ensure that user IDs and user objects are handled with their correct types, eliminating the need for type assertions when retrieving them.
3.  **Organization**: Groups related context values logically within a single structure.
4.  **Extensibility**: New standard values can be added to `SRouterContext` without introducing new context keys. The `Flags` map provides a way for custom middleware to add simple values without modifying the core struct.

## Adding Values to Context (Middleware Authors)

Middleware should use the provided helper functions (like `WithUserID`, `WithUser`, `WithClientIP`, `WithTraceID`, `WithFlag`) to add values. These functions handle creating or updating the `SRouterContext` wrapper within the `context.Context`.

```go
// Example within a middleware:
func MyMiddleware() common.Middleware {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            ctx := r.Context()
            // Add a custom flag/value
            requestUUID := uuid.NewString()
            ctx = middleware.WithFlag[string, MyUserType](ctx, "request_uuid", requestUUID) // Use router's T, U types

            // Add user ID after successful authentication
            // ctx = middleware.WithUserID[string, MyUserType](ctx, "user-123")

            next.ServeHTTP(w, r.WithContext(ctx))
        })
    }
}
```

## Accessing Context Values (Handler/Middleware Consumers)

Use the corresponding getter functions from the `pkg/middleware` package to retrieve values safely. These functions handle extracting the `SRouterContext` and returning the desired field along with a boolean indicating if it was found/set.

```go
import "github.com/Suhaibinator/SRouter/pkg/middleware"

func myHandler(w http.ResponseWriter, r *http.Request) {
    // Replace string, MyUserType with your router's UserIDType, UserObjectType

    // Get User ID
    userID, ok := middleware.GetUserIDFromRequest[string, MyUserType](r)
    if ok {
        fmt.Printf("User ID: %s\n", userID)
    }

    // Get User Object (returns *MyUserType)
    user, ok := middleware.GetUserFromRequest[string, MyUserType](r)
    if ok && user != nil {
         fmt.Printf("User Email: %s\n", user.Email) // Assuming MyUserType has Email
    }

    // Get Client IP
    clientIP, ok := middleware.GetClientIPFromRequest[string, MyUserType](r)
    if ok {
        fmt.Printf("Client IP: %s\n", clientIP)
    }

    // Get Trace ID (shortcut function available)
    traceID := middleware.GetTraceID(r) // No type params needed for this specific helper
    fmt.Printf("Trace ID: %s\n", traceID)

    // Get a custom flag/value
    requestUUIDAny, ok := middleware.GetFlagFromRequest[string, MyUserType](r, "request_uuid")
    if ok {
        if requestUUID, castOK := requestUUIDAny.(string); castOK {
             fmt.Printf("Request UUID: %s\n", requestUUID)
        }
    }

    // ... handler logic ...
}
```

Always use these helper functions to interact with SRouter-managed context values for safety and maintainability. Remember that the user object (`GetUserFromRequest`) is returned as a pointer (`*U`).
