# IP Configuration

SRouter provides a flexible way to extract the client's real IP address, which is crucial for logging, rate limiting, and security, especially when your application runs behind a reverse proxy or load balancer.

## Configuration

IP extraction is configured via the `IPConfig` field within the main `router.RouterConfig`. This field takes a pointer to a `middleware.IPConfig` struct, defined in the `pkg/middleware` package.

```go
import (
	"github.com/Suhaibinator/SRouter/pkg/middleware"
	"github.com/Suhaibinator/SRouter/pkg/router"
)

// Configure IP extraction to trust and use the X-Forwarded-For header
routerConfig := router.RouterConfig{
    // ... other config (logger, etc.)
    IPConfig: &middleware.IPConfig{
        Source:     middleware.IPSourceXForwardedFor, // Specify the source header
        TrustProxy: true,                           // Trust the header value
    },
    // ...
}

r := router.NewRouter[string, string](routerConfig, /* auth funcs */)
```

If `IPConfig` is `nil` or not provided, SRouter defaults to using the request's `RemoteAddr` field and does not trust any proxy headers.

## IP Source Types

The `Source` field in `middleware.IPConfig` determines where SRouter attempts to find the client IP. It uses constants defined in the `pkg/middleware` package:

1.  **`middleware.IPSourceRemoteAddr`**: (Default if `IPConfig` is nil or `Source` is not specified) Uses the `r.RemoteAddr` field directly. This is the IP address of the immediate peer connecting to your server (which might be the proxy itself).
2.  **`middleware.IPSourceXForwardedFor`**: Uses the `X-Forwarded-For` header. This header can contain a comma-separated list of IPs (`client, proxy1, proxy2`). SRouter typically extracts the *first* IP in the list as the original client IP when `TrustProxy` is true.
3.  **`middleware.IPSourceXRealIP`**: Uses the `X-Real-IP` header. This header is often set by proxies like Nginx to contain only the original client IP.
4.  **`middleware.IPSourceCustomHeader`**: Uses a custom header name specified in the `CustomHeader` field of `IPConfig`.

```go
import (
	"github.com/Suhaibinator/SRouter/pkg/middleware"
	"github.com/Suhaibinator/SRouter/pkg/router"
)

// Example: Using a custom header
routerConfig := router.RouterConfig{
    // ... other config
    IPConfig: &middleware.IPConfig{
        Source:       middleware.IPSourceCustomHeader,
        CustomHeader: "X-Client-IP",
        TrustProxy:   true,
    },
}
```

## Trust Proxy Setting (`TrustProxy`)

The `TrustProxy` boolean field is critical for security:

-   **`TrustProxy: true`**: SRouter will attempt to extract the IP from the header specified by `Source`. If the header is missing or invalid, it *may* fall back to `RemoteAddr` (check specific implementation details or source if needed, but generally `RemoteAddr` is the ultimate fallback). **Only enable this if you are certain your proxy correctly sets and sanitizes the relevant header.** Otherwise, a client could spoof their IP address by sending a fake header.
-   **`TrustProxy: false`**: SRouter will *ignore* the specified `Source` header and *always* use `r.RemoteAddr`. This is the safer default if you are unsure about your proxy setup.

## Accessing the Client IP

Once configured, the extracted client IP is stored in the request context. You can retrieve it using the `middleware.GetClientIPFromRequest` helper function, also from the `pkg/middleware` package:

```go
import (
	"fmt"
	"net/http"

	"github.com/Suhaibinator/SRouter/pkg/middleware"
)

func myHandler(w http.ResponseWriter, r *http.Request) {
    // Get the client IP extracted based on IPConfig settings
    // Replace string, string with your router's UserIDType, UserObjectType
    clientIP, ok := middleware.GetClientIPFromRequest[string, string](r)

    if ok {
        fmt.Fprintf(w, "Your IP address is: %s", clientIP)
        // Use clientIP for logging, rate limiting checks, etc.
    } else {
        // IP could not be determined (should generally not happen if RemoteAddr is available)
        http.Error(w, "Could not determine client IP", http.StatusInternalServerError)
    }
}
```

See the `examples/middleware` directory for runnable examples involving middleware and potentially IP configuration.
