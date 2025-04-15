# Metrics

SRouter features a flexible, interface-based metrics system located in the `pkg/metrics` package. This design allows integration with various metrics backends (like Prometheus, OpenTelemetry, StatsD, etc.) by providing implementations for key interfaces, promoting loose coupling and testability.

## Enabling and Configuring Metrics

Metrics collection is enabled by setting `EnableMetrics: true` in your `RouterConfig`. Further customization is done via the `MetricsConfig` field within `RouterConfig`.

```go
import (
	"net/http"
	"github.com/Suhaibinator/SRouter/pkg/router"
	"github.com/Suhaibinator/SRouter/pkg/metrics" // Import metrics package
	// Assume myMetricsCollector, myMiddlewareFactory are your implementations
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

        // Provide your implementation of metrics.MetricsMiddleware. Optional.
        // If nil, SRouter uses metrics.NewMetricsMiddleware(Collector, config) internally.
        MiddlewareFactory: myMiddlewareFactory, // Must implement metrics.MetricsMiddleware

        // Configure which default metrics the built-in middleware should collect
        // if MiddlewareFactory is nil. These flags are used by metrics.NewMetricsMiddleware.
        // Note: Namespace and Subsystem are typically configured within your MetricsRegistry implementation,
        // not directly in this config struct.
        EnableLatency:    true,  // Collect request latency histogram/summary
        EnableThroughput: true,  // Collect request/response size histogram/summary
        EnableQPS:        true,  // Collect request counter (requests_total)
        EnableErrors:     true,  // Collect error counter by status code (http_errors_total)
    },
    // ... other config (SubRouters, Middlewares, etc.)
}

r := router.NewRouter[string, string](routerConfig, authFunction, userIdFromUserFunction)

// --- Application-Level Metrics Endpoint ---
// Serving a /metrics endpoint (e.g., for Prometheus scraping) is the responsibility
// of the application using SRouter, not SRouter itself.
//
// You would typically:
// 1. Instantiate your chosen MetricsExporter implementation.
// 2. If it provides an HTTP handler (via the Handler() method), retrieve it.
// 3. Create your main HTTP server (e.g., using http.ListenAndServe).
// 4. Register the metrics handler on a specific path (e.g., "/metrics")
//    using your server's mux (e.g., http.DefaultServeMux or a custom one).
// 5. Register the SRouter instance ('r' in this example) to handle other paths (e.g., "/").
//
// Example structure (conceptual):
//
// func main() {
//   // ... (Setup logger, config, myMetricsRegistry implementation)
//
//   routerConfig := router.RouterConfig{ /* ... SRouter config ... */ }
//   srouterInstance := router.NewRouter[string, string](routerConfig, ...)
//
//   // Get handler from the MetricsRegistry implementation (if it provides one)
//   var metricsHandler http.Handler
//   // Type assert myMetricsRegistry to see if it has a Handler() method
//   // (This depends on your specific registry implementation)
//   if registryWithHandler, ok := myMetricsRegistry.(interface{ Handler() http.Handler }); ok {
// 	  metricsHandler = registryWithHandler.Handler()
//   }
//   if metricsHandler == nil {
//       // Handle case where registry doesn't provide a handler
//       metricsHandler = http.NotFoundHandler() // Or log an error, etc.
//   }
//
//   // Setup application's main ServeMux
//   appMux := http.NewServeMux()
//   appMux.Handle("/metrics", metricsHandler) // Expose metrics
//   appMux.Handle("/", srouterInstance)      // Handle API requests via SRouter
//
//   // Start the application server
//   log.Println("Starting server...")
//   log.Fatal(http.ListenAndServe(":8080", appMux))
// }
//

```

## Core Metrics Interfaces (`pkg/metrics`)

The system is built around dependency injection using these key interfaces defined in `pkg/metrics/metrics.go`:

1.  **`MetricsRegistry`**: The core interface responsible for managing the lifecycle of metrics within the system. It provides a method to `Register` metrics. It also offers builder methods (`NewCounter`, `NewGauge`, `NewHistogram`, `NewSummary`) that return specific *builder* interfaces (e.g., `CounterBuilder`). These builders are used to configure and finally `.Build()` the individual metric instruments (Counters, Gauges, etc.). Your implementation will wrap your chosen metrics library (e.g., `prometheus.NewRegistry`). This is the interface expected by the `MetricsConfig.Collector` field.
2.  **`MetricsMiddleware`**: Defines the interface for the metrics middleware itself. Its primary method is `Handler(name string, handler http.Handler) http.Handler`, which wraps an existing HTTP handler to collect metrics. Additionally, the interface provides methods for post-creation configuration (`Configure(config MetricsMiddlewareConfig)`), adding request filtering logic (`WithFilter(filter MetricsFilter)`), and adding request sampling logic (`WithSampler(sampler MetricsSampler)`). The `MetricsConfig.MiddlewareFactory` field expects an object implementing this interface. If `MiddlewareFactory` is `nil` in the config, SRouter internally creates an instance of `metrics.MetricsMiddlewareImpl` using the provided `MetricsRegistry` (Collector) and the `Enable*` flags from `MetricsConfig`.
3.  **Metric Types** (`Counter`, `Gauge`, `Histogram`, `Summary`) and **Builders** (`CounterBuilder`, etc.): Interfaces defining the individual metric instruments and how they are constructed. Your `MetricsRegistry` implementation will return builders that create objects satisfying these metric interfaces. The base `Metric` interface provides common methods like `Name()`, `Description()`, `Type()`, and `Tags()`. Builders typically offer fluent methods like `.Name()`, `.Description()`, `.Tag()`, and specific configuration (e.g., `.Buckets()` for HistogramBuilder, `.Objectives()` for SummaryBuilder) before calling `.Build()`.

## Middleware Filtering and Sampling

The `MetricsMiddleware` interface supports filtering and sampling to control which requests generate metrics:

-   **`MetricsFilter`**: An interface with a `Filter(r *http.Request) bool` method. If implemented and added via `WithFilter`, the middleware will only collect metrics for requests where `Filter` returns `true`.
-   **`MetricsSampler`**: An interface with a `Sample() bool` method. If implemented and added via `WithSampler`, the middleware will only collect metrics for a fraction of requests based on the sampler's logic (e.g., random sampling). The code provides a basic `RandomSampler`.
-   These allow for fine-grained control over metric collection, potentially reducing overhead or focusing on specific request types.

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
3.  Optionally, create a struct that implements `metrics.MetricsMiddleware` if you need highly custom middleware logic beyond what the default `MetricsMiddlewareImpl` provides.
4.  Instantiate your custom implementations.
5.  Pass your `MetricsRegistry` instance to `MetricsConfig.Collector`. If you created a custom `MetricsMiddleware`, pass it to `MetricsConfig.MiddlewareFactory`.
6.  **Prometheus Adapter**: SRouter provides a ready-to-use Prometheus adapter located in `pkg/metrics/prometheus`. This adapter implements the `metrics.MetricsRegistry` interface using the `prometheus/client_golang` library. You can instantiate `prometheus.NewPrometheusRegistry` and pass it to `MetricsConfig.Collector`. You will still need to expose the Prometheus metrics endpoint (e.g., `/metrics`) in your application, typically using the handler provided by the Prometheus client library registry.

See the `examples/prometheus` and `examples/custom-metrics` directories for potentially more detailed examples. Ensure these examples align with the current interface-based approach described here.
