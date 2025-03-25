// Package router provides a flexible and feature-rich HTTP routing framework.
package router

import (
	"context"
)

// RouterContext holds all values that a router instance adds to request contexts
// It allows storing multiple types of values with only a single level of context nesting
type RouterContext[T comparable, U any] struct {
	// User ID and User object storage
	UserID T
	User   *U
	Flags  map[string]bool

	// Track which fields are set
	userIDSet bool
	userSet   bool
}

// routerContextKey is a private type for the context key to avoid collisions
type routerContextKey struct{}

// NewRouterContext creates a new router context
func (r *Router[T, U]) NewRouterContext() *RouterContext[T, U] {
	return &RouterContext[T, U]{
		Flags: make(map[string]bool),
	}
}

// GetRouterContext retrieves the router context from a request context
func (r *Router[T, U]) GetRouterContext(ctx context.Context) (*RouterContext[T, U], bool) {
	rc, ok := ctx.Value(routerContextKey{}).(*RouterContext[T, U])
	return rc, ok
}

// WithRouterContext adds or updates the router context in the request context
func (r *Router[T, U]) WithRouterContext(ctx context.Context, rc *RouterContext[T, U]) context.Context {
	return context.WithValue(ctx, routerContextKey{}, rc)
}

// EnsureRouterContext retrieves or creates a router context
func (r *Router[T, U]) EnsureRouterContext(ctx context.Context) (*RouterContext[T, U], context.Context) {
	rc, ok := r.GetRouterContext(ctx)
	if !ok {
		rc = r.NewRouterContext()
		ctx = r.WithRouterContext(ctx, rc)
	}
	return rc, ctx
}

// WithUserID adds a user ID to the context
func (r *Router[T, U]) WithUserID(ctx context.Context, userID T) context.Context {
	rc, ctx := r.EnsureRouterContext(ctx)
	rc.UserID = userID
	rc.userIDSet = true
	return ctx
}

// GetUserIDFromContext retrieves a user ID from the router context
func (r *Router[T, U]) GetUserIDFromContext(ctx context.Context) (T, bool) {
	var zero T
	rc, ok := r.GetRouterContext(ctx)
	if !ok || !rc.userIDSet {
		return zero, false
	}
	return rc.UserID, true
}

// WithUser adds a user to the context
func (r *Router[T, U]) WithUser(ctx context.Context, user *U) context.Context {
	rc, ctx := r.EnsureRouterContext(ctx)
	rc.User = user
	rc.userSet = true
	return ctx
}

// GetUserFromContext retrieves a user from the router context
func (r *Router[T, U]) GetUserFromContext(ctx context.Context) (*U, bool) {
	rc, ok := r.GetRouterContext(ctx)
	if !ok || !rc.userSet {
		return nil, false
	}
	return rc.User, true
}

// WithFlag adds a flag to the context
func (r *Router[T, U]) WithFlag(ctx context.Context, name string, value bool) context.Context {
	rc, ctx := r.EnsureRouterContext(ctx)
	rc.Flags[name] = value
	return ctx
}

// GetFlagFromContext retrieves a flag from the router context
func (r *Router[T, U]) GetFlagFromContext(ctx context.Context, name string) (bool, bool) {
	rc, ok := r.GetRouterContext(ctx)
	if !ok {
		return false, false
	}
	value, exists := rc.Flags[name]
	return value, exists
}
