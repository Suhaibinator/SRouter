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
-   **`/examples/subrouters`**: Illustrates grouping routes under a common path prefix using `SubRouterConfig` and applying configuration overrides (like timeouts).
-   **`/examples/subrouter-generic-routes`**: Combines sub-routers with declarative registration of generic routes within the `SubRouterConfig.Routes` slice.
-   **`/examples/nested-subrouters`**: Demonstrates creating hierarchical routing structures by nesting `SubRouterConfig` instances.
-   **`/examples/middleware`**: Shows how to create and apply custom middleware at global, sub-router, and route-specific levels. May also demonstrate IP configuration.
-   **`/examples/auth`**: Basic example of implementing authentication, likely using middleware.
-   **`/examples/auth-levels`**: Demonstrates using the `AuthLevel` configuration (`NoAuth`, `AuthOptional`, `AuthRequired`) in conjunction with authentication middleware.
-   **`/examples/user-auth`**: Example focusing on authentication middleware that populates both User ID and a User object into the context.
-   **`/examples/rate-limiting`**: Shows how to configure IP-based, user-based, and potentially custom rate limiting strategies using `RateLimitConfig`.
-   **`/examples/graceful-shutdown`**: Provides a complete example of handling OS signals (SIGINT, SIGTERM) for graceful server shutdown using `http.Server.Shutdown` and `router.Shutdown`.
-   **`/examples/trace-logging`**: Demonstrates enabling and using trace IDs for correlating logs within a request lifecycle.
-   **`/examples/source-types`**: Shows how to use different `SourceType` options (Body, Base64QueryParameter, Base64PathParameter, etc.) for generic routes.
-   **`/examples/codec`**: Illustrates using different codecs, particularly `JSONCodec` and `ProtoCodec` (including the required factory function for proto).
-   **`/examples/prometheus`**: Example of integrating SRouter's metrics system with Prometheus by providing Prometheus-based implementations of the `metrics.Collector` and `metrics.Exporter` interfaces.
-   **`/examples/custom-metrics`**: Demonstrates implementing custom logic or integrating with a different backend using the `metrics` package interfaces.

Exploring these examples is highly recommended to understand how to effectively use SRouter's various features.
