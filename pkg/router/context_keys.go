package router

import (
	"net/http"

	"github.com/Suhaibinator/SRouter/pkg/scontext"
	"github.com/julienschmidt/httprouter"
)

// GetParams retrieves the httprouter.Params from the request context.
// This allows handlers to access route parameters extracted from the URL.
func GetParams(r *http.Request) httprouter.Params {
	params, _ := scontext.GetPathParamsFromContext(r.Context())
	return params
}

// GetParam retrieves a specific parameter from the request context.
// It's a convenience function that combines GetParams and ByName.
func GetParam(r *http.Request, name string) string {
	return GetParams(r).ByName(name)
}
