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

	// Define a simple route configuration
	helloRoute := router.RouteConfigBase{
		Path:    "/hello", // Path relative to sub-router prefix (root in this case)
		Methods: []router.HttpMethod{router.MethodGet},
		Handler: func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"message":"Hello, World!"}`))
		},
		// AuthLevel: nil, // Defaults to NoAuth
	}

	// Create a router configuration, including sub-routers and routes
	routerConfig := router.RouterConfig{
		Logger:            logger,
		GlobalTimeout:     2 * time.Second,
		GlobalMaxBodySize: 1 << 20, // 1 MB
		// Define sub-routers. Even top-level routes belong to a sub-router (e.g., with an empty prefix).
                SubRouters: []router.SubRouterConfig{
                        {
                                PathPrefix: "", // Root-level routes
                                Routes: []router.RouteDefinition{ // Holds RouteConfigBase or GenericRouteRegistrationFunc
                                        helloRoute,
                                        // Add more RouteConfigBase or GenericRouteRegistrationFunc here
                                },
                                // Middlewares specific to this sub-router can be added here
                        },
			// Add more sub-routers here (e.g., { PathPrefix: "/api/v1", Routes: [...] })
		},
                // Global middlewares can be added here
                // Middlewares: []common.Middleware{
                //  // Logging is handled automatically when EnableTraceLogging is true
                // },
	}

	// Define the authentication function (replace with your actual logic)
	// This function is required by NewRouter, even if AuthLevel is NoAuth everywhere.
        authFunction := func(ctx context.Context, token string) (*string, bool) {
                // Example: Check if token is "valid-token"
                if token == "valid-token" {
                        user := "user-id-from-token"
                        return &user, true // Return user pointer and true if valid
                }
                return nil, false // Return nil and false if invalid
        }

	// Define a function to extract a comparable UserID from the User object (returned by authFunction)
	// In this case, the User object is just a string (the user ID itself).
        userIdFromUserFunction := func(user *string) string {
                if user == nil {
                        return ""
                }
                return *user
        }

	// Create the router. Routes are defined within the routerConfig.
	// The type parameters define the UserID type (string) and User object type (string).
	r := router.NewRouter[string, string](routerConfig, authFunction, userIdFromUserFunction)

	// Start the server using the created router as the handler
	fmt.Println("Server listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", r))
}
```

Key components:

-   **`RouterConfig`**: Holds global settings like logger, timeouts, body size limits, and global middleware.
-   **`authFunction`**: A function `func(ctx context.Context, token string) (*UserObjectType, bool)` that validates an authentication token (currently expects Bearer token) and returns a pointer to the user object and a boolean indicating success. Used by the built-in middleware when `AuthLevel` is set.
-   **`userIdFromUserFunction`**: A function `func(user *UserObjectType) UserIDType` that extracts the comparable User ID from the user object returned by `authFunction`. Used by the built-in middleware.
-   **`NewRouter[UserIDType, UserObjectType]`**: The constructor for the router. The type parameters define the type used for user IDs (`UserIDType`, must be comparable) and the type used for the user object (`UserObjectType`, can be any type) potentially stored in the context.
-   **`RouterConfig.SubRouters`**: A slice of `SubRouterConfig` where routes are defined. Each `SubRouterConfig` has a `PathPrefix` and a `Routes` slice of `router.RouteDefinition` values.
-   **`RouteConfigBase` / `RouteConfig[T, U]`**: Structs used within the `SubRouterConfig.Routes` slice to define individual routes, their paths (relative to the sub-router prefix), methods, handlers, authentication levels, etc.

See the [Authentication](./authentication.md) and [Configuration Reference](./configuration.md) sections for more details on these components.
