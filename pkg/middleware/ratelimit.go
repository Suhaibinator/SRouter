// Package middleware provides a collection of HTTP middleware components for the SRouter framework.
package middleware

import (
	"errors" // Added for error handling
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/common"   // Import common for shared types
	"github.com/Suhaibinator/SRouter/pkg/scontext" // Use scontext for context functions
	"go.uber.org/ratelimit"
	"go.uber.org/zap"
)

// Note: RateLimitStrategy, RateLimiter, RateLimitConfig moved to pkg/common/types.go

// UberRateLimiter implements common.RateLimiter using Uber's ratelimit library (leaky bucket).
type UberRateLimiter struct {
	limiters sync.Map // map[string]ratelimit.Limiter
	mu       sync.Mutex
}

// NewUberRateLimiter creates a new rate limiter using Uber's ratelimit library.
func NewUberRateLimiter() *UberRateLimiter {
	return &UberRateLimiter{}
}

// getLimiter gets or creates a limiter for the given key and rate (requests per second).
// It uses a composite key including the RPS to handle different rate limits for the same base key.
func (u *UberRateLimiter) getLimiter(key string, rps int) ratelimit.Limiter {
	compositeKey := fmt.Sprintf("%s-%d", key, rps) // Combine key and rps

	if limiter, ok := u.limiters.Load(compositeKey); ok {
		return limiter.(ratelimit.Limiter)
	}

	u.mu.Lock()
	defer u.mu.Unlock()

	// Double-check after acquiring lock
	if limiter, ok := u.limiters.Load(compositeKey); ok {
		return limiter.(ratelimit.Limiter)
	}

	// Create new limiter
	limiter := ratelimit.New(rps)
	u.limiters.Store(compositeKey, limiter) // Store using the composite key
	return limiter
}

// Ensure UberRateLimiter implements the common.RateLimiter interface.
var _ common.RateLimiter = (*UberRateLimiter)(nil)

// Allow checks if a request is allowed based on the key and rate limit config.
// This implementation uses the leaky bucket algorithm.
func (u *UberRateLimiter) Allow(key string, limit int, window time.Duration) (bool, int, time.Duration) {
	// Convert limit and window to Requests Per Second (RPS) for Uber's limiter.
	// Ensure RPS is at least 1.
	rps := int(float64(limit) / window.Seconds())
	if rps < 1 {
		rps = 1
	}

	limiter := u.getLimiter(key, rps)

	// Take() blocks until a token is available or returns immediately if available.
	// It returns the time when the next token will be available.
	now := time.Now()
	nextAvailable := limiter.Take()
	waitTime := nextAvailable.Sub(now)

	// Estimate remaining tokens based on the wait time relative to the window.
	// This is an approximation for leaky bucket.
	remaining := int(float64(limit) * (1 - waitTime.Seconds()/window.Seconds()))
	if remaining < 0 {
		remaining = 0
	}

	// If the wait time is significant (e.g., > 1ms, indicating actual rate limiting), deny.
	// Uber's Take() might return a time slightly in the future even if not strictly limited.
	// A small threshold helps distinguish actual limiting from minor clock differences.
	// If waitTime is 0 or very small, the request is allowed.
	allowed := waitTime <= time.Millisecond // Allow if wait time is negligible

	// Reset time is the duration until the next token is available.
	resetDuration := waitTime
	if resetDuration < 0 {
		resetDuration = 0 // Cannot reset in the past
	}

	return allowed, remaining, resetDuration
}

// convertUserIDToString provides default conversions for common comparable types to string.
func convertUserIDToString[T comparable](userID T) string {
	switch v := any(userID).(type) {
	case string:
		return v
	case int:
		return strconv.Itoa(v)
	case int64:
		return strconv.FormatInt(v, 10)
	// Add other common types as needed (e.g., uint, uint64)
	default:
		// Fallback for types implementing fmt.Stringer or using fmt.Sprint
		if stringer, ok := any(userID).(fmt.Stringer); ok {
			return stringer.String()
		}
		return fmt.Sprint(userID)
	}
}

// extractUserKey extracts the user-based key (as a string) from the request context.
// It prioritizes the user object if UserIDFromUser is provided, otherwise uses the user ID directly.
// Returns an empty string if no user information is found or conversion fails.
func extractUserKey[T comparable, U any](r *http.Request, config *common.RateLimitConfig[T, U]) (string, error) { // Use common.RateLimitConfig
	if config.UserIDToString == nil {
		return "", errors.New("UserIDToString function is required for StrategyUser")
	}

	// Try getting the full user object first
	user, userOk := scontext.GetUserFromRequest[T, U](r) // Use scontext
	if userOk && user != nil {
		if config.UserIDFromUser == nil {
			// Cannot extract ID from user object without UserIDFromUser function
			// Try falling back to UserID directly
		} else {
			userID := config.UserIDFromUser(*user)
			return config.UserIDToString(userID), nil
		}
	}

	// Fallback: Try getting the user ID directly from context
	userID, idOk := scontext.GetUserIDFromRequest[T, U](r) // Use scontext
	if idOk {
		// Use the provided conversion function
		return config.UserIDToString(userID), nil
	}

	// No user information found in context
	return "", nil // Return empty key, let the caller decide how to handle (e.g., fallback to IP)
}

// RateLimit creates a middleware that enforces rate limits based on the provided configuration.
// T is the User ID type (comparable).
// U is the User object type (any).
//
// IMPORTANT: When using common.StrategyIP, ensure that router.ClientIPMiddleware is applied *before* this middleware in the chain.
func RateLimit[T comparable, U any](config *common.RateLimitConfig[T, U], limiter common.RateLimiter, logger *zap.Logger) common.Middleware { // Use common types and common.Middleware
	// Input validation
	if config == nil {
		// Return a no-op middleware if config is nil
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				next.ServeHTTP(w, r)
			})
		}
	}
	if limiter == nil {
		panic("RateLimit middleware requires a non-nil RateLimiter") // Or return an error-logging middleware
	}
	if logger == nil {
		logger = zap.NewNop() // Use a no-op logger if none provided
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var key string
			var err error
			var strategyUsed string // For logging

			switch config.Strategy {
			case common.StrategyIP: // Use common.StrategyIP
				strategyUsed = "IP"
				// Get IP from context (must be set by router.ClientIPMiddleware)
				ip, ipFound := scontext.GetClientIP[T, U](r.Context()) // Use scontext
				if !ipFound || ip == "" {
					logger.Error("Client IP not found in context for StrategyIP rate limiting. Ensure router.ClientIPMiddleware is applied first.",
						zap.String("method", r.Method),
						zap.String("path", r.URL.Path),
					)
					// Decide how to handle: block, allow, or use RemoteAddr as unsafe fallback?
					// Using RemoteAddr might be okay for basic DoS protection but not accurate behind proxies.
					// For now, let's use RemoteAddr as a fallback but log prominently.
					key = r.RemoteAddr // Unsafe fallback
					logger.Warn("Falling back to RemoteAddr for StrategyIP rate limiting.", zap.String("remote_addr", key))
				} else {
					key = ip
				}

			case common.StrategyUser: // Use common.StrategyUser
				strategyUsed = "User"
				key, err = extractUserKey(r, config)
				if err != nil {
					logger.Error("Failed to extract user key for rate limiting",
						zap.Error(err),
						zap.String("method", r.Method),
						zap.String("path", r.URL.Path),
					)
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
					return
				}
				// If no user key found, fall back to IP strategy as a safety measure
				if key == "" {
					strategyUsed = "User (fallback to IP)"
					ip, ipFound := scontext.GetClientIP[T, U](r.Context()) // Use scontext
					if !ipFound || ip == "" {
						key = r.RemoteAddr // Unsafe fallback
						logger.Warn("User key not found, falling back to RemoteAddr for rate limiting.", zap.String("remote_addr", key))
					} else {
						key = ip
						logger.Info("User key not found, falling back to ClientIP from context for rate limiting.", zap.String("client_ip", key))
					}
				}

			case common.StrategyCustom: // Use common.StrategyCustom
				strategyUsed = "Custom"
				if config.KeyExtractor == nil {
					logger.Error("KeyExtractor function is required for StrategyCustom rate limiting.",
						zap.String("method", r.Method),
						zap.String("path", r.URL.Path),
					)
					http.Error(w, "Internal Server Error: Rate limit configuration error", http.StatusInternalServerError)
					return
				}
				key, err = config.KeyExtractor(r)
				if err != nil {
					logger.Error("Custom KeyExtractor failed",
						zap.Error(err),
						zap.String("method", r.Method),
						zap.String("path", r.URL.Path),
					)
					http.Error(w, "Internal Server Error", http.StatusInternalServerError)
					return
				}
				// If custom extractor returns empty key, maybe fallback? Or treat as error?
				if key == "" {
					logger.Error("Custom KeyExtractor returned an empty key.",
						zap.String("method", r.Method),
						zap.String("path", r.URL.Path),
					)
					http.Error(w, "Internal Server Error: Rate limit key error", http.StatusInternalServerError)
					return
				}

			default:
				// Treat unknown strategy as IP-based, but log a warning
				strategyUsed = "Unknown (defaulting to IP)"
				logger.Warn("Unknown rate limit strategy specified, defaulting to IP.",
					zap.Int("strategy_value", int(config.Strategy)),
				)
				ip, ipFound := scontext.GetClientIP[T, U](r.Context()) // Use scontext
				if !ipFound || ip == "" {
					key = r.RemoteAddr // Unsafe fallback
					logger.Warn("Defaulting to RemoteAddr for rate limiting due to unknown strategy.", zap.String("remote_addr", key))
				} else {
					key = ip
				}
			}

			// Combine bucket name and key for the final limiter key
			bucketKey := config.BucketName + ":" + key

			// Check rate limit
			allowed, remaining, reset := limiter.Allow(bucketKey, config.Limit, config.Window)

			// Set rate limit headers regardless of outcome
			w.Header().Set("X-RateLimit-Limit", strconv.Itoa(config.Limit))
			w.Header().Set("X-RateLimit-Remaining", strconv.Itoa(remaining))
			// Reset is duration until reset, convert to Unix timestamp
			resetTimestamp := time.Now().Add(reset).Unix()
			w.Header().Set("X-RateLimit-Reset", strconv.FormatInt(resetTimestamp, 10))

			if allowed {
				// Call the next handler if allowed
				next.ServeHTTP(w, r)
			} else {
				// Rate limit exceeded
				// Set Retry-After header (seconds)
				retryAfterSeconds := int64(reset.Seconds())
				if retryAfterSeconds < 1 {
					retryAfterSeconds = 1 // Minimum 1 second
				}
				w.Header().Set("Retry-After", strconv.FormatInt(retryAfterSeconds, 10))

				logger.Warn("Rate limit exceeded",
					zap.String("bucket", config.BucketName),
					zap.String("key", key), // Log the actual key used (IP, user ID, custom)
					zap.String("strategy", strategyUsed),
					zap.Int("limit", config.Limit),
					zap.Duration("window", config.Window),
					zap.Int("remaining", remaining),
					zap.Duration("reset_duration", reset),
					zap.String("method", r.Method),
					zap.String("path", r.URL.Path),
				)

				// Use custom handler or default 429 response
				if config.ExceededHandler != nil {
					config.ExceededHandler.ServeHTTP(w, r)
				} else {
					http.Error(w, http.StatusText(http.StatusTooManyRequests), http.StatusTooManyRequests)
				}
				// Do not call next handler
			}
		})
	}
}

// CreateRateLimitMiddleware provides a simplified way to create a RateLimit middleware instance.
// T is the User ID type (comparable).
// U is the User object type (any).
//
// Deprecated: Prefer configuring common.RateLimitConfig directly and calling RateLimit for more clarity and flexibility.
func CreateRateLimitMiddleware[T comparable, U any](
	bucketName string,
	limit int,
	window time.Duration,
	strategy common.RateLimitStrategy, // Use common.RateLimitStrategy
	userIDFromUser func(U) T, // Optional, only for StrategyUser with user object
	userIDToString func(T) string, // Required for StrategyUser
	logger *zap.Logger,
) common.Middleware { // Use common.Middleware
	config := &common.RateLimitConfig[T, U]{ // Use common.RateLimitConfig
		BucketName:     bucketName,
		Limit:          limit,
		Window:         window,
		Strategy:       strategy,
		UserIDFromUser: userIDFromUser,
		UserIDToString: userIDToString,
		// KeyExtractor and ExceededHandler are left nil
	}

	// Use the default UberRateLimiter
	limiter := NewUberRateLimiter()

	return RateLimit(config, limiter, logger)
}
