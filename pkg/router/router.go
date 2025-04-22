// Package router provides a flexible and feature-rich HTTP routing framework.
// It supports middleware, sub-routers, generic handlers, and various configuration options.
package router

import (
	"bytes"
	"context"
	"encoding/json" // Added for JSON marshalling
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"sync/atomic" // Import atomic package
	"time"

	"github.com/Suhaibinator/SRouter/pkg/common"
	"github.com/Suhaibinator/SRouter/pkg/metrics"
	"github.com/Suhaibinator/SRouter/pkg/middleware" // Keep for middleware implementations like UberRateLimiter, IDGenerator, CreateTraceMiddleware
	"github.com/Suhaibinator/SRouter/pkg/scontext"   // Ensure scontext is imported
	"github.com/julien040/go-ternary"
	"github.com/julienschmidt/httprouter"
	"go.uber.org/zap"
)

// responseStateWriter wraps http.ResponseWriter to capture status code and buffer error responses.
type responseStateWriter struct {
	http.ResponseWriter
	statusCode    int
	wroteHeader   bool // Tracks if WriteHeader was called explicitly
	wroteBody     bool // Tracks if Write was called
	errorOccurred bool
	errorBody     []byte        // Buffer for pre-rendered error body
	successBody   *bytes.Buffer // Buffer for successful response body
	mu            sync.Mutex
}

// WriteHeader captures the status code and prevents multiple writes.
// It does NOT write to the underlying writer.
func (w *responseStateWriter) WriteHeader(statusCode int) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if !w.wroteHeader && !w.wroteBody { // Can only set status before writing body
		w.statusCode = statusCode
		w.wroteHeader = true
	}
}

// Write captures the successful response body.
// It does NOT write to the underlying writer.
func (w *responseStateWriter) Write(b []byte) (int, error) {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.errorOccurred {
		// Discard writes if an error was already set.
		return len(b), nil
	}
	// Initialize buffer if first write
	if w.successBody == nil {
		w.successBody = new(bytes.Buffer)
	}
	// Set implicit 200 OK if header wasn't explicitly set
	if !w.wroteHeader {
		w.statusCode = http.StatusOK
	}
	w.wroteBody = true // Mark that body has been written (or attempted)
	return w.successBody.Write(b)
}

// Flush calls the underlying Flush if available.
func (w *responseStateWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// setError prepares the writer to output a specific error response later.
// It captures the status code and the pre-rendered error body.
// It prevents further successful writes.
func (w *responseStateWriter) setError(statusCode int, body []byte) {
	w.mu.Lock()
	defer w.mu.Unlock()
	// Allow setting error even if Write was called (e.g., encode error)
	// but not if WriteHeader was already explicitly called with a success code?
	// Let's prioritize the error state.
	if !w.errorOccurred { // Prevent overwriting an earlier error? Or allow latest error? Let's allow latest.
		w.errorOccurred = true
		w.errorBody = body
		// Only set status if header wasn't explicitly written previously
		if !w.wroteHeader {
			w.statusCode = statusCode
		}
		// Clear any potentially buffered success body
		w.successBody = nil
	}
}

// getStatusCode returns the captured status code, defaulting to 200 if not set.
func (w *responseStateWriter) getStatusCode() int {
	w.mu.Lock()
	defer w.mu.Unlock()
	if w.statusCode == 0 { // Default if WriteHeader was never called
		return http.StatusOK
	}
	return w.statusCode
}

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
		metricsWriterPool: sync.Pool{
			New: func() any {
				// metricsResponseWriter might still be needed for metrics, keep for now
				return &metricsResponseWriter[T, U]{}
			},
		},
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
	if config.EnableMetrics {
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
				// Check if handler already started writing.
				// We need to signal the error state using the responseStateWriter.
				// Need to cast w back to *responseStateWriter.
				// Note: The wrappedW here is mutexResponseWriter, not responseStateWriter.
				// The actual writer passed down the chain is responseStateWriter (or metrics wrapping it).
				// We need access to the state writer instance.
				// Let's assume 'w' passed into the middleware func is the state writer (or wrapper).
				if stateW, ok := w.(*responseStateWriter); ok {
					// Prepare the error response body using the original error
					status, body := r.handleError(req, context.DeadlineExceeded, http.StatusRequestTimeout, "Request Timeout")
					// Set the error state. setError handles the lock and checks.
					stateW.setError(status, body)
				} else {
					// Fallback if casting fails (should not happen with current ServeHTTP)
					r.logger.Error("Timeout middleware could not access responseStateWriter, writing error directly")
					// This direct write bypasses other middleware.
					http.Error(w, "Request Timeout", http.StatusRequestTimeout)
				}
				// Unlock is handled by the defer in the outer scope of the select case.
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
// It handles HTTP requests by applying metrics and tracing if enabled,
// wrapping the response writer, and then delegating to the underlying httprouter.
// It handles writing the final error body if one was prepared.
func (r *Router[T, U]) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// Wrap the original ResponseWriter to capture state
	stateW := &responseStateWriter{ResponseWriter: w}

	// --- Original ServeHTTP logic starts here, using stateW ---
	var traceID string // Keep traceID for logging

	// Apply ClientIpMiddleware to the request (using original context)
	clientIP := extractClientIP(req, r.config.IPConfig)
	ctx := scontext.WithClientIP[T, U](req.Context(), clientIP) // Use scontext
	req = req.WithContext(ctx)

	// Apply metrics and tracing if enabled
	// Note: The metricsResponseWriter might need adjustment or replacement
	// if it conflicts with responseStateWriter. For now, let's assume
	// it can wrap the stateW or be integrated.
	// Let's simplify for now and use stateW directly, potentially losing metrics.
	// TODO: Re-integrate metrics capture properly with stateW.
	var finalWriter http.ResponseWriter = stateW // Use stateW as the base

	if r.config.EnableMetrics || r.config.TraceIDBufferSize > 0 {
		// Get a metricsResponseWriter from the pool
		mrw := r.metricsWriterPool.Get().(*metricsResponseWriter[T, U])

		// Initialize the writer with the current request data
		// IMPORTANT: Wrap the stateW, not the original w
		mrw.ResponseWriter = stateW
		mrw.statusCode = http.StatusOK // Initial status
		mrw.startTime = time.Now()
		mrw.request = req
		mrw.router = r
		mrw.bytesWritten = 0

		finalWriter = mrw // Use the metrics writer as the final writer passed down

		// Defer logging, metrics collection, and returning the writer to the pool
		defer func() {
			duration := time.Since(mrw.startTime)
			// Update status code from the state writer *before* logging/metrics
			mrw.statusCode = stateW.getStatusCode()

			// Get updated trace ID from context
			traceID = scontext.GetTraceIDFromRequest[T, U](req) // Use scontext
			ip, _ := scontext.GetClientIPFromRequest[T, U](req) // Use scontext

			// Log metrics (using mrw.statusCode which now reflects final status)
			if r.config.EnableTraceLogging {

				// Create log fields
				fields := []zap.Field{
					zap.String("method", req.Method),
					zap.String("path", req.URL.Path),
					zap.Int("status", mrw.statusCode),
					zap.Duration("duration", duration),
					zap.Int64("bytes", mrw.bytesWritten),
					zap.String("ip", ip),
				}

				// Add trace ID if enabled and present
				if r.config.TraceIDBufferSize > 0 && traceID != "" {
					fields = append(fields, zap.String("trace_id", traceID))
				}

				// Use Debug level for metrics to avoid log spam
				r.logger.Debug("Request metrics", fields...)

				// Log slow requests at Warn level
				if duration > 1*time.Second {
					// Create log fields
					fields := []zap.Field{
						zap.String("method", req.Method),
						zap.String("path", req.URL.Path),
						zap.Int("status", mrw.statusCode),
						zap.Duration("duration", duration),
						zap.String("ip", ip),
					}

					// Add trace ID if enabled and present
					if r.config.TraceIDBufferSize > 0 && traceID != "" {
						fields = append([]zap.Field{zap.String("trace_id", traceID)}, fields...)
					}

					r.logger.Warn("Slow request", fields...)
				}

				// Log errors at Error level
				if mrw.statusCode >= 500 {
					// Create log fields
					fields := []zap.Field{
						zap.String("method", req.Method),
						zap.String("path", req.URL.Path),
						zap.Int("status", mrw.statusCode),
						zap.Duration("duration", duration),
						zap.String("ip", ip),
					}

					// Add trace ID if enabled and present
					if r.config.TraceIDBufferSize > 0 && traceID != "" {
						fields = append([]zap.Field{zap.String("trace_id", traceID)}, fields...)
					}

					r.logger.Error("Server error", fields...)
				} else if mrw.statusCode >= 400 {
					// Create log fields
					fields := []zap.Field{
						zap.String("method", req.Method),
						zap.String("path", req.URL.Path),
						zap.Int("status", mrw.statusCode),
						zap.Duration("duration", duration),
						zap.String("ip", ip),
					}

					// Add trace ID if enabled and present
					if r.config.TraceIDBufferSize > 0 && traceID != "" {
						fields = append([]zap.Field{zap.String("trace_id", traceID)}, fields...)
					}

					r.logger.Warn("Client error", fields...)
				}
			}

			// Log tracing information
			if r.config.TraceIDBufferSize > 0 {
				// Create log fields
				fields := []zap.Field{
					zap.String("method", req.Method),
					zap.String("path", req.URL.Path),
					zap.String("ip", ip),
					zap.String("user_agent", req.UserAgent()),
					zap.Int("status", mrw.statusCode),
					zap.Duration("duration", duration),
				}

				// Add trace ID if enabled and present
				if r.config.TraceIDBufferSize > 0 && traceID != "" {
					fields = append([]zap.Field{zap.String("trace_id", traceID)}, fields...)
				}

				logFunc := ternary.If(r.config.TraceLoggingUseInfo, r.logger.Info, r.logger.Debug)
				logFunc("Request trace", fields...)
			}

			// Reset fields that might hold references to prevent memory leaks
			mrw.ResponseWriter = nil
			mrw.request = nil
			mrw.router = nil

			// Return the writer to the pool
			r.metricsWriterPool.Put(mrw)
		}()
	}
	// else: finalWriter remains stateW

	// Serve the request using the final writer (either stateW or mrw wrapping stateW)
	r.router.ServeHTTP(finalWriter, req)

	// --- Final response writing ---
	// After the handler and all middleware have run, write the response.
	stateW.mu.Lock() // Lock for final check and write

	finalStatusCode := stateW.statusCode
	if finalStatusCode == 0 { // Default to 200 if never set
		finalStatusCode = http.StatusOK
	}

	// Write header using the final status code
	stateW.ResponseWriter.WriteHeader(finalStatusCode)

	// Write the appropriate body
	if stateW.errorOccurred && stateW.errorBody != nil {
		stateW.ResponseWriter.Write(stateW.errorBody)
	} else if stateW.successBody != nil {
		stateW.ResponseWriter.Write(stateW.successBody.Bytes())
	}
	// else: No body was written (e.g., HEAD request, or handler didn't write)

	stateW.mu.Unlock()
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
// Determines the correct status code and message for an error, logs it,
// and prepares the JSON error body.
// Returns the final status code and the rendered JSON byte slice.
func (r *Router[T, U]) handleError(req *http.Request, err error, defaultStatusCode int, defaultMessage string) (int, []byte) {
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

	// Determine final status code and message based on error type
	finalStatusCode := defaultStatusCode
	finalMessage := defaultMessage
	var httpErr *HTTPError
	if errors.Is(err, context.DeadlineExceeded) {
		finalStatusCode = http.StatusRequestTimeout
		finalMessage = "Request Timeout"
		r.logger.Error("Request timed out (detected in handler)", fields...)
	} else if errors.As(err, &httpErr) {
		finalStatusCode = httpErr.StatusCode
		finalMessage = httpErr.Message
		// Log based on status code severity
		if finalStatusCode >= 500 {
			r.logger.Error(finalMessage, fields...)
		} else if finalStatusCode >= 400 {
			r.logger.Warn(finalMessage, fields...)
		}
	} else if err != nil && err.Error() == "http: request body too large" {
		finalStatusCode = http.StatusRequestEntityTooLarge
		finalMessage = "Request Entity Too Large"
		r.logger.Warn(finalMessage, fields...)
	} else {
		// Log generic internal server error for other non-nil errors
		if err != nil {
			r.logger.Error(defaultMessage, fields...)
		}
		// If err was nil, status/message remain defaults (e.g., for explicit 500)
	}

	// Prepare the JSON error body bytes
	bodyBytes, renderErr := r.renderJSONErrorBody(finalMessage, traceID)
	if renderErr != nil {
		// Log failure to render the error body itself
		r.logger.Error("Failed to render JSON error body",
			zap.Error(renderErr),
			zap.Int("original_status", finalStatusCode),
			zap.String("original_message", finalMessage),
			zap.String("trace_id", traceID),
		)
		// Fallback: return plain text error and internal server error status
		return http.StatusInternalServerError, []byte("Internal Server Error")
	}

	return finalStatusCode, bodyBytes
}

// renderJSONErrorBody renders the standard JSON error structure into a byte slice.
// It includes the trace ID if available and enabled.
func (r *Router[T, U]) renderJSONErrorBody(message string, traceID string) ([]byte, error) {
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

	// Marshal the JSON payload
	// Using json.Marshal is generally safe here as the structure is simple.
	return json.Marshal(errorPayload)
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
				// Prepare the error response body
				status, body := r.handleError(req, fmt.Errorf("panic: %v", rec), http.StatusInternalServerError, "Internal Server Error")

				// Attempt to set the error state on the response writer
				// Need to cast w, but this might be problematic if wrapped.
				// For now, we'll assume w is *responseStateWriter or compatible.
				// TODO: Find a cleaner way to pass the state writer or signal the error.
				if stateW, ok := w.(*responseStateWriter); ok {
					stateW.setError(status, body)
					// Headers like Content-Type will be set later if needed
				} else {
					// Fallback if casting fails (should not happen with current ServeHTTP)
					// This fallback writes directly, bypassing middleware response modification.
					r.logger.Error("Recovery middleware could not access responseStateWriter, writing error directly")
					w.Header().Set("Content-Type", "application/json; charset=utf-8")
					w.WriteHeader(status)
					w.Write(body)
				}
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
			// Prepare the error response body
			status, body := r.handleError(req, errors.New("no authorization header"), http.StatusUnauthorized, "Unauthorized")
			// Attempt to set the error state on the response writer
			if stateW, ok := w.(*responseStateWriter); ok {
				stateW.setError(status, body)
			} else {
				// Fallback
				r.logger.Error("Auth middleware could not access responseStateWriter, writing error directly")
				w.Header().Set("Content-Type", "application/json; charset=utf-8")
				w.WriteHeader(status)
				w.Write(body)
			}
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
		// Prepare the error response body
		status, body := r.handleError(req, errors.New("invalid token"), http.StatusUnauthorized, "Unauthorized")
		// Attempt to set the error state on the response writer
		if stateW, ok := w.(*responseStateWriter); ok {
			stateW.setError(status, body)
		} else {
			// Fallback
			r.logger.Error("Auth middleware could not access responseStateWriter, writing error directly")
			w.Header().Set("Content-Type", "application/json; charset=utf-8")
			w.WriteHeader(status)
			w.Write(body)
		}
		// Return after setting error state (no need to call next.ServeHTTP)
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
