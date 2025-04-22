# CORS Configuration

SRouter handles Cross-Origin Resource Sharing (CORS) directly within the router, ensuring correct context propagation and simplifying configuration. Instead of using a separate middleware, CORS is configured via the `CORSConfig` field within the main `RouterConfig`.

## Configuration

To enable and configure CORS, provide a `router.CORSConfig` struct to the `CORSConfig` field when creating your `RouterConfig`.

```go
import (
	"time"
	"github.com/Suhaibinator/SRouter/pkg/router"
)

// Example CORS configuration
corsCfg := &router.CORSConfig{
    Origins:          []string{"https://trusted.frontend.com", "http://localhost:3000"}, // Allowed origins
    Methods:          []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodOptions}, // Allowed methods
    Headers:          []string{"Content-Type", "Authorization", "X-Requested-With"}, // Allowed headers
    ExposeHeaders:    []string{"Content-Length", "X-Request-ID"}, // Headers browser JS can access
    AllowCredentials: true,                                       // Allow cookies/auth headers
    MaxAge:           12 * time.Hour,                             // Cache preflight results for 12 hours
}

// Configure the router
routerConfig := router.RouterConfig{
    // ... other config (logger, ServiceName, etc.)
    CORSConfig: corsCfg, // Assign the CORS configuration
    // ...
}

// Replace string, string with your actual UserIDType, UserObjectType
r := router.NewRouter[string, string](routerConfig, /* auth funcs */)
```

If `CORSConfig` is `nil` or not provided, CORS headers will not be added, and preflight requests will not be handled automatically.

## CORSConfig Fields

The `router.CORSConfig` struct (defined in `pkg/router/config.go`) has the following fields:

-   **`Origins`** (`[]string`): A list of allowed origin domains. You can specify exact domains (e.g., `"https://example.com"`) or use the wildcard `"*"` to allow any origin. **Using `"*"` is generally discouraged for production environments, especially if `AllowCredentials` is true.** This field is required if you want to enable CORS.
-   **`Methods`** (`[]string`): A list of allowed HTTP methods (e.g., `"GET"`, `"POST"`). If empty, it defaults to simple methods (`GET`, `POST`, `HEAD`).
-   **`Headers`** (`[]string`): A list of allowed request headers. If empty, it defaults to simple headers (`Accept`, `Accept-Language`, `Content-Language`, `Content-Type` if its value is `application/x-www-form-urlencoded`, `multipart/form-data`, or `text/plain`). Include headers like `"Authorization"` or custom headers (e.g., `"X-API-Key"`) if your frontend needs to send them.
-   **`ExposeHeaders`** (`[]string`): A list of response headers that the browser's JavaScript code should be allowed to access. By default, only simple response headers are exposed.
-   **`AllowCredentials`** (`bool`): If `true`, the browser is allowed to send credentials (like cookies or `Authorization` headers) with the cross-origin request. **Important:** If set to `true`, the `Origins` field **cannot** contain the wildcard `"*"`; you must list specific origins. The `Access-Control-Allow-Credentials` header will be set to `true` in responses.
-   **`MaxAge`** (`time.Duration`): Specifies how long the results of a preflight (`OPTIONS`) request can be cached by the browser, reducing the number of preflight requests needed. If zero or negative, the `Access-Control-Max-Age` header is not sent.

## How it Works

When `CORSConfig` is provided:

1.  **Incoming Request Check**: For every incoming request, SRouter checks for an `Origin` header.
2.  **Origin Matching**: It compares the request's `Origin` against the configured `Origins` list.
3.  **Context Update**: The result of the CORS check (the allowed origin, if any, and whether credentials are allowed) is stored in the request context using `scontext.WithCORSInfo`. This uses the router's specific generic types (`T`, `U`) ensuring context consistency.
4.  **Response Header Setting**: Appropriate CORS headers (`Access-Control-Allow-Origin`, `Access-Control-Allow-Credentials`, `Vary: Origin`, `Access-Control-Expose-Headers`) are set on the response *before* the request handler or other middleware are called.
5.  **Preflight Handling**: If the request method is `OPTIONS` (a preflight request), SRouter checks the requested method (`Access-Control-Request-Method`) and headers (`Access-Control-Request-Headers`) against the configured `Methods` and `Headers`.
    -   If allowed, it sets the necessary preflight response headers (`Access-Control-Allow-Methods`, `Access-Control-Allow-Headers`, `Access-Control-Max-Age`) and responds with `204 No Content`, terminating the request chain.
    -   If not allowed, it still responds with `204 No Content` but *without* the `Allow-*` headers, signaling failure to the browser.
6.  **Error Handling**: The `writeJSONError` function also checks the context for CORS information and sets the appropriate `Access-Control-Allow-Origin` and `Access-Control-Allow-Credentials` headers on error responses, ensuring CORS compliance even when errors occur.

This integrated approach ensures that CORS is handled consistently and correctly across all requests and responses, including errors, without needing a separate middleware step.
