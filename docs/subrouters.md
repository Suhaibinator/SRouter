# Sub-Routers

Sub-routers allow you to group related routes under a common path prefix and apply shared configuration like middleware, timeouts, body size limits, and rate limits.

## Defining Sub-Routers

Sub-routers are defined using the `SubRouterConfig` struct and added to the `SubRouters` slice within the main `RouterConfig`.

```go
// Define sub-router configurations
apiV1SubRouter := router.SubRouterConfig{
	PathPrefix:          "/api/v1",
	TimeoutOverride:     3 * time.Second,     // Overrides GlobalTimeout
	MaxBodySizeOverride: 2 << 20,         // 2 MB, overrides GlobalMaxBodySize
	// Middlewares specific to /api/v1 routes can be added here
	// Middlewares: []common.Middleware{ myV1Middleware },
	Routes: []any{ // Use []any to hold different route types (RouteConfigBase or GenericRouteDefinition)
		// Standard route
		router.RouteConfigBase{
			Path:    "/users", // Becomes /api/v1/users
			Methods: []router.HttpMethod{router.MethodGet},
			Handler: ListUsersHandler, // Assume this handler exists
		},
		router.RouteConfigBase{
			Path:    "/users/:id", // Becomes /api/v1/users/:id
			Methods: []router.HttpMethod{router.MethodGet},
			Handler: GetUserHandler, // Assume this handler exists
		},
		// Declarative generic route using the helper
		router.NewGenericRouteDefinition[CreateUserReq, CreateUserResp, string, string](
			router.RouteConfig[CreateUserReq, CreateUserResp]{
				Path:      "/users", // Path relative to the sub-router prefix (/api/v1/users)
				Methods:   []router.HttpMethod{router.MethodPost},
				AuthLevel: router.Ptr(router.AuthRequired), // Example: Requires authentication
				Codec:     codec.NewJSONCodec[CreateUserReq, CreateUserResp](), // Assume codec exists
				Handler:   CreateUserHandler, // Assume this generic handler exists
				// Middlewares, Timeout, MaxBodySize, RateLimit can be set here too, overriding sub-router settings
			},
		),
	},
}

apiV2SubRouter := router.SubRouterConfig{
	PathPrefix: "/api/v2",
	Routes: []any{ // Use []any
		router.RouteConfigBase{
			Path:    "/users", // Becomes /api/v2/users
			Methods: []router.HttpMethod{router.MethodGet},
			Handler: ListUsersV2Handler, // Assume this handler exists
		},
	},
}

// Create a router with sub-routers
routerConfig := router.RouterConfig{
	Logger:            logger, // Assume logger exists
	GlobalTimeout:     2 * time.Second,
	GlobalMaxBodySize: 1 << 20, // 1 MB
	SubRouters: []router.SubRouterConfig{apiV1SubRouter, apiV2SubRouter},
	// Auth functions assumed to exist
}

r := router.NewRouter[string, string](routerConfig, authFunction, userIdFromUserFunction)

// Routes are now registered:
// GET /api/v1/users
// GET /api/v1/users/:id
// POST /api/v1/users
// GET /api/v2/users
```

Key points:

-   `PathPrefix`: Defines the base path for all routes within the sub-router.
-   `TimeoutOverride`, `MaxBodySizeOverride`, `RateLimitOverride`: Allow overriding global settings specifically for this sub-router.
-   `Routes`: A slice of `any` that can contain `RouteConfigBase` for standard routes or `GenericRouteDefinition` (created via `NewGenericRouteDefinition`) for generic routes. Paths within these routes are relative to the `PathPrefix`.
-   `Middlewares`: Middleware applied only to routes within this sub-router.

## Nested Sub-Routers

You can nest `SubRouterConfig` structs within the `SubRouters` field of another `SubRouterConfig` to create hierarchical routing structures. Path prefixes are concatenated, and configuration overrides cascade down the hierarchy (inner settings override outer settings).

```go
// Define nested structure
usersV1SubRouter := router.SubRouterConfig{
	PathPrefix: "/users", // Relative to /api/v1 -> /api/v1/users
	// TimeoutOverride: 1 * time.Second, // Could override /api/v1's timeout
	Routes: []any{
		router.RouteConfigBase{ Path: "/:id", Methods: []router.HttpMethod{router.MethodGet}, Handler: GetUserHandler }, // /api/v1/users/:id
		router.NewGenericRouteDefinition[UserReq, UserResp, string, string]( // Assume types/codec/handler exist
			router.RouteConfig[UserReq, UserResp]{ Path: "/info", Methods: []router.HttpMethod{router.MethodPost}, Codec: userCodec, Handler: UserInfoHandler }, // /api/v1/users/info
		),
	},
}

apiV1SubRouter := router.SubRouterConfig{
	PathPrefix: "/v1", // Relative to /api -> /api/v1
	TimeoutOverride: 3 * time.Second, // Applies to /api/v1/status and potentially nested routes unless overridden again
	SubRouters: []router.SubRouterConfig{usersV1SubRouter}, // Nest the users sub-router
	Routes: []any{
		router.RouteConfigBase{ Path: "/status", Methods: []router.HttpMethod{router.MethodGet}, Handler: V1StatusHandler }, // /api/v1/status
	},
}

apiSubRouter := router.SubRouterConfig{
	PathPrefix: "/api", // Root prefix for this group
	SubRouters: []router.SubRouterConfig{apiV1SubRouter}, // Nest the v1 sub-router
	Routes: []any{
		router.RouteConfigBase{ Path: "/health", Methods: []router.HttpMethod{router.MethodGet}, Handler: HealthHandler }, // /api/health
	},
}

// Register top-level sub-router during NewRouter
routerConfig := router.RouterConfig{
    SubRouters: []router.SubRouterConfig{apiSubRouter},
    // ... other config, logger, auth functions
}
r := router.NewRouter[string, string](routerConfig, authFunction, userIdFromUserFunction)

// Routes are now registered:
// GET /api/health
// GET /api/v1/status
// GET /api/v1/users/:id
// POST /api/v1/users/info
```

Configuration (timeouts, body size, rate limits, auth level, middleware) cascades: Global -> Outer Sub-Router -> Inner Sub-Router -> Route. The most specific setting takes precedence.
