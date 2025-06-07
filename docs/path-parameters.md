# Path Parameters

SRouter, using `julienschmidt/httprouter` internally, allows you to define routes with named parameters in the path. These parameters capture dynamic segments of the URL.

## Defining Routes with Path Parameters

Path parameters are defined using a colon (`:`) followed by the parameter name in the route path definition.

```go
// Example route definition within a SubRouterConfig or RouteConfigBase/RouteConfig
router.RouteConfigBase{
    Path:    "/users/:id", // ':id' is the path parameter
    Methods: []router.HttpMethod{router.MethodGet},
    Handler: GetUserHandler,
}

router.RouteConfigBase{
    Path:    "/articles/:category/:slug", // Multiple parameters
    Methods: []router.HttpMethod{router.MethodGet},
    Handler: GetArticleHandler,
}
```

## Accessing Path Parameters

SRouter stores path parameters in the request context.  In recent versions the
parameters (and the original route template) are placed inside the
`scontext.SRouterContext` wrapper.  Convenience helpers are provided in both the
`router` and `scontext` packages to retrieve them from handlers.

### `router.GetParam`

Retrieves the value of a single named parameter.

```go
import "github.com/Suhaibinator/SRouter/pkg/router"

func GetUserHandler(w http.ResponseWriter, r *http.Request) {
    // Get the value of the 'id' parameter from the path /users/:id
    userID := router.GetParam(r, "id")

    if userID == "" {
        // Handle case where parameter might be missing unexpectedly
        http.Error(w, "User ID not found in path", http.StatusBadRequest)
        return
    }

    fmt.Fprintf(w, "Fetching user with ID: %s", userID)
    // ... fetch user data using userID ...
}
```

### `router.GetParams`

Retrieves all path parameters captured for the request as `httprouter.Params`, which is a slice of `httprouter.Param`. Each `Param` struct has `Key` and `Value` fields.

```go
import (
    "fmt"
    "net/http"
    "github.com/Suhaibinator/SRouter/pkg/router"
    "github.com/julienschmidt/httprouter" // Import for Params type
)

func GetArticleHandler(w http.ResponseWriter, r *http.Request) {
    params := router.GetParams(r)

    var category, slug string
    for _, p := range params {
        switch p.Key {
        case "category":
            category = p.Value
        case "slug":
            slug = p.Value
        }
    }

    if category == "" || slug == "" {
        http.Error(w, "Category or slug missing from path", http.StatusBadRequest)
        return
    }

    fmt.Fprintf(w, "Fetching article in category '%s' with slug '%s'", category, slug)
    // ... fetch article data ...
}

// Alternative using ByName method
func GetArticleHandlerAlternative(w http.ResponseWriter, r *http.Request) {
    params := router.GetParams(r)

    category := params.ByName("category")
    slug := params.ByName("slug")

     if category == "" || slug == "" {
        http.Error(w, "Category or slug missing from path", http.StatusBadRequest)
        return
    }

    fmt.Fprintf(w, "Fetching article in category '%s' with slug '%s'", category, slug)
    // ... fetch article data ...
}
```

### `scontext.GetPathParamsFromRequest`

Retrieves path parameters from the `scontext.SRouterContext`. Replace `string, any` with your router's type parameters.

```go
import "github.com/Suhaibinator/SRouter/pkg/scontext"

func GetArticleHandlerTyped(w http.ResponseWriter, r *http.Request) {
    params, ok := scontext.GetPathParamsFromRequest[string, any](r)
    if !ok {
        http.Error(w, "Path parameters missing", http.StatusBadRequest)
        return
    }
    category := params.ByName("category")
    slug := params.ByName("slug")
    fmt.Fprintf(w, "Fetching article in category '%s' with slug '%s'", category, slug)
}
```

You can also retrieve the original route pattern using `scontext.GetRouteTemplateFromRequest[string, any](r)` for logging or metrics.

Generally, `router.GetParam` is more convenient when you know the specific parameter name you need. `router.GetParams` is useful if you need to iterate over all parameters or access them dynamically.

## Path Parameter Reference

-   **`router.GetParam(r *http.Request, name string) string`**: Retrieves a specific parameter by name from the request context. Returns an empty string if the parameter is not found.
-   **`router.GetParams(r *http.Request) httprouter.Params`**: Retrieves all parameters from the request context as an `httprouter.Params` slice.
-   **`scontext.GetPathParamsFromRequest[T, U](r *http.Request) (httprouter.Params, bool)`**: Returns all parameters from the generic `scontext` wrapper along with a boolean indicating presence.
-   **`scontext.GetRouteTemplateFromRequest[T, U](r *http.Request) (string, bool)`**: Retrieves the original route pattern from the context for metrics or logging.
