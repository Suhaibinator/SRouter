# Trace ID Logging

SRouter provides built-in support for trace ID logging, which is crucial for correlating log entries across different parts of your application (and potentially across microservices) for a single incoming request.

When enabled, SRouter automatically assigns a unique trace ID (a UUID) to each incoming request. This trace ID is then:

1.  Added to the request's context.
2.  Included in all log messages generated by SRouter's internal logging (e.g., request start/end logs from the `Logging` middleware).
3.  Made accessible to your handlers and middleware.

## Enabling Trace ID Logging

You can enable trace ID logging in two ways:

1.  **Via `RouterConfig`:** Set the `EnableTraceID` flag to `true`.

    ```go
    routerConfig := router.RouterConfig{
        Logger:        logger, // Assume logger exists
        EnableTraceID: true,   // Enable trace ID generation and context injection
        // Other configuration...
    }
    r := router.NewRouter[string, string](routerConfig, authFunction, userIdFromUserFunction) // Assume auth funcs exist
    ```

2.  **Via `TraceMiddleware`:** Add `middleware.TraceMiddleware()` to your middleware chain (usually as the first middleware). This approach offers slightly more control, such as configuring the buffer size for UUID generation if needed.

    ```go
    import "github.com/Suhaibinator/SRouter/pkg/middleware"
    import "github.com/Suhaibinator/SRouter/pkg/common"

    routerConfig := router.RouterConfig{
        Logger: logger, // Assume logger exists
        Middlewares: []common.Middleware{
            middleware.TraceMiddleware(), // Add trace middleware
            // Other middleware...
            middleware.Logging(logger), // Logging middleware will pick up the trace ID
        },
        // Other configuration...
    }
    r := router.NewRouter[string, string](routerConfig, authFunction, userIdFromUserFunction) // Assume auth funcs exist
    ```

    Using both `EnableTraceID: true` and `TraceMiddleware` is redundant; choose one method. Using the middleware explicitly is often clearer.

## Accessing the Trace ID

Once enabled, you can retrieve the trace ID within your handlers or other middleware using helper functions from the `pkg/middleware` package.

### `middleware.GetTraceID`

This function retrieves the trace ID directly from the `http.Request` object.

```go
import "github.com/Suhaibinator/SRouter/pkg/middleware"

func myHandler(w http.ResponseWriter, r *http.Request) {
    // Get the trace ID associated with this request
    traceID := middleware.GetTraceID(r)

    // Use the trace ID, e.g., in custom logging or downstream requests
    fmt.Printf("[trace_id=%s] Handling request for %s\n", traceID, r.URL.Path)

    // You can also add it to your own structured logs
    // logger.Info("Processing request", zap.String("trace_id", traceID), ...)

    // ... rest of handler logic ...
    w.Write([]byte("Handled with trace ID: " + traceID))
}
```

### `middleware.GetTraceIDFromContext`

If you only have access to the `context.Context` (e.g., in a function called by your handler), you can use this function.

```go
import (
    "context"
    "log"
    "github.com/Suhaibinator/SRouter/pkg/middleware"
)

func processData(ctx context.Context, data string) {
    // Get the trace ID from the context passed down from the handler
    traceID := middleware.GetTraceIDFromContext(ctx)

    // Use the trace ID in logs or further processing
    log.Printf("[trace_id=%s] Processing data: %s\n", traceID, data)

    // ... processing logic ...
}

// In your handler:
func myHandlerWithContext(w http.ResponseWriter, r *http.Request) {
    traceID := middleware.GetTraceID(r) // Get it from request first if needed
    fmt.Printf("[trace_id=%s] Handler started\n", traceID)

    // Pass the request's context (which contains the trace ID) to downstream functions
    processData(r.Context(), "some data")

    w.Write([]byte("Processed data with trace ID: " + traceID))
}

```

## Propagating the Trace ID

To maintain a consistent trace across multiple services, you should propagate the trace ID when making requests to downstream services. This is typically done by adding the trace ID as an HTTP header (e.g., `X-Trace-ID`).

```go
import (
    "net/http"
    "github.com/Suhaibinator/SRouter/pkg/middleware"
)

func callDownstreamService(r *http.Request) (*http.Response, error) {
    // Get the trace ID from the incoming request
    traceID := middleware.GetTraceID(r)

    // Create a new request to the downstream service
    downstreamReq, err := http.NewRequestWithContext(r.Context(), "GET", "http://downstream-service/api/data", nil)
    if err != nil {
        return nil, err
    }

    // Add the trace ID to the outgoing request's headers
    if traceID != "" {
        downstreamReq.Header.Set("X-Trace-ID", traceID) // Common header name
    }

    // Make the request
    client := &http.Client{} // Use a shared client in real applications
    resp, err := client.Do(downstreamReq)

    // Handle response and error
    // ...

    return resp, err
}
```

The downstream service should then be configured to look for this header, extract the trace ID, and use it for its own logging and further propagation.

See the `examples/trace-logging` directory for a runnable example.
