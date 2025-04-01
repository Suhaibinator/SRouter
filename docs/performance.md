# Performance Considerations

SRouter is designed with performance in mind, building upon the speed of `julienschmidt/httprouter`. Here are some factors and tips related to performance:

## Path Matching

SRouter inherits the high-performance path matching capabilities of `julienschmidt/httprouter`. This router uses a radix tree structure, allowing for path lookups that are generally O(k), where k is the length of the path, or even O(1) in many practical scenarios, significantly faster than routers relying solely on regular expressions for every route.

## Middleware Ordering and Overhead

While middleware is powerful, each layer adds some overhead to the request processing time. Be mindful of the number of middleware functions applied globally or to frequently accessed routes.

The order in which middleware is applied matters. SRouter generally applies middleware in this sequence (wrapping the handler from outside-in):

`Internal Timeout/BodySize -> Global Middleware -> Sub-Router Middleware -> Route Middleware -> Handler`

Internal middleware (like recovery, authentication checks based on `AuthLevel`, rate limiting checks) are interleaved within this chain. Consult the `router.wrapHandler` (or similar) internal function for the precise execution order if necessary. Middleware defined earlier in a configuration slice (e.g., `RouterConfig.Middlewares`) typically executes *before* middleware defined later in the same slice.

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
