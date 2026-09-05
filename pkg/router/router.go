// Package router provides a flexible and feature-rich HTTP routing framework.
// It supports middleware, recursive route groups, generic handlers, and various configuration options.
package router

import (
	"bufio"
	"context"
	json "encoding/json/v2"
	"errors"
	"fmt"
	"net"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/common"
	"github.com/Suhaibinator/SRouter/pkg/logkeys"
	"github.com/Suhaibinator/SRouter/pkg/metrics"
	"github.com/Suhaibinator/SRouter/pkg/middleware"
	"github.com/Suhaibinator/SRouter/pkg/scontext"
	"github.com/julienschmidt/httprouter"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// RouterDependencies contains application behavior used by a Router. BuildID
// and ConfigID must be concurrency-safe, fast, and non-panicking.
type RouterDependencies[T comparable, U any] struct {
	// Authenticate validates a token and returns its user object.
	Authenticate func(context.Context, string) (*U, bool)
	// UserID extracts the stable user identity from an authenticated user.
	UserID func(*U) T
	// BuildID returns the current opaque application build identity.
	BuildID func() string
	// ConfigID returns the current opaque application configuration identity.
	ConfigID func() string
}

// Router is the main router struct that implements http.Handler.
// It provides routing, middleware support, graceful shutdown, and other features.
type Router[T comparable, U any] struct {
	config            RouterConfig
	dependencies      RouterDependencies[T, U]
	router            *httprouter.Router
	routeTree         *routeTree[T, U]
	logger            *zap.Logger
	middlewares       []common.Middleware
	rateLimiter       common.RateLimiter
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

var (
	defaultCORSMethods = [...]string{http.MethodGet, http.MethodHead, http.MethodPost}
	defaultCORSHeaders = [...]string{"Accept", "Accept-Language", "Content-Language", "Content-Type"}
)

func effectiveCORSMethods(config *CORSConfig) []string {
	if len(config.Methods) == 0 {
		return defaultCORSMethods[:]
	}
	return config.Methods
}

func effectiveCORSHeaders(config *CORSConfig) []string {
	if len(config.Headers) == 0 {
		return defaultCORSHeaders[:]
	}
	return config.Headers
}

type authTokenExtractor func(*http.Request) (string, bool, string)

// NewRouter creates a Router with an empty route tree and initializes the
// infrastructure enabled by config. Register routes with Router.Route and
// Router.Group before calling Build or serving the first request.
//
// Type parameters:
//   - T: The user ID type (must be comparable, e.g., string, int, uuid.UUID)
//   - U: The user object type (e.g., User, Account)
//
// Parameters:
//   - config: Router infrastructure, global middleware, and default route settings
//   - dependencies: application-provided authentication and runtime identity resolvers
//
// Authenticate and UserID may be nil when every route resolves to NoAuth. Build
// rejects an AuthOptional or AuthRequired route if either dependency is nil.
func NewRouter[T comparable, U any](config RouterConfig, dependencies RouterDependencies[T, U]) *Router[T, U] {
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

	// Create the built-in nonblocking sliding-window rate limiter.
	rateLimiter := middleware.NewUberRateLimiter()

	// Create the router
	r := &Router[T, U]{
		config:       config,
		dependencies: dependencies,
		routeTree:    newRouteTree[T, U](),
		logger:       logger.Named("SRouter"),
		middlewares:  config.Middlewares,
		rateLimiter:  rateLimiter,
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
		r.corsAllowMethods = strings.Join(effectiveCORSMethods(config.CORSConfig), ", ")
		r.corsAllowHeaders = strings.Join(effectiveCORSHeaders(config.CORSConfig), ", ")
		if len(config.CORSConfig.ExposeHeaders) > 0 {
			r.corsExposeHeaders = strings.Join(config.CORSConfig.ExposeHeaders, ", ")
		}
		if maxAgeSeconds := int(config.CORSConfig.MaxAge.Seconds()); maxAgeSeconds > 0 {
			r.corsMaxAge = strconv.Itoa(maxAgeSeconds)
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
				defaultTags["service"] = config.MetricsConfig.Namespace
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

	return r
}

type resolvedGroup[T comparable, U any] struct {
	prefix      string
	timeout     time.Duration
	maxBodySize int64
	rateLimit   *common.RateLimitConfig[T, U]
	authToken   authTokenConfigResolution
	authLevel   *AuthLevel
	middlewares []common.Middleware
}

// Build validates and compiles the route-group tree. The first call freezes the
// tree and caches either success or failure; later calls return the cached
// result. ServeHTTP calls Build automatically, so explicit use is primarily for
// failing fast during application startup. Mutating routes or groups after the
// first build attempt panics.
func (r *Router[T, U]) Build() (err error) {
	tree := r.routeTree
	if tree.ready.Load() {
		return tree.buildErr
	}
	tree.mu.Lock()
	defer tree.mu.Unlock()
	if tree.ready.Load() {
		return tree.buildErr
	}
	defer tree.ready.Store(true)
	defer clearRouteGroup(tree.root)
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("build route tree: %v", recovered)
			tree.buildErr = err
		}
	}()

	if r.config.GlobalTimeout < 0 {
		tree.buildErr = fmt.Errorf("global timeout must not be negative")
		return tree.buildErr
	}
	if r.config.GlobalMaxBodySize < 0 {
		tree.buildErr = fmt.Errorf("global max body size must not be negative")
		return tree.buildErr
	}
	if err := validateMiddlewares("router", r.middlewares); err != nil {
		tree.buildErr = err
		return err
	}

	candidate := httprouter.New()
	initial := resolvedGroup[T, U]{
		timeout:     r.config.GlobalTimeout,
		maxBodySize: r.config.GlobalMaxBodySize,
		rateLimit:   r.convertRateLimit(r.config.GlobalRateLimit),
		authToken:   r.initialAuthTokenConfig(),
	}
	if err := r.buildGroup(candidate, tree.root, initial, true); err != nil {
		tree.buildErr = err
		return err
	}
	r.router = candidate
	return nil
}

func (r *Router[T, U]) buildGroup(candidate *httprouter.Router, group *RouteGroup[T, U], inherited resolvedGroup[T, U], root bool) error {
	resolved := inherited
	if !root {
		if err := validateGroupPrefix(group.prefix); err != nil {
			return err
		}
		resolved.prefix = joinGroupPath(inherited.prefix, group.prefix)
	}
	if group.policy.timeout.set {
		if group.policy.timeout.value < 0 {
			return fmt.Errorf("route group %q timeout must not be negative", resolved.prefix)
		}
		resolved.timeout = group.policy.timeout.value
	}
	if group.policy.maxBodySize.set {
		if group.policy.maxBodySize.value < 0 {
			return fmt.Errorf("route group %q max body size must not be negative", resolved.prefix)
		}
		resolved.maxBodySize = group.policy.maxBodySize.value
	}
	if group.policy.rateLimit.set {
		resolved.rateLimit = group.policy.rateLimit.value
	}
	if group.policy.authToken.set {
		if group.policy.authToken.value == nil {
			resolved.authToken = authTokenConfigResolution{config: defaultAuthTokenConfig(), origin: authTokenOriginDefault}
		} else {
			resolved.authToken = authTokenConfigResolution{config: normalizeAuthTokenConfig(*group.policy.authToken.value), origin: authTokenOriginGroup}
		}
	}
	if group.policy.authLevel.set {
		if group.policy.authLevel.value < NoAuth || group.policy.authLevel.value > AuthRequired {
			return fmt.Errorf("route group %q has invalid authentication level %d", resolved.prefix, group.policy.authLevel.value)
		}
		level := group.policy.authLevel.value
		resolved.authLevel = &level
	}
	if err := validateMiddlewares(fmt.Sprintf("route group %q", resolved.prefix), group.middlewares); err != nil {
		return err
	}
	resolved.middlewares = combineMiddlewares(inherited.middlewares, group.middlewares)

	for _, definition := range group.routes {
		if definition == nil {
			return fmt.Errorf("route group %q contains a nil route", resolved.prefix)
		}
		route, err := definition.baseConfig(r, resolved.prefix)
		if err != nil {
			return err
		}
		if err := r.registerCompiledRoute(candidate, route, resolved); err != nil {
			return err
		}
	}
	for _, child := range group.children {
		if err := r.buildGroup(candidate, child, resolved, false); err != nil {
			return err
		}
	}
	return nil
}

func (r *Router[T, U]) registerCompiledRoute(candidate *httprouter.Router, route RouteConfigBase, group resolvedGroup[T, U]) error {
	fullPath, err := joinRoutePath(group.prefix, route.Path)
	if err != nil {
		return err
	}
	if route.Handler == nil {
		return fmt.Errorf("route %q has no handler", fullPath)
	}
	if len(route.Methods) == 0 {
		return fmt.Errorf("route %q has no HTTP methods", fullPath)
	}
	if route.Overrides.Timeout < 0 {
		return fmt.Errorf("route %q timeout must not be negative", fullPath)
	}
	if route.Overrides.MaxBodySize < 0 {
		return fmt.Errorf("route %q max body size must not be negative", fullPath)
	}
	if route.AuthLevel != nil && (*route.AuthLevel < NoAuth || *route.AuthLevel > AuthRequired) {
		return fmt.Errorf("route %q has invalid authentication level %d", fullPath, *route.AuthLevel)
	}
	if err := validateMiddlewares(fmt.Sprintf("route %q", fullPath), route.Middlewares); err != nil {
		return err
	}

	timeout := group.timeout
	if route.Overrides.HasTimeout() {
		timeout = route.Overrides.Timeout
	}
	if route.DisableTimeout {
		timeout = 0
	}
	maxBodySize := group.maxBodySize
	if route.Overrides.HasMaxBodySize() {
		maxBodySize = route.Overrides.MaxBodySize
	}
	rateLimitConfig := group.rateLimit
	if route.Overrides.HasRateLimit() {
		rateLimitConfig = r.convertRateLimit(route.Overrides.RateLimit)
	}
	authTokenResolution := group.authToken
	if route.Overrides.HasAuthToken() {
		authTokenResolution = authTokenConfigResolution{config: normalizeAuthTokenConfig(*route.Overrides.AuthToken), origin: authTokenOriginRoute}
	}

	authLevel := route.AuthLevel
	if authLevel == nil {
		authLevel = group.authLevel
	}
	if authLevel != nil && *authLevel != NoAuth {
		if r.dependencies.Authenticate == nil {
			return fmt.Errorf("route %q enables authentication without an authentication function", fullPath)
		}
		if r.dependencies.UserID == nil {
			return fmt.Errorf("route %q enables authentication without a user ID function", fullPath)
		}
	}
	r.warnOnBuiltinAuthTokenFallback(fullPath, route.Methods, authLevel, authTokenResolution)

	middlewares := combineMiddlewares(group.middlewares, route.Middlewares)
	handler := r.wrapHandler(route.Handler, authLevel, authTokenResolution.config, timeout, maxBodySize, rateLimitConfig, middlewares)
	for _, method := range route.Methods {
		if method == "" {
			return fmt.Errorf("route %q contains an empty HTTP method", fullPath)
		}
		if err := r.handleRoute(candidate, string(method), fullPath, handler); err != nil {
			return err
		}
	}
	return nil
}

func (r *Router[T, U]) handleRoute(candidate *httprouter.Router, method, path string, handler http.Handler) (err error) {
	defer func() {
		if recovered := recover(); recovered != nil {
			err = fmt.Errorf("register %s %s: %v", method, path, recovered)
		}
	}()
	candidate.Handle(method, path, r.convertToHTTPRouterHandle(handler, path))
	return nil
}

// convertToHTTPRouterHandle converts an http.Handler to an httprouter.Handle.
// It stores the route parameters and route template in the request context so they can be accessed by handlers.
func (r *Router[T, U]) convertToHTTPRouterHandle(handler http.Handler, routeTemplate string) httprouter.Handle {
	return func(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
		if routerContext, ok := scontext.GetSRouterContext[T, U](req.Context()); ok {
			scontext.SetRouteInfo(routerContext, ps, routeTemplate)
			handler.ServeHTTP(w, req)
			return
		}

		// Internal direct dispatcher use may not have passed through Router.ServeHTTP.
		ctx := scontext.WithRouteInfo[T, U](req.Context(), ps, routeTemplate)
		handler.ServeHTTP(w, req.WithContext(ctx))
	}
}

// wrapHandler wraps a handler with all the necessary middleware.
// It creates a complete request processing pipeline with the following middleware order,
// from outermost (first to see the request) to innermost (closest to the handler):
// 1. Recovery (outermost, catches panics from everything below it)
// 2. Trace ID injection (if enabled)
// 3. Authentication (if authLevel is set)
// 4. Rate limiting (if rateLimit is set)
// 5. Global middlewares (from RouterConfig, including metrics if enabled)
// 6. Group middlewares (root to leaf), followed by route middleware
// 7. Timeout (innermost, if timeout > 0)
// 8. Body size limit (in the base handler)
//
// Middlewares are combined additively, not replaced.
func (r *Router[T, U]) wrapHandler(handler http.HandlerFunc, authLevel *AuthLevel, authTokenConfig common.AuthTokenConfig, timeout time.Duration, maxBodySize int64, rateLimit *common.RateLimitConfig[T, U], middlewares []common.Middleware) http.Handler {
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

	// 5. Global middlewares (defined in RouterConfig)
	chain = chain.Append(r.middlewares...)

	// 6. Route-group middlewares (root to leaf), then route middleware
	chain = chain.Append(middlewares...)

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
					zap.Duration(logkeys.Timeout, timeout),
					zap.String(logkeys.ClientIP, req.RemoteAddr),
					zap.Int(logkeys.StatusCode, http.StatusRequestTimeout),
					zap.String(logkeys.TraceID, r.errorTraceID(req)),
				)
				r.logger.Warn("Request timed out", fields...)

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
				traceID := scontext.GetTraceIDFromContext[T, U](req.Context())
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

// combineMiddlewares returns parent middleware followed by child middleware.
// Build freezes the input slices, so an existing slice can be reused when the
// other side is empty; only a true combination needs a new backing array.
func combineMiddlewares(parent, child []common.Middleware) []common.Middleware {
	if len(parent) == 0 {
		return child
	}
	if len(child) == 0 {
		return parent
	}
	combined := make([]common.Middleware, 0, len(parent)+len(child))
	combined = append(combined, parent...)
	combined = append(combined, child...)
	return combined
}

// ServeHTTP implements http.Handler. It builds the route tree lazily, tracks the
// request for graceful shutdown, handles CORS, adds client information, wraps
// request-summary logging when enabled, and delegates route matching to
// httprouter. Trace IDs and configured metrics run inside matched route chains.
func (r *Router[T, U]) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	req = r.withRuntimeIdentities(req)

	var buildErr error
	if r.routeTree.ready.Load() {
		buildErr = r.routeTree.buildErr
	} else {
		buildErr = r.Build()
	}
	if buildErr != nil {
		fields := append(r.baseFields(req), zap.NamedError(logkeys.Error, buildErr))
		r.logger.Error("Failed to build route tree", fields...)
		http.Error(w, "Router configuration error", http.StatusInternalServerError)
		return
	}

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
	ctx := scontext.WithClientInfo[T, U](req.Context(), clientIP, req.UserAgent())
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
		mrw.wroteHeader = false
		mrw.startTime = time.Now()
		mrw.request = req
		mrw.router = r
		mrw.bytesWritten = 0

		rw = mrw

		// Defer logging, metrics collection, and returning the writer to the pool
		defer func() {
			// 1) Compute duration, traceID, ip
			duration := time.Since(mrw.startTime)
			ip, _ := scontext.GetClientIP[T, U](req.Context())
			ua, _ := scontext.GetUserAgent[T, U](req.Context())

			// 2) Build unified fields - the UNION of all previously separate log
			// fields. Sized for all fields (including the optional trace ID) up
			// front so this per-request path allocates the slice exactly once.
			fields := make([]zap.Field, 0, 10)
			fields = append(fields,
				zap.String(logkeys.Method, req.Method),
				zap.String(logkeys.Path, req.URL.Path),
				zap.Int(logkeys.Status, mrw.statusCode),
				zap.Duration(logkeys.Duration, duration),
				zap.Int64(logkeys.Bytes, mrw.bytesWritten),
				zap.String(logkeys.IP, ip),
				zap.String(logkeys.UserAgent, ua),
			)
			fields = r.addRuntimeIdentityFields(fields, req)
			fields = r.addTrace(fields, req)

			// 3) Decide the log level based on status code, duration, and trace config
			var lvl zapcore.Level
			switch {
			case mrw.statusCode >= 500:
				lvl = zapcore.ErrorLevel
			case duration > 500*time.Millisecond:
				lvl = zapcore.WarnLevel
			case mrw.statusCode >= 400:
				lvl = zapcore.InfoLevel
			case r.config.TraceLoggingUseInfo:
				lvl = zapcore.InfoLevel
			default:
				lvl = zapcore.DebugLevel
			}

			// 4) Emit a single, unified log with the appropriate level
			r.logger.Log(lvl, "Request summary statistics", fields...)

			// Reset fields that might hold references to prevent memory leaks
			mrw.baseResponseWriter = baseResponseWriter{}
			mrw.wroteHeader = false
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

// withRuntimeIdentities samples each configured provider once for this request
// and stores non-empty opaque identities in the shared SRouter context. Local
// provider values replace identities inherited on the incoming context.
func (r *Router[T, U]) withRuntimeIdentities(req *http.Request) *http.Request {
	ctx := req.Context()
	changed := false
	if provider := r.dependencies.BuildID; provider != nil {
		if buildID := provider(); buildID != "" {
			ctx = scontext.WithBuildID[T, U](ctx, buildID)
			changed = true
		}
	}
	if provider := r.dependencies.ConfigID; provider != nil {
		if configID := provider(); configID != "" {
			ctx = scontext.WithConfigID[T, U](ctx, configID)
			changed = true
		}
	}
	if !changed {
		return req
	}
	return req.WithContext(ctx)
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
				if slices.Contains(effectiveCORSMethods(corsConfig), reqMethod) {
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
				effectiveHeaders := effectiveCORSHeaders(corsConfig)
				wildcardAllowed := slices.Contains(effectiveHeaders, "*")

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
					// Compare requested header names case-insensitively.
					requestedHeadersList := strings.Split(reqHeaders, ",")
					allowedHeadersSet := make(map[string]struct{}, len(effectiveHeaders))
					for _, h := range effectiveHeaders {
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
		w.WriteHeader(http.StatusNoContent)
		return req, true // Request handled (preflight)
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
	wroteHeader  bool
	bytesWritten int64
	startTime    time.Time
	request      *http.Request
	router       *Router[T, U]
}

// WriteHeader forwards informational responses, then captures and forwards the
// first final status, matching net/http response semantics.
func (rw *metricsResponseWriter[T, U]) WriteHeader(statusCode int) {
	if rw.wroteHeader {
		return
	}
	if statusCode >= 100 && statusCode < 200 && statusCode != http.StatusSwitchingProtocols {
		rw.baseResponseWriter.WriteHeader(statusCode)
		return
	}
	rw.wroteHeader = true
	rw.statusCode = statusCode
	rw.baseResponseWriter.WriteHeader(statusCode)
}

// Write records an implicit 200 status, captures the number of bytes written,
// and delegates to the underlying writer.
func (rw *metricsResponseWriter[T, U]) Write(b []byte) (int, error) {
	if !rw.wroteHeader {
		rw.wroteHeader = true
		rw.statusCode = http.StatusOK
	}
	n, err := rw.baseResponseWriter.Write(b)
	rw.bytesWritten += int64(n)
	return n, err
}

// Flush records an implicit 200 status and delegates to the underlying writer.
func (rw *metricsResponseWriter[T, U]) Flush() {
	if !rw.wroteHeader {
		rw.wroteHeader = true
		rw.statusCode = http.StatusOK
	}
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

func defaultAuthTokenConfig() common.AuthTokenConfig {
	return common.AuthTokenConfig{
		Source:     common.AuthTokenSourceHeader,
		HeaderName: defaultAuthHeaderName,
	}
}

type authTokenConfigOrigin string

const (
	authTokenOriginRoute   authTokenConfigOrigin = "route"
	authTokenOriginGroup   authTokenConfigOrigin = "route group"
	authTokenOriginGlobal  authTokenConfigOrigin = "global"
	authTokenOriginDefault authTokenConfigOrigin = "built-in default"
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
		zap.String(logkeys.Path, path),
		zap.Strings(logkeys.Methods, routeMethodStrings(methods)),
		zap.String(logkeys.AuthTokenSource, "header"),
		zap.String(logkeys.HeaderName, defaultAuthHeaderName),
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

func (r *Router[T, U]) initialAuthTokenConfig() authTokenConfigResolution {
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

func (r *Router[T, U]) convertRateLimit(config *common.RateLimitConfig[any, any]) *common.RateLimitConfig[T, U] {
	if config == nil {
		return nil
	}

	var userIDFromUser func(U) T
	if fromUser := config.UserIDFromUser; fromUser != nil {
		userIDFromUser = func(user U) T {
			id, ok := fromUser(user).(T)
			if !ok {
				panic("router: rate limit UserIDFromUser returned an incompatible user ID type")
			}
			return id
		}
	}
	var userIDToString func(T) string
	if toString := config.UserIDToString; toString != nil {
		userIDToString = func(userID T) string {
			return toString(userID)
		}
	}

	return &common.RateLimitConfig[T, U]{
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

// baseFields returns common log fields for the request.
func (r *Router[T, U]) baseFields(req *http.Request) []zap.Field {
	fields := []zap.Field{
		zap.String(logkeys.Method, req.Method),
		zap.String(logkeys.Path, req.URL.Path),
	}
	return r.addRuntimeIdentityFields(fields, req)
}

// addRuntimeIdentityFields appends the opaque build and configuration
// identities installed for this request, when present.
func (r *Router[T, U]) addRuntimeIdentityFields(fields []zap.Field, req *http.Request) []zap.Field {
	if buildID, ok := scontext.GetBuildID[T, U](req.Context()); ok {
		fields = append(fields, zap.String(logkeys.BuildID, buildID))
	}
	if configID, ok := scontext.GetConfigID[T, U](req.Context()); ok {
		fields = append(fields, zap.String(logkeys.ConfigID, configID))
	}
	return fields
}

// addTrace appends the automatic trace_id field when generation is enabled and
// the request contains one.
func (r *Router[T, U]) addTrace(fields []zap.Field, req *http.Request) []zap.Field {
	if r.config.TraceIDBufferSize > 0 {
		if traceID := scontext.GetTraceIDFromContext[T, U](req.Context()); traceID != "" {
			fields = append(fields, zap.String(logkeys.TraceID, traceID))
		}
	}
	return fields
}

// errorTraceID returns the request trace ID or creates one for the error log.
// Error records must always be correlatable, even when request-wide trace ID
// generation is disabled.
func (r *Router[T, U]) errorTraceID(req *http.Request) string {
	if traceID := scontext.GetTraceIDFromContext[T, U](req.Context()); traceID != "" {
		return traceID
	}
	return middleware.GenerateTraceID()
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
	logErr := err
	logMessage := message
	var attachedFields []zap.Field
	var levelOverride zapcore.Level
	var hasLevelOverride bool

	if httpErr, ok := errors.AsType[*HTTPError](err); ok {
		statusCode = httpErr.StatusCode
		message = httpErr.Message
		logMessage = message
		attachedFields = httpErr.Fields()
		levelOverride, hasLevelOverride = httpErr.LogLevel()
		if cause := httpErr.Cause(); cause != nil {
			logErr = cause
		}
	} else if isMaxBytesError(err) {
		statusCode = http.StatusRequestEntityTooLarge
		message = "Request Entity Too Large"
		logMessage = message
	} else if errors.Is(err, context.DeadlineExceeded) {
		statusCode = http.StatusRequestTimeout
		message = "Request Timeout"
		logMessage = "Request timed out (detected in handler)"
	}

	invalidStatusCode := 0
	if statusCode < http.StatusBadRequest || statusCode > 599 {
		invalidStatusCode = statusCode
		statusCode = http.StatusInternalServerError
		message = "Internal Server Error"
		logMessage = message
	}

	level := zapcore.ErrorLevel
	switch {
	case hasLevelOverride:
		level = levelOverride
	case errors.Is(err, context.Canceled):
		level = zapcore.DebugLevel
	case errors.Is(err, context.DeadlineExceeded):
		level = zapcore.WarnLevel
	case statusCode >= http.StatusBadRequest && statusCode < http.StatusInternalServerError:
		level = zapcore.InfoLevel
	}

	traceID := r.errorTraceID(req)
	fields := make([]zap.Field, 0, 7+len(attachedFields))
	fields = append(fields, sanitizeHTTPErrorFields(attachedFields)...)
	if invalidStatusCode != 0 {
		fields = append(fields, zap.Int(logkeys.InvalidStatusCode, invalidStatusCode))
	}
	fields = append(fields,
		zap.NamedError(logkeys.Error, logErr),
		zap.Int(logkeys.StatusCode, statusCode),
	)
	fields = append(fields, r.baseFields(req)...)
	fields = append(fields, zap.String(logkeys.TraceID, traceID))
	r.logger.Log(level, logMessage, fields...)

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

		allowedOrigin, credentialsAllowed, corsOK := scontext.GetCORSInfo[T, U](req.Context())
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

		if err := json.MarshalWrite(mrw.ResponseWriter, errorPayload); err != nil {
			r.logJSONErrorWriteFailure(req, err, statusCode, message, traceID)
		}
		return
	}

	// Retrieve CORS info from context using the passed-in request
	allowedOrigin, credentialsAllowed, corsOK := scontext.GetCORSInfo[T, U](req.Context())

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
	if err := json.MarshalWrite(w, errorPayload); err != nil {
		r.logJSONErrorWriteFailure(req, err, statusCode, message, traceID)
	}
}

func (r *Router[T, U]) logJSONErrorWriteFailure(req *http.Request, err error, statusCode int, message, traceID string) {
	if traceID == "" {
		traceID = r.errorTraceID(req)
	}
	fields := []zap.Field{
		zap.NamedError(logkeys.Error, err),
		zap.Int(logkeys.StatusCode, statusCode),
		zap.Int(logkeys.OriginalStatus, statusCode),
		zap.String(logkeys.OriginalMessage, message),
	}
	fields = append(fields, r.baseFields(req)...)
	fields = append(fields, zap.String(logkeys.TraceID, traceID))
	r.logger.Error("Failed to write JSON error response", fields...)
}

// HTTPError represents a client-facing HTTP status and message with optional
// diagnostic context. The router exposes only StatusCode and Message in the
// response; an attached cause and structured fields are retained for logs.
// WithLogLevel can override the router's default severity classification.
type HTTPError struct {
	StatusCode int    // HTTP status code (e.g., 400, 404, 500)
	Message    string // Error message to be sent in the response body
	cause      error
	fields     []zap.Field
	logLevel   zapcore.Level
	hasLevel   bool
}

// Error implements the error interface.
// It returns a string representation of the HTTP error in the format "status: message".
func (e *HTTPError) Error() string {
	return fmt.Sprintf("%d: %s", e.StatusCode, e.Message)
}

// Unwrap returns the underlying cause, allowing errors.Is and errors.As to
// inspect errors translated into an HTTP response.
func (e *HTTPError) Unwrap() error {
	return e.cause
}

// Cause returns the diagnostic cause retained by the HTTP error. The cause is
// logged by the router but is never included in the HTTP response.
func (e *HTTPError) Cause() error {
	return e.cause
}

// Fields returns a copy of the structured diagnostic fields attached to the
// HTTP error. Mutating the returned slice cannot alter the error.
func (e *HTTPError) Fields() []zap.Field {
	return slices.Clone(e.fields)
}

// LogLevel returns the explicit boundary log level, when one was configured.
func (e *HTTPError) LogLevel() (zapcore.Level, bool) {
	return e.logLevel, e.hasLevel
}

// NewHTTPError creates an HTTPError with the specified client-facing status and
// message. Use NewHTTPErrorWithCause to retain a diagnostic cause, and use
// WithFields or WithLogLevel to attach structured log context or override the
// router's default log level.
func NewHTTPError(statusCode int, message string) *HTTPError {
	return &HTTPError{
		StatusCode: statusCode,
		Message:    message,
	}
}

// NewHTTPErrorWithCause creates an HTTPError that retains an internal cause
// for logging and errors.Is/errors.As without exposing it to the client.
func NewHTTPErrorWithCause(statusCode int, message string, cause error) *HTTPError {
	return &HTTPError{
		StatusCode: statusCode,
		Message:    message,
		cause:      cause,
	}
}

// WithFields returns a copy of the HTTPError with additional structured log
// fields. Field values are snapshotted by value and later additions with the
// same key take precedence. Boundary-owned keys are discarded when logging.
func (e *HTTPError) WithFields(fields ...zap.Field) *HTTPError {
	if e == nil {
		return nil
	}
	clone := *e
	clone.fields = make([]zap.Field, 0, len(e.fields)+len(fields))
	clone.fields = append(clone.fields, e.fields...)
	clone.fields = append(clone.fields, fields...)
	return &clone
}

// WithLogLevel returns a copy of the HTTPError with an explicit boundary log
// level. This is intended for cases such as invariant violations whose
// operational severity differs from the default HTTP-status classification.
func (e *HTTPError) WithLogLevel(level zapcore.Level) *HTTPError {
	if e == nil {
		return nil
	}
	clone := *e
	clone.fields = slices.Clone(e.fields)
	clone.logLevel = level
	clone.hasLevel = true
	return &clone
}

var reservedHTTPErrorFieldKeys = map[string]struct{}{
	logkeys.BuildID:    {},
	logkeys.ConfigID:   {},
	logkeys.Error:      {},
	logkeys.Method:     {},
	logkeys.Path:       {},
	logkeys.StatusCode: {},
	logkeys.TraceID:    {},
}

// sanitizeHTTPErrorFields removes boundary-owned fields and duplicate keys.
// Walking from the end makes the outermost (most recently attached) context
// win without mutating the HTTPError's immutable field snapshot.
func sanitizeHTTPErrorFields(fields []zap.Field) []zap.Field {
	if len(fields) == 0 {
		return nil
	}
	seen := make(map[string]struct{}, len(fields))
	reversed := make([]zap.Field, 0, len(fields))
	for i := len(fields) - 1; i >= 0; i-- {
		field := fields[i]
		if _, reserved := reservedHTTPErrorFieldKeys[field.Key]; reserved {
			continue
		}
		if _, duplicate := seen[field.Key]; duplicate {
			continue
		}
		seen[field.Key] = struct{}{}
		reversed = append(reversed, field)
	}
	slices.Reverse(reversed)
	return reversed
}

// recoveryMiddleware is a middleware that recovers from panics in handlers.
// It logs the panic and returns a 500 Internal Server Error response if the
// response has not been started yet. If the panic occurred after the handler
// began writing, no second response is written (the partial response cannot
// be repaired) and the panic is only logged.
// This prevents the server from crashing when a handler panics.
func (r *Router[T, U]) recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		rw := &recoveryResponseWriter{ResponseWriter: w}
		defer func() {
			if rec := recover(); rec != nil {
				fields := append([]zap.Field{zap.Any(logkeys.Panic, rec)}, r.baseFields(req)...)
				fields = append(fields,
					zap.Int(logkeys.StatusCode, http.StatusInternalServerError),
					zap.String(logkeys.TraceID, r.errorTraceID(req)),
				)
				r.logger.Error("Panic recovered", fields...)

				if rw.wrote {
					// The handler already started writing; appending a JSON
					// error would corrupt the response and trigger a
					// superfluous WriteHeader. Log only.
					return
				}

				// Return a 500 Internal Server Error as JSON
				traceID := scontext.GetTraceIDFromContext[T, U](req.Context())
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

	if user, valid := r.dependencies.Authenticate(req.Context(), token); valid {
		id := r.dependencies.UserID(user)
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
				traceID := r.errorTraceID(req)
				fields := append(r.baseFields(req),
					zap.String(logkeys.RemoteAddr, req.RemoteAddr),
					zap.String(logkeys.Error, reason),
					zap.Int(logkeys.StatusCode, http.StatusUnauthorized),
					zap.String(logkeys.TraceID, traceID),
				)
				r.logger.Info("Authentication failed", fields...)
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
