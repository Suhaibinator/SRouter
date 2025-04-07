# Metrics

SRouter features a flexible, interface-based metrics system located in the `pkg/metrics` package. This design allows integration with various metrics backends (like Prometheus, OpenTelemetry, StatsD, etc.) by providing implementations for key interfaces, promoting loose coupling and testability.

## Enabling and Configuring Metrics

Metrics collection is enabled by setting `EnableMetrics: true` in your `RouterConfig`. Further customization is done via the `MetricsConfig` field within `RouterConfig`.

```go
import (
	"net/http"
	"github.com/Suhaibinator/SRouter/pkg/router"
	"github.com/Suhaibinator/SRouter/pkg/metrics" // Import metrics package
	// Assume myMetricsCollector, myMetricsExporter, myMiddlewareFactory are your implementations
	// Assume logger, authFunction, userIdFromUserFunction exist
)


// Example: Configure metrics using MetricsConfig
routerConfig := router.RouterConfig{
    Logger:            logger,
    // ... other global config ...
    EnableMetrics:     true,      // Enable metrics collection
    MetricsConfig: &router.MetricsConfig{
        // Provide your implementations of metrics interfaces.
        // Provide your implementation of the metrics.MetricsRegistry interface. Required if EnableMetrics is true.
        Collector:        myMetricsRegistry, // Must implement metrics.MetricsRegistry

        // Provide your implementation of metrics.MetricsExporter. Optional.
        // SRouter itself doesn't use this directly, but you might use it to expose a /metrics endpoint.
        Exporter:         myMetricsExporter,

        // Provide your implementation of metrics.MetricsMiddleware. Optional.
        // If nil, SRouter uses metrics.NewMetricsMiddleware(Collector, config) internally.
        MiddlewareFactory: myMiddlewareFactory, // Must implement metrics.MetricsMiddleware if provided

        // Configure metric details (namespace, subsystem, which metrics to enable)
        Namespace:        "myapp",
        Subsystem:        "api",
        EnableLatency:    true,  // Collect request latency histogram/summary
        EnableThroughput: true,  // Collect request/response size histogram/summary
        EnableQPS:        true,  // Collect request counter (requests_total)
        EnableErrors:     true,  // Collect error counter by status code (http_errors_total)
    },
    // ... other config (SubRouters, Middlewares, etc.)
}

r := router.NewRouter[string, string](routerConfig, authFunction, userIdFromUserFunction)

// --- Serving the /metrics endpoint (Example for Prometheus) ---

// --- Serving the /metrics endpoint (Example using the Exporter) ---

// Check if the provided Exporter implementation has an HTTP handler method
var metricsHandler http.Handler = http.NotFoundHandler() // Default to Not Found
if myMetricsExporter != nil {
	// Assuming MetricsExporter interface has a Handler() method
    if handler := myMetricsExporter.Handler(); handler != nil {
		metricsHandler = handler // Get the handler (e.g., promhttp.Handler())
	} else {
		// Handle case where exporter exists but doesn't provide an HTTP handler
		// logger.Warn("Metrics exporter does not provide an HTTP handler")
	}
} else {
    metricsHandler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        http.Error(w, "Metrics endpoint not available", http.StatusNotFound)
    })
    // logger.Warn("Metrics exporter does not provide an HTTP handler")
}

// Serve metrics endpoint separately from your main API router
// (Common practice to avoid applying API middleware to the /metrics endpoint)
serveMux := http.NewServeMux()
serveMux.Handle("/metrics", metricsHandler) // Expose metrics at /metrics
serveMux.Handle("/", r)                     // Handle all other requests with SRouter

// Start the server with the ServeMux
// log.Println("Starting server with API on / and metrics on /metrics")
// log.Fatal(http.ListenAndServe(":8080", serveMux))

```

## Core Metrics Interfaces (`pkg/metrics`)

The system is built around dependency injection using these key interfaces defined in `pkg/metrics/metrics.go`:

1.  **`MetricsRegistry`**: The core interface responsible for creating and managing individual metric instruments (Counters, Gauges, Histograms, Summaries) via builder methods (`NewCounter`, `NewGauge`, etc.). Your implementation will wrap your chosen metrics library (e.g., `prometheus.NewRegistry`, `prometheus.NewCounterVec`). This is the interface expected by the `MetricsConfig.Collector` field.
2.  **`MetricsExporter`** (Optional): An interface for components that expose collected metrics. It includes methods like `Export(MetricsSnapshot)`, `Start()`, `Stop()`, and crucially `Handler() http.Handler` for providing an HTTP endpoint (like `/metrics`). SRouter does not use this internally during request handling, but you use it to set up the scraping endpoint.
3.  **`MetricsMiddleware`** (Optional Interface, used by `MiddlewareFactory`): Defines the interface for the metrics middleware itself, primarily the `Handler(name string, handler http.Handler) http.Handler` method. The `MetricsConfig.MiddlewareFactory` field expects an object implementing this interface. If `MiddlewareFactory` is `nil` in the config, SRouter internally creates an instance of `metrics.MetricsMiddlewareImpl` using the provided `MetricsRegistry` (Collector) and the `Enable*` flags from `MetricsConfig`.
4.  **Metric Types** (`Counter`, `Gauge`, `Histogram`, `Summary`) and **Builders** (`CounterBuilder`, etc.): Interfaces defining the individual metric instruments and how they are constructed. Your `MetricsRegistry` implementation will return builders that create objects satisfying these metric interfaces.

## Default Collected Metrics

When enabled via `MetricsConfig` fields (`EnableLatency`, `EnableQPS`, etc.), the default SRouter metrics middleware (`metrics.MetricsMiddlewareImpl`) collects:

-   **Latency**: Request duration (often as a Histogram or Summary). Labels might include method, path, status code.
-   **Throughput**: Request and response sizes (often as a Histogram or Summary). Labels might include method, path.
-   **QPS/Request Count**: Total number of requests (often as a Counter). Labels might include method, path, status code.
-   **Errors**: Count of requests resulting in different HTTP error status codes (often as a Counter). Labels usually include method, path, and status code.

The exact metric names and labels depend on the specific implementation within `metrics.MetricsMiddlewareImpl` and your provided `MetricsRegistry`.

## Implementing Your Own Metrics Backend

To integrate a different metrics system (e.g., OpenTelemetry, StatsD):

1.  Choose your Go metrics library (e.g., `go.opentelemetry.io/otel/metric`, `prometheus/client_golang`, a StatsD client).
2.  Create a struct that implements the `metrics.MetricsRegistry` interface. Its methods (`NewCounter`, `NewGauge`, etc.) will typically wrap the functions from your chosen library to create and register metrics.
3.  Optionally, create a struct that implements `metrics.MetricsExporter` if you need to expose metrics (e.g., via HTTP).
4.  Optionally, create a struct that implements `metrics.MetricsMiddleware` if you need highly custom middleware logic beyond what the default `MetricsMiddlewareImpl` provides.
5.  Instantiate your custom implementations.
6.  Pass your `MetricsRegistry` instance to `MetricsConfig.Collector`. Pass your optional `MetricsExporter` to `MetricsConfig.Exporter` (for your own use) and your optional custom `MetricsMiddleware` to `MetricsConfig.MiddlewareFactory`.

See the `examples/prometheus` and `examples/custom-metrics` directories for potentially more detailed examples. Ensure these examples align with the current interface-based approach described here.
