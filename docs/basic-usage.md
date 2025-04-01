# Basic Usage

This section covers the fundamental concepts of using SRouter.

## Creating a Router

To start, you need to create a router instance. This involves setting up a `RouterConfig` and providing authentication functions.

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

Key components:

-   **`RouterConfig`**: Holds global settings like logger, timeouts, body size limits, and global middleware.
-   **`authFunction`**: A function `func(ctx context.Context, token string) (UserIDType, bool)` that validates an authentication token (or other credential) and returns the user ID and a boolean indicating success.
-   **`userIdFromUserFunction`**: A function `func(user UserObjectType) UserIDType` that extracts the comparable User ID from the user object returned by authentication middleware (if applicable).
-   **`NewRouter[UserIDType, UserObjectType]`**: The constructor for the router. The type parameters define the type used for user IDs (`UserIDType`, must be comparable) and the type used for the user object (`UserObjectType`, can be any type) stored in the context after successful authentication.
-   **`RegisterRoute`**: Used to register standard `http.HandlerFunc` routes using `RouteConfigBase`.

See the [Authentication](./authentication.md) and [Configuration Reference](./configuration.md) sections for more details on these components.
