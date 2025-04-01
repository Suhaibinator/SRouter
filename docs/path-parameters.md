# Path Parameters

SRouter, using `julienschmidt/httprouter` internally, allows you to define routes with named parameters in the path. These parameters capture dynamic segments of the URL.

## Defining Routes with Path Parameters

Path parameters are defined using a colon (`:`) followed by the parameter name in the route path definition.

```go
// Example route definition within a SubRouterConfig or RouteConfigBase/RouteConfig
router.RouteConfigBase{
    Path:    "/users/:id", // ':id' is the path parameter
    Methods: []string{"GET"},
    Handler: GetUserHandler,
}

router.RouteConfigBase{
    Path:    "/articles/:category/:slug", // Multiple parameters
    Methods: []string{"GET"},
    Handler: GetArticleHandler,
}
```

## Accessing Path Parameters

SRouter provides helper functions to easily access the values of these parameters from the `http.Request` object within your handlers. The parameters are stored in the request's context.

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

Generally, `router.GetParam` is more convenient when you know the specific parameter name you need. `router.GetParams` is useful if you need to iterate over all parameters or access them dynamically.

## Path Parameter Reference

-   **`router.GetParam(r *http.Request, name string) string`**: Retrieves a specific parameter by name from the request context. Returns an empty string if the parameter is not found.
-   **`router.GetParams(r *http.Request) httprouter.Params`**: Retrieves all parameters from the request context as an `httprouter.Params` slice.
