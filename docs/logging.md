# Logging

SRouter emphasizes structured, intelligent logging, primarily using the [zap](https://github.com/uber-go/zap) library. A `*zap.Logger` instance is required in the `RouterConfig`.

## Structured Logging

Using a structured logger like zap allows for:

-   **Machine-Readable Logs**: Logs are typically output in formats like JSON, making them easy to parse, index, and query by log aggregation systems (e.g., ELK stack, Splunk, Datadog).
-   **Contextual Information**: Key-value pairs (fields) can be added to log entries, providing rich context (e.g., `trace_id`, `user_id`, `http_method`, `status_code`).
-   **Performance**: Zap is designed for high performance and low allocation overhead compared to standard library logging or reflection-based approaches.

## Log Levels

SRouter's internal components and built-in middleware (like `middleware.Logging`) aim to use appropriate log levels:

-   **`Error`**: Used for significant server-side problems, unrecoverable errors, panics caught by recovery middleware, failures during critical operations (like server shutdown). Status codes 500+ often trigger Error logs.
-   **`Warn`**: Used for client-side errors (status codes 400-499), potentially problematic situations (e.g., rate limit exceeded, configuration issues), or notable but non-critical events (e.g., slow request warnings if implemented).
-   **`Info`**: Used sparingly for high-level operational information (e.g., server starting, shutting down gracefully). Avoid spamming Info logs in the request path.
-   **`Debug`**: Used for detailed diagnostic information useful during development or troubleshooting. This includes request/response details (often logged by `middleware.Logging`), context values, trace information, and fine-grained steps within components.

This tiered approach helps manage log verbosity. In production, you typically configure your logger to output `Info` level and above, while `Debug` logs are suppressed unless needed for debugging.

## Configuring the Logger

You provide the `*zap.Logger` instance in `RouterConfig`. You can configure this logger according to your environment's needs.

```go
import "go.uber.org/zap"

// Production Logger Example: JSON format, Info level and above
logger, err := zap.NewProduction()
if err != nil {
    log.Fatalf("can't initialize zap logger: %v", err)
}
defer logger.Sync() // Flushes buffer, if any

// Development Logger Example: Human-readable console format, Debug level and above
devLogger, err := zap.NewDevelopment()
if err != nil {
    log.Fatalf("can't initialize zap logger: %v", err)
}
defer devLogger.Sync()

// Custom Logger Example: Info level, add caller info
config := zap.NewProductionConfig()
config.Level = zap.NewAtomicLevelAt(zap.InfoLevel)
config.EncoderConfig.EncodeTime = zapcore.ISO8601TimeEncoder // Example time format
customLogger, err := config.Build(zap.AddCaller()) // Add caller info (file:line)
if err != nil {
    log.Fatalf("can't initialize zap logger: %v", err)
}
defer customLogger.Sync()


// Pass the chosen logger to SRouter
routerConfig := router.RouterConfig{
    Logger: logger, // Use the production logger instance
    // ... other config
}
```

Refer to the [zap documentation](https://pkg.go.dev/go.uber.org/zap) for detailed configuration options.

## Trace ID Integration

When [Trace ID Logging](./trace-logging.md) is enabled (either via `EnableTraceID: true` or `middleware.TraceMiddleware`), SRouter's logging middleware automatically includes the `trace_id` field in its log entries. You should also include the `trace_id` in logs generated by your own handlers and middleware for consistent request tracing.

```go
// Within a handler or middleware where logger and traceID are available:
logger.Info("Processing user data",
    zap.String("trace_id", traceID),
    zap.String("user_id", userID),
    // ... other relevant fields
)
