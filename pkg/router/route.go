package router

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/codec"
	"github.com/Suhaibinator/SRouter/pkg/common"
	"github.com/Suhaibinator/SRouter/pkg/logkeys"
	"github.com/Suhaibinator/SRouter/pkg/scontext"
	"go.uber.org/zap"
)

// Route adds one or more standard or typed routes to the root route group.
//
// Middleware execution order:
// 1. Global middlewares (from RouterConfig)
// 2. Root middlewares (from Router.Use)
// 3. Route-specific middlewares
//
// Configuration precedence (most specific wins):
// - Route settings > root-group settings > global settings
func (r *Router[T, U]) Route(routes ...RouteDefinition) *Router[T, U] {
	r.routeTree.root.Route(routes...)
	return r
}

// Group creates a first-level route group.
func (r *Router[T, U]) Group(prefix string) *RouteGroup[T, U] {
	return r.routeTree.root.Group(prefix)
}

// Use appends middleware to the root route group.
func (r *Router[T, U]) Use(middlewares ...common.Middleware) *Router[T, U] {
	r.routeTree.root.Use(middlewares...)
	return r
}

// Timeout overrides the configured global timeout for root routes and groups.
// A zero duration disables the inherited global timeout.
func (r *Router[T, U]) Timeout(timeout time.Duration) *Router[T, U] {
	r.routeTree.root.Timeout(timeout)
	return r
}

// MaxBodySize overrides the configured global body limit for root routes and
// groups. Zero disables the inherited global limit.
func (r *Router[T, U]) MaxBodySize(bytes int64) *Router[T, U] {
	r.routeTree.root.MaxBodySize(bytes)
	return r
}

// RateLimit sets a type-safe root rate limit for all routes and groups. Nil
// disables an inherited global rate limit.
func (r *Router[T, U]) RateLimit(config *common.RateLimitConfig[T, U]) *Router[T, U] {
	r.routeTree.root.RateLimit(config)
	return r
}

// AuthToken overrides the configured global authentication token source for
// root routes and groups. Nil restores the built-in Authorization header.
func (r *Router[T, U]) AuthToken(config *common.AuthTokenConfig) *Router[T, U] {
	r.routeTree.root.AuthToken(config)
	return r
}

// Auth sets the default authentication level for root routes and groups.
func (r *Router[T, U]) Auth(level AuthLevel) *Router[T, U] {
	r.routeTree.root.Auth(level)
	return r
}

// baseConfig makes every RouteConfig instantiation a RouteDefinition without
// erasing its request or response types.
func (route RouteConfig[Req, Resp]) baseConfig(runtime routeRuntime, pathPrefix string) (RouteConfigBase, error) {
	fullPath, err := joinRoutePath(pathPrefix, route.Path)
	if err != nil {
		return RouteConfigBase{}, err
	}
	if route.Codec == nil {
		return RouteConfigBase{}, fmt.Errorf("typed route %q has no codec", fullPath)
	}
	if route.Handler == nil {
		return RouteConfigBase{}, fmt.Errorf("typed route %q has no handler", fullPath)
	}
	switch route.SourceType {
	case Body, Empty, Base64PathParameter, Base62PathParameter:
	case Base64QueryParameter, Base62QueryParameter:
		if route.SourceKey == "" {
			return RouteConfigBase{}, fmt.Errorf("typed route %q has no query parameter source key", fullPath)
		}
	default:
		return RouteConfigBase{}, fmt.Errorf("typed route %q has invalid source type %d", fullPath, route.SourceType)
	}
	if route.Sanitizer == nil {
		runtime.warnMissingSanitizer(fullPath, route.Methods)
	}

	return RouteConfigBase{
		Path:           route.Path,
		Methods:        route.Methods,
		AuthLevel:      route.AuthLevel,
		Overrides:      route.Overrides,
		Handler:        route.httpHandler(runtime),
		Middlewares:    route.Middlewares,
		DisableTimeout: route.DisableTimeout,
	}, nil
}

func (route RouteConfig[Req, Resp]) httpHandler(runtime routeRuntime) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		// Note: MaxBytesReader is applied in wrapHandler, no need to apply it again here.

		var data Req
		var err error

		// Get data based on source type
		switch route.SourceType {
		case Body: // Default is Body (0)
			// Use the codec's Decode method to read directly from the request body
			data, err = route.Codec.Decode(req)
			if err != nil {
				// Check if this is a MaxBytesReader error (applied in wrapHandler).
				// errors.As unwraps, so this works even when a codec wraps the error.
				if isMaxBytesError(err) {
					runtime.handleError(w, req, err, http.StatusRequestEntityTooLarge, "Request entity too large")
					return
				}
				runtime.handleError(w, req, err, http.StatusBadRequest, "Failed to decode request body")
				return
			}

		case Base64QueryParameter:
			// Get from query parameter and decode base64
			encodedData := req.URL.Query().Get(route.SourceKey)
			if encodedData == "" {
				runtime.handleError(w, req, errors.New("missing query parameter"),
					http.StatusBadRequest, "Missing required query parameter: "+route.SourceKey)
				return
			}

			// Decode from base64
			decodedData, err := codec.DecodeBase64(encodedData)
			if err != nil {
				runtime.handleError(w, req, err, http.StatusBadRequest,
					"Failed to decode base64 query parameter: "+route.SourceKey)
				return
			}

			// Use codec's DecodeBytes to unmarshal the decoded data
			data, err = route.Codec.DecodeBytes(decodedData)
			if err != nil {
				runtime.handleError(w, req, err, http.StatusBadRequest,
					"Failed to decode query parameter data")
				return
			}

		case Base62QueryParameter:
			// Get from query parameter and decode base62
			encodedData := req.URL.Query().Get(route.SourceKey)
			if encodedData == "" {
				runtime.handleError(w, req, errors.New("missing query parameter"),
					http.StatusBadRequest, "Missing required query parameter: "+route.SourceKey)
				return
			}

			// Decode from base62
			decodedData, err := codec.DecodeBase62(encodedData)
			if err != nil {
				runtime.handleError(w, req, err, http.StatusBadRequest,
					"Failed to decode base62 query parameter: "+route.SourceKey)
				return
			}

			// Use codec's DecodeBytes to unmarshal the decoded data
			data, err = route.Codec.DecodeBytes(decodedData)
			if err != nil {
				runtime.handleError(w, req, err, http.StatusBadRequest,
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
					runtime.handleError(w, req, errors.New("no path parameters found"),
						http.StatusBadRequest, "No path parameters found")
					return
				}
				paramName = params[0].Key
			}

			encodedData := GetParam(req, paramName)
			if encodedData == "" {
				runtime.handleError(w, req, errors.New("missing path parameter"),
					http.StatusBadRequest, "Missing required path parameter: "+paramName)
				return
			}

			// Decode from base64
			decodedData, err := codec.DecodeBase64(encodedData)
			if err != nil {
				runtime.handleError(w, req, err, http.StatusBadRequest,
					"Failed to decode base64 path parameter: "+paramName)
				return
			}

			// Use codec's DecodeBytes to unmarshal the decoded data
			data, err = route.Codec.DecodeBytes(decodedData)
			if err != nil {
				runtime.handleError(w, req, err, http.StatusBadRequest,
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
					runtime.handleError(w, req, errors.New("no path parameters found"),
						http.StatusBadRequest, "No path parameters found")
					return
				}
				paramName = params[0].Key
			}

			encodedData := GetParam(req, paramName)
			if encodedData == "" {
				runtime.handleError(w, req, errors.New("missing path parameter"),
					http.StatusBadRequest, "Missing required path parameter: "+paramName)
				return
			}

			// Decode from base62
			decodedData, err := codec.DecodeBase62(encodedData)
			if err != nil {
				runtime.handleError(w, req, err, http.StatusBadRequest,
					"Failed to decode base62 path parameter: "+paramName)
				return
			}

			// Use codec's DecodeBytes to unmarshal the decoded data
			data, err = route.Codec.DecodeBytes(decodedData)
			if err != nil {
				runtime.handleError(w, req, err, http.StatusBadRequest,
					"Failed to decode path parameter data")
				return
			}
		case Empty:

		default:
			runtime.handleError(w, req, errors.New("unsupported source type"),
				http.StatusInternalServerError, "Unsupported source type")
			return
		}

		// Apply sanitizer if provided
		if route.Sanitizer != nil {
			sanitizedData, err := route.Sanitizer(req.Context(), data)
			if err != nil {
				runtime.handleError(w, req, err, http.StatusBadRequest, "Sanitization failed")
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
			runtime.recordHandlerError(req, err)

			runtime.handleError(w, req, err, http.StatusInternalServerError, "Handler error")
			return
		}

		// Encode the response directly to the response writer
		err = route.Codec.Encode(w, resp)
		if err != nil {
			runtime.handleError(w, req, err, http.StatusInternalServerError, "Failed to encode response")
			return
		}

	}
}

func (r *Router[T, U]) recordHandlerError(req *http.Request, err error) {
	// SRouterContext is pointer-backed, so middleware already holding it observes
	// this mutation without replacing the request context.
	scontext.WithHandlerError[T, U](req.Context(), err)
}

func (r *Router[T, U]) warnMissingSanitizer(path string, methods []HttpMethod) {
	methodNames := make([]string, len(methods))
	for i, method := range methods {
		methodNames[i] = string(method)
	}
	r.logger.Warn("Route registered without sanitizer function",
		zap.String(logkeys.Path, path),
		zap.Strings(logkeys.Methods, methodNames),
	)
}
