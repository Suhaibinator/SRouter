# Prometheus metrics

This example connects SRouter's built-in request metrics middleware to a native
Prometheus registry through the shipped `pkg/metrics/prometheus` adapter. It
enables request counts, latency histograms, and error counters. The adapter's
namespace and subsystem produce metric names beginning with `example_api_`.

Run the example:

```bash
go run .
```

Generate traffic and inspect the Prometheus exposition:

```bash
curl http://localhost:8080/api/hello
curl http://localhost:8080/api/error
curl http://localhost:8080/metrics
```

The `/metrics` endpoint is mounted outside SRouter and is therefore not included
in the router's request metrics.
