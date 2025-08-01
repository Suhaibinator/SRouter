package router

import (
	"errors"
	"net/http"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/codec"
	"github.com/Suhaibinator/SRouter/pkg/common" // Ensure common is imported
	"github.com/Suhaibinator/SRouter/pkg/scontext"
	"go.uber.org/zap"
)

// RegisterRoute registers a standard (non-generic) route with the router.
// It creates a handler with all middlewares applied and registers it with the underlying httprouter.
//
// Middleware execution order:
// 1. Global middlewares (from RouterConfig)
// 2. Route-specific middlewares
//
// Configuration precedence (most specific wins):
// - Route settings > Global settings
//
// For generic routes with type parameters, use RegisterGenericRoute function instead.
func (r *Router[T, U]) RegisterRoute(route RouteConfigBase) {
	// Get effective timeout, max body size, and rate limit for this route
	timeout := r.getEffectiveTimeout(route.Overrides.Timeout, 0)
	maxBodySize := r.getEffectiveMaxBodySize(route.Overrides.MaxBodySize, 0)
	// Pass the specific route config (which is *common.RateLimitConfig[any, any])
	// to getEffectiveRateLimit. The conversion happens inside getEffectiveRateLimit.
	rateLimit := r.getEffectiveRateLimit(route.Overrides.RateLimit, nil)

	// Create a handler with all middlewares applied
	handler := r.wrapHandler(route.Handler, route.AuthLevel, timeout, maxBodySize, rateLimit, route.Middlewares)

	// Register the route with httprouter
	for _, method := range route.Methods {
		r.router.Handle(string(method), route.Path, r.convertToHTTPRouterHandle(handler, route.Path)) // Convert HttpMethod to string
	}
}

// RegisterGenericRoute registers a route with generic request and response types.
// This is a standalone function rather than a method because Go methods cannot have type parameters.
//
// The function creates a complete request/response pipeline:
// 1. Decodes request using the codec (based on SourceType)
// 2. Applies optional sanitizer function
// 3. Calls the generic handler with decoded data
// 4. Encodes response using the codec
// 5. Handles errors appropriately (including HTTPError for custom status codes)
//
// The effective settings (timeout, max body size, rate limit) must be pre-calculated
// by the caller. This is typically done by NewGenericRouteDefinition or RegisterGenericRouteOnSubRouter.
//
// Note: This function is primarily for internal use. Users should prefer NewGenericRouteDefinition
// for declarative route registration within SubRouterConfig.
func RegisterGenericRoute[Req any, Resp any, UserID comparable, User any](
	r *Router[UserID, User],
	route RouteConfig[Req, Resp],
	// Add effective settings calculated by the caller (e.g., RegisterGenericRouteOnSubRouter)
	effectiveTimeout time.Duration,
	effectiveMaxBodySize int64,
	effectiveRateLimit *common.RateLimitConfig[UserID, User], // Use common.RateLimitConfig
) {
	// Create a handler that uses the codec to decode the request and encode the response
	handler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// Note: MaxBytesReader is applied in wrapHandler, no need to apply it again here.

		var data Req
		var err error

		// Get data based on source type
		switch route.SourceType {
		case Body: // Default is Body (0)
			// Use the codec's Decode method to read directly from the request body
			data, err = route.Codec.Decode(req)
			if err != nil {
				// Check if this is a MaxBytesReader error (applied in wrapHandler)
				// Note: io.ReadAll is no longer called here, the codec handles reading.
				// We need to check for the specific error string potentially returned by http.MaxBytesReader
				// or similar errors from the codec's Decode implementation.
				if err.Error() == "http: request body too large" { // Keep this check
					r.handleError(w, req, err, http.StatusRequestEntityTooLarge, "Request entity too large")
					return
				}
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

			// Use codec's DecodeBytes to unmarshal the decoded data
			data, err = route.Codec.DecodeBytes(decodedData)
			if err != nil {
				r.handleError(w, req, err, http.StatusBadRequest,
					"Failed to decode query parameter data")
				return
			}

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

			// Use codec's DecodeBytes to unmarshal the decoded data
			data, err = route.Codec.DecodeBytes(decodedData)
			if err != nil {
				r.handleError(w, req, err, http.StatusBadRequest,
					"Failed to decode query parameter data")
				return
			}

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

			// Use codec's DecodeBytes to unmarshal the decoded data
			data, err = route.Codec.DecodeBytes(decodedData)
			if err != nil {
				r.handleError(w, req, err, http.StatusBadRequest,
					"Failed to decode path parameter data")
				return
			}

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

			// Use codec's DecodeBytes to unmarshal the decoded data
			data, err = route.Codec.DecodeBytes(decodedData)
			if err != nil {
				r.handleError(w, req, err, http.StatusBadRequest,
					"Failed to decode path parameter data")
				return
			}
		case Empty:

		default:
			r.handleError(w, req, errors.New("unsupported source type"),
				http.StatusInternalServerError, "Unsupported source type")
			return
		}

		// Warn if no sanitizer function is provided
		if route.Sanitizer == nil {
			r.logger.Warn("Route registered without sanitizer function",
				zap.String("path", route.Path),
				zap.Strings("methods", func() []string {
					methods := make([]string, len(route.Methods))
					for i, method := range route.Methods {
						methods[i] = string(method)
					}
					return methods
				}()),
			)
		}

		// Apply sanitizer if provided
		if route.Sanitizer != nil {
			sanitizedData, err := route.Sanitizer(data)
			if err != nil {
				r.handleError(w, req, err, http.StatusBadRequest, "Sanitization failed")
				return
			}
			data = sanitizedData
		}

		// Call the handler
		resp, err := route.Handler(req, data)
		if err != nil {
			// Store error in context for middleware to access
			// Note: We don't need to update req with the returned context because
			// if SRouterContext already exists (which it should), this modifies
			// the existing pointer that middleware already has access to
			scontext.WithHandlerError[UserID, User](req.Context(), err)

			r.handleError(w, req, err, http.StatusInternalServerError, "Handler error")
			return
		}

		// Encode the response directly to the response writer
		err = route.Codec.Encode(w, resp)
		if err != nil {
			r.handleError(w, req, err, http.StatusInternalServerError, "Failed to encode response")
			return
		}

	})

	// Create a handler with all middlewares applied, using the effective settings passed in
	wrappedHandler := r.wrapHandler(handler, route.AuthLevel, effectiveTimeout, effectiveMaxBodySize, effectiveRateLimit, route.Middlewares)

	// Register the route with httprouter
	for _, method := range route.Methods {
		r.router.Handle(string(method), route.Path, r.convertToHTTPRouterHandle(wrappedHandler, route.Path)) // Convert HttpMethod to string
	}
}

// NewGenericRouteDefinition creates a GenericRouteRegistrationFunc for declarative configuration.
// It captures the specific RouteConfig[Req, Resp] and returns a function that, when called
// by registerSubRouter, calculates effective settings and registers the generic route.
//
// This is the recommended way to register generic routes within SubRouterConfig.Routes.
// It ensures proper application of sub-router settings including:
// - Path prefix concatenation
// - Middleware combination (sub-router + route-specific)
// - Configuration override precedence
// - AuthLevel inheritance (route > sub-router > default)
//
// Type parameters must match those used in NewRouter[UserID, User].
func NewGenericRouteDefinition[Req any, Resp any, UserID comparable, User any](
	route RouteConfig[Req, Resp],
) GenericRouteRegistrationFunc[UserID, User] {
	return func(r *Router[UserID, User], sr SubRouterConfig) {
		// Create a new route config instance to avoid modifying the original
		finalRouteConfig := route

		// Prefix the path
		finalRouteConfig.Path = sr.PathPrefix + route.Path

		// Combine middleware: sub-router + route-specific
		// Note: Global middlewares are added later by wrapHandler
		allMiddlewares := make([]common.Middleware, 0, len(sr.Middlewares)+len(route.Middlewares)) // Use common.Middleware
		allMiddlewares = append(allMiddlewares, sr.Middlewares...)
		allMiddlewares = append(allMiddlewares, route.Middlewares...)
		finalRouteConfig.Middlewares = allMiddlewares // Overwrite middlewares in the config passed down

		// Determine effective AuthLevel
		authLevel := route.AuthLevel // Use route-specific first
		if authLevel == nil {
			authLevel = sr.AuthLevel // Fallback to sub-router default
		}
		finalRouteConfig.AuthLevel = authLevel // Set the effective auth level

		// Get effective timeout, max body size, rate limit considering overrides
		effectiveTimeout := r.getEffectiveTimeout(route.Overrides.Timeout, sr.Overrides.Timeout)
		effectiveMaxBodySize := r.getEffectiveMaxBodySize(route.Overrides.MaxBodySize, sr.Overrides.MaxBodySize)
		// Pass the specific route config (which is *common.RateLimitConfig[any, any])
		// to getEffectiveRateLimit. The conversion happens inside getEffectiveRateLimit.
		effectiveRateLimit := r.getEffectiveRateLimit(route.Overrides.RateLimit, sr.Overrides.RateLimit)

		// Call the underlying generic registration function with the modified config and effective settings
		RegisterGenericRoute(r, finalRouteConfig, effectiveTimeout, effectiveMaxBodySize, effectiveRateLimit)
	}
}
