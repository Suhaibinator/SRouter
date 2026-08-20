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
// middleware, and inherited route policy.
//
// A route tree is mutable until Router.Build is called (explicitly or by the
// first request). Mutating a frozen tree panics because httprouter does not
// support concurrent route registration.
type RouteGroup struct {
	tree        *routeTree
	prefix      string
	routes      []RouteDefinition
	children    []*RouteGroup
	middlewares []common.Middleware
	policy      groupPolicy
}

type routeTree struct {
	mu       sync.Mutex
	root     *RouteGroup
	frozen   bool
	buildErr error
	ready    atomic.Bool
}

func clearRouteGroup(group *RouteGroup) {
	for _, child := range group.children {
		clearRouteGroup(child)
	}
	group.routes = nil
	group.children = nil
	group.middlewares = nil
}

type groupValue[T any] struct {
	set   bool
	value T
}

type groupPolicy struct {
	timeout     groupValue[time.Duration]
	maxBodySize groupValue[int64]
	rateLimit   groupValue[*common.RateLimitConfig[any, any]]
	authToken   groupValue[*common.AuthTokenConfig]
	authLevel   groupValue[AuthLevel]
}

func newRouteTree() *routeTree {
	tree := &routeTree{}
	tree.root = &RouteGroup{tree: tree}
	return tree
}

func (g *RouteGroup) mutate(fn func()) {
	if g == nil || g.tree == nil {
		panic("router: nil route group")
	}

	g.tree.mu.Lock()
	defer g.tree.mu.Unlock()
	if g.tree.frozen {
		panic("router: route tree is frozen after Build or the first request")
	}
	fn()
}

// Group creates a child group whose prefix is relative to this group.
func (g *RouteGroup) Group(prefix string) *RouteGroup {
	var child *RouteGroup
	g.mutate(func() {
		child = &RouteGroup{tree: g.tree, prefix: prefix}
		g.children = append(g.children, child)
	})
	return child
}

// Route adds one or more standard or typed routes to the group.
func (g *RouteGroup) Route(routes ...RouteDefinition) *RouteGroup {
	g.mutate(func() {
		g.routes = append(g.routes, routes...)
	})
	return g
}

// Use appends middleware to the group. Middleware executes from the root group
// toward the innermost group, followed by route-specific middleware.
func (g *RouteGroup) Use(middlewares ...common.Middleware) *RouteGroup {
	g.mutate(func() {
		g.middlewares = append(g.middlewares, middlewares...)
	})
	return g
}

// Timeout overrides the inherited timeout. A zero duration disables timeouts
// for this group and its descendants.
func (g *RouteGroup) Timeout(timeout time.Duration) *RouteGroup {
	g.mutate(func() {
		g.policy.timeout = groupValue[time.Duration]{set: true, value: timeout}
	})
	return g
}

// MaxBodySize overrides the inherited request body limit. Zero disables the
// limit for this group and its descendants.
func (g *RouteGroup) MaxBodySize(bytes int64) *RouteGroup {
	g.mutate(func() {
		g.policy.maxBodySize = groupValue[int64]{set: true, value: bytes}
	})
	return g
}

// RateLimit overrides the inherited rate limit. Nil disables rate limiting for
// this group and its descendants.
func (g *RouteGroup) RateLimit(config *common.RateLimitConfig[any, any]) *RouteGroup {
	g.mutate(func() {
		g.policy.rateLimit = groupValue[*common.RateLimitConfig[any, any]]{set: true, value: config}
	})
	return g
}

// AuthToken overrides the inherited authentication token source. Nil resets
// the group to the built-in Authorization header source.
func (g *RouteGroup) AuthToken(config *common.AuthTokenConfig) *RouteGroup {
	g.mutate(func() {
		g.policy.authToken = groupValue[*common.AuthTokenConfig]{set: true, value: config}
	})
	return g
}

// Auth sets the default authentication level for this group and descendants.
// Individual routes may still override it.
func (g *RouteGroup) Auth(level AuthLevel) *RouteGroup {
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
