# Metrics

SRouter provides backend-neutral metric interfaces in `pkg/metrics` and a Prometheus adapter in `pkg/metrics/prometheus`. The router can either build its default request-metrics middleware from a `metrics.MetricsRegistry` or use a complete custom `metrics.MetricsMiddleware` implementation.

## Enabling metrics

A non-nil `MetricsConfig` does not enable metrics by itself. During `router.NewRouter[T, U]`, SRouter selects the first usable option in this order:

1. If `MiddlewareFactory` implements `metrics.MetricsMiddleware[T, U]` for the router's exact user ID and user types, SRouter uses it.
2. Otherwise, if `Collector` implements `metrics.MetricsRegistry`, SRouter creates the built-in middleware from that registry and the `Enable*` flags.
3. Otherwise, no metrics middleware is installed.

When both fields are valid, `MiddlewareFactory` takes precedence. SRouter passes `RouterConfig.ServiceName` as the fallback handler name, but it does not call `Configure` on a supplied factory or apply `Collector` and the router-level `Enable*` fields to it. Configure a custom middleware before passing it to the router.

```go
config := router.RouterConfig{
	ServiceName: "checkout-api",
	MetricsConfig: &router.MetricsConfig{
		Collector:        registry, // implements metrics.MetricsRegistry
		Namespace:        "checkout",
		Subsystem:        "http",
		EnableLatency:    true,
		EnableThroughput: true,
		EnableQPS:        true,
		EnableErrors:     true,
	},
}

r := router.NewRouter(config, router.RouterDependencies[string, User]{
	Authenticate: authenticate,
	UserID:       userIDFromUser,
})
```

With the built-in middleware, non-empty `Namespace` and `Subsystem` values become the default tags `service` and `subsystem`, respectively. They do not configure a backend's native namespace or subsystem. For example, the Prometheus adapter's namespace and subsystem are separately supplied to `prometheus.NewPrometheusRegistry` and become metric-name prefixes.

## Default request metrics

When installed through `RouterConfig`, metrics middleware is part of each matched route's global-middleware chain. It runs after built-in authentication and rate limiting and after middleware already present in `RouterConfig.Middlewares`. It therefore collects only requests that reach it and whose inner handler returns normally. It does not observe build or shutdown rejection, CORS responses, unmatched 404/405 responses, earlier authentication/rate-limit/global-middleware short-circuits, or an inner group/route/handler panic that unwinds through it to the router's outer recovery middleware. A filter or sampler can narrow the eligible requests further.

The built-in middleware requests the following instruments from the registry for those eligible requests. Every instrument also receives the non-empty default tags described above. Here, “global” means aggregated across eligible matched routes, not every request received by `Router.ServeHTTP`.

| Option | Route-specific instrument | Global instrument | Recorded value |
| --- | --- | --- | --- |
| `EnableLatency` | `request_latency_seconds`, tag `route` | `all_request_latency_seconds` | Request duration in seconds |
| `EnableThroughput` | `request_throughput_bytes`, tag `route` | `request_throughput_bytes_total` | Positive request `Content-Length` |
| `EnableQPS` | `requests_total`, tag `route` | `all_requests_total` | One increment per eligible request |
| `EnableErrors` | `request_errors_total`, tags `route` and `status_code` | `all_request_errors_total`, tag `status_code` | One increment for an eligible response with status `>= 400` |

`status_code` is the numeric decimal string, such as `"404"`, rather than HTTP status text. A route template such as `/users/:id` is used when available; otherwise the name passed to `MetricsMiddleware.Handler` is used.

The option named `EnableQPS` records cumulative request counters. Calculate a per-second rate in the metrics backend—for example, with a PromQL `rate` expression. It does not emit an instantaneous QPS gauge.

Likewise, throughput records declared request-body bytes, not bytes per second. It does not count response bytes, chunked request bodies with an unknown `Content-Length`, or requests whose content length is zero or negative.

Metric backends may prefix or expand these names. The Prometheus adapter prepends the native namespace and subsystem supplied to `NewPrometheusRegistry`; histogram and summary exposition also produces their conventional derived series. It does not rename arbitrary counter base names or add a missing `_total` suffix.

Example series before backend-specific name transformation:

```text
request_latency_seconds{route="/users/:id",service="accounts",subsystem="http"}
all_request_latency_seconds{service="accounts",subsystem="http"}
requests_total{route="/users/:id",service="accounts",subsystem="http"}
all_requests_total{service="accounts",subsystem="http"}
request_errors_total{route="/users/:id",status_code="404",service="accounts",subsystem="http"}
all_request_errors_total{status_code="404",service="accounts",subsystem="http"}
```

## Filtering and sampling

`metrics.MetricsMiddlewareImpl` supports a request filter and sampler:

- `WithFilter` collects metrics only when `Filter(*http.Request)` returns `true`.
- `WithSampler` collects metrics only when `Sample()` returns `true`.
- `Configure` replaces the configuration and resets the sampler from the new `SamplingRate`. Call `WithSampler` after `Configure` when using a custom sampler.

`MetricsMiddlewareConfig.SamplingRate` has these semantics in the built-in middleware:

- A value strictly between `0` and `1` installs a `RandomSampler` automatically.
- A value less than or equal to `0`, or greater than or equal to `1`, installs no sampler, so all otherwise-eligible requests are collected.

That configuration behavior is intentionally different from constructing `NewRandomSampler` directly: `NewRandomSampler(0)` rejects every request, while `NewRandomSampler(1)` accepts every request.

To use filtering, sampling, or custom default tags with the router, construct the middleware yourself and pass it through `MiddlewareFactory`:

```go
requestMetrics := metrics.NewMetricsMiddleware[string, User](registry, metrics.MetricsMiddlewareConfig{
	EnableLatency: true,
	EnableQPS:     true,
	SamplingRate:  0.10,
	DefaultTags: metrics.Tags{
		"service": "checkout",
	},
})
requestMetrics.WithFilter(healthCheckFilter{})

config := router.RouterConfig{
	ServiceName: "checkout-api",
	MetricsConfig: &router.MetricsConfig{
		MiddlewareFactory: requestMetrics,
	},
}
```

The factory's type parameters must match the router. For example, a `MetricsMiddleware[string, User]` is not usable by a `Router[int64, User]`; if no compatible `Collector` is present, metrics remain disabled.

## Core interfaces

`pkg/metrics` defines:

- `MetricsRegistry`, which returns builders for counters, gauges, histograms, and summaries. `Build` returns the configured instrument.
- `MetricsMiddleware[T, U]`, which wraps an `http.Handler` and supports configuration, filtering, and sampling.
- `Metric`, `Counter`, `Gauge`, `Histogram`, and `Summary`, which form the backend-neutral instrument API.
- The corresponding builder interfaces, which set names, descriptions, tags, buckets, and summary options.

The built-in middleware caches the instruments it uses by route, status, and metric kind. A cache key's builder executes at most once, including during concurrent first requests. Builders still run from request handling, so registry implementations should keep registration bounded and concurrency-safe.

## Prometheus adapter

The adapter registers instruments when their builder's `Build` method runs. Its `MetricsRegistry.Register` method is therefore a no-op. The extra `Get`, `Unregister`, and `Clear` methods on `PrometheusRegistry` are unsupported: `Get` reports not found, `Unregister` returns `false`, and `Clear` does nothing. Retain instrument or underlying registry references when lifecycle control is needed.

The concrete Prometheus builders expose deprecated `LabelNames` methods, but the backend-neutral metric interfaces have no operation for selecting label values. Consequently, mutation through a vector-backed `PrometheusCounter`, `PrometheusGauge`, `PrometheusHistogram`, or `PrometheusSummary` is unsupported and does nothing. SRouter's default middleware does not use `LabelNames`; it creates ordinary instruments with `Tag` values, which the adapter maps to Prometheus const labels. Use `Tag` for constant dimensions. When values must vary per observation, use the native Prometheus vector types and bind values with `WithLabelValues` instead.

Expose the scrape endpoint in the application; SRouter does not add one automatically:

```go
promRegistry := prometheus.NewRegistry()
collector := srouterprom.NewPrometheusRegistry(
	promRegistry,
	"myapp",
	"router",
	logger,
)

mux := http.NewServeMux()
mux.Handle("/metrics", promhttp.HandlerFor(promRegistry, promhttp.HandlerOpts{}))
mux.Handle("/", r)
```

See `examples/custom-metrics` and `examples/prometheus` for complete application structure.
