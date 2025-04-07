// Package common provides shared types and utilities used across the SRouter framework.
package common

import (
	"net/http"
	"time"
)

// Middleware defines the type for HTTP middleware functions.
// It takes an http.Handler and returns an http.Handler.
type Middleware func(http.Handler) http.Handler

// --- Rate Limiting Types (Moved from pkg/middleware) ---

// RateLimitStrategy defines how the rate limiter identifies clients.
type RateLimitStrategy int

const (
	// StrategyIP uses the client's IP address as the key for rate limiting.
	// Requires router.ClientIPMiddleware to be applied first.
	StrategyIP RateLimitStrategy = iota
	// StrategyUser uses the authenticated user's ID from the context.
	StrategyUser
	// StrategyCustom uses a custom key extractor function.
	StrategyCustom
)

// RateLimiter defines the interface for rate limiting algorithms.
type RateLimiter interface {
	// Allow checks if a request is allowed based on the key and rate limit config.
	// Returns true if the request is allowed, false otherwise.
	// Also returns the number of remaining requests and the approximate time until the limit resets.
	Allow(key string, limit int, window time.Duration) (allowed bool, remaining int, reset time.Duration)
}

// RateLimitConfig defines configuration for rate limiting with generic type parameters.
// T is the User ID type (comparable).
// U is the User object type (any).
// Note: This struct uses generic types T and U, which might require careful handling
// when used across different packages if the specific types T and U are not known
// at the point of configuration (e.g., in RouterConfig). Using `any` for T and U
// in RouterConfig might be necessary, with type assertions or conversions happening
// within the middleware or route registration logic where the types are known.
type RateLimitConfig[T comparable, U any] struct {
	// BucketName provides a namespace for the rate limit.
	// If multiple routes/subrouters share the same BucketName, they share the same rate limit counters.
	BucketName string

	// Limit is the maximum number of requests allowed within the Window.
	Limit int

	// Window is the time duration for the rate limit (e.g., 1*time.Minute, 1*time.Hour).
	Window time.Duration

	// Strategy determines how clients are identified for rate limiting.
	Strategy RateLimitStrategy

	// UserIDFromUser extracts the user ID (type T) from the user object (type U).
	// Required only when Strategy is StrategyUser and the user object is stored in the context.
	UserIDFromUser func(user U) T

	// UserIDToString converts the user ID (type T) to a string for use as a map key.
	// Required only when Strategy is StrategyUser.
	UserIDToString func(userID T) string

	// KeyExtractor provides a custom function to generate the rate limit key from the request.
	// Required only when Strategy is StrategyCustom.
	KeyExtractor func(r *http.Request) (key string, err error)

	// ExceededHandler is an optional http.Handler to call when the rate limit is exceeded.
	// If nil, a default 429 Too Many Requests response is sent.
	ExceededHandler http.Handler
}
