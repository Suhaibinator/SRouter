package router

import (
	"net/http"

	"github.com/Suhaibinator/SRouter/pkg/scontext"
)

// GetParams retrieves the path parameters of the matched route from the
// request context, in pattern order.
func GetParams(r *http.Request) scontext.Params {
	params, _ := scontext.GetPathParams(r.Context())
	return params
}

// GetParam retrieves a specific parameter from the request context.
// It's a convenience function that combines GetParams and ByName.
func GetParam(r *http.Request, name string) string {
	return GetParams(r).ByName(name)
}
