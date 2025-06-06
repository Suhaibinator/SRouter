# Rate Limiting

SRouter provides a flexible rate limiting system, configurable at the global, sub-router, or individual route level. It helps protect your application from abuse and ensures fair usage. Internally, it utilizes [Uber's `ratelimit` library](https://github.com/uber-go/ratelimit), which implements an efficient leaky bucket algorithm for smooth rate limiting (avoiding sudden bursts).

## Configuration

Rate limiting is configured using the `common.RateLimitConfig` struct (defined in `pkg/common/types.go`). You can set it globally (`GlobalRateLimit` in `RouterConfig`), per sub-router via `SubRouterConfig.Overrides.RateLimit`, or per route (`Overrides.RateLimit` in `RouteConfigBase`/`RouteConfig`). Settings cascade, with the most specific configuration taking precedence.

```go
import (
	"net/http"
	"time"
	"github.com/Suhaibinator/SRouter/pkg/common" // Use common package for config
	"github.com/Suhaibinator/SRouter/pkg/router"
)

// Example: Global Rate Limit (applied to all routes unless overridden)
routerConfig := router.RouterConfig{
    // ... other config
    GlobalRateLimit: &common.RateLimitConfig[any, any]{ // Use common.RateLimitConfig
        BucketName: "global_ip_limit", // Unique name for the bucket
        Limit:      100,               // Allow 100 requests...
        Window:     time.Minute,       // ...per minute
        Strategy:   common.RateLimitStrategyIP, // Use common constants
    },
}

// Example: Sub-Router Override
subRouter := router.SubRouterConfig{
    PathPrefix: "/api/v1/sensitive",
    Overrides: common.RouteOverrides{
        RateLimit: &common.RateLimitConfig[any, any]{ // Use common.RateLimitConfig
            BucketName: "sensitive_api_user_limit",
            Limit:      20,
            Window:     time.Hour,
            Strategy:   common.RateLimitStrategyUser, // Use common constants
        },
    },
    // ... other sub-router config
}

// Example: Route-Specific Limit
route := router.RouteConfig[MyReq, MyResp]{ // Use specific types for route config
    Path:    "/heavy-operation",
    Methods: []router.HttpMethod{router.MethodPost},
    RateLimit: &common.RateLimitConfig[any, any]{ // Use common.RateLimitConfig
        BucketName: "heavy_op_ip_limit",
        Limit:      5,
        Window:     time.Minute,
        Strategy:   common.RateLimitStrategyIP, // Use common constants
    },
    // ... other route config
}

// Note: The type parameters for RateLimitConfig are generally [any, any]
// as the rate limiting middleware itself doesn't usually depend on the
// specific request/response types T and U of a generic route.
```

Key `RateLimitConfig` fields:

-   `BucketName`: A unique string identifying the rate limit bucket. Requests sharing the same bucket name also share the same rate limit counter.
-   `Limit`: The maximum number of requests allowed within the specified `Window`.
-   `Window`: The duration over which the `Limit` is enforced.
-   `Strategy`: Determines how the rate limit is applied (see below).
-   `KeyExtractor`: (Used only with `RateLimitStrategyCustom`) A function to extract a custom key for rate limiting.
-   `ExceededHandler`: (Optional) An `http.HandlerFunc` to customize the response when the rate limit is exceeded (defaults to a standard 429 Too Many Requests response).

## Rate Limiting Strategies

SRouter defines several strategies using constants of type `common.RateLimitStrategy` (defined in `pkg/common/types.go`):

1.  **`common.RateLimitStrategyIP`**: Limits requests based on the client's IP address. The IP address is extracted internally based on the router's [IP Configuration](./ip-configuration.md) and stored in the context. This is the most common strategy for anonymous or global rate limiting.

    ```go
    RateLimit: &common.RateLimitConfig[any, any]{
        BucketName: "ip_limit",
        Limit:      100,
        Window:     time.Minute,
        Strategy:   common.RateLimitStrategyIP,
    }
    ```

2.  **`common.RateLimitStrategyUser`**: Limits requests based on the authenticated user ID stored in the request context (via `scontext.GetUserIDFromRequest`). This requires an [Authentication](./authentication.md) mechanism (built-in or custom middleware) to run *before* the rate limiter to populate the user ID.

    ```go
    RateLimit: &common.RateLimitConfig[any, any]{
        BucketName: "user_limit",
        Limit:      50,
        Window:     time.Hour,
        Strategy:   common.RateLimitStrategyUser, // Requires User ID in context
    }
    ```

3.  **`common.RateLimitStrategyCustom`**: Limits requests based on a custom key extracted from the request using the `KeyExtractor` function provided in the `RateLimitConfig`. This allows for flexible strategies, like limiting based on API keys, specific headers, or combinations of factors.

    ```go
    RateLimit: &common.RateLimitConfig[any, any]{
        BucketName: "api_key_limit",
        Limit:      200,
        Window:     time.Hour,
        Strategy:   common.RateLimitStrategyCustom,
        KeyExtractor: func(r *http.Request) (string, error) {
            // Example: Extract API key from header or query param
            apiKey := r.Header.Get("X-API-Key")
            if apiKey == "" {
                apiKey = r.URL.Query().Get("api_key")
            }
            if apiKey == "" {
                // Fall back to IP if no key found? Or return error?
                // Returning an error might block the request.
                // Returning a common key (like IP) groups unkeyed requests.
                ip, _ := scontext.GetClientIPFromRequest[string, string](r) // Use scontext, adjust types
                return "ip:" + ip, nil // Prefix to avoid collision with actual keys
            }
            return "key:" + apiKey, nil // Prefix to avoid collision
        },
    }
    ```

## Shared Rate Limit Buckets

You can enforce a shared rate limit across multiple endpoints by assigning the *same* `BucketName` in their respective `RateLimitConfig`.

```go
// Login endpoint shares a bucket with register endpoint
loginRoute := router.RouteConfigBase{
    Path:    "/login",
    Methods: []router.HttpMethod{router.MethodPost},
    RateLimit: &common.RateLimitConfig[any, any]{ // Use common.RateLimitConfig
        BucketName: "auth_ip_limit", // Shared bucket name
        Limit:      5,
        Window:     time.Minute,
        Strategy:   common.RateLimitStrategyIP, // Use common constants
    },
    // ...
}

registerRoute := router.RouteConfigBase{
    Path:    "/register",
    Methods: []router.HttpMethod{router.MethodPost},
    RateLimit: &common.RateLimitConfig[any, any]{ // Use common.RateLimitConfig
        BucketName: "auth_ip_limit", // Same bucket name
        Limit:      5,               // Limit applies to combined requests
        Window:     time.Minute,
        Strategy:   common.RateLimitStrategyIP, // Use common constants
    },
    // ...
}
```

## Custom Rate Limit Responses

Provide an `ExceededHandler` in `common.RateLimitConfig` to send a custom response when a limit is hit.

```go
RateLimit: &common.RateLimitConfig[any, any]{ // Use common.RateLimitConfig
    BucketName: "custom_response_limit",
    Limit:      10,
    Window:     time.Minute,
    Strategy:   common.RateLimitStrategyIP, // Use common constants
    ExceededHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        // Note: The default handler already sets X-RateLimit-* headers.
        // You might want to set additional headers or customize the body.
        w.Header().Set("Content-Type", "application/json")
        // w.Header().Set("X-RateLimit-Retry-After", "60") // Default handler sets Retry-After
        w.WriteHeader(http.StatusTooManyRequests) // 429
        w.Write([]byte(`{"error": "Slow down!", "message": "You have exceeded the rate limit. Please wait a minute."}`))
    }),
}
```

See the `examples/rate-limiting` directory for a runnable example.
