// Package router provides a flexible and feature-rich HTTP routing framework.
// It supports middleware, sub-routers, generic handlers, and various configuration options.
package router

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/codec"
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
	cacheHitCounter   metrics.Counter
	cacheMissCounter  metrics.Counter
	cacheHitRatio     metrics.Gauge
}

// contextKey is a type for context keys.
// It's used to store and retrieve values from request contexts.
type contextKey string

const (
	// ParamsKey is the key used to store httprouter.Params in the request context.
	// This allows route parameters to be accessed from handlers and middleware.
	ParamsKey contextKey = "params"
)

// userIDContextKey is a custom type for the user ID context key to avoid collisions
// It's now generic to support different user ID types
type userIDContextKey[T comparable] struct{}

// userObjectContextKey is a custom type for the user object context key to avoid collisions
// It's now generic to support different user object types
type userObjectContextKey[U any] struct{}

// GenericRouteRegistrar is an interface for registering generic routes
// It's used to store generic routes with different type parameters in SubRouterConfig
type GenericRouteRegistrar interface {
	RegisterWith(router interface{}, pathPrefix string) error
}

// GenericRouteConfig is a wrapper for RouteConfig that implements GenericRouteRegistrar
// It allows storing generic routes with different type parameters in SubRouterConfig
type GenericRouteConfig[Req any, Resp any, UserID comparable, User any] struct {
	Config RouteConfig[Req, Resp]
}

// RegisterWith registers the generic route with the router
// It implements the GenericRouteRegistrar interface
func (g GenericRouteConfig[Req, Resp, UserID, User]) RegisterWith(router interface{}, pathPrefix string) error {
	r, ok := router.(*Router[UserID, User])
	if !ok {
		return fmt.Errorf("router is not of type *Router[%T, %T]", *new(UserID), *new(User))
	}

	// Create a copy of the config with the path prefixed
	config := g.Config
	config.Path = pathPrefix + config.Path

	// Register the generic route with the router
	RegisterGenericRoute[Req, Resp, UserID, User](r, config)
	return nil
}

// GenericRouteConfigs is a slice of GenericRouteRegistrar
// It's used to store multiple generic routes in SubRouterConfig
type GenericRouteConfigs []GenericRouteRegistrar

// CreateGenericRouteForSubRouter creates a GenericRouteConfig that can be added to a SubRouterConfig's GenericRoutes field
// This is a helper function to make it easier to register generic routes with SubRouters
func CreateGenericRouteForSubRouter[Req any, Resp any, UserID comparable, User any](route RouteConfig[Req, Resp]) GenericRouteConfig[Req, Resp, UserID, User] {
	return GenericRouteConfig[Req, Resp, UserID, User]{
		Config: route,
	}
}

// RegisterGenericRouteWithSubRouter registers a generic route with a SubRouter
// This is a helper function that creates a GenericRouteConfig and adds it to the SubRouter's GenericRoutes field
func RegisterGenericRouteWithSubRouter[Req any, Resp any, UserID comparable, User any](sr *SubRouterConfig, route RouteConfig[Req, Resp]) {
	genericRoute := CreateGenericRouteForSubRouter[Req, Resp, UserID, User](route)

	// If GenericRoutes is nil, initialize it as a GenericRouteConfigs
	if sr.GenericRoutes == nil {
		sr.GenericRoutes = GenericRouteConfigs{genericRoute}
		return
	}

	// If GenericRoutes is already a GenericRouteConfigs, append to it
	if routes, ok := sr.GenericRoutes.(GenericRouteConfigs); ok {
		sr.GenericRoutes = append(routes, genericRoute)
		return
	}

	// If GenericRoutes is a single GenericRouteRegistrar, convert to GenericRouteConfigs and append
	if route, ok := sr.GenericRoutes.(GenericRouteRegistrar); ok {
		sr.GenericRoutes = GenericRouteConfigs{route, genericRoute}
		return
	}

	// If GenericRoutes is something else, log a warning and replace it
	// This shouldn't happen in normal usage, but we handle it just in case
	sr.GenericRoutes = GenericRouteConfigs{genericRoute}
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
	// This ensures that the client IP is available in the request context for all other middleware,
	// which is especially important for rate limiting by IP address. If IP middleware is not added
	// or is added after rate limiting middleware, rate limiting by IP will fall back to extracting
	// the IP from headers or RemoteAddr, which may not be as reliable.
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

				// Initialize cache metrics
				r.cacheHitCounter = registry.NewCounter().
					Name("cache_hits_total").
					Description("Total number of cache hits").
					Tag("service", config.MetricsConfig.Namespace).
					Build()

				r.cacheMissCounter = registry.NewCounter().
					Name("cache_misses_total").
					Description("Total number of cache misses").
					Tag("service", config.MetricsConfig.Namespace).
					Build()

				r.cacheHitRatio = registry.NewGauge().
					Name("cache_hit_ratio").
					Description("Ratio of cache hits to total cache lookups").
					Tag("service", config.MetricsConfig.Namespace).
					Build()
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

		// If caching is enabled for the sub-router, wrap the handler with a caching handler
		if sr.CacheResponse && r.config.CacheGet != nil && r.config.CacheSet != nil {
			// Get the effective cache key prefix
			cacheKeyPrefix := r.getEffectiveCacheKeyPrefix("", sr.CacheKeyPrefix)

			// Create a caching handler
			originalHandler := handler
			handler = http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				// Only cache GET requests
				if req.Method != "GET" {
					originalHandler.ServeHTTP(w, req)
					return
				}

				// Create a cache key from the request path and query
				cacheKey := req.URL.Path
				if req.URL.RawQuery != "" {
					cacheKey += "?" + req.URL.RawQuery
				}

				// Add the prefix if available
				if cacheKeyPrefix != "" {
					cacheKey = cacheKeyPrefix + ":" + cacheKey
				}

				// Try to get from cache
				if cachedResponse, found := r.config.CacheGet(cacheKey); found {
					// Log and record metrics for cache hit
					r.logger.Debug("Cache hit", zap.String("cache_key", cacheKey))
					if r.cacheHitCounter != nil {
						r.cacheHitCounter.Inc()

						// Update cache hit ratio
						if r.cacheHitRatio != nil {
							hits := r.cacheHitCounter.Value()
							misses := r.cacheMissCounter.Value()
							total := hits + misses
							if total > 0 {
								r.cacheHitRatio.Set(hits / total)
							}
						}
					}

					// Write the cached response
					_, _ = w.Write(cachedResponse)
					return
				}

				// Log and record metrics for cache miss
				r.logger.Debug("Cache miss", zap.String("cache_key", cacheKey))
				if r.cacheMissCounter != nil {
					r.cacheMissCounter.Inc()

					// Update cache hit ratio
					if r.cacheHitRatio != nil {
						hits := r.cacheHitCounter.Value()
						misses := r.cacheMissCounter.Value()
						total := hits + misses
						if total > 0 {
							r.cacheHitRatio.Set(hits / total)
						}
					}
				}

				// Create a recorder to capture the response
				recorder := httptest.NewRecorder()

				// Call the original handler with the recorder
				originalHandler.ServeHTTP(recorder, req)

				// Get the response from the recorder
				result := recorder.Result()
				defer result.Body.Close()

				// Read the response body
				responseBody, err := io.ReadAll(result.Body)
				if err != nil {
					r.handleError(w, req, err, http.StatusInternalServerError, "Failed to read response body")
					return
				}

				// Cache the response
				cacheErr := r.config.CacheSet(cacheKey, responseBody)
				if cacheErr != nil {
					// Log the error but don't fail the request
					r.logger.Warn("Failed to cache response",
						zap.String("cache_key", cacheKey),
						zap.Error(cacheErr))
				} else {
					r.logger.Debug("Response cached", zap.String("cache_key", cacheKey))
				}

				// Copy the headers from the recorder to the response writer
				for k, v := range recorder.Header() {
					w.Header()[k] = v
				}

				// Write the status code
				w.WriteHeader(recorder.Code)

				// Write the response body
				_, _ = w.Write(responseBody)
			})
		}

		// Register the route with httprouter
		for _, method := range route.Methods {
			r.router.Handle(method, fullPath, r.convertToHTTPRouterHandle(handler))
		}
	}

	// Register generic routes if any
	if sr.GenericRoutes != nil {
		// Try to cast to GenericRouteConfigs
		if routes, ok := sr.GenericRoutes.(GenericRouteConfigs); ok {
			for _, route := range routes {
				err := route.RegisterWith(r, sr.PathPrefix)
				if err != nil {
					r.logger.Error("Failed to register generic route",
						zap.Error(err),
						zap.String("path_prefix", sr.PathPrefix))
				}
			}
		} else {
			// Try to cast to a single GenericRouteRegistrar
			if route, ok := sr.GenericRoutes.(GenericRouteRegistrar); ok {
				err := route.RegisterWith(r, sr.PathPrefix)
				if err != nil {
					r.logger.Error("Failed to register generic route",
						zap.Error(err),
						zap.String("path_prefix", sr.PathPrefix))
				}
			} else {
				r.logger.Error("Invalid GenericRoutes type",
					zap.String("path_prefix", sr.PathPrefix),
					zap.String("type", fmt.Sprintf("%T", sr.GenericRoutes)))
			}
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

// getEffectiveCacheKeyPrefix returns the effective cache key prefix for a route.
// It considers route-specific, sub-router, and global cache key prefix settings in that order of precedence.
func (r *Router[T, U]) getEffectiveCacheKeyPrefix(routePrefix, subRouterPrefix string) string {
	if routePrefix != "" {
		return routePrefix
	}
	if subRouterPrefix != "" {
		return subRouterPrefix
	}
	return r.config.CacheKeyPrefix
}

// RegisterSubRouter registers a sub-router with the router.
// It applies the sub-router's path prefix to all routes and registers them with the router.
// This is an exported wrapper for the unexported registerSubRouter method.
func (r *Router[T, U]) RegisterSubRouter(sr SubRouterConfig) {
	r.registerSubRouter(sr)
}

// RegisterRoute registers a route with the router.
// It creates a handler with all middlewares applied and registers it with the underlying httprouter.
// For generic routes with type parameters, use RegisterGenericRoute function instead.
func (r *Router[T, U]) RegisterRoute(route RouteConfigBase) {
	// Get effective timeout, max body size, and rate limit for this route
	timeout := r.getEffectiveTimeout(route.Timeout, 0)
	maxBodySize := r.getEffectiveMaxBodySize(route.MaxBodySize, 0)
	rateLimit := r.getEffectiveRateLimit(route.RateLimit, nil)

	// Create a handler with all middlewares applied
	handler := r.wrapHandler(route.Handler, route.AuthLevel, timeout, maxBodySize, rateLimit, route.Middlewares)

	// Register the route with httprouter
	for _, method := range route.Methods {
		r.router.Handle(method, route.Path, r.convertToHTTPRouterHandle(handler))
	}
}

// RegisterGenericRoute registers a route with generic request and response types.
// This is a standalone function rather than a method because Go methods cannot have type parameters.
// It creates a handler that uses the codec to decode the request and encode the response,
// applies middleware, and registers the route with the router.
func RegisterGenericRoute[Req any, Resp any, UserID comparable, User any](r *Router[UserID, User], route RouteConfig[Req, Resp]) {
	// Get effective timeout, max body size, and rate limit for this route
	timeout := r.getEffectiveTimeout(route.Timeout, 0)
	maxBodySize := r.getEffectiveMaxBodySize(route.MaxBodySize, 0)
	rateLimit := r.getEffectiveRateLimit(route.RateLimit, nil)

	// Create a handler that uses the codec to decode the request and encode the response
	handler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		var data Req
		var err error
		var encodedData string
		var cacheKey string
		var canCache bool

		// Check if caching is enabled for this route
		if route.CacheResponse && r.config.CacheGet != nil && r.config.CacheSet != nil {
			// Caching is only supported for query and path parameter source types
			canCache = route.SourceType == Base64QueryParameter ||
				route.SourceType == Base62QueryParameter ||
				route.SourceType == Base64PathParameter ||
				route.SourceType == Base62PathParameter
		}

		// Try to get from cache if caching is enabled
		if canCache {
			// Get the encoded value based on source type
			switch route.SourceType {
			case Base64QueryParameter, Base62QueryParameter:
				encodedData = req.URL.Query().Get(route.SourceKey)
			case Base64PathParameter, Base62PathParameter:
				paramName := route.SourceKey
				if paramName == "" {
					// If no specific parameter name is provided, use the first path parameter
					params := GetParams(req)
					if len(params) > 0 {
						encodedData = params[0].Value
					}
				} else {
					encodedData = GetParam(req, paramName)
				}
			}

			// Use the encoded value as the cache key with prefix
			if encodedData != "" {
				// Apply cache key prefix if available
				prefix := r.getEffectiveCacheKeyPrefix(route.CacheKeyPrefix, "")

				if prefix != "" {
					cacheKey = prefix + ":" + encodedData
				} else {
					cacheKey = encodedData
				}

				// Try to get from cache
				if cachedResponse, found := r.config.CacheGet(cacheKey); found {
					// Log and record metrics for cache hit
					r.logger.Debug("Cache hit", zap.String("cache_key", cacheKey))
					if r.cacheHitCounter != nil {
						r.cacheHitCounter.Inc()

						// Update cache hit ratio
						if r.cacheHitRatio != nil {
							hits := r.cacheHitCounter.Value()
							misses := r.cacheMissCounter.Value()
							total := hits + misses
							if total > 0 {
								r.cacheHitRatio.Set(hits / total)
							}
						}
					}

					// Write the cached response
					_, _ = w.Write(cachedResponse)
					return
				}

				// Log and record metrics for cache miss
				r.logger.Debug("Cache miss", zap.String("cache_key", cacheKey))
				if r.cacheMissCounter != nil {
					r.cacheMissCounter.Inc()

					// Update cache hit ratio
					if r.cacheHitRatio != nil {
						hits := r.cacheHitCounter.Value()
						misses := r.cacheMissCounter.Value()
						total := hits + misses
						if total > 0 {
							r.cacheHitRatio.Set(hits / total)
						}
					}
				}
			}
		}

		// Get data based on source type
		switch route.SourceType {
		case Body: // Default is Body (0)
			// Use the codec's Decode method directly for body data
			data, err = route.Codec.Decode(req)
			if err != nil {
				r.handleError(w, req, err, http.StatusBadRequest, "Failed to decode request body")
				return
			}

		case Base64QueryParameter:
			// Get from query parameter and decode base64
			encodedData := req.URL.Query().Get(route.SourceKey)
			if encodedData == "" {
				r.handleError(w, req, errors.New("missing query parameter"),
					http.StatusBadRequest, "Missing required query parameter: "+route.SourceKey)
				return
			}

			// Decode from base64
			decodedData, err := codec.DecodeBase64(encodedData)
			if err != nil {
				r.handleError(w, req, err, http.StatusBadRequest,
					"Failed to decode base64 query parameter: "+route.SourceKey)
				return
			}

			// Unmarshal the decoded data
			var reqData Req
			err = json.Unmarshal(decodedData, &reqData)
			if err != nil {
				r.handleError(w, req, err, http.StatusBadRequest,
					"Failed to unmarshal decoded query parameter data")
				return
			}
			data = reqData

		case Base62QueryParameter:
			// Get from query parameter and decode base62
			encodedData := req.URL.Query().Get(route.SourceKey)
			if encodedData == "" {
				r.handleError(w, req, errors.New("missing query parameter"),
					http.StatusBadRequest, "Missing required query parameter: "+route.SourceKey)
				return
			}

			// Decode from base62
			decodedData, err := codec.DecodeBase62(encodedData)
			if err != nil {
				r.handleError(w, req, err, http.StatusBadRequest,
					"Failed to decode base62 query parameter: "+route.SourceKey)
				return
			}

			// Unmarshal the decoded data
			var reqData Req
			err = json.Unmarshal(decodedData, &reqData)
			if err != nil {
				r.handleError(w, req, err, http.StatusBadRequest,
					"Failed to unmarshal decoded query parameter data")
				return
			}
			data = reqData

		case Base64PathParameter:
			// Get from path parameter and decode base64
			paramName := route.SourceKey
			if paramName == "" {
				// If no specific parameter name is provided, use the first path parameter
				params := GetParams(req)
				if len(params) == 0 {
					r.handleError(w, req, errors.New("no path parameters found"),
						http.StatusBadRequest, "No path parameters found")
					return
				}
				paramName = params[0].Key
			}

			encodedData := GetParam(req, paramName)
			if encodedData == "" {
				r.handleError(w, req, errors.New("missing path parameter"),
					http.StatusBadRequest, "Missing required path parameter: "+paramName)
				return
			}

			// Decode from base64
			decodedData, err := codec.DecodeBase64(encodedData)
			if err != nil {
				r.handleError(w, req, err, http.StatusBadRequest,
					"Failed to decode base64 path parameter: "+paramName)
				return
			}

			// Unmarshal the decoded data
			var reqData Req
			err = json.Unmarshal(decodedData, &reqData)
			if err != nil {
				r.handleError(w, req, err, http.StatusBadRequest,
					"Failed to unmarshal decoded path parameter data")
				return
			}
			data = reqData

		case Base62PathParameter:
			// Get from path parameter and decode base62
			paramName := route.SourceKey
			if paramName == "" {
				// If no specific parameter name is provided, use the first path parameter
				params := GetParams(req)
				if len(params) == 0 {
					r.handleError(w, req, errors.New("no path parameters found"),
						http.StatusBadRequest, "No path parameters found")
					return
				}
				paramName = params[0].Key
			}

			encodedData := GetParam(req, paramName)
			if encodedData == "" {
				r.handleError(w, req, errors.New("missing path parameter"),
					http.StatusBadRequest, "Missing required path parameter: "+paramName)
				return
			}

			// Decode from base62
			decodedData, err := codec.DecodeBase62(encodedData)
			if err != nil {
				r.handleError(w, req, err, http.StatusBadRequest,
					"Failed to decode base62 path parameter: "+paramName)
				return
			}

			// Unmarshal the decoded data
			var reqData Req
			err = json.Unmarshal(decodedData, &reqData)
			if err != nil {
				r.handleError(w, req, err, http.StatusBadRequest,
					"Failed to unmarshal decoded path parameter data")
				return
			}
			data = reqData

		default:
			r.handleError(w, req, errors.New("unsupported source type"),
				http.StatusInternalServerError, "Unsupported source type")
			return
		}

		// Call the handler
		resp, err := route.Handler(req, data)
		if err != nil {
			r.handleError(w, req, err, http.StatusInternalServerError, "Handler error")
			return
		}

		// If caching is enabled and we have a cache key, we need to capture the response
		if canCache && cacheKey != "" && r.config.CacheSet != nil {
			// Create a recorder to capture the response
			recorder := httptest.NewRecorder()

			// Encode the response to the recorder
			err = route.Codec.Encode(recorder, resp)
			if err != nil {
				r.handleError(w, req, err, http.StatusInternalServerError, "Failed to encode response")
				return
			}

			// Get the response from the recorder
			result := recorder.Result()
			defer result.Body.Close()

			// Read the response body
			responseBody, err := io.ReadAll(result.Body)
			if err != nil {
				r.handleError(w, req, err, http.StatusInternalServerError, "Failed to read response body")
				return
			}

			// Cache the response
			cacheErr := r.config.CacheSet(cacheKey, responseBody)
			if cacheErr != nil {
				// Log the error but don't fail the request
				r.logger.Warn("Failed to cache response",
					zap.String("cache_key", cacheKey),
					zap.Error(cacheErr))
			} else {
				r.logger.Debug("Response cached", zap.String("cache_key", cacheKey))
			}

			// Write the response to the original response writer
			_, _ = w.Write(responseBody)
		} else {
			// Encode the response directly to the response writer
			err = route.Codec.Encode(w, resp)
			if err != nil {
				r.handleError(w, req, err, http.StatusInternalServerError, "Failed to encode response")
				return
			}
		}
	})

	// Create a handler with all middlewares applied
	wrappedHandler := r.wrapHandler(handler, route.AuthLevel, timeout, maxBodySize, rateLimit, route.Middlewares)

	// Register the route with httprouter
	for _, method := range route.Methods {
		r.router.Handle(method, route.Path, r.convertToHTTPRouterHandle(wrappedHandler))
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
	}))

	// Build the middleware chain
	chain := common.NewMiddlewareChain()

	// Add recovery middleware (always first in the chain)
	chain = chain.Prepend(r.recoveryMiddleware)

	// Add global middlewares
	chain = chain.Append(r.middlewares...)

	// Add route-specific middlewares
	chain = chain.Append(middlewares...)

	// Add rate limiting middleware if configured
	if rateLimit != nil {
		chain = chain.Append(middleware.RateLimit(rateLimit, r.rateLimiter, r.logger))
	}

	// Add authentication middleware based on the auth level
	switch authLevel {
	case AuthRequired:
		chain = chain.Append(r.authRequiredMiddleware)
	case AuthOptional:
		chain = chain.Append(r.authOptionalMiddleware)
	}

	// Apply the middleware chain to the handler
	return chain.Then(h)
}

// ServeHTTP implements the http.Handler interface.
// It handles HTTP requests by applying metrics and tracing if enabled,
// and then delegating to the underlying httprouter.
func (r *Router[T, U]) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	// Create a response writer that captures metrics
	var rw http.ResponseWriter

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

			// Get trace ID from context
			traceID := middleware.GetTraceID(req)

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
// This allows the router to track the HTTP status code for metrics and logging.
func (rw *metricsResponseWriter[T, U]) WriteHeader(statusCode int) {
	rw.statusCode = statusCode
	rw.ResponseWriter.WriteHeader(statusCode)
}

// Write captures the number of bytes written and calls the underlying ResponseWriter.Write.
// This allows the router to track the response size for metrics and logging.
func (rw *metricsResponseWriter[T, U]) Write(b []byte) (int, error) {
	n, err := rw.ResponseWriter.Write(b)
	rw.bytesWritten += int64(n)
	return n, err
}

// Flush calls the underlying ResponseWriter.Flush if it implements http.Flusher.
// This allows streaming responses to be flushed to the client immediately.
func (rw *metricsResponseWriter[T, U]) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Shutdown gracefully shuts down the router.
// It stops accepting new requests and waits for existing requests to complete.
// If the context is canceled before all requests complete, it returns the context's error.
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

// GetUserID retrieves the user ID from the request context.
// Returns the user ID if it exists in the context, the zero value of T otherwise.
func GetUserID[T comparable, U any](r *http.Request) (T, bool) {
	userID, ok := r.Context().Value(userIDContextKey[T]{}).(T)
	return userID, ok
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
		// Check for the presence of an Authorization header
		authHeader := req.Header.Get("Authorization")
		if authHeader == "" {
			// Get trace ID from context
			traceID := middleware.GetTraceID(req)

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
			// Add the user ID to the request context
			ctx := context.WithValue(req.Context(), userIDContextKey[T]{}, id)
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
				fields = append(fields, zap.String("trace_id", traceID))
			}

			// Log that authentication was successful
			r.logger.Debug("Authentication successful", fields...)
			next.ServeHTTP(w, req)
			return
		}

		// Get trace ID from context
		traceID := middleware.GetTraceID(req)

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
				// Add the user ID to the request context
				ctx := context.WithValue(req.Context(), userIDContextKey[T]{}, id)
				if r.config.AddUserObjectToCtx {
					ctx = context.WithValue(ctx, userObjectContextKey[U]{}, &user)
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
// This allows middleware to inspect the status code after the handler has completed.
func (rw *responseWriter) WriteHeader(statusCode int) {
	rw.statusCode = statusCode
	rw.ResponseWriter.WriteHeader(statusCode)
}

// Write calls the underlying ResponseWriter.Write.
// It passes through the write operation to the wrapped ResponseWriter.
func (rw *responseWriter) Write(b []byte) (int, error) {
	return rw.ResponseWriter.Write(b)
}

// Flush calls the underlying ResponseWriter.Flush if it implements http.Flusher.
// This allows streaming responses to be flushed to the client immediately.
func (rw *responseWriter) Flush() {
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// mutexResponseWriter is a wrapper around http.ResponseWriter that uses a mutex to protect access.
// This ensures thread-safety when writing to the response from multiple goroutines.
type mutexResponseWriter struct {
	http.ResponseWriter
	mu *sync.Mutex
}

// WriteHeader acquires the mutex and calls the underlying ResponseWriter.WriteHeader.
// This ensures thread-safety when setting the status code from multiple goroutines.
func (rw *mutexResponseWriter) WriteHeader(statusCode int) {
	rw.mu.Lock()
	defer rw.mu.Unlock()
	rw.ResponseWriter.WriteHeader(statusCode)
}

// Write acquires the mutex and calls the underlying ResponseWriter.Write.
// This ensures thread-safety when writing the response body from multiple goroutines.
func (rw *mutexResponseWriter) Write(b []byte) (int, error) {
	rw.mu.Lock()
	defer rw.mu.Unlock()
	return rw.ResponseWriter.Write(b)
}

// Flush acquires the mutex and calls the underlying ResponseWriter.Flush if it implements http.Flusher.
// This ensures thread-safety when flushing the response from multiple goroutines.
func (rw *mutexResponseWriter) Flush() {
	rw.mu.Lock()
	defer rw.mu.Unlock()
	if f, ok := rw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}
