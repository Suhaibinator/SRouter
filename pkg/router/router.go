// Package router provides a flexible and feature-rich HTTP routing framework.
// It supports middleware, sub-routers, generic handlers, and various configuration options.
package router

import (
	"context"
	"encoding/json" // Added for JSON marshalling
	"errors"
	"fmt"
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

// RegisterSubRouterWithSubRouter registers a nested SubRouter with a parent SubRouter
// This is a helper function that adds a SubRouter to the parent SubRouter's SubRouters field
func RegisterSubRouterWithSubRouter(parent *SubRouterConfig, child SubRouterConfig) {
	// Add the child SubRouter to the parent's SubRouters field
	parent.SubRouters = append(parent.SubRouters, child)
}

// NewRouter creates a new Router with the given configuration.
// It initializes the underlying httprouter, sets up logging, and registers routes from sub-routers.
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
		// Create trace middleware using the router's generator, providing the router's T and U types
		traceMW := middleware.CreateTraceMiddleware[T, U](r.traceIDGenerator)
		// Add trace middleware as the first middleware (before any other middleware)
		r.middlewares = append([]common.Middleware{traceMW}, r.middlewares...)
	}

	// Add metrics middleware if configured
	if config.MetricsConfig != nil {
		var metricsMiddleware common.Middleware

		// Use the MetricsConfig
		if config.MetricsConfig != nil {
			// Check if the collector is a metrics registry
			if registry, ok := config.MetricsConfig.Collector.(metrics.MetricsRegistry); ok {
				// Create the generic middleware implementation using the router's T and U types
				metricsMiddlewareImpl := metrics.NewMetricsMiddleware[T, U](registry, metrics.MetricsMiddlewareConfig{
					EnableLatency:    config.MetricsConfig.EnableLatency,
					EnableThroughput: config.MetricsConfig.EnableThroughput,
					EnableQPS:        config.MetricsConfig.EnableQPS,
					EnableErrors:     config.MetricsConfig.EnableErrors,
					DefaultTags: metrics.Tags{
						"service": config.MetricsConfig.Namespace, // Assuming Namespace is intended for service tag
					},
				})
				// The middleware instance itself is now generic, but its Handler method
				// returns a standard http.Handler, so the adapter function remains the same.
				metricsMiddleware = func(next http.Handler) http.Handler {
					// Use the ServiceName from the config for the application name
					return metricsMiddlewareImpl.Handler(config.ServiceName, next)
				}
			}
		}

		if metricsMiddleware != nil {
			r.middlewares = append(r.middlewares, metricsMiddleware)
		}
	}

	// Register routes from sub-routers
	for _, sr := range config.SubRouters {
		r.registerSubRouter(sr)
	}

	return r
}

// registerSubRouter registers all routes and nested sub-routers defined in a SubRouterConfig.
// It applies the sub-router's path prefix, overrides, and middlewares.
func (r *Router[T, U]) registerSubRouter(sr SubRouterConfig) {
	// Register routes defined in this sub-router
	for _, routeDefinition := range sr.Routes {
		switch route := routeDefinition.(type) {
		case RouteConfigBase:
			// Handle standard RouteConfigBase
			fullPath := sr.PathPrefix + route.Path

			// Get effective settings considering overrides
			timeout := r.getEffectiveTimeout(route.Timeout, sr.TimeoutOverride)
			maxBodySize := r.getEffectiveMaxBodySize(route.MaxBodySize, sr.MaxBodySizeOverride)
			rateLimit := r.getEffectiveRateLimit(route.RateLimit, sr.RateLimitOverride)
			authLevel := route.AuthLevel // Use route-specific first
			if authLevel == nil {
				authLevel = sr.AuthLevel // Fallback to sub-router default
			}

			// Combine middlewares: sub-router + route-specific
			allMiddlewares := make([]common.Middleware, 0, len(sr.Middlewares)+len(route.Middlewares))
			allMiddlewares = append(allMiddlewares, sr.Middlewares...)
			allMiddlewares = append(allMiddlewares, route.Middlewares...)

			// Create a handler with all middlewares applied (global middlewares are added inside wrapHandler)
			handler := r.wrapHandler(route.Handler, authLevel, timeout, maxBodySize, rateLimit, allMiddlewares)

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
		// Create a new sub-router with the combined path prefix
		nestedSRWithPrefix := nestedSR
		nestedSRWithPrefix.PathPrefix = sr.PathPrefix + nestedSR.PathPrefix

		// Register the nested sub-router
		r.registerSubRouter(nestedSRWithPrefix)
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
// It applies authentication, timeout, body size limits, rate limiting, and other middleware
// to create a complete request processing pipeline.
func (r *Router[T, U]) wrapHandler(handler http.HandlerFunc, authLevel *AuthLevel, timeout time.Duration, maxBodySize int64, rateLimit *common.RateLimitConfig[T, U], middlewares []Middleware) http.Handler { // Use common.RateLimitConfig
	// Create a base handler that only handles shutdown check and body size limit directly
	// Timeout is now handled by timeoutMiddleware setting the context.
	h := http.Handler(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// Shutdown Check
		r.wg.Add(1)
		defer r.wg.Done()
		r.shutdownMu.RLock()
		isShutdown := r.shutdown
		r.shutdownMu.RUnlock()
		if isShutdown {
			http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
			return
		}

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

	// 1. Recovery (Innermost before handler)
	chain = chain.Append(r.recoveryMiddleware)

	// 2. Authentication (Runs early)
	if authLevel != nil {
		switch *authLevel {
		case AuthRequired:
			chain = chain.Append(r.authRequiredMiddleware)
		case AuthOptional:
			chain = chain.Append(r.authOptionalMiddleware)
		}
	}

	// 3. Rate Limiting
	if rateLimit != nil {
		// Ensure the rate limiter implementation is compatible
		// Since r.rateLimiter is common.RateLimiter, this should work directly
		chain = chain.Append(middleware.RateLimit(rateLimit, r.rateLimiter, r.logger))
	}

	// 4. Route-Specific Middlewares
	chain = chain.Append(middlewares...)

	// 5. Global Middlewares (defined in RouterConfig)
	chain = chain.Append(r.middlewares...) // Note: This now includes ClientIPMiddleware added in NewRouter

	// 6. Timeout Handling (Sets context deadline)
	if timeout > 0 {
		chain = chain.Append(r.timeoutMiddleware(timeout))
	}

	// 7. Body Size Limit (Applied within the base handler 'h' now)
	// No separate middleware needed here anymore.

	// 8. Shutdown Handling (Applied within the base handler 'h' now)
	// No separate middleware needed here anymore.

	// Apply the chain to the base handler 'h'
	return chain.Then(h)
}

// timeoutMiddleware creates a middleware that handles request timeouts.
// It sets a context deadline and attempts to write a timeout error if the handler exceeds it,
// but only if the handler hasn't already started writing the response.
func (r *Router[T, U]) timeoutMiddleware(timeout time.Duration) Middleware {
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
				traceID := scontext.GetTraceIDFromRequest[T, U](req) // Use scontext
				fields := []zap.Field{
					zap.String("method", req.Method),
					zap.String("path", req.URL.Path),
					zap.Duration("timeout", timeout),
					zap.String("client_ip", req.RemoteAddr),
				}
				if traceID != "" {
					fields = append(fields, zap.String("trace_id", traceID))
				}
				r.logger.Error("Request timed out", fields...)

				// Acquire lock to safely check and potentially write timeout response.
				wrappedW.mu.Lock()
				// Check if handler already started writing. Use Swap for atomic check-and-set.
				if !wrappedW.wroteHeader.Swap(true) {
					// Handler hasn't written yet, we can write the timeout error.
					// Hold the lock while writing headers and body for timeout.
					// Use the new JSON error writer, passing the request
					r.writeJSONError(wrappedW.ResponseWriter, req, http.StatusRequestTimeout, "Request Timeout", traceID)
				}
				// If wroteHeader was already true, handler won the race, do nothing here.
				// Unlock should happen regardless of whether we wrote the error or not.
				wrappedW.mu.Unlock()
				return
			}
		})
	}
}

// findSubRouterConfig recursively searches for a SubRouterConfig matching the target prefix.
// Note: This performs an exact match on the prefix. More complex matching might be needed
// if overlapping prefixes or inheritance are desired.
func findSubRouterConfig(subRouters []SubRouterConfig, targetPrefix string) (*SubRouterConfig, bool) {
	for i := range subRouters {
		sr := &subRouters[i] // Use pointer for direct access
		if sr.PathPrefix == targetPrefix {
			return sr, true
		}
		// Check nested sub-routers recursively
		if foundSR, found := findSubRouterConfig(sr.SubRouters, targetPrefix); found {
			return foundSR, true
		}
	}
	return nil, false
}

// RegisterGenericRouteOnSubRouter registers a generic route intended to be part of a sub-router.
// It finds the SubRouterConfig matching the provided pathPrefix, applies relevant overrides
// and middleware, prefixes the route's path, and then registers it using RegisterGenericRoute.
// This function should be called *after* NewRouter has been called.
func RegisterGenericRouteOnSubRouter[Req any, Resp any, UserID comparable, User any](
	r *Router[UserID, User],
	pathPrefix string,
	route RouteConfig[Req, Resp],
) error {
	// Find the sub-router config matching the prefix
	sr, found := findSubRouterConfig(r.config.SubRouters, pathPrefix)
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
	if sr != nil {
		subRouterTimeout = sr.TimeoutOverride
		subRouterMaxBodySize = sr.MaxBodySizeOverride
		subRouterRateLimit = sr.RateLimitOverride // This is already common.RateLimitConfig[any, any]
		subRouterMiddlewares = sr.Middlewares
	}

	// Create a new route config instance to avoid modifying the original
	finalRouteConfig := route

	// Prefix the path
	finalRouteConfig.Path = pathPrefix + route.Path

	// Combine middleware: global + sub-router + route-specific
	allMiddlewares := make([]common.Middleware, 0, len(r.middlewares)+len(subRouterMiddlewares)+len(route.Middlewares))
	allMiddlewares = append(allMiddlewares, r.middlewares...)        // Global first
	allMiddlewares = append(allMiddlewares, subRouterMiddlewares...) // Then sub-router
	allMiddlewares = append(allMiddlewares, route.Middlewares...)    // Then route-specific
	finalRouteConfig.Middlewares = allMiddlewares                    // Overwrite middlewares in the config passed down

	// Get effective timeout, max body size, rate limit considering overrides
	effectiveTimeout := r.getEffectiveTimeout(route.Timeout, subRouterTimeout)
	effectiveMaxBodySize := r.getEffectiveMaxBodySize(route.MaxBodySize, subRouterMaxBodySize)
	effectiveRateLimit := r.getEffectiveRateLimit(route.RateLimit, subRouterRateLimit) // This returns *common.RateLimitConfig[UserID, User]

	// Call the underlying generic registration function with the modified config
	RegisterGenericRoute(r, finalRouteConfig, effectiveTimeout, effectiveMaxBodySize, effectiveRateLimit)

	return nil
}

// ServeHTTP implements the http.Handler interface.
// It handles HTTP requests by applying CORS, client IP extraction, metrics, tracing,
// and then delegating to the underlying httprouter.
func (r *Router[T, U]) ServeHTTP(w http.ResponseWriter, req *http.Request) {
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
	req = req.WithContext(ctx)

	// Apply metrics and tracing if enabled
	if r.config.TraceIDBufferSize > 0 {
		// Get a metricsResponseWriter from the pool
		mrw := r.metricsWriterPool.Get().(*metricsResponseWriter[T, U])

		// Initialize the writer with the current request data
		mrw.ResponseWriter = w
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
			traceID := scontext.GetTraceIDFromRequest[T, U](req)
			ip, _ := scontext.GetClientIPFromRequest[T, U](req)

			// 2) Build unified fields - the UNION of all previously separate log fields
			fields := []zap.Field{
				zap.String("method", req.Method),
				zap.String("path", req.URL.Path),
				zap.Int("status", mrw.statusCode),
				zap.Duration("duration", duration),
				zap.Int64("bytes", mrw.bytesWritten),
				zap.String("ip", ip),
				zap.String("user_agent", req.UserAgent()),
			}

			// Add trace ID if enabled and present
			if traceID != "" {
				fields = append(fields, zap.String("trace_id", traceID))
			}

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
			mrw.ResponseWriter = nil
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
			// We also need to store the *lack* of allowance in the context.
			ctx = scontext.WithCORSInfo[T, U](ctx, "", false) // Store empty origin, false credentials
			req = req.WithContext(ctx)
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

// metricsResponseWriter is a wrapper around http.ResponseWriter that captures metrics.
// It tracks the status code, bytes written, and timing information for each response.
type metricsResponseWriter[T comparable, U any] struct {
	http.ResponseWriter
	statusCode   int
	bytesWritten int64
	startTime    time.Time
	request      *http.Request
	router       *Router[T, U]
}

// WriteHeader captures the status code and calls the underlying ResponseWriter.WriteHeader.
func (rw *metricsResponseWriter[T, U]) WriteHeader(statusCode int) {
	rw.statusCode = statusCode
	rw.ResponseWriter.WriteHeader(statusCode)
}

// Write captures the number of bytes written and calls the underlying ResponseWriter.Write.
func (rw *metricsResponseWriter[T, U]) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.bytesWritten += int64(n)
	return n, err
}

// Flush calls the underlying ResponseWriter.Flush if it implements http.Flusher.
func (rw *metricsResponseWriter[T, U]) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
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
// It considers route-specific, sub-router, and global timeout settings in that order of precedence.
func (r *Router[T, U]) getEffectiveTimeout(routeTimeout, subRouterTimeout time.Duration) time.Duration {
	if routeTimeout > 0 {
		return routeTimeout
	}
	if subRouterTimeout > 0 {
		return subRouterTimeout
	}
	return r.config.GlobalTimeout
}

// getEffectiveMaxBodySize returns the effective max body size for a route.
// It considers route-specific, sub-router, and global max body size settings in that order of precedence.
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
// It considers route-specific, sub-router, and global rate limit settings in that order of precedence.
func (r *Router[T, U]) getEffectiveRateLimit(routeRateLimit, subRouterRateLimit *common.RateLimitConfig[any, any]) *common.RateLimitConfig[T, U] { // Use common types
	// Convert the rate limit config to the correct type
	convertConfig := func(config *common.RateLimitConfig[any, any]) *common.RateLimitConfig[T, U] { // Use common types
		if config == nil {
			return nil
		}

		// Create a new config with the correct type parameters
		return &common.RateLimitConfig[T, U]{ // Use common.RateLimitConfig
			BucketName:      config.BucketName,
			Limit:           config.Limit,
			Window:          config.Window,
			Strategy:        config.Strategy,
			UserIDFromUser:  nil, // These will need to be set by the caller if needed
			UserIDToString:  nil, // These will need to be set by the caller if needed
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

// handleError handles an error by logging it and returning an appropriate HTTP response.
// It checks if the error is a specific HTTPError and uses its status code and message if available.
// It also checks for context deadline exceeded errors.
func (r *Router[T, U]) handleError(w http.ResponseWriter, req *http.Request, err error, statusCode int, message string) {
	// Get trace ID from context
	traceID := scontext.GetTraceIDFromRequest[T, U](req) // Use scontext

	// Create log fields
	fields := []zap.Field{
		zap.Error(err),
		zap.String("method", req.Method),
		zap.String("path", req.URL.Path),
	}

	// Add trace ID if enabled and present
	if r.config.TraceIDBufferSize > 0 && traceID != "" {
		fields = append([]zap.Field{zap.String("trace_id", traceID)}, fields...)
	}

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
	} else if err != nil && err.Error() == "http: request body too large" {
		// Specifically handle MaxBytesReader error
		statusCode = http.StatusRequestEntityTooLarge
		message = "Request Entity Too Large"
		r.logger.Warn(message, fields...) // Log as Warn for client error
	} else {
		// Log generic internal server error
		r.logger.Error(message, fields...)
	}

	// Return the error response as JSON
	r.writeJSONError(w, req, statusCode, message, traceID) // Pass req
}

// writeJSONError writes a JSON error response to the client.
// It sets the Content-Type header to application/json and writes the status code.
// It includes the trace ID in the JSON payload if available and enabled.
// It also adds CORS headers based on information stored in the context by the CORS middleware.
func (r *Router[T, U]) writeJSONError(w http.ResponseWriter, req *http.Request, statusCode int, message string, traceID string) { // Add req parameter
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
	errorPayload := map[string]interface{}{
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
// It logs the panic and returns a 500 Internal Server Error response.
// This prevents the server from crashing when a handler panics.
func (r *Router[T, U]) recoveryMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				// Get trace ID from context
				traceID := scontext.GetTraceIDFromRequest[T, U](req) // Use scontext

				// Create log fields
				fields := []zap.Field{
					zap.Any("panic", rec),
					zap.String("method", req.Method),
					zap.String("path", req.URL.Path),
				}

				// Add trace ID if enabled and present
				if r.config.TraceIDBufferSize > 0 && traceID != "" {
					fields = append([]zap.Field{zap.String("trace_id", traceID)}, fields...)
				}

				// Log the panic
				r.logger.Error("Panic recovered", fields...)

				// Return a 500 Internal Server Error
				// Return a 500 Internal Server Error as JSON
				// We attempt to write the JSON error. If headers were already written,
				// writeJSONError might log an error, but we can't do much more here.
				r.writeJSONError(w, req, http.StatusInternalServerError, "Internal Server Error", traceID) // Pass req
			}
		}()

		next.ServeHTTP(w, req)
	})
}

// authRequiredMiddleware is a middleware that requires authentication for a request.
// If authentication fails, it returns a 401 Unauthorized response.
// It uses the middleware.AuthenticationWithUser function with a configurable authentication function.
func (r *Router[T, U]) authRequiredMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodOptions {
			// Allow preflight requests without authentication
			next.ServeHTTP(w, req)
			return
		}
		// Declare traceID variable to be used throughout the function
		var traceID string
		// Check for the presence of an Authorization header
		authHeader := req.Header.Get("Authorization")
		if authHeader == "" {
			// Get updated trace ID from context
			traceID = scontext.GetTraceIDFromRequest[T, U](req) // Use scontext

			// Create log fields
			fields := []zap.Field{
				zap.String("method", req.Method),
				zap.String("path", req.URL.Path),
				zap.String("remote_addr", req.RemoteAddr),
				zap.String("error", "no authorization header"),
			}

			// Add trace ID if enabled and present
			if r.config.TraceIDBufferSize > 0 && traceID != "" {
				fields = append([]zap.Field{zap.String("trace_id", traceID)}, fields...)
			}

			// Log that authentication failed
			r.logger.Warn("Authentication failed", fields...)
			r.writeJSONError(w, req, http.StatusUnauthorized, "Unauthorized", traceID) // Pass req
			return
		}

		// Extract the token from the Authorization header
		token := strings.TrimPrefix(authHeader, "Bearer ")

		// Try to authenticate using the authFunction
		if user, valid := r.authFunction(req.Context(), token); valid {
			id := r.getUserIdFromUser(user)
			// Add the user ID to the request context using scontext package functions
			// First get the trace ID so we can preserve it
			traceID = scontext.GetTraceIDFromRequest[T, U](req) // Use scontext

			// Add the user ID to the context
			ctx := scontext.WithUserID[T, U](req.Context(), id) // Use scontext

			// If there was a trace ID, make sure it's preserved
			if traceID != "" {
				ctx = scontext.WithTraceID[T, U](ctx, traceID) // Use scontext directly
			}
			req = req.WithContext(ctx)

			// Get updated trace ID from context
			traceID = scontext.GetTraceIDFromRequest[T, U](req) // Use scontext

			// Create log fields
			fields := []zap.Field{
				zap.String("method", req.Method),
				zap.String("path", req.URL.Path),
			}

			// Add trace ID if enabled and present
			if r.config.TraceIDBufferSize > 0 && traceID != "" {
				fields = append([]zap.Field{zap.String("trace_id", traceID)}, fields...)
			}

			// Log that authentication was successful
			r.logger.Debug("Authentication successful", fields...)
			next.ServeHTTP(w, req)
			return
		}

		// Get trace ID from context
		traceID = scontext.GetTraceIDFromRequest[T, U](req) // Use scontext

		// Create log fields
		fields := []zap.Field{
			zap.String("method", req.Method),
			zap.String("path", req.URL.Path),
			zap.String("remote_addr", req.RemoteAddr),
			zap.String("error", "invalid token"),
		}

		// Add trace ID if enabled and present
		if r.config.TraceIDBufferSize > 0 && traceID != "" {
			fields = append([]zap.Field{zap.String("trace_id", traceID)}, fields...)
		}

		// Log that authentication failed
		r.logger.Warn("Authentication failed", fields...)
		r.writeJSONError(w, req, http.StatusUnauthorized, "Unauthorized", traceID) // Pass req
	})
}

// authOptionalMiddleware is a middleware that attempts authentication for a request,
// but allows the request to proceed even if authentication fails.
// It tries to authenticate the request and adds the user ID to the context if successful,
// but allows the request to proceed even if authentication fails.
func (r *Router[T, U]) authOptionalMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method == http.MethodOptions {
			// Allow preflight requests without authentication
			next.ServeHTTP(w, req)
			return
		}
		// Try to authenticate the request
		authHeader := req.Header.Get("Authorization")
		if authHeader != "" {
			// Extract the token from the Authorization header
			token := strings.TrimPrefix(authHeader, "Bearer ")

			// Try to authenticate using the authFunction
			if user, valid := r.authFunction(req.Context(), token); valid {
				id := r.getUserIdFromUser(user)
				// Add the user ID to the request context using scontext package functions
				ctx := scontext.WithUserID[T, U](req.Context(), id) // Use scontext
				if r.config.AddUserObjectToCtx {
					ctx = scontext.WithUser[T](ctx, user) // Use scontext
				}
				req = req.WithContext(ctx)
				// Get trace ID from context
				traceID := scontext.GetTraceIDFromRequest[T, U](req) // Use scontext

				// Create log fields
				fields := []zap.Field{
					zap.String("method", req.Method),
					zap.String("path", req.URL.Path),
				}

				// Add trace ID if enabled and present
				if r.config.TraceIDBufferSize > 0 && traceID != "" {
					fields = append([]zap.Field{zap.String("trace_id", traceID)}, fields...)
				}

				// Log that authentication was successful
				r.logger.Debug("Authentication successful", fields...)
			}
		}

		// Call the next handler regardless of authentication result
		next.ServeHTTP(w, req)
	})
}

// responseWriter is a wrapper around http.ResponseWriter that captures the status code.
// This allows middleware to inspect the status code after the handler has completed.
type responseWriter struct {
	http.ResponseWriter
	statusCode int
}

// WriteHeader captures the status code and calls the underlying ResponseWriter.WriteHeader.
func (rw *responseWriter) WriteHeader(statusCode int) {
	rw.statusCode = statusCode
	rw.ResponseWriter.WriteHeader(statusCode)
}

// Write calls the underlying ResponseWriter.Write.
func (rw *responseWriter) Write(b []byte) (int, error) {
	return rw.ResponseWriter.Write(b)
}

// Flush calls the underlying ResponseWriter.Flush if it implements http.Flusher.
func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// mutexResponseWriter is a wrapper around http.ResponseWriter that uses a mutex to protect access
// and tracks if headers/body have been written.
type mutexResponseWriter struct {
	http.ResponseWriter
	mu          *sync.Mutex
	wroteHeader atomic.Bool // Tracks if WriteHeader or Write has been called
}

// Header acquires the mutex and returns the underlying Header map.
func (rw *mutexResponseWriter) Header() http.Header {
	rw.mu.Lock()
	defer rw.mu.Unlock()
	return rw.ResponseWriter.Header()
}

// WriteHeader acquires the mutex, marks headers as written, and calls the underlying ResponseWriter.WriteHeader.
func (rw *mutexResponseWriter) WriteHeader(statusCode int) {
	rw.mu.Lock()
	defer rw.mu.Unlock()
	if !rw.wroteHeader.Swap(true) { // Atomically set flag and check previous value
		rw.ResponseWriter.WriteHeader(statusCode)
	}
	// If header was already written, do nothing (consistent with http.ResponseWriter behavior)
}

// Write acquires the mutex, marks headers/body as written, and calls the underlying ResponseWriter.Write.
func (rw *mutexResponseWriter) Write(b []byte) (int, error) {
	rw.mu.Lock()
	defer rw.mu.Unlock()
	rw.wroteHeader.Store(true) // Mark as written (headers might be implicitly written here)
	return rw.ResponseWriter.Write(b)
}

// Flush acquires the mutex and calls the underlying ResponseWriter.Flush if it implements http.Flusher.
func (rw *mutexResponseWriter) Flush() {
	rw.mu.Lock()
	defer rw.mu.Unlock()
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
