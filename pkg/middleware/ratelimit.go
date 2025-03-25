// Package middleware provides a collection of HTTP middleware components for the SRouter framework.
package middleware

import (
	"fmt"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"go.uber.org/ratelimit"
	"go.uber.org/zap"
)

type RateLimitStrategy int

const (
	// StrategyIP uses the client's IP address as the key for rate limiting
	StrategyIP RateLimitStrategy = iota
	// StrategyUser uses the authenticated user's ID as the key for rate limiting
	StrategyUser
	// StrategyCustom uses a custom key extractor function for rate limiting
	StrategyCustom
)

// RateLimiter defines the interface for rate limiting algorithms
type RateLimiter interface {
	// Allow checks if a request is allowed based on the key and rate limit config
	// Returns true if the request is allowed, false otherwise
	// Also returns the number of remaining requests and time until reset
	Allow(key string, limit int, window time.Duration) (bool, int, time.Duration)
}

// UberRateLimiter implements RateLimiter using Uber's ratelimit library
type UberRateLimiter struct {
	limiters sync.Map // map[string]ratelimit.Limiter
	mu       sync.Mutex
}

// NewUberRateLimiter creates a new rate limiter using Uber's ratelimit library
func NewUberRateLimiter() *UberRateLimiter {
	return &UberRateLimiter{}
}

// getLimiter gets or creates a limiter for the given key and rate
func (u *UberRateLimiter) getLimiter(key string, rps int) ratelimit.Limiter {
	if limiter, ok := u.limiters.Load(key); ok {
		return limiter.(ratelimit.Limiter)
	}

	u.mu.Lock()
	defer u.mu.Unlock()

	// Double-check after acquiring lock
	if limiter, ok := u.limiters.Load(key); ok {
		return limiter.(ratelimit.Limiter)
	}

	// Create new limiter
	limiter := ratelimit.New(rps)
	u.limiters.Store(key, limiter)
	return limiter
}

// Allow checks if a request is allowed based on the key and rate limit config
// This implementation uses only the leaky bucket algorithm for simplicity and efficiency
func (u *UberRateLimiter) Allow(key string, limit int, window time.Duration) (bool, int, time.Duration) {
	// Convert limit and window to RPS
	rps := int(float64(limit) / window.Seconds())
	if rps < 1 {
		rps = 1
	}

	limiter := u.getLimiter(key, rps)

	// Take from the limiter
	now := time.Now()
	next := limiter.Take()

	// Calculate remaining based on time until next permit
	remaining := int(float64(limit) * (1 - next.Sub(now).Seconds()/window.Seconds()))
	if remaining < 0 {
		remaining = 0
	}

	// If the wait is too long, deny the request
	if next.Sub(now) > time.Second {
		return false, remaining, next.Sub(now)
	}

	return true, remaining, window
}

// extractIP extracts the client IP address from the request context
// If the IP is not in the context, it falls back to the old method
func extractIP(r *http.Request, logger *zap.Logger) string {
	// Try to get the IP from the context first (set by ClientIPMiddleware)
	if ip := ClientIP(r); ip != "" {
		return ip
	}

	// Log a warning that we're using the fallback mechanism
	if logger != nil {
		logger.Warn("IP middleware not properly configured or applied before rate limiting",
			zap.String("method", r.Method),
			zap.String("path", r.URL.Path),
			zap.String("remote_addr", r.RemoteAddr),
		)
	}

	// Fall back to the old method for backward compatibility
	// This should not be needed if ClientIPMiddleware is properly configured
	// Try X-Forwarded-For header first
	ip := r.Header.Get("X-Forwarded-For")
	if ip != "" {
		// The leftmost IP is the original client
		ips := strings.Split(ip, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	// Try X-Real-IP header next
	ip = r.Header.Get("X-Real-IP")
	if ip != "" {
		return ip
	}

	// Fall back to RemoteAddr
	return r.RemoteAddr
}

// RateLimitConfig defines configuration for rate limiting with generic type parameters
// The type parameter T represents the user ID type, which can be any comparable type.
// The type parameter U represents the user type, which can be any type.
type RateLimitConfig[T comparable, U any] struct {
	// Unique identifier for this rate limit bucket
	// If multiple routes/subrouters share the same BucketName, they share the same rate limit
	BucketName string

	// Maximum number of requests allowed in the time window
	Limit int

	// Time window for the rate limit (e.g., 1 minute, 1 hour)
	Window time.Duration

	// Strategy for identifying clients (IP, User, Custom)
	// - "ip": Use client IP address
	// - "user": Use authenticated user ID
	// - "custom": Use a custom key extractor
	Strategy RateLimitStrategy

	// Function to extract user ID from user object (only used when Strategy is StrategyUser)
	// This allows for efficient user ID extraction without trying multiple types
	UserIDFromUser func(U) T

	// Function to convert user ID to string (only used when Strategy is StrategyUser)
	// This allows for efficient user ID conversion without type assertions
	UserIDToString func(T) string

	// Custom key extractor function (used when Strategy is "custom")
	// This allows for complex rate limiting scenarios
	KeyExtractor func(*http.Request) (string, error)

	// Response to send when rate limit is exceeded
	// If nil, a default 429 Too Many Requests response is sent
	ExceededHandler http.Handler
}

// convertUserIDToString converts a user ID of any comparable type to a string
func convertUserIDToString[T comparable](userID T) string {
	// Use type assertion to convert the user ID to a string
	switch v := any(userID).(type) {
	case string:
		return v
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	case float64:
		return strconv.FormatFloat(v, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(v)
	default:
		// For other types, use the String() method if available
		if stringer, ok := any(userID).(interface{ String() string }); ok {
			return stringer.String()
		}
		return fmt.Sprint(userID)
	}
}

// extractUser extracts the user ID from the request context using generic type parameters
func extractUser[T comparable, U any](r *http.Request, config *RateLimitConfig[T, U]) string {
	// Get the user from the context
	user, userOk := GetUserFromRequest[T, U](r)
	if !userOk || user == nil {
		// If no user is found, try to get the user ID directly
		userID, ok := GetUserIDFromRequest[T, U](r)
		if !ok {
			return ""
		}

		// Convert the user ID to string using the provided function
		if config.UserIDToString != nil {
			return config.UserIDToString(userID)
		}

		// Fall back to default conversion
		return convertUserIDToString(userID)
	}

	// Extract the user ID from the user object using the provided function
	if config.UserIDFromUser != nil {
		userID := config.UserIDFromUser(*user)

		// Convert the user ID to string using the provided function
		if config.UserIDToString != nil {
			return config.UserIDToString(userID)
		}

		// Fall back to default conversion
		return convertUserIDToString(userID)
	}

	// If no user ID extraction function is provided, return an empty string
	return ""
}

// RateLimit creates a middleware that enforces rate limits using generic type parameters
// The type parameter T represents the user ID type, which can be any comparable type.
// The type parameter U represents the user type, which can be any type.
func RateLimit[T comparable, U any](config *RateLimitConfig[T, U], limiter RateLimiter, logger *zap.Logger) func(http.Handler) http.Handler {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Skip rate limiting if config is nil
			if config == nil {
				next.ServeHTTP(w, r)
				return
			}

			// Extract key based on strategy
			var key string
			var err error

			switch config.Strategy {
			case StrategyIP:
				key = extractIP(r, logger)
			case StrategyUser:
				key = extractUser(r, config)
				// If no user is found and strategy is user, fall back to IP
				if key == "" {
					key = extractIP(r, logger)
				}
			case StrategyCustom:
				if config.KeyExtractor != nil {
					key, err = config.KeyExtractor(r)
					if err != nil {
						logger.Error("Failed to extract rate limit key",
							zap.Error(err),
							zap.String("method", r.Method),
							zap.String("path", r.URL.Path),
						)
						http.Error(w, "Internal Server Error", http.StatusInternalServerError)
						return
					}
				} else {
					// If no key extractor is provided, fall back to IP
					key = extractIP(r, logger)
				}
			default:
				key = extractIP(r, logger)
			}

			// Combine bucket name and key to create a unique identifier
			bucketKey := config.BucketName + ":" + key

			// Check rate limit
			allowed, remaining, reset := limiter.Allow(bucketKey, config.Limit, config.Window)

			// Set rate limit headers
			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(config.Limit))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
			w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(time.Now().Add(reset).Unix(), 10))

			// If rate limit exceeded
			if !allowed {
				w.Header().Set("Retry-After", strconv.FormatInt(int64(reset.Seconds()), 10))

				// Use custom handler if provided, otherwise return 429
				if config.ExceededHandler != nil {
					config.ExceededHandler.ServeHTTP(w, r)
				} else {
					http.Error(w, "Too Many Requests", http.StatusTooManyRequests)
				}

				logger.Warn("Rate limit exceeded",
					zap.String("method", r.Method),
					zap.String("path", r.URL.Path),
					zap.String("key", key),
					zap.Int("limit", config.Limit),
					zap.Int("remaining", remaining),
				)

				return
			}

			// Headers are already set above

			// Call the next handler
			next.ServeHTTP(w, r)
		})
	}
}

// CreateRateLimitMiddleware is a helper function to create a rate limit middleware with generic type parameters
// This function is useful when you want to create a rate limit middleware with a specific user ID type and user type
// The type parameter T represents the user ID type, which can be any comparable type.
// The type parameter U represents the user type, which can be any type.
func CreateRateLimitMiddleware[T comparable, U any](
	bucketName string,
	limit int,
	window time.Duration,
	strategy RateLimitStrategy,
	userIDFromUser func(U) T,
	userIDToString func(T) string,
	logger *zap.Logger,
) func(http.Handler) http.Handler {
	config := &RateLimitConfig[T, U]{
		BucketName:     bucketName,
		Limit:          limit,
		Window:         window,
		Strategy:       strategy,
		UserIDFromUser: userIDFromUser,
		UserIDToString: userIDToString,
	}

	limiter := NewUberRateLimiter()

	return RateLimit(config, limiter, logger)
}
