package router

import (
	"encoding/json"
	"errors"
	"net/http"

	"time" // <-- Add time import

	"github.com/Suhaibinator/SRouter/pkg/codec"
	"github.com/Suhaibinator/SRouter/pkg/middleware" // <-- Add middleware import
)

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
// applies middleware using the provided effective settings, and registers the route with the router.
func RegisterGenericRoute[Req any, Resp any, UserID comparable, User any](
	r *Router[UserID, User],
	route RouteConfig[Req, Resp],
	// Add effective settings calculated by the caller (e.g., RegisterGenericRouteOnSubRouter)
	effectiveTimeout time.Duration,
	effectiveMaxBodySize int64,
	effectiveRateLimit *middleware.RateLimitConfig[UserID, User],
) {
	// Create a handler that uses the codec to decode the request and encode the response
	handler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// Note: MaxBytesReader is applied in wrapHandler, no need to apply it again here.

		var data Req
		var err error

		// Get data based on source type
		switch route.SourceType {
		case Body: // Default is Body (0)
			// Use the codec's Decode method directly for body data
			data, err = route.Codec.Decode(req)
			if err != nil {
				// Check if this is a MaxBytesReader error
				if err.Error() == "http: request body too large" {
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
		r.router.Handle(method, route.Path, r.convertToHTTPRouterHandle(wrappedHandler))
	}
}
