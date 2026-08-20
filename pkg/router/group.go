package router

import (
	"fmt"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/common"
)

// RouteGroup is a recursively nestable group of routes. Groups share the
// Router's dispatcher and infrastructure while contributing a path prefix,
// middleware, and inherited route policy. UserID and User match the owning
// Router so user-based rate-limit policies remain type-safe.
//
// A route tree is mutable until Router.Build is called (explicitly or by the
// first request). Mutating a frozen tree panics because httprouter does not
// support concurrent route registration.
type RouteGroup[UserID comparable, User any] struct {
	tree        *routeTree[UserID, User]
	prefix      string
	routes      []RouteDefinition
	children    []*RouteGroup[UserID, User]
	middlewares []common.Middleware
	policy      groupPolicy[UserID, User]
}

type routeTree[UserID comparable, User any] struct {
	mu       sync.Mutex
	root     *RouteGroup[UserID, User]
	buildErr error
	ready    atomic.Bool
}

func clearRouteGroup[UserID comparable, User any](group *RouteGroup[UserID, User]) {
	for _, child := range group.children {
		clearRouteGroup(child)
	}
	group.prefix = ""
	group.routes = nil
	group.children = nil
	group.middlewares = nil
	group.policy = groupPolicy[UserID, User]{}
}

type groupValue[T any] struct {
	set   bool
	value T
}

type groupPolicy[UserID comparable, User any] struct {
	timeout     groupValue[time.Duration]
	maxBodySize groupValue[int64]
	rateLimit   groupValue[*common.RateLimitConfig[UserID, User]]
	authToken   groupValue[*common.AuthTokenConfig]
	authLevel   groupValue[AuthLevel]
}

func newRouteTree[UserID comparable, User any]() *routeTree[UserID, User] {
	tree := &routeTree[UserID, User]{}
	tree.root = &RouteGroup[UserID, User]{tree: tree}
	return tree
}

func (g *RouteGroup[UserID, User]) mutate(fn func()) {
	if g == nil || g.tree == nil {
		panic("router: nil route group")
	}

	g.tree.mu.Lock()
	defer g.tree.mu.Unlock()
	if g.tree.ready.Load() {
		panic("router: route tree is frozen after Build or the first request")
	}
	fn()
}

// Group creates a child group whose prefix is relative to this group.
func (g *RouteGroup[UserID, User]) Group(prefix string) *RouteGroup[UserID, User] {
	var child *RouteGroup[UserID, User]
	g.mutate(func() {
		child = &RouteGroup[UserID, User]{tree: g.tree, prefix: prefix}
		g.children = append(g.children, child)
	})
	return child
}

// Route adds one or more standard or typed routes to the group.
func (g *RouteGroup[UserID, User]) Route(routes ...RouteDefinition) *RouteGroup[UserID, User] {
	g.mutate(func() {
		g.routes = append(g.routes, routes...)
	})
	return g
}

// Use appends middleware to the group. Middleware executes from the root group
// toward the innermost group, followed by route-specific middleware.
func (g *RouteGroup[UserID, User]) Use(middlewares ...common.Middleware) *RouteGroup[UserID, User] {
	g.mutate(func() {
		g.middlewares = append(g.middlewares, middlewares...)
	})
	return g
}

// Timeout overrides the inherited timeout. A zero duration disables timeouts
// for this group and its descendants.
func (g *RouteGroup[UserID, User]) Timeout(timeout time.Duration) *RouteGroup[UserID, User] {
	g.mutate(func() {
		g.policy.timeout = groupValue[time.Duration]{set: true, value: timeout}
	})
	return g
}

// MaxBodySize overrides the inherited request body limit. Zero disables the
// limit for this group and its descendants.
func (g *RouteGroup[UserID, User]) MaxBodySize(bytes int64) *RouteGroup[UserID, User] {
	g.mutate(func() {
		g.policy.maxBodySize = groupValue[int64]{set: true, value: bytes}
	})
	return g
}

// RateLimit overrides the inherited rate limit. Nil disables rate limiting for
// this group and its descendants.
func (g *RouteGroup[UserID, User]) RateLimit(config *common.RateLimitConfig[UserID, User]) *RouteGroup[UserID, User] {
	g.mutate(func() {
		g.policy.rateLimit = groupValue[*common.RateLimitConfig[UserID, User]]{set: true, value: config}
	})
	return g
}

// AuthToken overrides the inherited authentication token source. Nil resets
// the group to the built-in Authorization header source.
func (g *RouteGroup[UserID, User]) AuthToken(config *common.AuthTokenConfig) *RouteGroup[UserID, User] {
	g.mutate(func() {
		g.policy.authToken = groupValue[*common.AuthTokenConfig]{set: true, value: config}
	})
	return g
}

// Auth sets the default authentication level for this group and descendants.
// Individual routes may still override it.
func (g *RouteGroup[UserID, User]) Auth(level AuthLevel) *RouteGroup[UserID, User] {
	g.mutate(func() {
		g.policy.authLevel = groupValue[AuthLevel]{set: true, value: level}
	})
	return g
}

func validateGroupPrefix(prefix string) error {
	if prefix == "" {
		return fmt.Errorf("group prefix must not be empty")
	}
	if !strings.HasPrefix(prefix, "/") {
		return fmt.Errorf("group prefix %q must begin with '/'", prefix)
	}
	if prefix != "/" && strings.HasSuffix(prefix, "/") {
		return fmt.Errorf("group prefix %q must not end with '/'", prefix)
	}
	if strings.ContainsAny(prefix, "?#") {
		return fmt.Errorf("group prefix %q must not contain '?' or '#'", prefix)
	}
	return nil
}

func joinGroupPath(parent, child string) string {
	if child == "/" {
		return parent
	}
	return parent + child
}

func joinRoutePath(prefix, path string) (string, error) {
	if path == "" {
		if prefix == "" {
			return "", fmt.Errorf("root route path must not be empty")
		}
		return prefix, nil
	}
	if !strings.HasPrefix(path, "/") {
		return "", fmt.Errorf("route path %q must begin with '/'", path)
	}
	if strings.ContainsAny(path, "?#") {
		return "", fmt.Errorf("route path %q must not contain '?' or '#'", path)
	}
	return prefix + path, nil
}

func validateMiddlewares(label string, middlewares []common.Middleware) error {
	for index, middleware := range middlewares {
		if middleware == nil {
			return fmt.Errorf("%s contains nil middleware at index %d", label, index)
		}
	}
	return nil
}
