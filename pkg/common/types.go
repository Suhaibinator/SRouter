// Package common provides middleware and policy types shared by SRouter packages.
package common

import (
	"net/http"
	"time"
)

// Middleware wraps an HTTP handler.
type Middleware func(http.Handler) http.Handler

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
	// Allow reports whether the key may proceed, the approximate remaining
	// capacity, and a backend-defined reset duration.
	Allow(key string, limit int, window time.Duration) (allowed bool, remaining int, reset time.Duration)
}

// RateLimitConfig configures a rate-limit policy. T is the router's comparable
// user-ID type and U is its user-object type.
type RateLimitConfig[T comparable, U any] struct {
	// BucketName namespaces the derived client key. Routes share counters only
	// when the bucket name, derived key, Limit, and Window all match.
	BucketName string

	// Limit is the maximum number of requests allowed within the Window.
	Limit int

	// Window is the time duration for the rate limit (e.g., 1*time.Minute, 1*time.Hour).
	Window time.Duration

	// Strategy determines how clients are identified for rate limiting.
	Strategy RateLimitStrategy

	// UserIDFromUser optionally extracts an ID from a stored user object for
	// StrategyUser. Without it, the limiter uses the user ID in the context.
	UserIDFromUser func(user U) T

	// UserIDToString converts the user ID (type T) to a string for use as a map key.
	// Used only when Strategy is StrategyUser. Optional: if nil, a default
	// conversion is used (handles string, int, int64, fmt.Stringer, and falls
	// back to fmt.Sprint).
	UserIDToString func(userID T) string

	// KeyExtractor provides a custom function to generate the rate limit key from the request.
	// Required only when Strategy is StrategyCustom.
	KeyExtractor func(r *http.Request) (key string, err error)

	// ExceededHandler handles a rejected request after rate-limit headers are
	// set. It owns the response status and body. Nil uses the default 429 response.
	ExceededHandler http.Handler
}
