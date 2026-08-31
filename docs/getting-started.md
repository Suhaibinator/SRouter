# Getting started with SRouter

## Installation

```bash
go get github.com/Suhaibinator/SRouter
```

SRouter requires Go 1.27 or newer.

## Basic usage

```go
package main

import (
	"log"
	"net/http"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/router"
	"go.uber.org/zap"
)

func main() {
	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatal(err)
	}
	defer func() { _ = logger.Sync() }()

	config := router.RouterConfig{
		ServiceName:       "example-service",
		Logger:            logger,
		GlobalTimeout:     2 * time.Second,
		GlobalMaxBodySize: 1 << 20,
	}

	r := router.NewRouter[string, string](config, nil, nil)
	r.Route(router.RouteConfigBase{
		Path:    "/hello",
		Methods: []router.HttpMethod{router.MethodGet},
		Handler: func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"message":"Hello, World!"}`))
		},
	})

	api := r.Group("/api").Timeout(3 * time.Second)
	api.Group("/v1").Route(router.RouteConfigBase{
		Path:    "/health",
		Methods: []router.HttpMethod{router.MethodGet},
		Handler: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		},
	})

	if err := r.Build(); err != nil {
		log.Fatal(err)
	}
	log.Fatal(http.ListenAndServe(":8080", r))
}
```

`RouterConfig` contains global infrastructure and defaults. Routes are added
with `r.Route`; recursive path scopes are created with `r.Group`. Both root and
group `Route` methods accept standard `RouteConfigBase` values and typed
`RouteConfig[Req, Resp]` values.

The authentication callbacks may be nil while every effective route auth level
is `NoAuth`. Supply them before adding `AuthOptional` or `AuthRequired` routes.

Calling `Build` during startup is recommended. It validates the full route tree
and freezes it before serving; `ServeHTTP` builds automatically if necessary.

## Next steps

- [Route groups and inheritance](route-groups.md)
- [Typed generic routes](generic-routes.md)
- [Authentication](authentication.md)
- [Configuration reference](configuration.md)
- [Runnable examples](examples.md)
