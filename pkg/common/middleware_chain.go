// Package common provides common utilities and interfaces for the SRouter framework.
package common

import (
	"net/http"
)

// MiddlewareChain represents a chain of middleware
type MiddlewareChain []Middleware

// NewMiddlewareChain creates a new middleware chain
func NewMiddlewareChain(middlewares ...Middleware) MiddlewareChain {
	return middlewares
}

// Append returns a new chain with the given middleware added to the end.
// The original chain is never modified, and the result has its own backing
// array, so multiple Appends on the same parent chain are safe.
func (c MiddlewareChain) Append(middlewares ...Middleware) MiddlewareChain {
	result := make(MiddlewareChain, 0, len(c)+len(middlewares))
	result = append(result, c...)
	result = append(result, middlewares...)
	return result
}

// Prepend adds middleware to the beginning of the chain
func (c MiddlewareChain) Prepend(middlewares ...Middleware) MiddlewareChain {
	result := make(MiddlewareChain, len(middlewares)+len(c))
	copy(result, middlewares)
	copy(result[len(middlewares):], c)
	return result
}

// Then applies the middleware chain to a handler
func (c MiddlewareChain) Then(h http.Handler) http.Handler {
	for i := len(c) - 1; i >= 0; i-- {
		h = c[i](h)
	}
	return h
}

// ThenFunc applies the middleware chain to a handler function
func (c MiddlewareChain) ThenFunc(f http.HandlerFunc) http.Handler {
	return c.Then(http.HandlerFunc(f))
}
