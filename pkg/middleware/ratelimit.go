// Package middleware provides a collection of HTTP middleware components for the SRouter framework.
package middleware

import (
	"fmt"
	"net/http"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/common"   // Import common for shared types
	"github.com/Suhaibinator/SRouter/pkg/scontext" // Use scontext for context functions
	"go.uber.org/zap"
)

// Note: RateLimitStrategy, RateLimiter, RateLimitConfig moved to pkg/common/types.go

// limiterSweepInterval is the minimum time between sweeps of stale limiter
// entries. Sweeps are triggered by entry creation — the only operation that
// grows the map — so a stable key set never pays for sweeping, while key
// churn (the only way the map can grow without bound) triggers a sweep at
// most once per interval. Each sweep runs in its own short-lived goroutine.
const limiterSweepInterval = time.Minute

// UberRateLimiter implements the common.RateLimiter interface using a
// non-blocking sliding-window counter. Over-limit requests are denied
// immediately (never queued or slept on), and the configured limit/window
// semantics are honored exactly — e.g. "2 per minute" allows 2 requests per
// minute, not 1 per second.
//
// Memory is bounded by eviction: entries idle for at least two of their own
// windows hold no usable history (the next request would reset their counters
// anyway), so they are deleted by an amortized sweep and lazily recreated on
// next use. Eviction therefore never changes a rate-limit decision. Sweeps
// are scheduled only when a new entry is created, so live entries are
// roughly those active within the last 2×window + limiterSweepInterval;
// stale entries beyond that linger only until the next new key arrives.
//
// The name is retained for backwards compatibility with earlier versions that
// were backed by Uber's ratelimit library. The implementation maintains one
// window counter per unique key.
type UberRateLimiter struct {
	limiters sync.Map // map[string]*slidingWindowLimiter

	// lastSweep is the unix-nano time at which a sweep was last started.
	// Entry creation CASes it forward to claim the right to start the next
	// sweep, guaranteeing at most one sweep per limiterSweepInterval.
	lastSweep atomic.Int64

	// nowFunc overrides the clock in tests. nil means time.Now. Must be set
	// before the limiter is first used.
	nowFunc func() time.Time
}

// timeNow returns the current time from the configured clock.
func (u *UberRateLimiter) timeNow() time.Time {
	if u.nowFunc != nil {
		return u.nowFunc()
	}
	return time.Now()
}

// NewUberRateLimiter creates a new UberRateLimiter instance.
// It maintains separate rate limit counters for different keys
// (e.g., different IPs or users).
func NewUberRateLimiter() *UberRateLimiter {
	return &UberRateLimiter{}
}

// slidingWindowLimiter tracks request counts for the current and previous
// windows. The effective count is the current window's count plus the
// previous window's count weighted by how much of it still overlaps a
// sliding window ending now. This smooths bursts at window boundaries while
// keeping O(1) memory per key.
type slidingWindowLimiter struct {
	mu          sync.Mutex
	windowStart time.Time
	prevCount   int
	currCount   int

	// window is fixed at creation (the map's composite key includes it) and
	// is read by the sweeper to decide staleness.
	window time.Duration

	// evicted is set under mu by the sweeper immediately before the entry is
	// deleted from the map. Counting against an evicted entry would silently
	// lose the count, so tryAllow refuses and the caller re-fetches.
	evicted bool
}

// allow locks the limiter and counts the request. Unlike tryAllow it ignores
// the evicted flag; use it only where the entry is known not to be shared
// with a sweeper (e.g. unit tests on a bare slidingWindowLimiter).
func (l *slidingWindowLimiter) allow(limit int, window time.Duration, now time.Time) (bool, int, time.Duration) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.allowLocked(limit, window, now)
}

// tryAllow counts the request unless the entry has been evicted from the
// limiter map, in which case valid is false and the caller must re-fetch the
// entry and try again.
func (l *slidingWindowLimiter) tryAllow(limit int, window time.Duration, now time.Time) (allowed bool, remaining int, reset time.Duration, valid bool) {
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.evicted {
		return false, 0, 0, false
	}
	allowed, remaining, reset = l.allowLocked(limit, window, now)
	return allowed, remaining, reset, true
}

// allowLocked implements the sliding-window decision. l.mu must be held.
func (l *slidingWindowLimiter) allowLocked(limit int, window time.Duration, now time.Time) (bool, int, time.Duration) {
	if l.windowStart.IsZero() {
		l.windowStart = now
	}

	// now may have been read before another goroutine created or rolled this
	// entry (each request reads the clock once, before the map lookup), so
	// elapsed can be slightly negative. Clamp it so overlap stays in [0, 1]
	// and reset never exceeds the window.
	elapsed := max(now.Sub(l.windowStart), 0)
	if elapsed >= window {
		// Roll the window forward, keeping alignment to window boundaries.
		if elapsed >= 2*window {
			l.prevCount = 0
		} else {
			l.prevCount = l.currCount
		}
		l.currCount = 0
		periods := elapsed / window
		l.windowStart = l.windowStart.Add(window * periods)
		elapsed = now.Sub(l.windowStart)
	}

	// Weight the previous window by its remaining overlap with a sliding
	// window ending now.
	overlap := 1 - float64(elapsed)/float64(window)
	estimated := int(float64(l.prevCount)*overlap) + l.currCount

	if estimated >= limit {
		// Denied: report how long until the current window rolls over.
		// The rollover above keeps elapsed within [0, window), so this is
		// always positive.
		return false, 0, window - elapsed
	}

	l.currCount++
	// estimated < limit here, so the remaining count is never negative.
	return true, limit - estimated - 1, 0
}

// getLimiter gets or creates the window counter stored under compositeKey.
func (u *UberRateLimiter) getLimiter(compositeKey string, window time.Duration, now time.Time) *slidingWindowLimiter {
	// Fast path: Check if limiter already exists.
	if limiter, ok := u.limiters.Load(compositeKey); ok {
		return limiter.(*slidingWindowLimiter)
	}

	// Slow path: atomically load or store a new counter. windowStart is
	// initialized to now so a concurrent sweep can never see the brand-new
	// entry as stale.
	actualLimiter, loaded := u.limiters.LoadOrStore(compositeKey, &slidingWindowLimiter{window: window, windowStart: now})
	if !loaded {
		// The map just grew — the only way it can accumulate garbage — so
		// this is the only place that schedules a sweep.
		u.maybeSweep(now)
	}
	return actualLimiter.(*slidingWindowLimiter)
}

// maybeSweep starts a sweep of stale entries unless one already started within
// the last limiterSweepInterval. The CAS allows at most one winner per
// interval; losing callers return immediately.
func (u *UberRateLimiter) maybeSweep(now time.Time) {
	last := u.lastSweep.Load()
	if now.UnixNano()-last < int64(limiterSweepInterval) {
		return
	}
	if !u.lastSweep.CompareAndSwap(last, now.UnixNano()) {
		return
	}
	go u.sweep(now)
}

// sweep deletes entries that can no longer influence any rate-limit decision.
// An entry is stale once it has been idle for at least two of its own windows:
// at that point the next request would zero both window counters anyway, so
// deleting the entry (and lazily recreating it on next use) is unobservable.
//
// The staleness check and the evicted flag are written under the same lock
// acquisition that tryAllow counts under, so no request can record a count
// between the decision to evict and the eviction itself: any request that
// counted before the check rolled windowStart forward (making the entry
// non-stale), and any request arriving after sees the evicted flag and
// re-fetches a fresh entry.
func (u *UberRateLimiter) sweep(now time.Time) {
	u.limiters.Range(func(key, value any) bool {
		l := value.(*slidingWindowLimiter)
		l.mu.Lock()
		stale := l.window > 0 && !l.windowStart.IsZero() && now.Sub(l.windowStart) >= 2*l.window
		if stale {
			l.evicted = true
		}
		l.mu.Unlock()
		if stale {
			u.limiters.Delete(key)
		}
		return true
	})
}

// Ensure UberRateLimiter implements the common.RateLimiter interface.
var _ common.RateLimiter = (*UberRateLimiter)(nil)

// Allow checks if a request is allowed based on the key and rate limit configuration.
// It implements the common.RateLimiter interface using a sliding-window counter.
// The check never blocks: over-limit requests are denied immediately.
//
// Parameters:
//   - key: Unique identifier for the rate limit bucket (e.g., "api:IP:192.168.1.1")
//   - limit: Maximum number of requests allowed within the window
//   - window: Time duration for the rate limit window
//
// Returns:
//   - allowed: true if the request is allowed, false if rate limit exceeded
//   - remaining: Estimated number of remaining requests in the current window
//   - reset: Duration until the next request will be allowed (0 if allowed now)
func (u *UberRateLimiter) Allow(key string, limit int, window time.Duration) (bool, int, time.Duration) {
	if limit <= 0 {
		// A non-positive limit allows nothing.
		return false, 0, window
	}
	if window <= 0 {
		window = time.Second
	}

	// The composite key includes limit and window so different rate limits
	// for the same base key don't share counters. Built with strconv instead
	// of fmt.Sprintf: this runs on every rate-limited request and Sprintf's
	// reflection is measurably slower.
	compositeKey := key + "|" + strconv.Itoa(limit) + "|" + strconv.FormatInt(int64(window), 10)

	now := u.timeNow()
	for {
		limiter := u.getLimiter(compositeKey, window, now)
		allowed, remaining, reset, valid := limiter.tryAllow(limit, window, now)
		if valid {
			return allowed, remaining, reset
		}
		// The sweeper marked this entry evicted between our lookup and use.
		// Its own Delete may not have landed yet, so remove the entry here —
		// CompareAndDelete is a no-op if the sweeper already removed it or a
		// fresh entry has replaced it — and retry with a fresh entry.
		u.limiters.CompareAndDelete(compositeKey, limiter)
	}
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
// If config.UserIDToString is nil, a default conversion (convertUserIDToString) is used,
// so StrategyUser works out of the box for common ID types.
// Returns an empty string if no user information is found.
func extractUserKey[T comparable, U any](r *http.Request, config *common.RateLimitConfig[T, U]) string { // Use common.RateLimitConfig
	userIDToString := config.UserIDToString
	if userIDToString == nil {
		userIDToString = convertUserIDToString[T]
	}

	// Try getting the full user object first. Extracting an ID from it
	// requires the UserIDFromUser function; without it, fall through to the
	// user ID from the context.
	user, userOk := scontext.GetUserFromRequest[T, U](r) // Use scontext
	if userOk && user != nil && config.UserIDFromUser != nil {
		userID := config.UserIDFromUser(*user)
		return userIDToString(userID)
	}

	// Fallback: Try getting the user ID directly from context
	userID, idOk := scontext.GetUserIDFromRequest[T, U](r) // Use scontext
	if idOk {
		// Use the conversion function
		return userIDToString(userID)
	}

	// No user information found in context
	return "" // Return empty key, let the caller decide how to handle (e.g., fallback to IP)
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
				key = extractUserKey(r, config)
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
