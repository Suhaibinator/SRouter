# Typed routes

`router.RouteConfig[Req, Resp]` adds decoding, optional validation, a typed
handler, and response encoding around an HTTP route. Standard and typed routes
can be registered together on a `Router` or `RouteGroup`.

## Basic example

```go
type CreateUserRequest struct {
	Name string `json:"name"`
}

type UserResponse struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

createUser := router.RouteConfig[CreateUserRequest, UserResponse]{
	Path:    "/users",
	Methods: []router.HttpMethod{router.MethodPost},
	Codec:   codec.NewJSONCodec[CreateUserRequest, UserResponse](),
	Sanitizer: func(_ context.Context, req CreateUserRequest) (CreateUserRequest, error) {
		req.Name = strings.TrimSpace(req.Name)
		if req.Name == "" {
			return CreateUserRequest{}, router.NewHTTPError(
				http.StatusBadRequest,
				"name is required",
			)
		}
		return req, nil
	},
	Handler: func(_ *http.Request, req CreateUserRequest) (UserResponse, error) {
		return UserResponse{ID: "user-123", Name: req.Name}, nil
	},
}

r.Route(createUser)
```

`Codec`, `Handler`, and at least one method are required. `Build` reports a
missing codec or handler before serving. A missing `Sanitizer` is allowed but
produces a warning.

The handler signature is:

```go
func(*http.Request, Req) (Resp, error)
```

Use the request to read authentication, trace, transaction, path-parameter, or
other context values. SRouter encodes a successful response with the configured
codec.

## Request sources

`SourceType` selects how `Req` is populated:

| Source | Input | `SourceKey` |
| --- | --- | --- |
| `Body` | `Codec.Decode(req)` | Ignored |
| `Empty` | No decoding; the zero value of `Req` | Ignored |
| `Base64QueryParameter` | Base64 decode, then `Codec.DecodeBytes` | Required query key |
| `Base62QueryParameter` | Base62 decode, then `Codec.DecodeBytes` | Required query key |
| `Base64PathParameter` | Base64 decode, then `Codec.DecodeBytes` | Optional path key |
| `Base62PathParameter` | Base62 decode, then `Codec.DecodeBytes` | Optional path key |

`Body` is the zero value. Omitting `SourceType` on a bodyless GET or DELETE
therefore still asks the codec to decode the body; an empty JSON body returns a
400 before the handler runs. Use `Empty` when there is no typed input:

```go
r.Route(router.RouteConfig[struct{}, HealthResponse]{
	Path:       "/health",
	Methods:    []router.HttpMethod{router.MethodGet},
	Codec:      codec.NewJSONCodec[struct{}, HealthResponse](),
	SourceType: router.Empty,
	Handler: func(_ *http.Request, _ struct{}) (HealthResponse, error) {
		return HealthResponse{Status: "ok"}, nil
	},
})
```

Query sources require a nonempty `SourceKey`; `Build` rejects the route without
one. For a path source, an empty `SourceKey` selects the first path parameter.
Using an explicit name is clearer:

```go
r.Route(router.RouteConfig[LookupRequest, LookupResponse]{
	Path:       "/lookup/:payload",
	Methods:    []router.HttpMethod{router.MethodGet},
	Codec:      codec.NewJSONCodec[LookupRequest, LookupResponse](),
	SourceType: router.Base64PathParameter,
	SourceKey:  "payload",
	Handler:    lookup,
})
```

Base64 uses standard padded RFC 4648 encoding. SRouter's Base62 alphabet is
`0-9A-Za-z`; use `codec.EncodeBase62` to create compatible values. See
[`examples/source-types`](../examples/source-types) for all source variants.

## Sanitization and errors

When configured, `Sanitizer` runs after decoding and before the handler. It
receives the active request context and may validate or transform `Req`.

Error behavior is:

- decode failures return 400, except an exceeded body limit returns 413;
- sanitizer failures return 400 by default;
- handler failures return 500 by default;
- `router.HTTPError` overrides the default status and public message; and
- response-encoding failures are handled as server errors.

When the handler completes through the normal chain, its error is stored in the
SRouter context before outer middleware resumes. This lets transaction or
observability middleware inspect the result with
`scontext.GetHandlerErrorFromRequest`. A timed-out handler that ignores context
cancellation can continue after the timeout stage returns and record a later
error; outer middleware cannot assume the value is already present in that
case.

## Registration and policy

Typed routes implement `RouteDefinition`, so they can be mixed with standard
routes:

```go
api := r.Group("/api").Timeout(5 * time.Second)
api.Route(
	createUser,
	router.RouteConfigBase{
		Path:    "/health",
		Methods: []router.HttpMethod{router.MethodGet},
		Handler: health,
	},
)
```

The route inherits authentication, rate limiting, timeout, body-size, and
middleware policy just like `RouteConfigBase`. Its `Overrides` take precedence
over group and global policy. Middleware remains additive.

Set `DisableTimeout` for a long-lived typed endpoint that must bypass its
effective timeout. Route registration freezes after `Build` or the first
request. See [Routing](routing.md), [Route groups](route-groups.md), and
[Codecs](codecs.md) for the surrounding APIs.
