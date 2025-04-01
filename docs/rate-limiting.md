# Rate Limiting

SRouter provides a flexible rate limiting system, configurable at the global, sub-router, or individual route level. It helps protect your application from abuse and ensures fair usage. Internally, it utilizes [Uber's `ratelimit` library](https://github.com/uber-go/ratelimit), which implements an efficient leaky bucket algorithm for smooth rate limiting (avoiding sudden bursts).

## Configuration

Rate limiting is configured using the `middleware.RateLimitConfig` struct. You can set it globally (`GlobalRateLimit` in `RouterConfig`), per sub-router (`RateLimitOverride` in `SubRouterConfig`), or per route (`RateLimit` in `RouteConfigBase` or `RouteConfig`). Settings cascade, with the most specific configuration taking precedence.

```go
import (
	"net/http"
	"time"
	"github.com/Suhaibinator/SRouter/pkg/middleware"
	"github.com/Suhaibinator/SRouter/pkg/router"
)

// Example: Global Rate Limit (applied to all routes unless overridden)
routerConfig := router.RouterConfig{
    // ... other config
    GlobalRateLimit: &middleware.RateLimitConfig[any, any]{ // Use [any, any] for global/sub-router
        BucketName: "global_ip_limit", // Unique name for the bucket
        Limit:      100,               // Allow 100 requests...
        Window:     time.Minute,       // ...per minute
        Strategy:   middleware.RateLimitStrategyIP, // Limit based on client IP
    },
}

// Example: Sub-Router Override
subRouter := router.SubRouterConfig{
    PathPrefix: "/api/v1/sensitive",
    RateLimitOverride: &middleware.RateLimitConfig[any, any]{ // Use [any, any]
        BucketName: "sensitive_api_user_limit",
        Limit:      20,
        Window:     time.Hour,
        Strategy:   middleware.RateLimitStrategyUser, // Limit based on authenticated user ID
    },
    // ... other sub-router config
}

// Example: Route-Specific Limit
route := router.RouteConfig[MyReq, MyResp]{ // Use specific types for route config
    Path:    "/heavy-operation",
    Methods: []string{"POST"},
    RateLimit: &middleware.RateLimitConfig[any, any]{ // Use [any, any] here too
        BucketName: "heavy_op_ip_limit",
        Limit:      5,
        Window:     time.Minute,
        Strategy:   middleware.RateLimitStrategyIP,
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

SRouter defines several strategies using constants in the `pkg/middleware` package:

1.  **`middleware.RateLimitStrategyIP`**: Limits requests based on the client's IP address (as determined by the [IP Configuration](./ip-configuration.md)). This is the most common strategy for anonymous or global rate limiting.

    ```go
    RateLimit: &middleware.RateLimitConfig[any, any]{
        BucketName: "ip_limit",
        Limit:      100,
        Window:     time.Minute,
        Strategy:   middleware.RateLimitStrategyIP,
    }
    ```

2.  **`middleware.RateLimitStrategyUser`**: Limits requests based on the authenticated user ID. This requires an [Authentication](./authentication.md) middleware to run *before* the rate limiter to populate the user ID in the request context.

    ```go
    RateLimit: &middleware.RateLimitConfig[any, any]{
        BucketName: "user_limit",
        Limit:      50,
        Window:     time.Hour,
        Strategy:   middleware.RateLimitStrategyUser, // Requires Auth middleware first
    }
    ```

3.  **`middleware.RateLimitStrategyCustom`**: Limits requests based on a custom key extracted from the request using the `KeyExtractor` function. This allows for flexible strategies, like limiting based on API keys, specific headers, or combinations of factors.

    ```go
    RateLimit: &middleware.RateLimitConfig[any, any]{
        BucketName: "api_key_limit",
        Limit:      200,
        Window:     time.Hour,
        Strategy:   middleware.RateLimitStrategyCustom,
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
                ip, _ := middleware.GetClientIPFromRequest[string, string](r) // Adjust types
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
    Methods: []string{"POST"},
    RateLimit: &middleware.RateLimitConfig[any, any]{
        BucketName: "auth_ip_limit", // Shared bucket name
        Limit:      5,
        Window:     time.Minute,
        Strategy:   middleware.RateLimitStrategyIP,
    },
    // ...
}

registerRoute := router.RouteConfigBase{
    Path:    "/register",
    Methods: []string{"POST"},
    RateLimit: &middleware.RateLimitConfig[any, any]{
        BucketName: "auth_ip_limit", // Same bucket name
        Limit:      5,               // Limit applies to combined requests
        Window:     time.Minute,
        Strategy:   middleware.RateLimitStrategyIP,
    },
    // ...
}
```

## Custom Rate Limit Responses

Provide an `ExceededHandler` in `RateLimitConfig` to send a custom response when a limit is hit.

```go
RateLimit: &middleware.RateLimitConfig[any, any]{
    BucketName: "custom_response_limit",
    Limit:      10,
    Window:     time.Minute,
    Strategy:   middleware.RateLimitStrategyIP,
    ExceededHandler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Content-Type", "application/json")
        w.Header().Set("X-RateLimit-Retry-After", "60") // Example custom header
        w.WriteHeader(http.StatusTooManyRequests) // 429
        w.Write([]byte(`{"error": "Slow down!", "message": "You have exceeded the rate limit. Please wait a minute."}`))
    }),
}
```

See the `examples/rate-limiting` directory for a runnable example.
