# Generic Routes

SRouter leverages Go 1.18+ generics to provide type-safe handling of request and response data. This eliminates the need for manual type assertions and reduces boilerplate code.

## Defining Generic Routes

Generic routes are defined using the `RouteConfig[T, U]` struct, where `T` is the request type and `U` is the response type. They require a `Codec` for marshaling/unmarshaling and a `GenericHandler`.

```go
// Define request and response types
type CreateUserReq struct {
 Name  string `json:"name"`
 Email string `json:"email"`
}

type CreateUserResp struct {
 ID    string `json:"id"`
 Name  string `json:"name"`
 Email string `json:"email"`
}

// Define a generic handler function
 // It takes the http.Request and the decoded request object (type T)
 // It returns the response object (type U) and an error
 func CreateUserHandler(r *http.Request, req CreateUserReq) (CreateUserResp, error) {
  // Access request context if needed, e.g., for UserID, Transaction, etc.
  // userID, ok := scontext.GetUserIDFromRequest[string, string](r) // Use scontext, replace types as needed
  // txInterface, txOK := scontext.GetTransactionFromRequest[string, string](r) // Use scontext
  // if txOK { gormTx := txInterface.GetDB() /* use gormTx */ }

  fmt.Printf("Received request to create user: Name=%s, Email=%s\n", req.Name, req.Email)

  // In a real application, you would interact with a database or service
 // If an error occurs (e.g., validation, database error), return it:
 // if req.Name == "" {
 //  return CreateUserResp{}, router.NewHTTPError(http.StatusBadRequest, "Name cannot be empty")
 // }

 // Simulate successful creation
 createdUser := CreateUserResp{
  ID:    "user-" + uuid.NewString(), // Example ID
  Name:  req.Name,
  Email: req.Email,
 }

 return createdUser, nil // Return the response object and nil error on success
}

// Define the route configuration
createUserRoute := router.RouteConfig[CreateUserReq, CreateUserResp]{
 Path:      "/users",
 Methods:   []router.HttpMethod{router.MethodPost},
 AuthLevel: router.Ptr(router.AuthRequired), // Example: Requires authentication
 Codec:     codec.NewJSONCodec[CreateUserReq, CreateUserResp](), // Specify the codec
 Handler:   CreateUserHandler, // Assign the generic handler
 // Optional overrides for timeout, body size, or rate limit
 Overrides: common.RouteOverrides{
     // Timeout:     3 * time.Second,
     // MaxBodySize: 2 << 20, // 2 MB
     // RateLimit:   &common.RateLimitConfig[any, any]{...},
 },
 Sanitizer: func(req CreateUserReq) (CreateUserReq, error) { // Optional: Sanitize data after decoding
  if req.Name == "invalid" {
   return CreateUserReq{}, router.NewHTTPError(http.StatusBadRequest, "Invalid name provided")
  }
  // Example: Trim spaces
  req.Name = strings.TrimSpace(req.Name)
  return req, nil // Return the modified request (or original if no changes) and nil error
 },
}
```

## Registering Generic Routes

The **preferred and recommended** way to register generic routes is declaratively within a `SubRouterConfig` using the `NewGenericRouteDefinition` helper function. This ensures that path prefixes, middleware, and configuration overrides (timeout, max body size, rate limit) are correctly applied.

```go
// Define the route configuration (as shown previously)
createUserRoute := router.RouteConfig[CreateUserReq, CreateUserResp]{ /* ... */ }

// Define the SubRouterConfig
apiV1SubRouter := router.SubRouterConfig{
    PathPrefix: "/api/v1",
    // Middlewares specific to this sub-router can go here
    Routes: []router.RouteDefinition{
        // ... other routes (RouteConfigBase or other NewGenericRouteDefinition calls) ...

        // Use NewGenericRouteDefinition to wrap the generic RouteConfig.
        // The last two type parameters (string, string) must match the
        // UserIDType and UserObjectType used in NewRouter[UserIDType, UserObjectType].
        router.NewGenericRouteDefinition[CreateUserReq, CreateUserResp, string, string](createUserRoute),
    },
    // Optional overrides for all routes in this sub-router
    Overrides: common.RouteOverrides{
        // Timeout:     5 * time.Second,
        // MaxBodySize: 4 << 20,
        // RateLimit:   &common.RateLimitConfig[any, any]{...},
    },
}

// This SubRouterConfig is then included in the main RouterConfig.SubRouters slice
// passed to router.NewRouter.
routerConfig := router.RouterConfig{
    // ... Logger, GlobalTimeout, etc. ...
    SubRouters: []router.SubRouterConfig{
        apiV1SubRouter,
        // Potentially other sub-routers (e.g., for root path: { PathPrefix: "", Routes: [...] })
    },
    // ...
}

// Create the router
// r := router.NewRouter[string, string](routerConfig, authFunc, userIDFunc)
```

**Note on Direct Registration:** While a `router.RegisterGenericRoute` function exists, it's primarily used internally by `NewGenericRouteDefinition`. Direct use is discouraged as it bypasses the sub-router configuration logic (path prefixing, middleware application, override calculation) and requires manual calculation and passing of effective settings, which can be error-prone. Always prefer the declarative approach using `NewGenericRouteDefinition` within `SubRouterConfig`.

## Key Components

-   **`RouteConfig[T, U]`**: Defines the configuration for a generic route, including path, methods, auth level, codec, handler, **sanitizer**, and overrides.
-   **`GenericHandler[T, U]`**: The function signature `func(*http.Request, T) (U, error)`. It receives the `http.Request` (for accessing context, headers, etc.) and the *potentially sanitized* decoded request object `T`. It returns the response object `U` and an `error`. If the error is non-nil, SRouter handles sending the appropriate HTTP error response (using `router.HTTPError` for specific status codes).
-   **`Sanitizer func(T) (T, error)`**: An optional function that runs *after* the request data `T` is successfully decoded by the `Codec` but *before* the `GenericHandler` is called. It receives the decoded data (`T`) and can return a modified version of it (`T`). If it returns a non-nil error, the request processing stops, and a `400 Bad Request` (or the error specified if it's an `HTTPError`) is returned. If it returns the modified (or original) data and a nil error, that data is passed to the `GenericHandler`.
-   **`Codec[T, U]`**: An interface responsible for decoding the request (`T`) and encoding the response (`U`). See [Custom Codecs](./codecs.md).
-   **`NewGenericRouteDefinition`**: The **recommended** helper function used within `SubRouterConfig.Routes` to wrap a `RouteConfig[T, U]` for declarative registration, ensuring proper application of sub-router settings.
-   **`RegisterGenericRoute`**: An internal function called by `NewGenericRouteDefinition`. Direct use is discouraged.

Using generic routes significantly improves type safety and developer experience when dealing with structured request and response data.
