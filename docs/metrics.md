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
        // If nil, SRouter might use default no-op implementations or panic
        // depending on its design. It's best to provide implementations if enabled.
        Collector:        myMetricsCollector, // Must implement metrics.Collector
        Exporter:         myMetricsExporter,  // Optional: Must implement metrics.Exporter if needed (e.g., for /metrics endpoint)
        MiddlewareFactory: myMiddlewareFactory, // Optional: Must implement metrics.MiddlewareFactory (SRouter likely has a default)

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

// Check if the provided Exporter implementation has an HTTP handler
var metricsHandler http.Handler
if exporter, ok := myMetricsExporter.(metrics.HTTPExporter); ok { // Check for specific HTTPExporter interface if defined
    metricsHandler = exporter.Handler() // Get the handler (e.g., promhttp.Handler())
} else {
    // Handle the case where the exporter doesn't provide an HTTP handler
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

The system is built around dependency injection using these key interfaces:

1.  **`Collector`**: The core interface responsible for creating and managing individual metric instruments (Counters, Gauges, Histograms, Summaries). Your implementation will wrap your chosen metrics library (e.g., `prometheus.NewCounterVec`, `prometheus.NewHistogramVec`). SRouter's internal metrics middleware will call methods on this interface to record observations.
2.  **`Exporter`** (Optional): An interface for components that expose collected metrics. A common implementation provides an `http.Handler` for scraping endpoints like Prometheus's `/metrics`. If your backend pushes metrics (e.g., StatsD), you might not need an Exporter in this sense. SRouter might define sub-interfaces like `HTTPExporter` if applicable.
3.  **`MiddlewareFactory`** (Optional): Creates the actual `http.Handler` middleware (`common.Middleware`) that intercepts requests, records metrics using the `Collector`, and passes the request down the chain. SRouter likely provides a default factory that uses the configured `Collector` and `MetricsConfig` settings. You might provide your own factory for advanced customization of metric labels or recording logic.

## Default Collected Metrics

When enabled via `MetricsConfig` fields (`EnableLatency`, `EnableQPS`, etc.), the default SRouter metrics middleware typically collects:

-   **Latency**: Request duration (often as a Histogram or Summary). Labels might include method, path, status code.
-   **Throughput**: Request and response sizes (often as a Histogram or Summary). Labels might include method, path.
-   **QPS/Request Count**: Total number of requests (often as a Counter). Labels might include method, path, status code.
-   **Errors**: Count of requests resulting in different HTTP error status codes (often as a Counter). Labels usually include method, path, and status code.

The exact metric names and labels depend on the specific implementation within SRouter and the provided `Collector`.

## Implementing Your Own Metrics Backend

To integrate a different metrics system (e.g., OpenTelemetry, StatsD):

1.  Choose your Go metrics library (e.g., `go.opentelemetry.io/otel/metric`, a StatsD client).
2.  Create structs that implement the `metrics.Collector` interface (and `Exporter`, `MiddlewareFactory` if needed/customized) from `pkg/metrics`. Your implementation will bridge SRouter's interface calls to your chosen library's functions.
3.  Instantiate your custom implementations.
4.  Pass your instances into the `MetricsConfig` when creating the `router.NewRouter`.

See the `examples/prometheus` and `examples/custom-metrics` directories for potentially more detailed examples. Ensure these examples align with the current interface-based approach described here.
