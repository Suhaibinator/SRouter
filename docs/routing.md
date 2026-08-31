# Routing

SRouter registers standard and typed routes on one runtime router. Recursive
`RouteGroup` handles provide path scoping, middleware, and inherited policy
without creating additional HTTP handlers or dispatchers.

## Root routes

```go
r.Route(
	router.RouteConfigBase{
		Path:    "/health",
		Methods: []router.HttpMethod{router.MethodGet},
		Handler: healthHandler,
	},
	router.RouteConfig[CreateRequest, CreateResponse]{
		Path:    "/users",
		Methods: []router.HttpMethod{router.MethodPost},
		Codec:   codec.NewJSONCodec[CreateRequest, CreateResponse](),
		Handler: createUser,
	},
)
```

`Route` accepts any number of `RouteDefinition` values. `RouteConfigBase` and
every `RouteConfig[Req, Resp]` instantiation implement that sealed interface.

## Route groups

```go
api := r.Group("/api").Use(apiMiddleware)
v1 := api.Group("/v1").Timeout(3 * time.Second)
users := v1.Group("/users").Auth(router.AuthRequired)

users.Route(
	router.RouteConfigBase{
		Path:    "/:id",
		Methods: []router.HttpMethod{router.MethodGet},
		Handler: getUser,
	},
	router.RouteConfig[ListRequest, ListResponse]{
		Path:       "",
		Methods:    []router.HttpMethod{router.MethodGet},
		Codec:      codec.NewJSONCodec[ListRequest, ListResponse](),
		SourceType: router.Empty,
		Handler:    listUsers,
	},
)
```

This registers `/api/v1/users/:id` and the exact `/api/v1/users` path.
Prefixes must begin with `/`; non-root prefixes must not end with `/`.

Groups can be nested to any practical depth. Retain the handle rather than
looking it up by a path string:

```go
func registerUsers(group *router.RouteGroup[string, User]) {
	group.Route(/* user routes */)
}

api := r.Group("/api")
registerUsers(api.Group("/users"))
```

See [Route groups](route-groups.md) for policy inheritance, middleware order,
explicit disabling, and the build/freeze lifecycle.

## Path parameters

The underlying `httprouter` syntax is preserved:

- `:name` captures one path segment.
- `*name` captures the remaining path.

```go
r.Route(router.RouteConfigBase{
	Path:    "/users/:id/files/*path",
	Methods: []router.HttpMethod{router.MethodGet},
	Handler: func(w http.ResponseWriter, req *http.Request) {
		id := router.GetParam(req, "id")
		path := router.GetParam(req, "path")
		_, _ = fmt.Fprintf(w, "%s: %s", id, path)
	},
})
```

`router.GetParams(req)` returns all parameters. SRouter also stores the compiled
route template in its request context for built-in metrics and application
middleware or logging.

## Methods and conflicts

A route may register multiple methods:

```go
r.Route(router.RouteConfigBase{
	Path:    "/items/:id",
	Methods: []router.HttpMethod{router.MethodGet, router.MethodDelete},
	Handler: itemHandler,
})
```

`Build` rejects missing/empty methods, duplicate method/path pairs, and path
patterns that conflict under `httprouter` rules.

## Build before serving

```go
if err := r.Build(); err != nil {
	log.Fatal(err)
}

log.Fatal(http.ListenAndServe(":8080", r))
```

Explicit build is recommended so configuration failures stop startup. The first
request builds automatically when needed. Once built, the route tree is frozen
and steady-state dispatch performs no group traversal or policy resolution.
