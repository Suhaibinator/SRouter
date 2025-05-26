# Context Management

SRouter employs a structured approach to manage values added to the `http.Request` context by its middleware and internal components. Instead of nesting multiple `context.WithValue` calls, it uses a single wrapper struct, `scontext.SRouterContext`, stored under a specific key.

## The `SRouterContext` Wrapper

This generic struct, defined in `pkg/scontext/context.go`, consolidates common context values:

```go
package scontext // Defined in pkg/scontext

import (
	"context"
	"net/http"
	"gorm.io/gorm" // Needed for DatabaseTransaction interface definition
)

// sRouterContextKey is a private type for the context key to avoid collisions
type sRouterContextKey struct{}

// DatabaseTransaction defines an interface for essential transaction control methods.
type DatabaseTransaction interface {
	Commit() error
	Rollback() error
	SavePoint(name string) error
	RollbackTo(name string) error
	GetDB() *gorm.DB
}


// SRouterContext holds values added to the request context by SRouter components.
// T is the UserID type (comparable), U is the User object type (any).
type SRouterContext[T comparable, U any] struct {
	// UserID holds the authenticated user's ID.
	UserID T
	// User holds a pointer to the authenticated user object.
	User *U // Pointer to avoid copying potentially large structs

        // ClientIP holds the determined client IP address.
        ClientIP string

        // UserAgent holds the user agent string from the request.
        UserAgent string

	// TraceID holds the unique identifier for the request trace.
	TraceID string

	// Transaction holds an active database transaction object.
	// It uses the DatabaseTransaction interface for abstraction.
	Transaction DatabaseTransaction

	// --- Internal tracking flags ---

	// UserIDSet indicates if the UserID field has been explicitly set.
	UserIDSet bool
	// UserSet indicates if the User field has been explicitly set.
	UserSet bool
        // ClientIPSet indicates if the ClientIP field has been explicitly set.
        ClientIPSet bool
        // UserAgentSet indicates if the UserAgent field has been explicitly set.
        UserAgentSet bool
	// TraceIDSet indicates if the TraceID field has been explicitly set.
	TraceIDSet bool
	// TransactionSet indicates if the Transaction field has been explicitly set.
	TransactionSet bool

	// Flags allow storing arbitrary boolean flags.
	Flags map[string]bool
}

// Helper functions like NewSRouterContext, GetSRouterContext, WithSRouterContext,
// EnsureSRouterContext are also defined in pkg/scontext.
```

The type parameters `T` (UserID type) and `U` (User object type) must match the types used when creating the `router.NewRouter[T, U]` instance.

**Using Transactions:**

*   The `DatabaseTransaction` interface is defined in `pkg/scontext`.
*   Because GORM's transaction methods (like `Commit`) return `*gorm.DB` for chaining, they don't directly match the `DatabaseTransaction` interface which expects methods like `Commit() error`.
*   Therefore, a wrapper `GormTransactionWrapper` is provided in the `pkg/middleware` package. You must wrap your GORM transaction (`*gorm.DB`) using `middleware.NewGormTransactionWrapper` before adding it to the context with `scontext.WithTransaction`.
*   When retrieving the transaction using `scontext.GetTransactionFromRequest` (or `scontext.GetTransaction`), you get the `scontext.DatabaseTransaction` interface. You can call `Commit`/`Rollback` on this interface. To perform GORM operations (like `Find`, `Create`), call `GetDB()` on the interface to get the underlying `*gorm.DB`.

## Benefits

This approach offers several advantages over traditional `context.WithValue` nesting:

1.  **Reduced Nesting**: Avoids deeply nested contexts, potentially improving lookup performance slightly and simplifying context propagation.
2.  **Type Safety**: Generics ensure that user IDs and user objects are handled with their correct types, eliminating the need for type assertions when retrieving them.
3.  **Organization**: Groups related context values logically within a single structure.
4.  **Extensibility**: New standard values can be added to `SRouterContext` without introducing new context keys. The `Flags` map provides a way for custom middleware to add simple values without modifying the core struct.

## Adding Values to Context (Middleware Authors)

Middleware should use the provided helper functions from the `pkg/scontext` package (like `scontext.WithUserID`, `scontext.WithUser`, `scontext.WithClientIP`, `scontext.WithUserAgent`, `scontext.WithTraceID`, `scontext.WithFlag`, `scontext.WithTransaction`) to add values. These functions handle creating or updating the `SRouterContext` wrapper within the `context.Context`.

```go
// Example within a middleware:
func MyMiddleware() common.Middleware {
    return func(next http.Handler) http.Handler {
        return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
            ctx := r.Context()
            // Add a custom boolean flag
            isAdminRequest := checkAdminPermissions(r) // Your logic here
            ctx = scontext.WithFlag[string, MyUserType](ctx, "is_admin", isAdminRequest) // Use router's T, U types

            // Add user ID after successful authentication
            // ctx = scontext.WithUserID[string, MyUserType](ctx, "user-123")

            // Example: Start and add a DB transaction
            var db *gorm.DB // Assume db is initialized elsewhere
            tx := db.Begin()
            if tx.Error != nil {
                // Handle transaction start error
                // Log error, maybe return HTTP 500
                // next.ServeHTTP(w, r.WithContext(ctx)) // Or maybe don't proceed
                return
            }

            // Wrap the GORM transaction (Wrapper is in pkg/middleware)
            txWrapper := middleware.NewGormTransactionWrapper(tx)

            // Add the wrapper (which implements scontext.DatabaseTransaction) to the context
            ctx = scontext.WithTransaction[string, MyUserType](ctx, txWrapper)

            // It's crucial to have another middleware later in the chain
            // (or deferred logic in this one) to Commit or Rollback the transaction
            // based on the handler's outcome.

            next.ServeHTTP(w, r.WithContext(ctx))

            // Example cleanup logic (could be in a separate middleware):
            // finalTx, ok := scontext.GetTransaction[string, MyUserType](r.Context()) // Use r.Context() after handler
            // if ok { // Check if transaction exists
            //     // Determine if handler succeeded or failed (e.g., check response status, error flags)
            //     if handlerFailed {
            //         finalTx.Rollback()
            //     } else {
            //         finalTx.Commit()
            //     }
            // }
        })
    }
}
```

## Accessing Context Values (Handler/Middleware Consumers)

Use the corresponding getter functions from the `pkg/scontext` package to retrieve values safely. These functions handle extracting the `SRouterContext` and returning the desired field along with a boolean indicating if it was found/set.

```go
import (
	"fmt"
	"net/http"
	"github.com/Suhaibinator/SRouter/pkg/scontext" // Use scontext package
	// "github.com/Suhaibinator/SRouter/pkg/middleware" // Only needed if using GormTransactionWrapper directly
)

// Assume MyUserType and MyModel are defined elsewhere
type MyUserType struct { Email string }
type MyModel struct { /* ... */ }
var someID = "some-model-id" // Example ID

func myHandler(w http.ResponseWriter, r *http.Request) {
    // Replace string, MyUserType with your router's actual UserIDType, UserObjectType

    // Get User ID
    userID, ok := scontext.GetUserIDFromRequest[string, MyUserType](r)
    if ok {
        fmt.Printf("User ID: %s\n", userID)
    }

    // Get User Object (returns *MyUserType)
    user, ok := scontext.GetUserFromRequest[string, MyUserType](r)
    if ok && user != nil {
         fmt.Printf("User Email: %s\n", user.Email) // Assuming MyUserType has Email
    }

    // Get Client IP
    clientIP, ok := scontext.GetClientIPFromRequest[string, MyUserType](r)
    if ok {
        fmt.Printf("Client IP: %s\n", clientIP)
    }

    // Get User Agent
    userAgent, ok := scontext.GetUserAgentFromRequest[string, MyUserType](r)
    if ok {
        fmt.Printf("User Agent: %s\n", userAgent)
    }

    // Get Trace ID
    traceID := scontext.GetTraceIDFromRequest[string, MyUserType](r) // Use the correct function
    fmt.Printf("Trace ID: %s\n", traceID)

    // Get a custom boolean flag
    isAdmin, ok := scontext.GetFlagFromRequest[string, MyUserType](r, "is_admin")
    if ok {
        fmt.Printf("Is Admin Request: %t\n", isAdmin)
    }

    // Get Database Transaction Interface
    txInterface, ok := scontext.GetTransactionFromRequest[string, MyUserType](r)
    if ok {
        // Option 1: Control the transaction via the interface
        // err := txInterface.Commit() // Usually done in middleware after handler

        // Option 2: Get the underlying *gorm.DB for GORM operations
        gormTx := txInterface.GetDB()
        if gormTx != nil {
            // Perform GORM operations using gormTx
            var result MyModel
            if err := gormTx.Where("id = ?", someID).First(&result).Error; err != nil {
                // Handle GORM error within the transaction
                // The transaction might be rolled back later by middleware
            } else {
                fmt.Printf("Found model: %+v\n", result)
            }
        }
    } else {
        fmt.Println("No database transaction found in context.")
    }


    // ... handler logic ...
}
```

Always use these helper functions from `pkg/scontext` to interact with SRouter-managed context values for safety and maintainability. Remember that the user object (`scontext.GetUserFromRequest`) is returned as a pointer (`*U`). For transactions, add the `middleware.GormTransactionWrapper` using `scontext.WithTransaction`, and retrieve the `scontext.DatabaseTransaction` interface using `scontext.GetTransactionFromRequest`. Use `GetDB()` on the retrieved interface to perform GORM-specific operations.
