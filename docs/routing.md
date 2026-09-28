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

Wildcards fill a whole path segment:

- `:name` captures one non-empty path segment.
- `*name` captures the remaining path, including its leading `/`. It must be
  the final segment.

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

`router.GetParams(req)` returns all parameters as `scontext.Params`, in pattern
order; `router.GetParam(req, name)` returns one value. Parameters are taken
from the decoded request path, so `%20` becomes a space. SRouter also stores
the compiled route template in its request context for built-in metrics and
application middleware or logging.

A catch-all value is not cleaned: `/files/*path` matches `/files/../x` with
`path=/../x`. Clean it before using it as a file path.

## Matching precedence

A static segment and a wildcard can share a position. At each segment the
router tries a static match first, then a `:name` parameter, then a `*name`
catch-all, and backtracks when a choice fails deeper in the path:

```go
get := []router.HttpMethod{router.MethodGet}
r.Route(
	router.RouteConfigBase{Path: "/users/new", Methods: get, Handler: newUserForm},
	router.RouteConfigBase{Path: "/users/:id", Methods: get, Handler: showUser},
)
```

`/users/new` reaches `newUserForm` and `/users/42` reaches `showUser`. The
result never depends on registration order.

## Unmatched requests

When no route matches the method and path, the router responds in this order:

1. `301` (for `GET`) or `307` (other methods) to the same path with the
   trailing slash added or removed, when that path matches a route for the
   request method.
2. The same redirect to the cleaned path, with repeated slashes, `.` and `..`
   resolved. Matching stays case-sensitive; there is no case-correcting
   redirect.
3. For `OPTIONS`, `200` with an `Allow` header when other methods match the
   path.
4. `405 Method Not Allowed` with an `Allow` header when other methods match.
5. `404 Not Found`.

Redirects keep the query string and only point at routes that exist on the
same host. `HEAD` is not routed to `GET` handlers. The full rules are in the
[route table specification](plans/route-table.md#unmatched-requests).

## Methods and conflicts

A route may register multiple methods:

```go
r.Route(router.RouteConfigBase{
	Path:    "/items/:id",
	Methods: []router.HttpMethod{router.MethodGet, router.MethodDelete},
	Handler: itemHandler,
})
```

`Build` rejects missing/empty methods, duplicate method/path pairs, invalid
wildcards (such as `/user_:name` or a repeated name), and two different
parameter or catch-all names at the same position for the same method.

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
