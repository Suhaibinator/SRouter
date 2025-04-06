// Package middleware provides a collection of HTTP middleware components for the SRouter framework.
package middleware

import (
	"github.com/Suhaibinator/SRouter/pkg/common"
)

// Middleware is an alias for the common.Middleware type.
// It represents a function that wraps an http.Handler to provide additional functionality.
type Middleware = common.Middleware

// Public middleware constructors exposed via variables.
// The actual implementations are kept private within their respective files.
var (
	// From middleware.go
	Recovery    = recovery
	Logging     = logging
	MaxBodySize = maxBodySize
	Timeout     = timeout
	CORS        = cors
)

// Note: Generic middleware constructors (Authentication*, New*Middleware, RateLimit, etc.)
// remain public in their original files (auth.go, ip.go, ratelimit.go).
// Supporting types like CORSOptions, AuthProvider, RateLimiter, IPConfig, etc.,
// and helper functions like Chain, ClientIP, GetTraceID, etc., remain public
// in their original files. Context-related functions in context.go and DB types
// in db.go also remain public.
