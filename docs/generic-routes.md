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
  // userID, ok := middleware.GetUserIDFromRequest[string, string](r) // Replace types as needed
  // txInterface, txOK := middleware.GetTransactionFromRequest[string, string](r)
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
 Path:        "/users",
 Methods:     []router.HttpMethod{router.MethodPost},
 AuthLevel:   router.Ptr(router.AuthRequired), // Example: Requires authentication (use router.AuthRequired)
 Codec:       codec.NewJSONCodec[CreateUserReq, CreateUserResp](), // Specify the codec
 Handler:     CreateUserHandler, // Assign the generic handler
 // Middlewares, Timeout, MaxBodySize, RateLimit can be set here too
}
```

## Registering Generic Routes

There are two main ways to register generic routes:

1.  **Declaratively within `SubRouterConfig`:** This is the preferred method for routes belonging to a sub-router. Use the `NewGenericRouteDefinition` helper function.

    ```go
    // Inside a SubRouterConfig's Routes slice:
    apiV1SubRouter := router.SubRouterConfig{
        PathPrefix: "/api/v1",
        Routes: []any{
            // ... other routes
            router.NewGenericRouteDefinition[CreateUserReq, CreateUserResp, string, string](createUserRoute), // Pass the RouteConfig
        },
        // ... other sub-router config
    }
    ```
    The last two type parameters (`string, string` in this example) must match the `UserIDType` and `UserObjectType` of your `router.NewRouter` instance.

2.  **Directly on the Router Instance:** Use this for routes that don't belong to a sub-router.

    ```go
    // Assuming 'r' is your router.Router[string, string] instance
    // Note: You need to provide effective settings (timeout, max body size, rate limit)
    // which are usually the global defaults (0/nil) when registering directly.
    err := router.RegisterGenericRoute[CreateUserReq, CreateUserResp, string, string](
        r,
        createUserRoute, // The RouteConfig defined earlier
        r.Config.GlobalTimeout,     // Effective timeout
        r.Config.GlobalMaxBodySize, // Effective max body size
        r.Config.GlobalRateLimit,   // Effective rate limit
    )
    if err != nil {
        log.Fatalf("Failed to register generic route: %v", err)
    }
    ```
    Again, the last two type parameters (`string, string`) must match your router's types. The `RegisterGenericRoute` function requires the *effective* configuration values (timeout, max body size, rate limit) that apply to this route. When registering directly on the root router, these are typically the global defaults from the `RouterConfig`.

## Key Components

-   **`RouteConfig[T, U]`**: Defines the configuration for a generic route, including path, methods, auth level, codec, handler, and overrides.
-   **`GenericHandler[T, U]`**: The function signature `func(*http.Request, T) (U, error)`. It receives the `http.Request` (for accessing context, headers, etc.) and the decoded request object `T`. It returns the response object `U` and an `error`. If the error is non-nil, SRouter handles sending the appropriate HTTP error response (using `router.HTTPError` for specific status codes).
-   **`Codec[T, U]`**: An interface responsible for decoding the request (`T`) and encoding the response (`U`). See [Custom Codecs](./codecs.md).
-   **`NewGenericRouteDefinition`**: A helper function used within `SubRouterConfig.Routes` to wrap a `RouteConfig[T, U]` for declarative registration.
-   **`RegisterGenericRoute`**: Function to register a generic route directly on the router instance.

Using generic routes significantly improves type safety and developer experience when dealing with structured request and response data.
