# Custom metrics middleware

This example supplies a custom implementation of
`metrics.MetricsMiddleware[string, struct{}]` through
`router.MetricsConfig.MiddlewareFactory`. It records request counts, status
codes, and cumulative duration in memory, then exposes a JSON snapshot at
`/metrics`.

Use this integration point when an application's metrics backend does not
implement SRouter's `metrics.MetricsRegistry` interface. A custom middleware
controls its own collection behavior; the built-in metric feature flags apply
only to the built-in middleware.

Run the example:

```bash
go run .
```

Generate a successful and an error response, then inspect the snapshot:

```bash
curl http://localhost:8080/hello
curl http://localhost:8080/unavailable
curl http://localhost:8080/metrics
```

The `/metrics` endpoint is mounted outside SRouter, so reading the snapshot
does not add another observation.
