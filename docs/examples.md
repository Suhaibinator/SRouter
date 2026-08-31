# Examples

Example applications live in the repository's `examples` directory. Run an
example as a complete package so supporting files are included:

```bash
cd examples/simple
go run .
```

Several servers print ready-to-run curl commands at startup.

| Example | Focus |
| --- | --- |
| [simple](../examples/simple/) | Standard and typed routes under a shared group |
| [generic](../examples/generic/) | JSON typed handlers, sanitization, and structured errors |
| [subrouters](../examples/subrouters/) | Group prefixes, inherited policy, and group middleware |
| [subrouter-generic-routes](../examples/subrouter-generic-routes/) | Standard and typed routes in one group tree |
| [nested-subrouters](../examples/nested-subrouters/) | Recursive route groups and inherited configuration |
| [middleware](../examples/middleware/) | Global, group, route, recovery, and rate-limit middleware |
| [auth](../examples/auth/) | Bearer/API-key middleware and built-in required authentication |
| [auth-levels](../examples/auth-levels/) | Built-in `NoAuth`, `AuthOptional`, and `AuthRequired` behavior |
| [user-auth](../examples/user-auth/) | Boolean, custom-user, bearer-user, and basic-user middleware |
| [rate-limiting](../examples/rate-limiting/) | IP-based limits and authenticated user-based limits |
| [graceful-shutdown](../examples/graceful-shutdown/) | Signal handling with HTTP-server and router shutdown |
| [trace-logging](../examples/trace-logging/) | Trace-ID generation, propagation, and request correlation |
| [cors-error-test](../examples/cors-error-test/) | CORS preflight and CORS headers on error responses |
| [source-types](../examples/source-types/) | Body, Empty, base64, and base62 query/path request sources |
| [codec](../examples/codec/) | Protocol Buffer request and response handling |
| [custom-codec](../examples/custom-codec/) | A custom line-based codec for body and encoded-query sources |
| [prometheus](../examples/prometheus/) | SRouter metrics collection with a Prometheus registry |
| [custom-metrics](../examples/custom-metrics/) | Injecting a custom SRouter metrics middleware with JSON exposition |
| [handler-error-middleware](../examples/handler-error-middleware/) | Reading typed-handler errors while middleware unwinds |
| [websocket](../examples/websocket/) | `DisableTimeout` for long-lived WebSocket connections |
