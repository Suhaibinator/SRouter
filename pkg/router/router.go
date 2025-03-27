// Package router provides a flexible and feature-rich HTTP routing framework.
// It supports middleware, sub-routers, generic handlers, and various configuration options.
package router

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/common"
	"github.com/Suhaibinator/SRouter/pkg/metrics"
	"github.com/Suhaibinator/SRouter/pkg/middleware"
	"github.com/julienschmidt/httprouter"
	"go.uber.org/zap"
)

// Router is the main router struct that implements http.Handler.
// It provides routing, middleware support, graceful shutdown, and other features.
type Router[T comparable, U any] struct {
	config            RouterConfig
	router            *httprouter.Router
	logger            *zap.Logger
	middlewares       []common.Middleware
	authFunction      func(context.Context, string) (U, bool)
	getUserIdFromUser func(U) T
	rateLimiter       middleware.RateLimiter
	wg                sync.WaitGroup
	shutdown          bool
	shutdownMu        sync.RWMutex
	metricsWriterPool sync.Pool // Pool for reusing metricsResponseWriter objects
}

// RegisterSubRouterWithSubRouter registers a nested SubRouter with a parent SubRouter
// This is a helper function that adds a SubRouter to the parent SubRouter's SubRouters field
func RegisterSubRouterWithSubRouter(parent *SubRouterConfig, child SubRouterConfig) {
	// Add the child SubRouter to the parent's SubRouters field
	parent.SubRouters = append(parent.SubRouters, child)
}

// NewRouter creates a new Router with the given configuration.
// It initializes the underlying httprouter, sets up logging, and registers routes from sub-routers.
func NewRouter[T comparable, U any](config RouterConfig, authFunction func(context.Context, string) (U, bool), userIdFromuserFunction func(U) T) *Router[T, U] {
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
		logger:            logger,
		authFunction:      authFunction,
		getUserIdFromUser: userIdFromuserFunction,
		middlewares:       config.Middlewares,
		rateLimiter:       rateLimiter,
		metricsWriterPool: sync.Pool{
			New: func() interface{} {
				return &metricsResponseWriter[T, U]{}
			},
		},
	}

	// Add IP middleware as the first middleware (before any other middleware)
	ipConfig := config.IPConfig
	if ipConfig == nil {
		ipConfig = middleware.DefaultIPConfig()
	}
	r.middlewares = append([]common.Middleware{middleware.ClientIPMiddleware(ipConfig)}, r.middlewares...)

	// Add metrics middleware if configured
	if config.EnableMetrics {
		var metricsMiddleware common.Middleware

		// Use the MetricsConfig
		if config.MetricsConfig != nil {
			// Check if the collector is a metrics registry
			if registry, ok := config.MetricsConfig.Collector.(metrics.MetricsRegistry); ok {
				// Create a middleware using the registry
				metricsMiddlewareImpl := metrics.NewMetricsMiddleware(registry, metrics.MetricsMiddlewareConfig{
					EnableLatency:    config.MetricsConfig.EnableLatency,
					EnableThroughput: config.MetricsConfig.EnableThroughput,
					EnableQPS:        config.MetricsConfig.EnableQPS,
					EnableErrors:     config.MetricsConfig.EnableErrors,
					DefaultTags: metrics.Tags{
						"service": config.MetricsConfig.Namespace,
					},
				})
				// Create an adapter function that converts the middleware.Handler method to a common.Middleware
				metricsMiddleware = func(next http.Handler) http.Handler {
					return metricsMiddlewareImpl.Handler("", next)
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

// registerSubRouter registers all routes in a sub-router.
// It applies the sub-router's path prefix to all routes and registers them with the router.
func (r *Router[T, U]) registerSubRouter(sr SubRouterConfig) {
	// Register regular routes
	for _, route := range sr.Routes {
		// Create a full path by combining the sub-router prefix with the route path
		fullPath := sr.PathPrefix + route.Path

		// Get effective timeout, max body size, and rate limit for this route
		timeout := r.getEffectiveTimeout(route.Timeout, sr.TimeoutOverride)
		maxBodySize := r.getEffectiveMaxBodySize(route.MaxBodySize, sr.MaxBodySizeOverride)
		rateLimit := r.getEffectiveRateLimit(route.RateLimit, sr.RateLimitOverride)

		// Create a handler with all middlewares applied
		handler := r.wrapHandler(route.Handler, route.AuthLevel, timeout, maxBodySize, rateLimit, append(sr.Middlewares, route.Middlewares...))

		// Register the route with httprouter
		for _, method := range route.Methods {
			r.router.Handle(method, fullPath, r.convertToHTTPRouterHandle(handler))
		}
	}

	// Register nested sub-routers if any
	for _, nestedSR := range sr.SubRouters {
		// Create a new sub-router with the combined path prefix
		nestedSRWithPrefix := nestedSR
		nestedSRWithPrefix.PathPrefix = sr.PathPrefix + nestedSR.PathPrefix

		// Register the nested sub-router
		r.registerSubRouter(nestedSRWithPrefix)
	}
}

// convertToHTTPRouterHandle converts an http.Handler to an httprouter.Handle.
// It stores the route parameters in the request context so they can be accessed by handlers.
func (r *Router[T, U]) convertToHTTPRouterHandle(handler http.Handler) httprouter.Handle {
	return func(w http.ResponseWriter, req *http.Request, ps httprouter.Params) {
		// Store the params in the request context
		ctx := context.WithValue(req.Context(), ParamsKey, ps)
		req = req.WithContext(ctx)

		// Call the handler
		handler.ServeHTTP(w, req)
	}
}

// wrapHandler wraps a handler with all the necessary middleware.
// It applies authentication, timeout, body size limits, rate limiting, and other middleware
// to create a complete request processing pipeline.
func (r *Router[T, U]) wrapHandler(handler http.HandlerFunc, authLevel AuthLevel, timeout time.Duration, maxBodySize int64, rateLimit *middleware.RateLimitConfig[T, U], middlewares []Middleware) http.Handler {
	// Create a handler that applies all the router's functionality
	h := http.Handler(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// First add to the wait group before checking shutdown status
		r.wg.Add(1)

		// Then check if the router is shutting down
		r.shutdownMu.RLock()
		isShutdown := r.shutdown
		r.shutdownMu.RUnlock()

		if isShutdown {
			// If shutting down, decrement the wait group and return error
			r.wg.Done()
			http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
			return
		}

		// Process the request and ensure wg.Done() is called when finished
		defer r.wg.Done()

		// Apply body size limit
		if maxBodySize > 0 {
			req.Body = http.MaxBytesReader(w, req.Body, maxBodySize)
		}

		// Apply timeout
		if timeout > 0 {
			ctx, cancel := context.WithTimeout(req.Context(), timeout)
			defer cancel()
			req = req.WithContext(ctx)

			// Create a mutex to protect access to the response writer
			var wMutex sync.Mutex

			// Create a wrapped response writer that uses the mutex
			wrappedW := &mutexResponseWriter{
				ResponseWriter: w,
				mu:             &wMutex,
			}

			// Use a channel to signal when the handler is done
			done := make(chan struct{})
			go func() {
				handler(wrappedW, req)
				close(done)
			}()

			select {
			case <-done:
				// Handler finished normally
				return
			case <-ctx.Done():
				// Timeout occurred
				r.logger.Error("Request timed out",
					zap.String("method", req.Method),
					zap.String("path", req.URL.Path),
					zap.Duration("timeout", timeout),
					zap.String("client_ip", req.RemoteAddr),
				)

				// Lock the mutex before writing to the response
				wMutex.Lock()
				http.Error(w, "Request Timeout", http.StatusRequestTimeout)
				wMutex.Unlock()
				return
			}
		} else {
			// No timeout, just call the handler
			handler(w, req)
		}
	})) // End of the base http.HandlerFunc

	// Build the middleware chain
	chain := common.NewMiddlewareChain()

	// Append middleware in order of execution (outermost first)
	// Note: Assuming Append adds to the end of the chain, meaning it runs *earlier* in the request flow.
	// If Append adds to the start (runs later), reverse the order.

	// 1. Recovery (Innermost, runs last before handler panic, but added first to chain.Then)
	chain = chain.Append(r.recoveryMiddleware)

	// 2. Authentication (Runs early)
	switch authLevel {
	case AuthRequired:
		chain = chain.Append(r.authRequiredMiddleware)
	case AuthOptional:
		chain = chain.Append(r.authOptionalMiddleware)
	}

	// 3. Rate Limiting
	if rateLimit != nil {
		// Assuming RateLimit returns a Middleware
		chain = chain.Append(middleware.RateLimit(rateLimit, r.rateLimiter, r.logger))
	}

	// 4. Route-Specific Middlewares
	chain = chain.Append(middlewares...)

	// 5. Global Middlewares
	chain = chain.Append(r.middlewares...)

	// 6. Timeout Handling
	if timeout > 0 {
		chain = chain.Append(r.timeoutMiddleware(timeout)) // Use router method
	}

	// 7. Body Size Limit
	if maxBodySize > 0 {
		chain = chain.Append(bodyLimitMiddleware(maxBodySize)) // Use standalone function
	}

	// 8. Shutdown Handling (Runs very early, just after recovery)
	chain = chain.Append(r.shutdownMiddleware()) // Use router method

	// Apply the chain to the original handler 'h'
	return chain.Then(h)
}

// shutdownMiddleware creates a middleware that handles graceful shutdown checks.
func (r *Router[T, U]) shutdownMiddleware() Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			r.wg.Add(1)
			defer r.wg.Done()

			r.shutdownMu.RLock()
			isShutdown := r.shutdown
			r.shutdownMu.RUnlock()

			if isShutdown {
				http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
				return
			}
			next.ServeHTTP(w, req)
		})
	}
}

// bodyLimitMiddleware creates a middleware that applies request body size limits.
func bodyLimitMiddleware(maxBodySize int64) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			// Apply body size limit if configured
			// Note: This only sets the limit. Errors typically occur on Read within the handler/codec.
			if maxBodySize > 0 {
				req.Body = http.MaxBytesReader(w, req.Body, maxBodySize)
			}
			next.ServeHTTP(w, req)
		})
	}
}

// timeoutMiddleware creates a middleware that handles request timeouts.
func (r *Router[T, U]) timeoutMiddleware(timeout time.Duration) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			// No timeout needed if duration is zero or negative
			if timeout <= 0 {
				next.ServeHTTP(w, req)
				return
			}

			ctx, cancel := context.WithTimeout(req.Context(), timeout)
			defer cancel()
			req = req.WithContext(ctx)

			var wMutex sync.Mutex
			wrappedW := &mutexResponseWriter{
				ResponseWriter: w,
				mu:             &wMutex,
			}

			done := make(chan struct{})
			panicChan := make(chan interface{}, 1) // Channel to capture panic

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
				// Timeout occurred
				traceID := middleware.GetTraceID(req) // Use middleware package function
				fields := []zap.Field{
					zap.String("method", req.Method),
					zap.String("path", req.URL.Path),
					zap.Duration("timeout", timeout),
					zap.String("client_ip", req.RemoteAddr), // Consider using IP from IP middleware if available
				}
				if r.config.EnableTraceID && traceID != "" {
					fields = append(fields, zap.String("trace_id", traceID))
				}
				r.logger.Error("Request timed out", fields...)

				// Manually write the error response using the mutex-protected wrappedW methods
				// This avoids potential races within http.Error's header manipulation.
				wMutex.Lock()
				// Check if headers were already sent (best effort, not foolproof with standard ResponseWriter)
				// We assume if WriteHeader hasn't been called via wrappedW, it's safe.
				// The mutex ensures atomicity of the check-and-write sequence below.
				// Note: A more complex wrapper could track if WriteHeader was called.
				if _, ok := wrappedW.Header()["Content-Type"]; !ok { // Simple check
					wrappedW.Header().Set("Content-Type", "text/plain; charset=utf-8")
					wrappedW.WriteHeader(http.StatusRequestTimeout)
					fmt.Fprintln(wrappedW, "Request Timeout") // Use fmt.Fprintln or similar
				}
				// If headers were already sent, we can't change status code,
				// and writing might fail or be ignored by the client.
				wMutex.Unlock()
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
	var subRouterRateLimit *middleware.RateLimitConfig[any, any]
	var subRouterMiddlewares []common.Middleware
	if sr != nil {
		subRouterTimeout = sr.TimeoutOverride
		subRouterMaxBodySize = sr.MaxBodySizeOverride
		subRouterRateLimit = sr.RateLimitOverride
		subRouterMiddlewares = sr.Middlewares
	}

	// Create a new route config instance to avoid modifying the original
	finalRouteConfig := route

	// Prefix the path
	finalRouteConfig.Path = pathPrefix + route.Path

	// Calculate effective settings (these will be used inside RegisterGenericRoute's call to wrapHandler)
	// We need to pass these calculated values *into* RegisterGenericRoute or modify how wrapHandler gets them.
	// Let's modify RegisterGenericRoute slightly to accept these.

	// Combine middleware: global + sub-router + route-specific
	// Note: The order depends on how common.MiddlewareChain works. Assuming Append runs things earlier.
	allMiddlewares := make([]common.Middleware, 0, len(r.middlewares)+len(subRouterMiddlewares)+len(route.Middlewares))
	allMiddlewares = append(allMiddlewares, r.middlewares...)        // Global first
	allMiddlewares = append(allMiddlewares, subRouterMiddlewares...) // Then sub-router
	allMiddlewares = append(allMiddlewares, route.Middlewares...)    // Then route-specific
	finalRouteConfig.Middlewares = allMiddlewares                    // Overwrite middlewares in the config passed down

	// Get effective timeout, max body size, rate limit considering overrides
	// These need to be passed to wrapHandler eventually.
	// Let's adjust RegisterGenericRoute to accept these calculated values.
	effectiveTimeout := r.getEffectiveTimeout(route.Timeout, subRouterTimeout)
	effectiveMaxBodySize := r.getEffectiveMaxBodySize(route.MaxBodySize, subRouterMaxBodySize)
	effectiveRateLimit := r.getEffectiveRateLimit(route.RateLimit, subRouterRateLimit) // This returns *RateLimitConfig[UserID, User]

	// Call the underlying generic registration function with the modified config
	// We need to modify RegisterGenericRoute to accept the calculated effective values.
	RegisterGenericRoute[Req, Resp, UserID, User](r, finalRouteConfig, effectiveTimeout, effectiveMaxBodySize, effectiveRateLimit)

	return nil
}

// ServeHTTP implements the http.Handler interface.
// It handles HTTP requests by applying metrics and tracing if enabled,
// and then delegating to the underlying httprouter.
func (r *Router[T, U]) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// Create a response writer that captures metrics
	var rw http.ResponseWriter
	var traceID string

	// Apply metrics and tracing if enabled
	if r.config.EnableMetrics || r.config.EnableTracing {
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
			duration := time.Since(mrw.startTime)

			// Get updated trace ID from context
			traceID = middleware.GetTraceID(req)

			// Log metrics
			if r.config.EnableMetrics {
				// Create log fields
				fields := []zap.Field{
					zap.String("method", req.Method),
					zap.String("path", req.URL.Path),
					zap.Int("status", mrw.statusCode),
					zap.Duration("duration", duration),
					zap.Int64("bytes", mrw.bytesWritten),
				}

				// Add trace ID if enabled and present
				if r.config.EnableTraceID && traceID != "" {
					fields = append([]zap.Field{zap.String("trace_id", traceID)}, fields...)
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
					}

					// Add trace ID if enabled and present
					if r.config.EnableTraceID && traceID != "" {
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
					}

					// Add trace ID if enabled and present
					if r.config.EnableTraceID && traceID != "" {
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
					}

					// Add trace ID if enabled and present
					if r.config.EnableTraceID && traceID != "" {
						fields = append([]zap.Field{zap.String("trace_id", traceID)}, fields...)
					}

					r.logger.Warn("Client error", fields...)
				}
			}

			// Log tracing information
			if r.config.EnableTracing {
				// Create log fields
				fields := []zap.Field{
					zap.String("method", req.Method),
					zap.String("path", req.URL.Path),
					zap.String("remote_addr", req.RemoteAddr),
					zap.String("user_agent", req.UserAgent()),
					zap.Int("status", mrw.statusCode),
					zap.Duration("duration", duration),
				}

				// Add trace ID if enabled and present
				if r.config.EnableTraceID && traceID != "" {
					fields = append([]zap.Field{zap.String("trace_id", traceID)}, fields...)
				}

				// Use Debug level for tracing to avoid log spam
				r.logger.Debug("Request trace", fields...)
			}

			// Reset fields that might hold references to prevent memory leaks
			mrw.ResponseWriter = nil
			mrw.request = nil
			mrw.router = nil

			// Return the writer to the pool
			r.metricsWriterPool.Put(mrw)
		}()
	} else {
		// Use the original response writer if metrics and tracing are disabled
		rw = w
	}

	// Serve the request
	r.router.ServeHTTP(rw, req)
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
func (r *Router[T, U]) getEffectiveRateLimit(routeRateLimit, subRouterRateLimit *middleware.RateLimitConfig[any, any]) *middleware.RateLimitConfig[T, U] {
	// Convert the rate limit config to the correct type
	convertConfig := func(config *middleware.RateLimitConfig[any, any]) *middleware.RateLimitConfig[T, U] {
		if config == nil {
			return nil
		}

		// Create a new config with the correct type parameters
		return &middleware.RateLimitConfig[T, U]{
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
func (r *Router[T, U]) handleError(w http.ResponseWriter, req *http.Request, err error, statusCode int, message string) {
	// Get trace ID from context
	traceID := middleware.GetTraceID(req)

	// Create log fields
	fields := []zap.Field{
		zap.Error(err),
		zap.String("method", req.Method),
		zap.String("path", req.URL.Path),
	}

	// Add trace ID if enabled and present
	if r.config.EnableTraceID && traceID != "" {
		fields = append([]zap.Field{zap.String("trace_id", traceID)}, fields...)
	}

	// Log the error
	r.logger.Error(message, fields...)

	// Check if the error is a specific HTTP error
	var httpErr *HTTPError
	if errors.As(err, &httpErr) {
		statusCode = httpErr.StatusCode
		message = httpErr.Message
	} else if err != nil && err.Error() == "http: request body too large" {
		// Specifically handle MaxBytesReader error
		statusCode = http.StatusRequestEntityTooLarge
		message = "Request Entity Too Large"
	}

	// Return the error response
	http.Error(w, message, statusCode)
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
				traceID := middleware.GetTraceID(req)

				// Create log fields
				fields := []zap.Field{
					zap.Any("panic", rec),
					zap.String("method", req.Method),
					zap.String("path", req.URL.Path),
				}

				// Add trace ID if enabled and present
				if r.config.EnableTraceID && traceID != "" {
					fields = append([]zap.Field{zap.String("trace_id", traceID)}, fields...)
				}

				// Log the panic
				r.logger.Error("Panic recovered", fields...)

				// Return a 500 Internal Server Error
				http.Error(w, "Internal Server Error", http.StatusInternalServerError)
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
		// Declare traceID variable to be used throughout the function
		var traceID string
		// Check for the presence of an Authorization header
		authHeader := req.Header.Get("Authorization")
		if authHeader == "" {
			// Get updated trace ID from context
			traceID = middleware.GetTraceID(req)

			// Create log fields
			fields := []zap.Field{
				zap.String("method", req.Method),
				zap.String("path", req.URL.Path),
				zap.String("remote_addr", req.RemoteAddr),
				zap.String("error", "no authorization header"),
			}

			// Add trace ID if enabled and present
			if r.config.EnableTraceID && traceID != "" {
				fields = append([]zap.Field{zap.String("trace_id", traceID)}, fields...)
			}

			// Log that authentication failed
			r.logger.Warn("Authentication failed", fields...)
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}

		// Extract the token from the Authorization header
		token := strings.TrimPrefix(authHeader, "Bearer ")

		// Try to authenticate using the authFunction
		if user, valid := r.authFunction(req.Context(), token); valid {
			id := r.getUserIdFromUser(user)
			// Add the user ID to the request context using middleware package functions
			// First get the trace ID so we can preserve it
			traceID = middleware.GetTraceID(req)

			// Add the user ID to the context
			ctx := middleware.WithUserID[T, U](req.Context(), id)

			// If there was a trace ID, make sure it's preserved
			if traceID != "" {
				ctx = middleware.AddTraceIDToRequest(req.WithContext(ctx), traceID).Context()
			}
			req = req.WithContext(ctx)
			// Get updated trace ID from context
			traceID = middleware.GetTraceID(req)

			// Create log fields
			fields := []zap.Field{
				zap.String("method", req.Method),
				zap.String("path", req.URL.Path),
			}

			// Add trace ID if enabled and present
			if r.config.EnableTraceID && traceID != "" {
				fields = append([]zap.Field{zap.String("trace_id", traceID)}, fields...)
			}

			// Log that authentication was successful
			r.logger.Debug("Authentication successful", fields...)
			next.ServeHTTP(w, req)
			return
		}

		// Get trace ID from context
		traceID = middleware.GetTraceID(req)

		// Create log fields
		fields := []zap.Field{
			zap.String("method", req.Method),
			zap.String("path", req.URL.Path),
			zap.String("remote_addr", req.RemoteAddr),
			zap.String("error", "invalid token"),
		}

		// Add trace ID if enabled and present
		if r.config.EnableTraceID && traceID != "" {
			fields = append([]zap.Field{zap.String("trace_id", traceID)}, fields...)
		}

		// Log that authentication failed
		r.logger.Warn("Authentication failed", fields...)
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
	})
}

// authOptionalMiddleware is a middleware that attempts authentication for a request,
// but allows the request to proceed even if authentication fails.
// It tries to authenticate the request and adds the user ID to the context if successful,
// but allows the request to proceed even if authentication fails.
func (r *Router[T, U]) authOptionalMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// Try to authenticate the request
		authHeader := req.Header.Get("Authorization")
		if authHeader != "" {
			// Extract the token from the Authorization header
			token := strings.TrimPrefix(authHeader, "Bearer ")

			// Try to authenticate using the authFunction
			if user, valid := r.authFunction(req.Context(), token); valid {
				id := r.getUserIdFromUser(user)
				// Add the user ID to the request context using middleware package functions
				ctx := middleware.WithUserID[T, U](req.Context(), id)
				if r.config.AddUserObjectToCtx {
					ctx = middleware.WithUser[T, U](ctx, &user)
				}
				req = req.WithContext(ctx)
				// Get trace ID from context
				traceID := middleware.GetTraceID(req)

				// Create log fields
				fields := []zap.Field{
					zap.String("method", req.Method),
					zap.String("path", req.URL.Path),
				}

				// Add trace ID if enabled and present
				if r.config.EnableTraceID && traceID != "" {
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

// LoggingMiddleware is a middleware that logs HTTP requests and responses.
// It captures the request method, path, status code, and duration.
// If enableTraceID is true, it will include the trace ID in the logs if present.
func LoggingMiddleware(logger *zap.Logger, enableTraceID bool) Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			start := time.Now()

			// Create a response writer that captures the status code
			rw := &responseWriter{
				ResponseWriter: w,
				statusCode:     http.StatusOK,
			}

			// Call the next handler
			next.ServeHTTP(rw, req)

			// Get trace ID from context
			traceID := middleware.GetTraceID(req)

			// Create log fields
			fields := []zap.Field{
				zap.String("method", req.Method),
				zap.String("path", req.URL.Path),
				zap.Int("status", rw.statusCode),
				zap.Duration("duration", time.Since(start)),
			}

			// Add trace ID if enabled and present
			if enableTraceID && traceID != "" {
				fields = append([]zap.Field{zap.String("trace_id", traceID)}, fields...)
			}

			// Log the request
			logger.Info("Request", fields...)
		})
	}
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

// mutexResponseWriter is a wrapper around http.ResponseWriter that uses a mutex to protect access.
type mutexResponseWriter struct {
	http.ResponseWriter
	mu *sync.Mutex
}

// WriteHeader acquires the mutex and calls the underlying ResponseWriter.WriteHeader.
func (rw *mutexResponseWriter) WriteHeader(statusCode int) {
	rw.mu.Lock()
	defer rw.mu.Unlock()
	rw.ResponseWriter.WriteHeader(statusCode)
}

// Write acquires the mutex and calls the underlying ResponseWriter.Write.
func (rw *mutexResponseWriter) Write(b []byte) (int, error) {
	rw.mu.Lock()
	defer rw.mu.Unlock()
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
