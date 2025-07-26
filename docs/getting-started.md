# Getting Started with SRouter

This guide covers installation and basic usage of SRouter, a high-performance HTTP router framework built on `julienschmidt/httprouter` with Go generics support.

## Installation

To install SRouter, use `go get`:

```bash
go get github.com/Suhaibinator/SRouter
```

## Requirements

- Go 1.24.0 or higher
- Dependencies (managed via Go modules):
  - [julienschmidt/httprouter](https://github.com/julienschmidt/httprouter) v1.3.0 (Core routing engine)
  - [go.uber.org/zap](https://github.com/uber-go/zap) v1.27.0 (Structured logging)
  - [github.com/google/uuid](https://github.com/google/uuid) v1.6.0 (Used internally, e.g., trace IDs)
  - [go.uber.org/ratelimit](https://github.com/uber-go/ratelimit) v0.3.1 (Used by built-in rate limiting middleware)
  - [google.golang.org/protobuf](https://github.com/protocolbuffers/protobuf-go) v1.36.6 (Required if using `ProtoCodec`)
  - [gorm.io/gorm](https://gorm.io/) v1.30.0 (Required if using `GormTransactionWrapper` or related DB features)
  - [github.com/prometheus/client_golang](https://github.com/prometheus/client_golang) v1.22.0 (Required if using the Prometheus metrics system)

All required dependencies will be automatically downloaded and installed when you run `go get github.com/Suhaibinator/SRouter`.

## Basic Usage

### Creating a Router

To start, you need to create a router instance. This involves setting up a `RouterConfig` and providing authentication functions.

```go
package main

import (
	"context"
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
		ServiceName:       "example-service", // Required field
		Logger:            logger,
		GlobalTimeout:     2 * time.Second,
		GlobalMaxBodySize: 1 << 20, // 1 MB
		// Define sub-routers. Even top-level routes belong to a sub-router (e.g., with an empty prefix).
		SubRouters: []router.SubRouterConfig{
			{
				PathPrefix: "", // Root-level routes
				Routes: []router.RouteDefinition{
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

	// Create the router. Routes are defined within the routerConfig.
	// The type parameters define the UserID type (string) and User object type (string).
	r := router.NewRouter[string, string](routerConfig, authFunction, userIdFromUserFunction)

	// Start the server using the created router as the handler
	fmt.Println("Server listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", r))
}
```

### Key Components

- **`RouterConfig`**: Holds global settings like logger, timeouts, body size limits, and global middleware.
- **`authFunction`**: A function `func(ctx context.Context, token string) (UserObjectType, bool)` that validates an authentication token (currently expects Bearer token) and returns the user object and a boolean indicating success. Used by the built-in middleware when `AuthLevel` is set.
- **`userIdFromUserFunction`**: A function `func(user UserObjectType) UserIDType` that extracts the comparable User ID from the user object returned by `authFunction`. Used by the built-in middleware.
- **`NewRouter[UserIDType, UserObjectType]`**: The constructor for the router. The type parameters define the type used for user IDs (`UserIDType`, must be comparable) and the type used for the user object (`UserObjectType`, can be any type) potentially stored in the context.
- **`RouterConfig.SubRouters`**: A slice of `SubRouterConfig` where routes are defined. Each `SubRouterConfig` has a `PathPrefix` and a `Routes` slice.
- **`RouteConfigBase` / `RouteConfig[T, U]`**: Structs used within the `SubRouterConfig.Routes` slice to define individual routes, their paths (relative to the sub-router prefix), methods, handlers, authentication levels, etc.

## Next Steps

- Learn about [Authentication](./authentication.md) to secure your routes
- Explore the [Configuration Reference](./configuration.md) for all available options
- Check out [Routing](./routing.md) for advanced routing features
- See [Examples](./examples.md) for more complex use cases