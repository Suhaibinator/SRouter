# Examples

SRouter includes a comprehensive set of examples in the `/examples` directory of the repository. Each example is a self-contained, runnable Go application demonstrating a specific feature or combination of features.

To run an example:

1.  Navigate to the example's directory (e.g., `cd examples/simple`).
2.  Run the `main.go` file: `go run main.go`.
3.  Follow any instructions printed by the example application (e.g., curl commands to test endpoints).

## List of Examples

Here's a brief overview of the available examples (refer to the source code and READMEs within each example directory for full details):

-   **`/examples/simple`**: Demonstrates basic router setup and registration of simple `http.HandlerFunc` routes.
-   **`/examples/generic`**: Shows how to define and register generic routes (`RouteConfig[T, U]`) with type-safe request/response handling using a JSON codec.
-   **`/examples/subrouters`**: Illustrates route-group policy and middleware under shared prefixes.
-   **`/examples/subrouter-generic-routes`**: Combines typed generic routes with route groups.
-   **`/examples/nested-subrouters`**: Demonstrates recursive route groups.
-   **`/examples/middleware`**: Shows middleware at global, route-group, and route-specific levels.
-   **`/examples/auth`**: Basic example of implementing authentication, likely using middleware.
-   **`/examples/auth-levels`**: Demonstrates using the `AuthLevel` configuration (`NoAuth`, `AuthOptional`, `AuthRequired`) in conjunction with authentication middleware.
-   **`/examples/user-auth`**: Example focusing on authentication middleware that populates both User ID and a User object into the context.
-   **`/examples/rate-limiting`**: Shows how to configure IP-based, user-based, and potentially custom rate limiting strategies using `RateLimitConfig`.
-   **`/examples/graceful-shutdown`**: Provides a complete example of handling OS signals (SIGINT, SIGTERM) for graceful server shutdown using `http.Server.Shutdown` and `router.Shutdown`.
-   **`/examples/trace-logging`**: Demonstrates enabling and using trace IDs for correlating logs within a request lifecycle.
-   **`/examples/cors-error-test`**: Demonstrates handling CORS preflight and error scenarios, including how SRouter writes CORS headers on error responses.
-   **`/examples/source-types`**: Shows how to use different `SourceType` options (Body, Base64QueryParameter, Base64PathParameter, etc.) for generic routes.
-   **`/examples/codec`**: Illustrates using different codecs, particularly `JSONCodec` and `ProtoCodec`.
-   **`/examples/custom-codec`**: Implements a playful, line-based “Rune Scroll” codec from scratch and uses it with both request-body and base64 query-parameter sources.
-   **`/examples/prometheus`**: Example of integrating SRouter's metrics system with Prometheus by providing a Prometheus-based implementation of the `metrics.MetricsRegistry` interface and showing how the application can expose the metrics via an HTTP handler.
-   **`/examples/custom-metrics`**: Demonstrates implementing a custom `metrics.MetricsRegistry` or `metrics.MetricsMiddleware`.
-   **`/examples/handler-error-middleware`**: Shows how middleware can access errors returned by generic handlers to make decisions (e.g., transaction rollback, custom error logging) using `scontext.GetHandlerErrorFromRequest`.
-   **`/examples/websocket`**: Demonstrates `DisableTimeout: true` so long-lived connections are not terminated by a global or route-group timeout.

Exploring these examples is highly recommended to understand how to effectively use SRouter's various features.
