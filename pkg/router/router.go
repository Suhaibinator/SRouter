// Package router provides a flexible and feature-rich HTTP routing framework.
// It supports middleware, sub-routers, generic handlers, and various configuration options.
package router

import (
	"bufio"
	"context"
	"encoding/json" // Added for JSON marshalling
	"errors"
	"fmt"
	"net"
	"net/http"
	"slices"  // Added for CORS
	"strconv" // Added for CORS
	"strings"
	"sync"
	"sync/atomic" // Import atomic package
	"time"

	"github.com/Suhaibinator/SRouter/pkg/common"
	"github.com/Suhaibinator/SRouter/pkg/metrics"
	"github.com/Suhaibinator/SRouter/pkg/middleware" // Keep for middleware implementations like UberRateLimiter, IDGenerator, CreateTraceMiddleware
	"github.com/Suhaibinator/SRouter/pkg/scontext"   // Ensure scontext is imported
	"github.com/julienschmidt/httprouter"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore" // Added for log level constants
)

// Router is the main router struct that implements http.Handler.
// It provides routing, middleware support, graceful shutdown, and other features.
type Router[T comparable, U any] struct {
	config            RouterConfig
	router            *httprouter.Router
	logger            *zap.Logger
	middlewares       []common.Middleware
	authFunction      func(context.Context, string) (*U, bool)
	getUserIdFromUser func(*U) T
	rateLimiter       common.RateLimiter // Use common.RateLimiter
	wg                sync.WaitGroup
	shutdown          bool
	shutdownMu        sync.RWMutex
	metricsWriterPool sync.Pool               // Pool for reusing metricsResponseWriter objects
	traceIDGenerator  *middleware.IDGenerator // Generator for trace IDs

	// Precomputed CORS headers
	corsAllowMethods  string
	corsAllowHeaders  string
	corsExposeHeaders string
	corsMaxAge        string
}

const defaultAuthHeaderName = "Authorization"

type authTokenExtractor func(*http.Request) (string, bool, string)

// RegisterSubRouterWithSubRouter registers a nested SubRouter with a parent SubRouter.
// This helper function enables hierarchical route organization by adding a child
// SubRouter to the parent's SubRouters slice.
//
// Important behaviors:
// - Path prefixes are concatenated (parent + child)
// - Configuration overrides are inherited unless the child sets IsolateOverrides
// - Middlewares will be combined additively when routes are registered
// - This modifies the parent SubRouterConfig by appending to its SubRouters slice
//
// Example:
//
//	parentRouter := SubRouterConfig{PathPrefix: "/api"}
//	childRouter := SubRouterConfig{PathPrefix: "/v1", Overrides: common.RouteOverrides{Timeout: 5*time.Second}}
//	RegisterSubRouterWithSubRouter(&parentRouter, childRouter)
//	// Results in routes under /api/v1 with the child's timeout override
func RegisterSubRouterWithSubRouter(parent *SubRouterConfig, child SubRouterConfig) {
	// Add the child SubRouter to the parent's SubRouters field
	parent.SubRouters = append(parent.SubRouters, child)
}

// NewRouter creates a new Router instance with the given configuration.
// It initializes all components including the underlying httprouter, logging, middleware,
// metrics, rate limiting, and registers all routes defined in the configuration.
//
// Type parameters:
//   - T: The user ID type (must be comparable, e.g., string, int, uuid.UUID)
//   - U: The user object type (e.g., User, Account)
//
// Parameters:
//   - config: Router configuration including sub-routers, middleware, and settings
//   - authFunction: Function to validate tokens and return user objects
//   - userIdFromuserFunction: Function to extract user ID from user object
//
// The router automatically sets up trace ID generation, metrics collection, and
// CORS handling based on the provided configuration.
func NewRouter[T comparable, U any](config RouterConfig, authFunction func(context.Context, string) (*U, bool), userIdFromuserFunction func(*U) T) *Router[T, U] {
	// Initialize the httprouter
	hr := httprouter.New()

	// Set up the logger
	logger := config.Logger
	if logger == nil {
		// Create a default logger if none is provided
		var err error
		logger, err = zap.NewProduction()
		if err != nil {
			// Fallback to a no-op logger if we can't create a production logger
			logger = zap.NewNop()
		}
	}

	// Create a rate limiter using Uber's ratelimit library
	rateLimiter := middleware.NewUberRateLimiter()

	// Create the router
	r := &Router[T, U]{
		config:            config,
		router:            hr,
		logger:            logger.Named("SRouter"),
		authFunction:      authFunction,
		getUserIdFromUser: userIdFromuserFunction,
		middlewares:       config.Middlewares,
		rateLimiter:       rateLimiter,
		// CORS headers initialized below
		metricsWriterPool: sync.Pool{
			New: func() any {
				// metricsResponseWriter might still be needed for metrics, keep for now
				return &metricsResponseWriter[T, U]{}
			},
		},
	}

	// Precompute CORS headers if configured
	if config.CORSConfig != nil {
		// Warn about a contradictory configuration: the CORS spec forbids
		// credentials with a wildcard origin, so the credentials header will
		// never be emitted for wildcard matches.
		if config.CORSConfig.AllowCredentials && slices.Contains(config.CORSConfig.Origins, "*") {
			r.logger.Warn("CORS config combines wildcard origin with AllowCredentials; " +
				"credentials are never allowed for wildcard origins per the CORS spec. " +
				"List explicit origins to enable credentials.")
		}
		if len(config.CORSConfig.Methods) > 0 {
			r.corsAllowMethods = strings.Join(config.CORSConfig.Methods, ", ")
		}
		if len(config.CORSConfig.Headers) > 0 {
			r.corsAllowHeaders = strings.Join(config.CORSConfig.Headers, ", ")
		}
		if len(config.CORSConfig.ExposeHeaders) > 0 {
			r.corsExposeHeaders = strings.Join(config.CORSConfig.ExposeHeaders, ", ")
		}
		if config.CORSConfig.MaxAge > 0 {
			r.corsMaxAge = strconv.Itoa(int(config.CORSConfig.MaxAge.Seconds()))
		}
	}

	// Initialize trace ID generator if trace ID is enabled
	if config.TraceIDBufferSize > 0 {
		r.traceIDGenerator = middleware.NewIDGenerator(config.TraceIDBufferSize)
		// Note: trace middleware now added in wrapHandler, not here
	}

	// Add metrics middleware if configured
	if config.MetricsConfig != nil {
		var metricsMiddleware common.Middleware

		// A user-supplied middleware factory takes precedence over building one
		// from the Collector.
		if factory, ok := config.MetricsConfig.MiddlewareFactory.(metrics.MetricsMiddleware[T, U]); ok {
			metricsMiddleware = func(next http.Handler) http.Handler {
				return factory.Handler(config.ServiceName, next)
			}
		} else if registry, ok := config.MetricsConfig.Collector.(metrics.MetricsRegistry); ok {
			// Tags applied to every metric emitted by the middleware.
			defaultTags := metrics.Tags{}
			if config.MetricsConfig.Namespace != "" {
				defaultTags["service"] = config.MetricsConfig.Namespace // Assuming Namespace is intended for service tag
			}
			if config.MetricsConfig.Subsystem != "" {
				defaultTags["subsystem"] = config.MetricsConfig.Subsystem
			}

			// Create the generic middleware implementation using the router's T and U types
			metricsMiddlewareImpl := metrics.NewMetricsMiddleware[T, U](registry, metrics.MetricsMiddlewareConfig{
				EnableLatency:    config.MetricsConfig.EnableLatency,
				EnableThroughput: config.MetricsConfig.EnableThroughput,
				EnableQPS:        config.MetricsConfig.EnableQPS,
				EnableErrors:     config.MetricsConfig.EnableErrors,
				DefaultTags:      defaultTags,
			})
			// The middleware instance itself is now generic, but its Handler method
			// returns a standard http.Handler, so the adapter function remains the same.
			metricsMiddleware = func(next http.Handler) http.Handler {
				// Use the ServiceName from the config for the application name
				return metricsMiddlewareImpl.Handler(config.ServiceName, next)
			}
		}

		if metricsMiddleware != nil {
			r.middlewares = append(r.middlewares, metricsMiddleware)
		}
	}

	// Register routes from sub-routers
	for _, sr := range config.SubRouters {
		r.registerSubRouterWithResolvedOverrides(sr, common.RouteOverrides{})
	}

	return r
}

// RegisterSubRouter registers a sub-router with the router after router creation.
// This method allows dynamic registration of sub-routers at runtime.
//
// The sub-router's configuration is applied as follows:
// - Path prefix is prepended to all routes in the sub-router
// - Middlewares are added to (not replacing) global middlewares
// - Configuration overrides (timeout, max body size, rate limit, auth token) inherit into nested sub-routers by default
// - Nested sub-routers can set IsolateOverrides to ignore parent sub-router overrides
//
// This is useful for conditionally adding routes or building routes programmatically.
func (r *Router[T, U]) RegisterSubRouter(sr SubRouterConfig) {
	// Record the sub-router so later lookups (e.g. RegisterGenericRouteOnSubRouter)
	// can resolve it just like sub-routers provided in the initial config.
	r.config.SubRouters = append(r.config.SubRouters, sr)
	r.registerSubRouter(sr)
}

// registerSubRouter registers all routes and nested sub-routers defined in a SubRouterConfig.
// It applies the sub-router's path prefix, overrides, and middlewares.
// For nested sub-routers, path prefixes are concatenated and configuration overrides are inherited unless isolated.
// Middlewares are combined additively: global + sub-router + route-specific.
func (r *Router[T, U]) registerSubRouter(sr SubRouterConfig) {
	r.registerSubRouterWithResolvedOverrides(sr, common.RouteOverrides{})
}

func (r *Router[T, U]) registerSubRouterWithResolvedOverrides(sr SubRouterConfig, parentOverrides common.RouteOverrides) {
	resolvedOverrides := resolveSubRouterOverrides(parentOverrides, sr)
	sr.Overrides = resolvedOverrides

	// Register routes defined in this sub-router
	for _, routeDefinition := range sr.Routes {
		switch route := routeDefinition.(type) {
		case RouteConfigBase:
			// Handle standard RouteConfigBase
			fullPath := sr.PathPrefix + route.Path

			// Get effective settings considering overrides
			timeout := r.getEffectiveTimeout(route.Overrides.Timeout, sr.Overrides.Timeout)

			// If route has timeout disabled, set timeout to 0
			if route.DisableTimeout {
				timeout = 0
			}

			maxBodySize := r.getEffectiveMaxBodySize(route.Overrides.MaxBodySize, sr.Overrides.MaxBodySize)
			rateLimit := r.getEffectiveRateLimit(route.Overrides.RateLimit, sr.Overrides.RateLimit)
			authTokenResolution := r.getEffectiveAuthTokenConfigWithOrigin(route.Overrides.AuthToken, sr.Overrides.AuthToken)
			authLevel := route.AuthLevel // Use route-specific first
			if authLevel == nil {
				authLevel = sr.AuthLevel // Fallback to sub-router default
			}
			r.warnOnBuiltinAuthTokenFallback(fullPath, route.Methods, authLevel, authTokenResolution)

			// Combine middlewares: sub-router + route-specific
			allMiddlewares := make([]common.Middleware, 0, len(sr.Middlewares)+len(route.Middlewares))
			allMiddlewares = append(allMiddlewares, sr.Middlewares...)
			allMiddlewares = append(allMiddlewares, route.Middlewares...)

			// Create a handler with all middlewares applied (global middlewares are added inside wrapHandler)
			handler := r.wrapHandler(route.Handler, authLevel, authTokenResolution.config, timeout, maxBodySize, rateLimit, allMiddlewares)

			// Register the route with httprouter
			for _, method := range route.Methods {
				r.router.Handle(string(method), fullPath, r.convertToHTTPRouterHandle(handler, fullPath)) // Convert HttpMethod to string
			}

		case GenericRouteRegistrationFunc[T, U]:
			// Handle generic route registration function
			// The function itself will handle calculating effective settings and calling RegisterGenericRoute
			route(r, sr) // Call the registration function

		default:
			// Log or handle unexpected type in Routes slice
			r.logger.Warn("Unsupported type found in SubRouterConfig.Routes",
				zap.String("pathPrefix", sr.PathPrefix),
				zap.Any("type", fmt.Sprintf("%T", routeDefinition)),
			)
		}
	}

	// Register nested sub-routers recursively
	for _, nestedSR := range sr.SubRouters {
		// Create a new sub-router with the combined path prefix, inherited
		// middlewares (additive: parent's run before the nested sub-router's),
		// and inherited AuthLevel (unless the nested sub-router sets its own).
		nestedSRWithPrefix := nestedSR
		nestedSRWithPrefix.PathPrefix = sr.PathPrefix + nestedSR.PathPrefix
		nestedSRWithPrefix.Middlewares = combineMiddlewares(sr.Middlewares, nestedSR.Middlewares)
		if nestedSRWithPrefix.AuthLevel == nil {
			nestedSRWithPrefix.AuthLevel = sr.AuthLevel
		}

		// Register the nested sub-router
		r.registerSubRouterWithResolvedOverrides(nestedSRWithPrefix, resolvedOverrides)
	}
}

// convertToHTTPRouterHandle converts an http.Handler to an httprouter.Handle.
// It stores the route parameters and route template in the request context so they can be accessed by handlers.
func (r *Router[T, U]) convertToHTTPRouterHandle(handler http.Handler, routeTemplate string) httprouter.Handle {
	return func(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
		// Add both path params and route template to the SRouterContext
		ctx := scontext.WithRouteInfo[T, U](req.Context(), ps, routeTemplate)

		// For backwards compatibility, also store params at the ParamsKey for existing code
		ctx = context.WithValue(ctx, ParamsKey, ps)

		// Update the request object with the new context
		reqWithParams := req.WithContext(ctx)

		// Call the handler with the updated request object
		handler.ServeHTTP(w, reqWithParams)
	}
}

// wrapHandler wraps a handler with all the necessary middleware.
// It creates a complete request processing pipeline with the following middleware order,
// from outermost (first to see the request) to innermost (closest to the handler):
// 1. Recovery (outermost, catches panics from everything below it)
// 2. Trace ID injection (if enabled)
// 3. Authentication (if authLevel is set)
// 4. Rate limiting (if rateLimit is set)
// 5. Route-specific middlewares (from the middlewares parameter)
// 6. Global middlewares (from RouterConfig, includes metrics if enabled)
// 7. Timeout (innermost, if timeout > 0)
// 8. Body size limit (in the base handler)
//
// Middlewares are combined additively, not replaced.
func (r *Router[T, U]) wrapHandler(handler http.HandlerFunc, authLevel *AuthLevel, authTokenConfig common.AuthTokenConfig, timeout time.Duration, maxBodySize int64, rateLimit *common.RateLimitConfig[T, U], middlewares []common.Middleware) http.Handler { // Use common.RateLimitConfig
	// Create a base handler that only handles shutdown check and body size limit directly
	// Timeout is now handled by timeoutMiddleware setting the context.
	h := http.Handler(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// Note: shutdown check and request tracking happen at the top of
		// ServeHTTP so the whole middleware chain is covered, not just the
		// base handler.

		// Apply body size limit
		if maxBodySize > 0 {
			req.Body = http.MaxBytesReader(w, req.Body, maxBodySize)
		}

		// Call the actual handler (timeout context is applied by middleware)
		handler(w, req)
	}))

	// Build the middleware chain
	chain := common.NewMiddlewareChain()

	// Append middleware in order of execution (outermost first)

	// 1. Recovery (outermost, catches panics from the whole chain)
	chain = chain.Append(r.recoveryMiddleware)

	// 2. Trace middleware (if enabled) - positioned early so all middlewares have access to trace ID
	if r.traceIDGenerator != nil {
		traceMW := middleware.CreateTraceMiddleware[T, U](r.traceIDGenerator)
		chain = chain.Append(traceMW)
	}

	// 3. Authentication (Runs early)
	if authLevel != nil {
		switch *authLevel {
		case AuthRequired:
			chain = chain.Append(r.authRequiredMiddlewareWithConfig(authTokenConfig))
		case AuthOptional:
			chain = chain.Append(r.authOptionalMiddlewareWithConfig(authTokenConfig))
		}
	}

	// 4. Rate Limiting
	if rateLimit != nil {
		// Ensure the rate limiter implementation is compatible
		// Since r.rateLimiter is common.RateLimiter, this should work directly
		chain = chain.Append(middleware.RateLimit(rateLimit, r.rateLimiter, r.logger))
	}

	// 5. Route-Specific Middlewares
	chain = chain.Append(middlewares...)

	// 6. Global Middlewares (defined in RouterConfig)
	chain = chain.Append(r.middlewares...) // Note: This now includes ClientIPMiddleware added in NewRouter

	// 7. Timeout Handling (Sets context deadline)
	if timeout > 0 {
		chain = chain.Append(r.timeoutMiddleware(timeout))
	}

	// 8. Body Size Limit (Applied within the base handler 'h' now)
	// No separate middleware needed here anymore.

	// 9. Shutdown Handling (Applied within the base handler 'h' now)
	// No separate middleware needed here anymore.

	// Apply the chain to the base handler 'h'
	return chain.Then(h)
}

// timeoutMiddleware creates a middleware that handles request timeouts.
// It sets a context deadline and attempts to write a timeout error if the handler exceeds it,
// but only if the handler hasn't already started writing the response.
func (r *Router[T, U]) timeoutMiddleware(timeout time.Duration) common.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			if timeout <= 0 {
				next.ServeHTTP(w, req) // No timeout needed
				return
			}

			ctx, cancel := context.WithTimeout(req.Context(), timeout)
			defer cancel()
			req = req.WithContext(ctx)

			var wMutex sync.Mutex
			wrappedW := &mutexResponseWriter{
				ResponseWriter: w,
				mu:             &wMutex,
				// wroteHeader initialized to false
			}

			done := make(chan struct{})
			panicChan := make(chan any, 1) // Channel to capture panic

			go func() {
				defer func() {
					if p := recover(); p != nil {
						panicChan <- p // Send panic to the channel
					}
					close(done) // Signal completion (normal or panic)
				}()
				next.ServeHTTP(wrappedW, req)
			}()

			select {
			case <-done:
				// Handler finished (normally or panicked). Check panicChan.
				select {
				case p := <-panicChan:
					// Re-panic so the recoveryMiddleware can handle it
					panic(p)
				default:
					// No panic, normal completion
				}
				return
			case <-ctx.Done():
				// Timeout occurred. Log it.
				fields := append(r.baseFields(req),
					zap.Duration("timeout", timeout),
					zap.String("client_ip", req.RemoteAddr),
				)
				fields = r.addTrace(fields, req)
				r.logger.Error("Request timed out", fields...)

				// If the handler already started writing, don't attempt to take over the response.
				// Wait for the handler to finish to avoid returning while another goroutine is writing.
				if wrappedW.wroteHeader.Load() {
					<-done
					select {
					case p := <-panicChan:
						panic(p)
					default:
					}
					return
				}

				// Mark timed out so any in-flight handler writes fail fast and don't touch the underlying writer.
				wrappedW.timedOut.Store(true)

				// Reserve the response so the handler can't race to write its own error response.
				if !wrappedW.wroteHeader.CompareAndSwap(false, true) {
					<-done
					select {
					case p := <-panicChan:
						panic(p)
					default:
					}
					return
				}

				// Serialize the timeout response write with any handler goroutine currently inside rw methods.
				wrappedW.mu.Lock()
				traceID := scontext.GetTraceIDFromRequest[T, U](req)
				r.writeJSONError(wrappedW.ResponseWriter, req, http.StatusRequestTimeout, "Request Timeout", traceID)
				wrappedW.mu.Unlock()

				// Give the handler a chance to observe cancellation and exit promptly.
				select {
				case <-done:
					select {
					case p := <-panicChan:
						panic(p)
					default:
					}
				case <-time.After(50 * time.Millisecond):
				}
				return
			}
		})
	}
}

type subRouterConfigPrefixMatch struct {
	config SubRouterConfig
	found  bool
	exact  bool
}

func resolveSubRouterConfigForPrefix(subRouters []SubRouterConfig, targetPrefix string, parentPrefix string, parentOverrides common.RouteOverrides) (SubRouterConfig, bool) {
	match := resolveSubRouterConfigForPrefixMatch(subRouters, targetPrefix, parentPrefix, parentOverrides, nil, nil)
	if !match.found {
		return SubRouterConfig{}, false
	}
	return match.config, true
}

func resolveSubRouterConfigForPrefixMatch(subRouters []SubRouterConfig, targetPrefix string, parentPrefix string, parentOverrides common.RouteOverrides, parentMiddlewares []common.Middleware, parentAuthLevel *AuthLevel) subRouterConfigPrefixMatch {
	var fallback SubRouterConfig
	fallbackFound := false

	for _, sr := range subRouters {
		fullPathPrefix := parentPrefix + sr.PathPrefix
		resolvedOverrides := resolveSubRouterOverrides(parentOverrides, sr)
		resolvedMiddlewares := combineMiddlewares(parentMiddlewares, sr.Middlewares)
		resolvedAuthLevel := sr.AuthLevel
		if resolvedAuthLevel == nil {
			resolvedAuthLevel = parentAuthLevel
		}
		resolvedSR := sr
		resolvedSR.PathPrefix = fullPathPrefix
		resolvedSR.Overrides = resolvedOverrides
		resolvedSR.Middlewares = resolvedMiddlewares
		resolvedSR.AuthLevel = resolvedAuthLevel

		if fullPathPrefix == targetPrefix {
			return subRouterConfigPrefixMatch{config: resolvedSR, found: true, exact: true}
		}
		if sr.PathPrefix == targetPrefix && !fallbackFound {
			fallback = resolvedSR
			fallbackFound = true
		}
		childMatch := resolveSubRouterConfigForPrefixMatch(sr.SubRouters, targetPrefix, fullPathPrefix, resolvedOverrides, resolvedMiddlewares, resolvedAuthLevel)
		if childMatch.found {
			if childMatch.exact {
				return childMatch
			}
			if !fallbackFound {
				fallback = childMatch.config
				fallbackFound = true
			}
		}
	}
	if fallbackFound {
		return subRouterConfigPrefixMatch{config: fallback, found: true}
	}
	return subRouterConfigPrefixMatch{}
}

// combineMiddlewares returns a new slice containing parent middlewares followed
// by child middlewares. The inputs are never modified and the result has its
// own backing array.
func combineMiddlewares(parent, child []common.Middleware) []common.Middleware {
	if len(parent) == 0 && len(child) == 0 {
		return nil
	}
	combined := make([]common.Middleware, 0, len(parent)+len(child))
	combined = append(combined, parent...)
	combined = append(combined, child...)
	return combined
}

// RegisterGenericRouteOnSubRouter registers a generic route on a specific sub-router after router creation.
// This function is primarily used for dynamic route registration after the router has been initialized.
// For static route configuration, prefer using NewGenericRouteDefinition within SubRouterConfig.Routes.
//
// The function locates the sub-router by resolved full path prefix, applies its configuration
// (middleware, timeouts, rate limits), and registers the route with the combined settings.
// Relative sub-router prefixes are still accepted as a compatibility fallback when no resolved
// full path prefix matches.
//
// Type parameters:
//   - Req: Request type for the route
//   - Resp: Response type for the route
//   - UserID: Must match the router's T type parameter
//   - User: Must match the router's U type parameter
//
// Returns an error if no sub-router with the given prefix exists.
//
// Note: This is considered an advanced use case. The preferred approach is declarative
// route registration using NewGenericRouteDefinition in SubRouterConfig.
func RegisterGenericRouteOnSubRouter[Req any, Resp any, UserID comparable, User any](
	r *Router[UserID, User],
	pathPrefix string,
	route RouteConfig[Req, Resp],
) error {
	// Find the sub-router config matching the prefix
	sr, found := resolveSubRouterConfigForPrefix(r.config.SubRouters, pathPrefix, "", common.RouteOverrides{})
	if !found {
		// Option 1: Return an error if prefix doesn't match any configured sub-router
		return fmt.Errorf("no sub-router found with prefix: %s", pathPrefix)
		// Option 2: Log a warning and proceed with global defaults (less strict)
		// r.logger.Warn("No sub-router found with prefix, registering route with global defaults", zap.String("prefix", pathPrefix))
		// sr = nil // Ensure sr is nil if not found
	}

	// Determine effective settings using sub-router overrides if found
	var subRouterTimeout time.Duration
	var subRouterMaxBodySize int64
	var subRouterRateLimit *common.RateLimitConfig[any, any] // Use common type here
	var subRouterMiddlewares []common.Middleware
	subRouterTimeout = sr.Overrides.Timeout
	subRouterMaxBodySize = sr.Overrides.MaxBodySize
	subRouterRateLimit = sr.Overrides.RateLimit // This is already common.RateLimitConfig[any, any]
	subRouterMiddlewares = sr.Middlewares

	// Create a new route config instance to avoid modifying the original
	finalRouteConfig := route

	// Prefix the path
	finalRouteConfig.Path = sr.PathPrefix + route.Path

	// Combine middleware: sub-router + route-specific.
	// Note: Global middlewares are added later by wrapHandler; including them
	// here would apply them twice.
	allMiddlewares := make([]common.Middleware, 0, len(subRouterMiddlewares)+len(route.Middlewares))
	allMiddlewares = append(allMiddlewares, subRouterMiddlewares...) // Sub-router first
	allMiddlewares = append(allMiddlewares, route.Middlewares...)    // Then route-specific
	finalRouteConfig.Middlewares = allMiddlewares                    // Overwrite middlewares in the config passed down

	// Get effective timeout, max body size, rate limit considering overrides
	effectiveTimeout := r.getEffectiveTimeout(route.Overrides.Timeout, subRouterTimeout)
	effectiveMaxBodySize := r.getEffectiveMaxBodySize(route.Overrides.MaxBodySize, subRouterMaxBodySize)
	effectiveRateLimit := r.getEffectiveRateLimit(route.Overrides.RateLimit, subRouterRateLimit) // This returns *common.RateLimitConfig[UserID, User]
	effectiveAuthTokenResolution := r.getEffectiveAuthTokenConfigWithOrigin(route.Overrides.AuthToken, sr.Overrides.AuthToken)
	finalRouteConfig.Overrides.AuthToken = &effectiveAuthTokenResolution.config

	// Call the underlying generic registration function with the modified config
	registerGenericRouteWithAuthTokenResolution(r, finalRouteConfig, effectiveTimeout, effectiveMaxBodySize, effectiveRateLimit, effectiveAuthTokenResolution)

	return nil
}

// ServeHTTP implements the http.Handler interface.
// It handles HTTP requests by applying CORS, client IP extraction, metrics, tracing,
// and then delegating to the underlying httprouter.
func (r *Router[T, U]) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// Track the in-flight request for graceful shutdown. The Add must happen
	// under the shutdown lock so it can never race with Shutdown's wg.Wait:
	// Shutdown takes the write lock before waiting, so either this request is
	// rejected below, or it is registered before Wait can observe the counter.
	r.shutdownMu.RLock()
	if r.shutdown {
		r.shutdownMu.RUnlock()
		http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
		return
	}
	r.wg.Add(1)
	r.shutdownMu.RUnlock()
	defer r.wg.Done()

	// Handle CORS first
	var corsHandled bool
	req, corsHandled = r.handleCORS(w, req)
	if corsHandled {
		return // CORS preflight or invalid origin handled
	}

	// Default to the original writer, override if metrics/tracing enabled
	rw := w

	// Apply Client IP Extraction
	clientIP := extractClientIP(req, r.config.IPConfig)
	ctx := scontext.WithClientIP[T, U](req.Context(), clientIP) // Use scontext
	ctx = scontext.WithUserAgent[T, U](ctx, req.UserAgent())
	req = req.WithContext(ctx)

	// Apply request summary logging and status/bytes capture if enabled.
	// This is independent of trace IDs: EnableTraceLogging turns it on even
	// when TraceIDBufferSize is 0 (trace_id fields are simply absent then).
	if r.config.TraceIDBufferSize > 0 || r.config.EnableTraceLogging {
		// Get a metricsResponseWriter from the pool
		mrw := r.metricsWriterPool.Get().(*metricsResponseWriter[T, U])

		// Initialize the writer with the current request data
		mrw.baseResponseWriter = baseResponseWriter{ResponseWriter: w}
		mrw.statusCode = http.StatusOK
		mrw.startTime = time.Now()
		mrw.request = req
		mrw.router = r
		mrw.bytesWritten = 0

		rw = mrw

		// Defer logging, metrics collection, and returning the writer to the pool
		defer func() {
			// 1) Compute duration, traceID, ip
			duration := time.Since(mrw.startTime)
			ip, _ := scontext.GetClientIPFromRequest[T, U](req)
			ua, _ := scontext.GetUserAgentFromRequest[T, U](req)

			// 2) Build unified fields - the UNION of all previously separate log
			// fields. Sized for all fields (including the optional trace ID) up
			// front so this per-request path allocates the slice exactly once.
			fields := make([]zap.Field, 0, 8)
			fields = append(fields,
				zap.String("method", req.Method),
				zap.String("path", req.URL.Path),
				zap.Int("status", mrw.statusCode),
				zap.Duration("duration", duration),
				zap.Int64("bytes", mrw.bytesWritten),
				zap.String("ip", ip),
				zap.String("user_agent", ua),
			)
			fields = r.addTrace(fields, req)

			// 3) Decide the log level based on status code, duration, and trace config
			var lvl zapcore.Level
			switch {
			case mrw.statusCode >= 500:
				lvl = zapcore.ErrorLevel
			case mrw.statusCode >= 400 || duration > 500*time.Millisecond:
				lvl = zapcore.WarnLevel
			case r.config.TraceLoggingUseInfo:
				lvl = zapcore.InfoLevel
			default:
				lvl = zapcore.DebugLevel
			}

			// 4) Emit a single, unified log with the appropriate level
			r.logger.Log(lvl, "Request summary statistics", fields...)

			// Reset fields that might hold references to prevent memory leaks
			mrw.baseResponseWriter = baseResponseWriter{}
			mrw.request = nil
			mrw.router = nil

			// Return the writer to the pool
			r.metricsWriterPool.Put(mrw)
		}()
	}
	// Note: The 'else' block for rw = w is removed as rw is now defaulted to w earlier.

	// Serve the request via the underlying router
	r.router.ServeHTTP(rw, req)
}

// handleCORS applies CORS logic based on the router's configuration.
// It checks the origin, sets appropriate headers, handles preflight requests,
// and stores CORS information in the request context using the router's T and U types.
// It returns the modified request and a boolean indicating if the request was fully handled (e.g., preflight).
func (r *Router[T, U]) handleCORS(w http.ResponseWriter, req *http.Request) (*http.Request, bool) {
	if r.config.CORSConfig == nil {
		return req, false // CORS not configured
	}

	corsConfig := r.config.CORSConfig
	origin := req.Header.Get("Origin")
	ctx := req.Context()

	// Variables for the *correct* CORS decision to store in context
	correctAllowOrigin := ""
	correctAllowCredentials := false

	// Determine the correct Access-Control-Allow-Origin value for context
	if origin != "" { // Only process if Origin header is present
		isAllowed := false
		// Check for wildcard first
		if slices.Contains(corsConfig.Origins, "*") {
			correctAllowOrigin = "*" // Correct value is '*'
			isAllowed = true
		}
		// If not wildcard, check for specific match
		if !isAllowed {
			if slices.Contains(corsConfig.Origins, origin) {
				correctAllowOrigin = origin // Correct value is the specific origin
				isAllowed = true            // Mark as allowed
			}
		}
		// If origin wasn't allowed by config, correctAllowOrigin remains ""
		if !isAllowed {
			// Origin is not allowed. For preflight, we still need to return 204 but without Allow-* headers.
			// For actual requests, we *could* block here, but it's often better to let the request proceed
			// and let the browser enforce the lack of Allow-Origin header.
			// However, we MUST NOT set the Allow-Origin header.
			// The *lack* of allowance is stored in the context by the
			// unconditional WithCORSInfo call below (correctAllowOrigin stays "").
			// The response still varies by Origin (an allowed origin would get
			// CORS headers), so set Vary to keep shared caches correct.
			w.Header().Add("Vary", "Origin")
			// If it's a preflight, handle it below (it will fail the checks).
			// If it's not preflight, let it continue, but CORS headers won't be set.
		}
	} else {
		// No Origin header present. Store empty info in context.
		ctx = scontext.WithCORSInfo[T, U](ctx, "", false)
		req = req.WithContext(ctx)
		// Not a CORS request, proceed normally.
		return req, false
	}

	// Determine if credentials should be allowed (for context)
	// Credentials require a specific origin match (not '*') and config flag set
	if correctAllowOrigin != "" && correctAllowOrigin != "*" && corsConfig.AllowCredentials {
		correctAllowCredentials = true
	}

	// Store the *correct* CORS info in the context BEFORE handling preflight or calling next handler.
	// Use the router's specific T and U types.
	ctx = scontext.WithCORSInfo[T, U](ctx, correctAllowOrigin, correctAllowCredentials)
	req = req.WithContext(ctx)

	// --- Set Headers on Response Writer (Actual Response Headers) ---
	// Set Allow-Origin if an origin was allowed by the spec-compliant check
	if correctAllowOrigin != "" {
		w.Header().Set("Access-Control-Allow-Origin", correctAllowOrigin)
	}
	// Set Allow-Credentials if determined to be allowed by the spec-compliant check
	if correctAllowCredentials {
		w.Header().Set("Access-Control-Allow-Credentials", "true")
	}
	// Add Vary header if the allowed origin isn't always '*' (important for caching)
	if correctAllowOrigin != "" && correctAllowOrigin != "*" {
		w.Header().Add("Vary", "Origin")
	}
	// Set Expose-Headers for actual requests (not OPTIONS) only if origin was allowed
	if correctAllowOrigin != "" && r.corsExposeHeaders != "" && req.Method != http.MethodOptions {
		w.Header().Set("Access-Control-Expose-Headers", r.corsExposeHeaders)
	}

	// --- Handle preflight (OPTIONS) requests ---
	if req.Method == http.MethodOptions {
		// Only set preflight-specific headers if the origin was allowed
		if correctAllowOrigin != "" {
			// Check if the requested method is allowed
			reqMethod := req.Header.Get("Access-Control-Request-Method")
			methodAllowed := false
			if reqMethod != "" {
				// Check against configured list (case-sensitive comparison as per spec)
				if slices.Contains(corsConfig.Methods, reqMethod) {
					methodAllowed = true
				}
			} else {
				// If no request method header, it's not a valid preflight for methods?
				// Let's assume it needs to be explicitly allowed if requested.
				// If the header is *absent*, the browser isn't asking about methods,
				// so we don't need to restrict based on it. Default to true if absent.
				methodAllowed = true
			}

			// Check if the requested headers are allowed
			reqHeaders := req.Header.Get("Access-Control-Request-Headers")
			headersAllowed := true // Assume allowed unless specific headers requested and not found
			if reqHeaders != "" {
				// Check if wildcard is in the allowed headers list
				wildcardAllowed := slices.Contains(corsConfig.Headers, "*")

				// If wildcard is allowed, all headers are allowed
				if wildcardAllowed {
					headersAllowed = true

					// When wildcard is allowed, we'll echo back the exact headers the browser is requesting
					// This is stored in the context for later use when setting the response headers
					// We'll override the corsAllowHeaders value when responding to the preflight request
					if reqHeaders != "" {
						// Store the original requested headers to echo back
						ctx = scontext.WithCORSRequestedHeaders[T, U](ctx, reqHeaders)
						req = req.WithContext(ctx)
					}
				} else {
					// Original header checking logic
					requestedHeadersList := strings.Split(reqHeaders, ",")
					allowedHeadersSet := make(map[string]struct{}, len(corsConfig.Headers))
					for _, h := range corsConfig.Headers {
						allowedHeadersSet[strings.TrimSpace(strings.ToLower(h))] = struct{}{}
					}

					headersAllowed = true // Reset to true, only set to false if a requested header is *not* found
					for _, reqH := range requestedHeadersList {
						trimmedLowerReqH := strings.TrimSpace(strings.ToLower(reqH))
						if trimmedLowerReqH == "" {
							continue
						}
						if _, ok := allowedHeadersSet[trimmedLowerReqH]; !ok {
							headersAllowed = false
							break
						}
					}
				}
			} // If reqHeaders is empty, headersAllowed remains true

			// Only proceed with preflight response headers if origin, method, and headers are allowed
			if methodAllowed && headersAllowed {
				if r.corsAllowMethods != "" {
					w.Header().Set("Access-Control-Allow-Methods", r.corsAllowMethods)
				}

				// Check if we have stored requested headers to echo back (for wildcard case)
				if requestedHeaders, ok := scontext.GetCORSRequestedHeaders[T, U](ctx); ok && requestedHeaders != "" {
					// Echo back the exact headers the browser requested
					w.Header().Set("Access-Control-Allow-Headers", requestedHeaders)
				} else if r.corsAllowHeaders != "" {
					// Otherwise use the configured list
					w.Header().Set("Access-Control-Allow-Headers", r.corsAllowHeaders)
				}

				if r.corsMaxAge != "" {
					w.Header().Set("Access-Control-Max-Age", r.corsMaxAge)
				}
				// Note: Allow-Origin and Allow-Credentials are set earlier based on correct logic
			}
			// If origin, method or headers are not allowed, don't set the Allow-* headers for preflight.
			// The browser will treat this as a CORS failure. We still return 204 below,
			// but the absence of the Allow-* headers signals the failure.
		}

		// Preflight requests don't need to go further down the chain.
		// Respond with 204 No Content (preferred for preflight) regardless of success/failure of checks above.
		// The absence of Allow-* headers signals failure to the browser.
		w.WriteHeader(http.StatusNoContent) // Use 204 No Content
		return req, true                    // Request handled (preflight)
	}

	// Not a preflight request, continue processing
	return req, false // Request not fully handled by CORS logic
}

// baseResponseWriter provides common ResponseWriter functionality.
// It is embedded by other writers to avoid code duplication.
type baseResponseWriter struct {
	http.ResponseWriter
}

// Unwrap returns the underlying ResponseWriter.
// This enables Go 1.20+'s http.ResponseController to reach optional interfaces (e.g. Flusher, Hijacker)
// implemented by the original writer when this writer is wrapped.
func (bw *baseResponseWriter) Unwrap() http.ResponseWriter {
	return bw.ResponseWriter
}

// WriteHeader calls the underlying ResponseWriter's WriteHeader.
func (bw *baseResponseWriter) WriteHeader(statusCode int) {
	bw.ResponseWriter.WriteHeader(statusCode)
}

// Write delegates to the underlying ResponseWriter.
func (bw *baseResponseWriter) Write(b []byte) (int, error) {
	return bw.ResponseWriter.Write(b)
}

// Flush calls Flush on the underlying ResponseWriter when available.
func (bw *baseResponseWriter) Flush() {
	if f, ok := bw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Hijack delegates to the underlying ResponseWriter when it supports http.Hijacker.
// This is required for WebSocket upgrades to work through ResponseWriter wrappers.
func (bw *baseResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	h, ok := bw.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("underlying ResponseWriter (%T) does not support hijacking: %w", bw.ResponseWriter, http.ErrNotSupported)
	}
	return h.Hijack()
}

// metricsResponseWriter is a wrapper around http.ResponseWriter that captures metrics.
// It tracks the status code, bytes written, and timing information for each response.
// baseResponseWriter is embedded by value so pooled writers are reinitialized
// without allocating a fresh wrapper per request.
type metricsResponseWriter[T comparable, U any] struct {
	baseResponseWriter
	statusCode   int
	bytesWritten int64
	startTime    time.Time
	request      *http.Request
	router       *Router[T, U]
}

// WriteHeader captures the status code and calls the underlying ResponseWriter.WriteHeader.
func (rw *metricsResponseWriter[T, U]) WriteHeader(statusCode int) {
	rw.statusCode = statusCode
	rw.baseResponseWriter.WriteHeader(statusCode)
}

// Write captures the number of bytes written and calls the underlying ResponseWriter.Write.
func (rw *metricsResponseWriter[T, U]) Write(b []byte) (int, error) {
	n, err := rw.baseResponseWriter.Write(b)
	rw.bytesWritten += int64(n)
	return n, err
}

// Flush calls the underlying ResponseWriter.Flush if it implements http.Flusher.
func (rw *metricsResponseWriter[T, U]) Flush() {
	rw.baseResponseWriter.Flush()
}

// Shutdown gracefully shuts down the router.
// It stops accepting new requests and waits for existing requests to complete.
func (r *Router[T, U]) Shutdown(ctx context.Context) error {
	// Check if context is already done before proceeding
	if ctx.Err() != nil {
		return ctx.Err()
	}

	// Mark the router as shutting down
	r.shutdownMu.Lock()
	r.shutdown = true
	r.shutdownMu.Unlock()

	if r.traceIDGenerator != nil {
		r.traceIDGenerator.Stop()
	}

	// Create a channel to signal when all requests are done
	done := make(chan struct{})
	go func() {
		r.wg.Wait()
		close(done)
	}()

	// Wait for all requests to finish or for the context to be canceled
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// getEffectiveTimeout returns the effective timeout for a route.
// Precedence order (first non-zero value wins):
// 1. Route-specific timeout
// 2. Current or inherited sub-router timeout override
// 3. Global timeout from RouterConfig
func (r *Router[T, U]) getEffectiveTimeout(routeTimeout, subRouterTimeout time.Duration) time.Duration {
	if routeTimeout > 0 {
		return routeTimeout
	}
	if subRouterTimeout > 0 {
		return subRouterTimeout
	}
	return r.config.GlobalTimeout
}

func resolveSubRouterOverrides(parentOverrides common.RouteOverrides, sr SubRouterConfig) common.RouteOverrides {
	resolved := parentOverrides
	if sr.IsolateOverrides {
		resolved = common.RouteOverrides{}
	}

	if sr.Overrides.Timeout > 0 {
		resolved.Timeout = sr.Overrides.Timeout
	}
	if sr.Overrides.MaxBodySize > 0 {
		resolved.MaxBodySize = sr.Overrides.MaxBodySize
	}
	if sr.Overrides.RateLimit != nil {
		resolved.RateLimit = sr.Overrides.RateLimit
	}
	if sr.Overrides.AuthToken != nil {
		resolved.AuthToken = sr.Overrides.AuthToken
	}

	return resolved
}

func defaultAuthTokenConfig() common.AuthTokenConfig {
	return common.AuthTokenConfig{
		Source:     common.AuthTokenSourceHeader,
		HeaderName: defaultAuthHeaderName,
	}
}

type authTokenConfigOrigin string

const (
	authTokenOriginRoute     authTokenConfigOrigin = "route"
	authTokenOriginSubRouter authTokenConfigOrigin = "sub-router"
	authTokenOriginGlobal    authTokenConfigOrigin = "global"
	authTokenOriginDefault   authTokenConfigOrigin = "built-in default"
)

type authTokenConfigResolution struct {
	config common.AuthTokenConfig
	origin authTokenConfigOrigin
}

func normalizeAuthTokenConfig(config common.AuthTokenConfig) common.AuthTokenConfig {
	if config.Source == common.AuthTokenSourceHeader && config.HeaderName == "" {
		config.HeaderName = defaultAuthHeaderName
	}
	return config
}

func (r *Router[T, U]) warnOnInvalidAuthTokenConfig(config common.AuthTokenConfig) {
	if config.Source == common.AuthTokenSourceCookie && config.CookieName == "" {
		r.logger.Warn("Auth token cookie name not configured")
	}
}

func routeMethodStrings(methods []HttpMethod) []string {
	methodStrings := make([]string, len(methods))
	for i, method := range methods {
		methodStrings[i] = string(method)
	}
	return methodStrings
}

func (r *Router[T, U]) warnOnBuiltinAuthTokenFallback(path string, methods []HttpMethod, authLevel *AuthLevel, resolution authTokenConfigResolution) {
	if authLevel == nil || *authLevel != AuthRequired || resolution.origin != authTokenOriginDefault {
		return
	}

	r.logger.Warn("Auth-required route using built-in default auth token source",
		zap.String("path", path),
		zap.Strings("methods", routeMethodStrings(methods)),
		zap.String("auth_token_source", "header"),
		zap.String("header_name", defaultAuthHeaderName),
	)
}

func buildAuthTokenExtractor(config common.AuthTokenConfig) authTokenExtractor {
	switch config.Source {
	case common.AuthTokenSourceHeader:
		headerName := config.HeaderName
		if headerName == "" {
			headerName = defaultAuthHeaderName
		}
		missingReason := "no auth header"
		if headerName == defaultAuthHeaderName {
			missingReason = "no authorization header"
		}
		return func(req *http.Request) (string, bool, string) {
			authHeader := req.Header.Get(headerName)
			if authHeader == "" {
				return "", false, missingReason
			}
			token := strings.TrimPrefix(authHeader, "Bearer ")
			return token, true, ""
		}
	case common.AuthTokenSourceCookie:
		cookieName := config.CookieName
		if cookieName == "" {
			return func(*http.Request) (string, bool, string) {
				return "", false, "auth cookie name not configured"
			}
		}
		return func(req *http.Request) (string, bool, string) {
			cookie, err := req.Cookie(cookieName)
			if err != nil {
				return "", false, "no auth cookie"
			}
			return cookie.Value, true, ""
		}
	default:
		return func(*http.Request) (string, bool, string) {
			return "", false, "unsupported auth token source"
		}
	}
}

// getEffectiveAuthTokenConfigWithOrigin returns the effective auth token config
// for a route along with the origin it was resolved from.
// Precedence order (first non-nil value wins):
// 1. Route-specific auth token config
// 2. Sub-router auth token override
// 3. Global auth token config from RouterConfig
// 4. Default header-based auth token config
func (r *Router[T, U]) getEffectiveAuthTokenConfigWithOrigin(routeAuth, subRouterAuth *common.AuthTokenConfig) authTokenConfigResolution {
	if routeAuth != nil {
		return authTokenConfigResolution{
			config: normalizeAuthTokenConfig(*routeAuth),
			origin: authTokenOriginRoute,
		}
	}
	if subRouterAuth != nil {
		return authTokenConfigResolution{
			config: normalizeAuthTokenConfig(*subRouterAuth),
			origin: authTokenOriginSubRouter,
		}
	}
	if r.config.GlobalAuthToken != nil {
		return authTokenConfigResolution{
			config: normalizeAuthTokenConfig(*r.config.GlobalAuthToken),
			origin: authTokenOriginGlobal,
		}
	}
	return authTokenConfigResolution{
		config: defaultAuthTokenConfig(),
		origin: authTokenOriginDefault,
	}
}

// getEffectiveMaxBodySize returns the effective max body size for a route.
// Precedence order (first non-zero value wins):
// 1. Route-specific max body size
// 2. Current or inherited sub-router max body size override
// 3. Global max body size from RouterConfig
func (r *Router[T, U]) getEffectiveMaxBodySize(routeMaxBodySize, subRouterMaxBodySize int64) int64 {
	if routeMaxBodySize > 0 {
		return routeMaxBodySize
	}
	if subRouterMaxBodySize > 0 {
		return subRouterMaxBodySize
	}
	return r.config.GlobalMaxBodySize
}

// getEffectiveRateLimit returns the effective rate limit for a route.
// Precedence order (first non-nil value wins):
// 1. Route-specific rate limit
// 2. Current or inherited sub-router rate limit override
// 3. Global rate limit from RouterConfig
// The function also converts the generic type parameters from [any, any] to [T, U].
func (r *Router[T, U]) getEffectiveRateLimit(routeRateLimit, subRouterRateLimit *common.RateLimitConfig[any, any]) *common.RateLimitConfig[T, U] { // Use common types
	// Convert the rate limit config to the correct type
	convertConfig := func(config *common.RateLimitConfig[any, any]) *common.RateLimitConfig[T, U] { // Use common types
		if config == nil {
			return nil
		}

		// Adapt the user ID extraction functions across the type conversion so
		// StrategyUser keeps working when configured via [any, any] overrides.
		var userIDFromUser func(U) T
		if fromUser := config.UserIDFromUser; fromUser != nil {
			userIDFromUser = func(user U) T {
				id, _ := fromUser(user).(T)
				return id
			}
		}
		var userIDToString func(T) string
		if toString := config.UserIDToString; toString != nil {
			userIDToString = func(userID T) string {
				return toString(userID)
			}
		}

		// Create a new config with the correct type parameters
		return &common.RateLimitConfig[T, U]{ // Use common.RateLimitConfig
			BucketName:      config.BucketName,
			Limit:           config.Limit,
			Window:          config.Window,
			Strategy:        config.Strategy,
			UserIDFromUser:  userIDFromUser,
			UserIDToString:  userIDToString,
			KeyExtractor:    config.KeyExtractor,
			ExceededHandler: config.ExceededHandler,
		}
	}

	if routeRateLimit != nil {
		return convertConfig(routeRateLimit)
	}
	if subRouterRateLimit != nil {
		return convertConfig(subRouterRateLimit)
	}
	return convertConfig(r.config.GlobalRateLimit)
}

// baseFields returns common log fields for the request.
func (r *Router[T, U]) baseFields(req *http.Request) []zap.Field {
	return []zap.Field{
		zap.String("method", req.Method),
		zap.String("path", req.URL.Path),
	}
}

// addTrace appends the trace_id field when available.
func (r *Router[T, U]) addTrace(fields []zap.Field, req *http.Request) []zap.Field {
	if r.config.TraceIDBufferSize > 0 {
		if traceID := scontext.GetTraceIDFromRequest[T, U](req); traceID != "" {
			fields = append(fields, zap.String("trace_id", traceID))
		}
	}
	return fields
}

// isMaxBytesError reports whether err was caused by http.MaxBytesReader
// rejecting a request body, even if a codec has wrapped the error.
func isMaxBytesError(err error) bool {
	var maxBytesErr *http.MaxBytesError
	return errors.As(err, &maxBytesErr)
}

// handleError handles an error by logging it and returning an appropriate HTTP response.
// It checks if the error is a specific HTTPError and uses its status code and message if available.
// It also checks for context deadline exceeded errors.
func (r *Router[T, U]) handleError(w http.ResponseWriter, req *http.Request, err error, statusCode int, message string) {
	fields := append([]zap.Field{zap.Error(err)}, r.baseFields(req)...)
	fields = r.addTrace(fields, req)

	// Check for specific error types
	var httpErr *HTTPError
	if errors.Is(err, context.DeadlineExceeded) {
		// Handle timeout specifically
		statusCode = http.StatusRequestTimeout // Or http.StatusGatewayTimeout
		message = "Request Timeout"
		// Log specifically as timeout
		r.logger.Error("Request timed out (detected in handler)", fields...)
	} else if errors.As(err, &httpErr) {
		// Handle custom HTTPError
		statusCode = httpErr.StatusCode
		message = httpErr.Message
		r.logger.Error(message, fields...) // Log with the custom message
	} else if isMaxBytesError(err) {
		// Specifically handle MaxBytesReader error
		statusCode = http.StatusRequestEntityTooLarge
		message = "Request Entity Too Large"
		r.logger.Warn(message, fields...) // Log as Warn for client error
	} else {
		// Log generic internal server error
		r.logger.Error(message, fields...)
	}

	// Return the error response as JSON
	traceID := scontext.GetTraceIDFromRequest[T, U](req)
	r.writeJSONError(w, req, statusCode, message, traceID)
}

// writeJSONError writes a JSON error response to the client.
// It sets the Content-Type header to application/json and writes the status code.
// It includes the trace ID in the JSON payload if available and enabled.
// It also adds CORS headers based on information stored in the context by the CORS middleware.
func (r *Router[T, U]) writeJSONError(w http.ResponseWriter, req *http.Request, statusCode int, message string, traceID string) { // Add req parameter
	if mrw, ok := w.(*mutexResponseWriter); ok {
		if mrw.timedOut.Load() {
			return
		}
		if !mrw.wroteHeader.CompareAndSwap(false, true) {
			return
		}

		mrw.mu.Lock()
		defer mrw.mu.Unlock()

		allowedOrigin, credentialsAllowed, corsOK := scontext.GetCORSInfoFromRequest[T, U](req)
		header := mrw.ResponseWriter.Header()

		if corsOK {
			if allowedOrigin != "" {
				header.Set("Access-Control-Allow-Origin", allowedOrigin)
			}
			if credentialsAllowed {
				header.Set("Access-Control-Allow-Credentials", "true")
			}
			if allowedOrigin != "" && allowedOrigin != "*" {
				header.Add("Vary", "Origin")
			}
		}

		header.Set("Content-Type", "application/json; charset=utf-8")
		mrw.ResponseWriter.WriteHeader(statusCode)

		errorPayload := map[string]any{
			"error": map[string]string{
				"message": message,
			},
		}
		if r.config.TraceIDBufferSize > 0 && traceID != "" {
			errorMap := errorPayload["error"].(map[string]string)
			errorMap["trace_id"] = traceID
		}

		if err := json.NewEncoder(mrw.ResponseWriter).Encode(errorPayload); err != nil {
			r.logger.Error("Failed to write JSON error response",
				zap.Error(err),
				zap.Int("original_status", statusCode),
				zap.String("original_message", message),
				zap.String("trace_id", traceID),
			)
		}
		return
	}

	// Retrieve CORS info from context using the passed-in request
	allowedOrigin, credentialsAllowed, corsOK := scontext.GetCORSInfoFromRequest[T, U](req)

	// Set CORS headers if applicable BEFORE writing status code or body
	if corsOK {
		if allowedOrigin != "" {
			w.Header().Set("Access-Control-Allow-Origin", allowedOrigin)
		}
		if credentialsAllowed {
			w.Header().Set("Access-Control-Allow-Credentials", "true")
		}
		// Add Vary: Origin header if the allowed origin isn't always '*'
		// This logic should ideally mirror the CORS middleware's Vary logic
		// We might need access to the original CORSOptions here, or assume the middleware added Vary if needed.
		// For simplicity, let's add Vary if allowedOrigin is specific.
		if allowedOrigin != "" && allowedOrigin != "*" {
			w.Header().Add("Vary", "Origin")
		}
	}

	// Check if headers have already been written (best effort)
	// This check might not be foolproof depending on the ResponseWriter implementation.
	// http.Error handles this internally, but we need to be careful here.
	// A common pattern is to use a custom ResponseWriter wrapper that tracks this state.
	// Since we have mutexResponseWriter and metricsResponseWriter, they might offer ways,
	// but for simplicity, we'll rely on the fact that these error handlers are often
	// called before the main handler writes anything. If a panic/timeout happens *after*
	// writing has started, writing the JSON error might fail or corrupt the response.

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	// Ensure the status code is written *before* the body.
	// CORS headers are set above, before this.
	w.WriteHeader(statusCode)

	// Prepare the JSON payload
	errorPayload := map[string]any{
		"error": map[string]string{
			"message": message,
		},
	}

	// Add trace ID if enabled and available
	if r.config.TraceIDBufferSize > 0 && traceID != "" {
		errorMap := errorPayload["error"].(map[string]string)
		errorMap["trace_id"] = traceID
	}

	// Marshal and write the JSON response
	if err := json.NewEncoder(w).Encode(errorPayload); err != nil {
		// Log an error if we fail to marshal/write the JSON error response itself
		// At this point, we can't easily send a different error to the client.
		r.logger.Error("Failed to write JSON error response",
			zap.Error(err),
			zap.Int("original_status", statusCode),
			zap.String("original_message", message),
			zap.String("trace_id", traceID), // Log trace ID even if writing failed
		)
	}
}

// HTTPError represents an HTTP error with a status code and message.
// It can be used to return specific HTTP errors from handlers.
// When returned from a handler, the router will use the status code and message
// to generate an appropriate HTTP response. This allows handlers to control
// the exact error response sent to clients.
type HTTPError struct {
	StatusCode int    // HTTP status code (e.g., 400, 404, 500)
	Message    string // Error message to be sent in the response body
}

// Error implements the error interface.
// It returns a string representation of the HTTP error in the format "status: message".
func (e *HTTPError) Error() string {
	return fmt.Sprintf("%d: %s", e.StatusCode, e.Message)
}

// NewHTTPError creates a new HTTPError with the specified status code and message.
// It's a convenience function for creating HTTP errors in handlers.
func NewHTTPError(statusCode int, message string) *HTTPError {
	return &HTTPError{
		StatusCode: statusCode,
		Message:    message,
	}
}

// recoveryMiddleware is a middleware that recovers from panics in handlers.
// It logs the panic and returns a 500 Internal Server Error response if the
// response has not been started yet. If the panic occurred after the handler
// began writing, no second response is written (the partial response cannot
// be repaired) and the panic is only logged.
// This prevents the server from crashing when a handler panics.
func (r *Router[T, U]) recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		rw := &recoveryResponseWriter{baseResponseWriter: baseResponseWriter{ResponseWriter: w}}
		defer func() {
			if rec := recover(); rec != nil {
				fields := append([]zap.Field{zap.Any("panic", rec)}, r.baseFields(req)...)
				fields = r.addTrace(fields, req)
				r.logger.Error("Panic recovered", fields...)

				if rw.wrote {
					// The handler already started writing; appending a JSON
					// error would corrupt the response and trigger a
					// superfluous WriteHeader. Log only.
					return
				}

				// Return a 500 Internal Server Error as JSON
				traceID := scontext.GetTraceIDFromRequest[T, U](req)
				r.writeJSONError(rw, req, http.StatusInternalServerError, "Internal Server Error", traceID)
			}
		}()

		next.ServeHTTP(rw, req)
	})
}

// recoveryResponseWriter tracks whether the response has been started so the
// recovery middleware can avoid writing a second response after a panic that
// occurred mid-write. baseResponseWriter is embedded by value so the
// per-request wrapper costs a single allocation.
type recoveryResponseWriter struct {
	baseResponseWriter
	wrote bool
}

// WriteHeader marks the response as started and delegates to the underlying writer.
func (rw *recoveryResponseWriter) WriteHeader(statusCode int) {
	rw.wrote = true
	rw.baseResponseWriter.WriteHeader(statusCode)
}

// Write marks the response as started and delegates to the underlying writer.
func (rw *recoveryResponseWriter) Write(b []byte) (int, error) {
	rw.wrote = true
	return rw.baseResponseWriter.Write(b)
}

// authenticateRequest attempts to authenticate the request and, if successful,
// returns a new request with user information stored in the context.
// It does not perform any logging; callers handle logging based on the result.
func (r *Router[T, U]) authenticateRequest(req *http.Request, extractToken authTokenExtractor) (*http.Request, bool, string) {
	token, ok, reason := extractToken(req)
	if !ok {
		return req, false, reason
	}

	if user, valid := r.authFunction(req.Context(), token); valid {
		id := r.getUserIdFromUser(user)
		// When an SRouterContext already exists (always the case for requests
		// routed through ServeHTTP, which installs it before dispatch), the
		// With* helpers mutate it in place and return the same context. Any
		// trace ID already on that shared context is preserved automatically,
		// and cloning the request is only needed when a context was created.
		_, hadSRouterCtx := scontext.GetSRouterContext[T, U](req.Context())
		ctx := scontext.WithUserID[T, U](req.Context(), id)
		if r.config.AddUserObjectToCtx {
			ctx = scontext.WithUser[T](ctx, user)
		}
		if !hadSRouterCtx {
			req = req.WithContext(ctx)
		}
		return req, true, ""
	}
	return req, false, "invalid token"
}

func (r *Router[T, U]) authRequiredMiddlewareWithConfig(authTokenConfig common.AuthTokenConfig) common.Middleware {
	authTokenConfig = normalizeAuthTokenConfig(authTokenConfig)
	r.warnOnInvalidAuthTokenConfig(authTokenConfig)
	extractToken := buildAuthTokenExtractor(authTokenConfig)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			var ok bool
			var reason string
			req, ok, reason = r.authenticateRequest(req, extractToken)
			if !ok {
				fields := append(r.baseFields(req),
					zap.String("remote_addr", req.RemoteAddr),
					zap.String("error", reason),
				)
				fields = r.addTrace(fields, req)
				r.logger.Warn("Authentication failed", fields...)
				traceID := scontext.GetTraceIDFromRequest[T, U](req)
				r.writeJSONError(w, req, http.StatusUnauthorized, "Unauthorized", traceID)
				return
			}

			fields := r.addTrace(r.baseFields(req), req)
			r.logger.Debug("Authentication successful", fields...)
			next.ServeHTTP(w, req)
		})
	}
}

func (r *Router[T, U]) authOptionalMiddlewareWithConfig(authTokenConfig common.AuthTokenConfig) common.Middleware {
	authTokenConfig = normalizeAuthTokenConfig(authTokenConfig)
	r.warnOnInvalidAuthTokenConfig(authTokenConfig)
	extractToken := buildAuthTokenExtractor(authTokenConfig)
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			var ok bool
			req, ok, _ = r.authenticateRequest(req, extractToken)
			if ok {
				fields := r.addTrace(r.baseFields(req), req)
				r.logger.Debug("Authentication successful", fields...)
			}

			// Call the next handler regardless of authentication result
			next.ServeHTTP(w, req)
		})
	}
}

// responseWriter is a wrapper around http.ResponseWriter that captures the status code.
// This allows middleware to inspect the status code after the handler has completed.
type responseWriter struct {
	*baseResponseWriter
	statusCode int
}

// WriteHeader captures the status code and calls the underlying ResponseWriter.WriteHeader.
func (rw *responseWriter) WriteHeader(statusCode int) {
	rw.statusCode = statusCode
	rw.baseResponseWriter.WriteHeader(statusCode)
}

// Write calls the underlying ResponseWriter.Write.
func (rw *responseWriter) Write(b []byte) (int, error) {
	return rw.baseResponseWriter.Write(b)
}

// Flush calls the underlying ResponseWriter.Flush if it implements http.Flusher.
func (rw *responseWriter) Flush() {
	rw.baseResponseWriter.Flush()
}

// mutexResponseWriter is a wrapper around http.ResponseWriter that uses a mutex to protect access
// and tracks if headers/body have been written.
type mutexResponseWriter struct {
	http.ResponseWriter
	mu          *sync.Mutex
	wroteHeader atomic.Bool // Tracks if WriteHeader or Write has been called
	timedOut    atomic.Bool // When true, reject all writes to the underlying writer
}

// Header acquires the mutex and returns the underlying Header map.
func (rw *mutexResponseWriter) Header() http.Header {
	if rw.timedOut.Load() {
		return make(http.Header)
	}
	rw.mu.Lock()
	defer rw.mu.Unlock()
	return rw.ResponseWriter.Header()
}

// WriteHeader acquires the mutex, marks headers as written, and calls the underlying ResponseWriter.WriteHeader.
func (rw *mutexResponseWriter) WriteHeader(statusCode int) {
	if rw.timedOut.Load() {
		return
	}
	rw.mu.Lock()
	defer rw.mu.Unlock()
	if !rw.wroteHeader.Swap(true) { // Atomically set flag and check previous value
		rw.ResponseWriter.WriteHeader(statusCode)
	}
	// If header was already written, do nothing (consistent with http.ResponseWriter behavior)
}

// Write acquires the mutex, marks headers/body as written, and calls the underlying ResponseWriter.Write.
func (rw *mutexResponseWriter) Write(b []byte) (int, error) {
	if rw.timedOut.Load() {
		return 0, http.ErrHandlerTimeout
	}
	rw.mu.Lock()
	defer rw.mu.Unlock()
	// Re-check under the lock: the timeout response may have been written
	// while this write was waiting for the mutex.
	if rw.timedOut.Load() {
		return 0, http.ErrHandlerTimeout
	}
	rw.wroteHeader.Store(true) // Mark as written (headers might be implicitly written here)
	return rw.ResponseWriter.Write(b)
}

// Flush acquires the mutex and calls the underlying ResponseWriter.Flush if it implements http.Flusher.
func (rw *mutexResponseWriter) Flush() {
	if rw.timedOut.Load() {
		return
	}
	rw.mu.Lock()
	defer rw.mu.Unlock()
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
