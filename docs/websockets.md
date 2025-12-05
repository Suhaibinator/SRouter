# WebSocket Support

SRouter can now register WebSocket endpoints alongside standard HTTP routes. WebSocket routes reuse the same middleware and authentication pipeline used by REST handlers, so cross-cutting behavior such as tracing, logging, rate limiting, and custom middleware still apply to the initial upgrade request.

## Declaring a WebSocket route

Use `WebSocketRouteConfig` inside a `SubRouterConfig` just like any other `RouteDefinition`:

```go
wsRoute := router.WebSocketRouteConfig{
    Path: "/echo",
    Middlewares: []common.Middleware{loggingMiddleware},
    Handler: func(ctx context.Context, conn *websocket.Conn) {
        for {
            msgType, payload, err := conn.ReadMessage()
            if err != nil {
                return
            }
            _ = conn.WriteMessage(msgType, payload)
        }
    },
}

r := router.NewRouter(router.RouterConfig{
    Logger: logger,
    SubRouters: []router.SubRouterConfig{{
        PathPrefix: "/ws",
        Routes:     []router.RouteDefinition{wsRoute},
    }},
}, authFn, userFromUserFn)
```

The route is registered under the sub-router path prefix (e.g., `/ws/echo`). Only `GET` is used for WebSocket registration because the handshake is defined on `GET`.

## Upgrader configuration

`WebSocketRouteConfig` accepts an optional `Upgrader`. When omitted, a permissive upgrader is used (it allows all origins). Provide a custom `websocket.Upgrader` when you need stricter origin checks or other advanced settings.

## Middleware, authentication, and limits

The router wraps WebSocket routes with the same middleware chain as HTTP routes:

- Global, sub-router, and route-level middleware are executed before the upgrade occurs.
- Authentication levels (`AuthRequired`, `AuthOptional`, `NoAuth`) are honored for the handshake request.
- Timeout, rate limit, and max body size overrides are applied to the handshake phase. For long-lived WebSocket sessions, consider leaving timeouts unset or explicitly set them to `0` for the route.

## Shutdown behavior

During graceful shutdown, SRouter waits for active WebSocket handlers to return before completing shutdown, just like regular HTTP handlers. Your handler should monitor `ctx.Done()` and exit when requested to allow a timely shutdown.
