# SRouter

SRouter is an HTTP router for Go built on
[`julienschmidt/httprouter`](https://github.com/julienschmidt/httprouter). It adds
recursive route groups, inherited route policy, typed request/response handlers,
authentication, rate limiting, metrics, structured logging, and graceful
shutdown support.

[![Go Report Card](https://goreportcard.com/badge/github.com/Suhaibinator/SRouter)](https://goreportcard.com/report/github.com/Suhaibinator/SRouter)
[![Go Reference](https://pkg.go.dev/badge/github.com/Suhaibinator/SRouter.svg)](https://pkg.go.dev/github.com/Suhaibinator/SRouter)
[![Tests](https://github.com/Suhaibinator/SRouter/actions/workflows/tests.yml/badge.svg)](https://github.com/Suhaibinator/SRouter/actions/workflows/tests.yml)
[![codecov](https://codecov.io/gh/Suhaibinator/SRouter/graph/badge.svg?token=NNIYO5HKX7)](https://codecov.io/gh/Suhaibinator/SRouter)

## Requirements and installation

SRouter requires Go 1.27 or newer.

```bash
go get github.com/Suhaibinator/SRouter
```

Go modules install the router and its dependencies automatically.

## Quick start

```go
package main

import (
	"log"
	"net/http"

	"github.com/Suhaibinator/SRouter/pkg/router"
)

func main() {
	r := router.NewRouter[string, string](router.RouterConfig{
		ServiceName: "hello-service",
	}, nil, nil)

	r.Route(router.RouteConfigBase{
		Path:    "/hello",
		Methods: []router.HttpMethod{router.MethodGet},
		Handler: func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"message":"hello"}`))
		},
	})

	if err := r.Build(); err != nil {
		log.Fatal(err)
	}
	log.Fatal(http.ListenAndServe(":8080", r))
}
```

```bash
curl http://localhost:8080/hello
```

The authentication callbacks may be nil when every route uses `NoAuth`, as in
this example. A nil logger is replaced by a production logger, with a no-op
fallback if logger creation fails.

## Core model

Routes can be registered directly or beneath recursive groups. Each policy
setting inherits independently from the router through outer groups, inner
groups, and finally the route. With application-specific middleware and route
definitions omitted, a tree looks like this:

```go
api := r.Group("/api").
	Timeout(3 * time.Second).
	MaxBodySize(2 << 20).
	Use(apiMiddleware)

v1 := api.Group("/v1").Auth(router.AuthRequired)
v1.Route(getUserRoute, createUserRoute)
```

Root and group `Route` methods accept both standard `RouteConfigBase` values and
typed `RouteConfig[Request, Response]` values. Typed routes decode a configured
request source, optionally sanitize it, invoke a type-safe handler, and encode
the response.

Calling `Build` during startup is recommended. It validates the complete route
tree and freezes registration. The first request builds lazily if `Build` was
not called. A failed build is terminal for that router; later mutation panics.

## Features

- Radix-tree HTTP routing with path parameters
- Recursive groups with inherited authentication, timeout, body-size, and
  rate-limit policy
- Standard `http.HandlerFunc` routes and generic typed routes in the same tree
- Request decoding from bodies, query parameters, or path parameters
- Global, group, and route middleware
- Built-in optional or required authentication
- Nonblocking, in-memory sliding-window rate limiting with lazy stale-key eviction
- Configurable client-IP and trusted-proxy handling
- CORS and preflight handling
- Structured HTTP errors and configurable request-summary logging
- Pluggable metrics, including a Prometheus adapter
- Trace IDs, graceful shutdown, WebSocket support, and context helpers

## Documentation

| Topic | Guide |
| --- | --- |
| Installation and first server | [Getting started](docs/getting-started.md) |
| Paths, methods, and parameters | [Routing](docs/routing.md) |
| Recursive groups and policy inheritance | [Route groups](docs/route-groups.md) |
| Typed handlers and request sources | [Generic routes](docs/generic-routes.md) |
| Router and route configuration | [Configuration reference](docs/configuration.md) |
| Middleware order and built-ins | [Middleware](docs/middleware.md) |
| Authentication levels and providers | [Authentication](docs/authentication.md) |
| Rate-limit strategies and buckets | [Rate limiting](docs/rate-limiting.md) |
| Client IP and proxy trust | [IP configuration](docs/ip-configuration.md) |
| Cross-origin requests | [CORS](docs/cors-configuration.md) |
| Request-scoped state | [Context management](docs/context-management.md) |
| JSON, Protocol Buffers, and custom formats | [Codecs](docs/codecs.md) |
| Registries, middleware, and Prometheus | [Metrics](docs/metrics.md) |
| Trace IDs and request summaries | [Logging](docs/logging.md) |
| Structured handler errors | [Error handling](docs/error-handling.md) |
| Shutdown, deployment, and security | [Production](docs/production.md) |
| Runnable programs | [Examples](docs/examples.md) |

## Important behavior

- Route registration is frozen after `Build` or the first request.
- Middleware order is recovery, automatic trace-ID injection, built-in
  authentication, configured rate limiting, global middleware, outer-to-inner group
  middleware, route middleware, timeout, then handler.
- Custom authentication added as global or group middleware runs after the
  configured rate limiter. User-based configured limits therefore require the
  built-in authentication stage to populate identity first.
- `TraceIDBufferSize` controls trace-ID generation. `EnableTraceLogging`
  independently enables request-summary logs.
- Optional build and config identity providers are sampled once per request and
  stored in the shared SRouter context.
- Proxy headers are trusted only when explicitly configured. Review the IP
  guide before enabling them because client-IP choice affects security and
  rate-limit keys.
- Generic routes default to the request body. Use the appropriate path/query
  source or `Empty` for bodyless requests.

## Examples

All example programs are listed in [docs/examples.md](docs/examples.md). Run an
example as a package so that supporting files are included:

```bash
cd examples/simple
go run .
```

## Development

```bash
go test ./...
go vet ./...
go fmt ./...
```

CI enforces at least 80% aggregate coverage across the library packages.

## License

SRouter is available under the [MIT License](LICENSE).
