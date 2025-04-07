# IP Configuration

SRouter provides a flexible way to extract the client's real IP address, which is crucial for logging, rate limiting, and security, especially when your application runs behind a reverse proxy or load balancer.

## Configuration

IP extraction is configured via the `IPConfig` field within the main `RouterConfig`. This field takes a pointer to a `router.IPConfig` struct (defined in `pkg/router/ip.go`).

```go
import "github.com/Suhaibinator/SRouter/pkg/router" // IPConfig is in the router package

// Configure IP extraction to trust and use the X-Forwarded-For header
routerConfig := router.RouterConfig{
    // ... other config (logger, etc.)
    IPConfig: &router.IPConfig{
        Source:     router.IPSourceXForwardedFor, // Specify the source header
        TrustProxy: true,                         // Trust the header value
    },
    // ...
}

r := router.NewRouter[string, string](routerConfig, /* auth funcs */)
```

If `IPConfig` is `nil` or not provided, SRouter defaults to using `Source: router.IPSourceXForwardedFor` and `TrustProxy: true`. If you want to explicitly use only `RemoteAddr`, you must provide an `IPConfig` like:

```go
routerConfig := router.RouterConfig{
    // ...
    IPConfig: &router.IPConfig{
        Source:     router.IPSourceRemoteAddr,
        TrustProxy: false, // Explicitly disable trusting headers
    },
    // ...
}
```

## IP Source Types

The `Source` field in `router.IPConfig` determines where SRouter attempts to find the client IP. It uses constants of type `router.IPSourceType` defined in `pkg/router/ip.go`:

1.  **`router.IPSourceRemoteAddr`**: Uses the `r.RemoteAddr` field directly. This is the IP address of the immediate peer connecting to your server (which might be the proxy itself).
2.  **`router.IPSourceXForwardedFor`**: (Default `Source` if `IPConfig` is nil) Uses the `X-Forwarded-For` header. This header can contain a comma-separated list of IPs (`client, proxy1, proxy2`). SRouter extracts the *first* IP in the list as the original client IP when `TrustProxy` is true.
3.  **`router.IPSourceXRealIP`**: Uses the `X-Real-IP` header. This header is often set by proxies like Nginx to contain only the original client IP.
4.  **`router.IPSourceCustomHeader`**: Uses a custom header name specified in the `CustomHeader` field of `IPConfig`.

```go
// Example: Using a custom header
routerConfig := router.RouterConfig{
    // ...
    IPConfig: &router.IPConfig{
        Source:       router.IPSourceCustomHeader,
        CustomHeader: "CF-Connecting-IP", // Example: Cloudflare header
        TrustProxy:   true,
    },
    // ...
}
```

## Trust Proxy Setting (`TrustProxy`)

The `TrustProxy` boolean field is critical for security:

-   **`TrustProxy: true`**: (Default if `IPConfig` is nil) SRouter will attempt to extract the IP from the header specified by `Source`. If the header is missing or invalid, it falls back to `r.RemoteAddr`. **Only use this if you are certain your proxy correctly sets and sanitizes the relevant header.** Otherwise, a client could spoof their IP address by sending a fake header.
-   **`TrustProxy: false`**: SRouter will *ignore* the specified `Source` header (even if set to `IPSourceXForwardedFor`, etc.) and *always* use `r.RemoteAddr`. This is the safer option if you are unsure about your proxy setup or don't run behind a trusted proxy.

## Accessing the Client IP

Once configured, the extracted client IP is stored in the request context. You can retrieve it using the `scontext.GetClientIPFromRequest` helper function:

```go
import (
	"fmt"
	"net/http"
	"github.com/Suhaibinator/SRouter/pkg/scontext" // Use scontext package
)

func myHandler(w http.ResponseWriter, r *http.Request) {
    // Get the client IP extracted based on IPConfig settings
    // Replace string, string with your router's actual UserIDType, UserObjectType
    clientIP, ok := scontext.GetClientIPFromRequest[string, string](r)

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
