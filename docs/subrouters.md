# Sub-Routers

Sub-routers allow you to group related routes under a common path prefix and apply shared configuration like middleware, timeouts, body size limits, and rate limits.

## Defining Sub-Routers

Sub-routers are defined using the `SubRouterConfig` struct and added to the `SubRouters` slice within the main `RouterConfig`. Routes can be added declaratively within the sub-router configuration or imperatively after router creation using `RegisterGenericRouteOnSubRouter` or by registering entire sub-routers with `RegisterSubRouter`.

```go
// Define sub-router configurations
apiV1SubRouter := router.SubRouterConfig{
        PathPrefix: "/api/v1",
        Overrides: common.RouteOverrides{
                Timeout:     3 * time.Second, // Overrides GlobalTimeout
                MaxBodySize: 2 << 20,         // 2 MB, overrides GlobalMaxBodySize
        },
        // Middlewares specific to /api/v1 routes can be added here
        // Middlewares: []common.Middleware{ myV1Middleware },
        Routes: []router.RouteDefinition{ // Holds RouteConfigBase or GenericRouteDefinition
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
        Routes: []router.RouteDefinition{
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
-   `Overrides`: Use `common.RouteOverrides` to set sub-router specific `Timeout`, `MaxBodySize`, or `RateLimit` settings. These overrides do **not** cascade to nested sub-routers.
-   `Routes`: A slice of `router.RouteDefinition` that can contain `RouteConfigBase` for standard routes or `GenericRouteDefinition` (created via `NewGenericRouteDefinition`) for generic routes. Paths within these routes are relative to the `PathPrefix`.
-   `Middlewares`: Middleware applied to routes within this sub-router. These are **added to** (not replacing) any global middlewares defined in RouterConfig.
-   `AuthLevel`: Default authentication level for all routes in this sub-router (can be overridden at the route level).

## Nested Sub-Routers

You can nest `SubRouterConfig` structs within the `SubRouters` field of another `SubRouterConfig` to create hierarchical routing structures. Path prefixes are concatenated automatically.

**Important notes about nested sub-routers:**
- Path prefixes are concatenated (e.g., `/api` + `/v1` = `/api/v1`)
- Configuration overrides (timeout, max body size, rate limit) are **not inherited** - each sub-router level must explicitly set its own overrides
- Middlewares are **additive** - they combine as you go deeper (global + parent sub-router + child sub-router + route)
- The most specific setting takes precedence for overrides

```go
// Define nested structure
usersV1SubRouter := router.SubRouterConfig{
        PathPrefix: "/users", // Relative to /api/v1 -> /api/v1/users
        // Overrides: common.RouteOverrides{Timeout: 1 * time.Second}, // Could override /api/v1's timeout
        Routes: []router.RouteDefinition{
                router.RouteConfigBase{ Path: "/:id", Methods: []router.HttpMethod{router.MethodGet}, Handler: GetUserHandler }, // /api/v1/users/:id
                router.NewGenericRouteDefinition[UserReq, UserResp, string, string]( // Assume types/codec/handler exist
                        router.RouteConfig[UserReq, UserResp]{ Path: "/info", Methods: []router.HttpMethod{router.MethodPost}, Codec: userCodec, Handler: UserInfoHandler }, // /api/v1/users/info
                ),
        },
}

apiV1SubRouter := router.SubRouterConfig{
        PathPrefix: "/v1", // Relative to /api -> /api/v1
        Overrides: common.RouteOverrides{Timeout: 3 * time.Second}, // Applies to routes in this sub-router only, NOT inherited by nested sub-routers
        SubRouters: []router.SubRouterConfig{usersV1SubRouter}, // Nest the users sub-router
        Routes: []router.RouteDefinition{
                router.RouteConfigBase{ Path: "/status", Methods: []router.HttpMethod{router.MethodGet}, Handler: V1StatusHandler }, // /api/v1/status
        },
}

apiSubRouter := router.SubRouterConfig{
        PathPrefix: "/api", // Root prefix for this group
        SubRouters: []router.SubRouterConfig{apiV1SubRouter}, // Nest the v1 sub-router
        Routes: []router.RouteDefinition{
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

**Configuration precedence:**
- **Overrides** (timeouts, body size, rate limits, auth level): The most specific setting wins (Route > Sub-Router > Global). Each level must explicitly set overrides; they are not inherited.
- **Middlewares**: These are combined additively in order: Global → Outer Sub-Router → Inner Sub-Router → Route-specific. All applicable middlewares run in this sequence.

## Imperative Route Registration

While routes are typically defined declaratively within `SubRouterConfig`, you can also register routes imperatively after router creation. This is useful for dynamic route registration or when working with routes that need to be conditionally added.

### Registering Individual Routes

Use `RegisterGenericRouteOnSubRouter` to add a single generic route to an existing sub-router:

```go
// After creating the router
r := router.NewRouter[string, string](routerConfig, authFunction, userIdFromUserFunction)

// Register a generic route on a specific sub-router
err := router.RegisterGenericRouteOnSubRouter(
    r,
    "/api/v1", // Target sub-router path prefix
    router.RouteConfig[CreateUserReq, CreateUserResp]{
        Path:      "/users", // Path relative to the sub-router prefix
        Methods:   []router.HttpMethod{router.MethodPost},
        AuthLevel: router.Ptr(router.AuthRequired),
        Codec:     codec.NewJSONCodec[CreateUserReq, CreateUserResp](),
        Handler:   CreateUserHandler,
    },
)
if err != nil {
    log.Fatalf("Failed to register route: %v", err)
}
```

This function will:
- Find the sub-router with the matching path prefix
- Apply the sub-router's configuration (middleware and any overrides)
- Prefix the route path with the sub-router's path prefix
- Register the route with all appropriate settings

### Registering Entire Sub-Routers

You can also register complete sub-routers after router creation using `RegisterSubRouter`:

```go
// Create the router first
r := router.NewRouter[string, string](routerConfig, authFunction, userIdFromUserFunction)

// Define a new sub-router
adminSubRouter := router.SubRouterConfig{
    PathPrefix: "/admin",
    AuthLevel:  router.Ptr(router.AuthRequired), // All admin routes require auth by default
    Routes: []router.RouteDefinition{
        router.RouteConfigBase{
            Path:    "/users",
            Methods: []router.HttpMethod{router.MethodGet},
            Handler: ListAdminUsersHandler,
        },
        router.RouteConfigBase{
            Path:    "/settings",
            Methods: []router.HttpMethod{router.MethodGet, router.MethodPost},
            Handler: AdminSettingsHandler,
        },
    },
}

// Register the sub-router dynamically
r.RegisterSubRouter(adminSubRouter)
```

### Programmatic Sub-Router Nesting

For building sub-router hierarchies programmatically, use `RegisterSubRouterWithSubRouter`:

```go
// Create parent and child sub-routers
apiRouter := router.SubRouterConfig{PathPrefix: "/api"}
v1Router := router.SubRouterConfig{
    PathPrefix: "/v1",
    Routes: []router.RouteDefinition{
        router.RouteConfigBase{
            Path:    "/status",
            Methods: []router.HttpMethod{router.MethodGet},
            Handler: StatusHandler,
        },
    },
}

// Nest v1Router under apiRouter
router.RegisterSubRouterWithSubRouter(&apiRouter, v1Router)

// Now apiRouter contains v1Router, resulting in routes under /api/v1
```
