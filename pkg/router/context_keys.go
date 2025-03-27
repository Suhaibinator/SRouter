package router

import (
	"net/http"

	"github.com/julienschmidt/httprouter"
)

// contextKey is a type for context keys.
// It's used to store and retrieve values from request contexts.
type contextKey string

const (
	// ParamsKey is the key used to store httprouter.Params in the request context.
	// This allows route parameters to be accessed from handlers and middleware.
	ParamsKey contextKey = "params"
)

// GetParams retrieves the httprouter.Params from the request context.
// This allows handlers to access route parameters extracted from the URL.
func GetParams(r *http.Request) httprouter.Params {
	params, _ := r.Context().Value(ParamsKey).(httprouter.Params)
	return params
}

// GetParam retrieves a specific parameter from the request context.
// It's a convenience function that combines GetParams and ByName.
func GetParam(r *http.Request, name string) string {
	return GetParams(r).ByName(name)
}
