# Performance Considerations

SRouter is designed with performance in mind, building upon the speed of `julienschmidt/httprouter`. Here are some factors and tips related to performance:

## Path Matching

SRouter inherits the high-performance path matching capabilities of `julienschmidt/httprouter`. This router uses a radix tree structure, allowing for path lookups that are generally O(k), where k is the length of the path, or even O(1) in many practical scenarios, significantly faster than routers relying solely on regular expressions for every route.

## Middleware Ordering and Overhead

While middleware is powerful, each layer adds some overhead to the request processing time. Be mindful of the number of middleware functions applied globally or to frequently accessed routes.

The order in which middleware is applied matters. SRouter applies middleware by wrapping the final handler. The effective order, from outermost (runs first) to innermost (runs last before handler), based on the internal `wrapHandler` and `registerSubRouter` logic, is:

1.  **Timeout Middleware** (Applied first if `timeout > 0`)
2.  **Global Middlewares** (`RouterConfig.Middlewares`, including Trace if enabled)
3.  **Sub-Router Middlewares** (`SubRouterConfig.Middlewares` for the relevant sub-router)
4.  **Route-Specific Middlewares** (`RouteConfig.Middlewares`)
5.  **Rate Limiting Middleware** (Applied internally if `rateLimit` config exists)
6.  **Authentication Middleware** (Applied internally if `AuthLevel` is `AuthRequired` or `AuthOptional`)
7.  **Recovery Middleware** (Applied internally)
8.  **Base Handler Logic** (Internal shutdown check, Max Body Size check)
9.  **Your Actual Handler** (`http.HandlerFunc` or `GenericHandler`)

Note: The `registerSubRouter` function combines sub-router and route-specific middleware before passing the combined list to `wrapHandler`. `wrapHandler` then applies global middleware before this combined list.

Middleware within the *same slice* (e.g., `RouterConfig.Middlewares`) are applied in the order they appear in the slice; the first one in the slice becomes the outermost wrapper. Placing frequently used but potentially short-circuiting middleware (like authentication or rate limiting, though these are often handled internally) earlier in the *effective* chain can improve performance by avoiding unnecessary work in later middleware or the handler itself.

## Memory Allocation

SRouter aims to minimize memory allocations in the hot path (request processing). However, certain operations can still lead to allocations:

-   **Context Management**: While `SRouterContext` avoids deep nesting, adding values to the context still involves allocations.
-   **Logging**: Structured logging (like with `zap`) involves allocations for creating log entries and fields. Ensure your logger is configured appropriately for production (e.g., sampling, appropriate level) to minimize performance impact.
-   **Data Encoding/Decoding**: Codecs (JSON, Proto, etc.) inherently involve allocations for marshaling and unmarshaling data.
-   **Custom Middleware**: Be mindful of allocations within your own middleware functions.

**Tips for Reducing Allocations:**

-   Use `sync.Pool` for frequently allocated temporary objects within handlers or middleware if profiling shows significant allocation pressure.
-   Avoid unnecessary string formatting or concatenation within the request path.
-   Reuse objects like HTTP clients or database connections instead of creating them per request.

## Timeouts

Setting appropriate timeouts via `GlobalTimeout`, `TimeoutOverride` (sub-router), and `Timeout` (route) is crucial for both performance and stability:

-   Prevents slow client connections or long-running handlers from consuming server resources indefinitely.
-   Helps protect against certain types of Denial-of-Service (DoS) attacks.
-   Ensures predictable response times for clients.

Set timeouts based on the expected latency of the underlying operations for each route or group of routes.

## Body Size Limits

Configuring maximum request body sizes using `GlobalMaxBodySize`, `MaxBodySizeOverride` (sub-router), and `MaxBodySize` (route) is important for:

-   **Security**: Prevents DoS attacks where clients send excessively large request bodies to exhaust server memory or bandwidth.
-   **Performance**: Avoids processing unnecessarily large amounts of data.

Set limits based on the expected maximum payload size for each endpoint.

## Benchmarks

Consider running Go's built-in benchmarking tools (`go test -bench=.`) on your handlers and middleware to identify performance bottlenecks. SRouter itself likely includes benchmarks for its core routing and middleware components.
