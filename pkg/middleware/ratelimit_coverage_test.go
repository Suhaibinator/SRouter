package middleware

import (
	"fmt"
	"net/http"          // Added import
	"net/http/httptest" // Added import

	// Added import
	"sync" // Added import
	"testing"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/common"   // Added import
	"github.com/Suhaibinator/SRouter/pkg/scontext" // Added import
	"github.com/stretchr/testify/assert"
	"go.uber.org/ratelimit" // Added import for assert.Same
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// TestNewUberRateLimiter tests the NewUberRateLimiter function
func TestNewUberRateLimiter_Coverage(t *testing.T) {
	// Create a new UberRateLimiter
	limiter := NewUberRateLimiter()

	// Verify that the limiter is not nil
	if limiter == nil {
		t.Errorf("Expected limiter to be non-nil")
	}

	// Test the Allow method with a simple case
	allowed, _, _ := limiter.Allow("test-key", 10, time.Millisecond)
	if !allowed {
		t.Errorf("Expected request to be allowed, but it was denied")
	}
}

// TestGetLimiter_Coverage tests the getLimiter function indirectly through the Allow method, including concurrency.
func TestGetLimiter_Coverage(t *testing.T) {
	limiter := NewUberRateLimiter()
	key := "concurrent-get-limiter-key"
	limit := 1000
	window := 10 * time.Millisecond
	// Calculate rps the same way Allow does
	rps := int(float64(limit) / window.Seconds())

	numGoroutines := 20
	callsPerGoroutine := 5

	var wg sync.WaitGroup
	wg.Add(numGoroutines)

	// Use a map to store the first limiter instance seen by any goroutine
	var firstLimiter ratelimit.Limiter
	var once sync.Once

	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < callsPerGoroutine; j++ {
				// Call Allow which will internally call getLimiter
				allowed, _, _ := limiter.Allow(key, limit, window)
				assert.True(t, allowed, "Request should be allowed initially")

				// Check if the same limiter instance is returned across goroutines
				// This indirectly tests that the double-check prevents creating multiple limiters
				compositeKey := fmt.Sprintf("%s-%d", key, rps)
				val, ok := limiter.limiters.Load(compositeKey)
				// Explicitly check 'ok' before proceeding to prevent panic on nil interface conversion
				if !assert.True(t, ok, "Limiter should exist in the map for key %s", compositeKey) {
					// If the assertion fails (ok is false), skip the rest of the checks for this iteration
					// as val will be nil, causing a panic on type assertion.
					// This indicates a potential timing issue or problem in limiter creation/storage.
					continue // Skip to the next iteration of the inner loop
				}
				// Only proceed if ok is true
				currentLimiter := val.(ratelimit.Limiter)

				once.Do(func() {
					firstLimiter = currentLimiter // Capture the first successfully retrieved limiter
				})

				// Use assert.Same to check if it's the exact same instance in memory
				assert.Same(t, firstLimiter, currentLimiter, "Expected the same limiter instance to be returned")
			}
		}()
	}

	wg.Wait()

	// Final check that only one limiter was created for the key/rps combination
	count := 0
	limiter.limiters.Range(func(k, v interface{}) bool {
		if k == fmt.Sprintf("%s-%d", key, rps) {
			count++
		}
		return true
	})
	assert.Equal(t, 1, count, "Expected exactly one limiter instance for the key/rps")
}

// TestCreateRateLimitMiddleware_Coverage tests the CreateRateLimitMiddleware function
func TestCreateRateLimitMiddleware_Coverage(t *testing.T) {
	// Create a logger
	logger := zap.NewNop()

	// Create a rate limit middleware
	middleware := CreateRateLimitMiddleware(
		"test-bucket",
		10,
		time.Minute,
		common.StrategyIP, // Use common.StrategyIP
		func(user string) string { return user },
		func(userID string) string { return userID },
		logger,
	)

	// Verify that the middleware is not nil
	if middleware == nil {
		t.Errorf("Expected middleware to be non-nil")
	}

	// Create a test handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	// Wrap the handler with the middleware
	wrappedHandler := middleware(handler)

	// Create a test request
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.1:1234"

	// Test the middleware with a simple case
	w := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(w, req)

	// Verify that the request was allowed
	if w.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
	}
}

// TestRateLimitWithCustomKeyExtractor tests the RateLimit function with a custom key extractor
func TestRateLimitWithCustomKeyExtractor(t *testing.T) {
	// Create a logger
	logger := zap.NewNop()

	// Create a rate limiter
	limiter := NewUberRateLimiter()

	// Create a custom key extractor
	keyExtractor := func(r *http.Request) (string, error) {
		return "custom-key", nil
	}

	// Create a rate limit config with custom key extractor
	config := &common.RateLimitConfig[string, string]{ // Use common.RateLimitConfig
		BucketName:   "test-bucket",
		Limit:        10,
		Window:       time.Minute,
		Strategy:     common.StrategyCustom, // Use common.StrategyCustom
		KeyExtractor: keyExtractor,
	}

	// Create a test handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	// Create the middleware
	middleware := RateLimit(config, limiter, logger)

	// Wrap the handler with the middleware
	wrappedHandler := middleware(handler)

	// Create a test request
	req := httptest.NewRequest("GET", "/", nil)

	// Test the middleware with a simple case
	w := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(w, req)

	// Verify that the request was allowed
	if w.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
	}
}

// TestRateLimitWithUserStrategy tests the RateLimit function with user strategy
func TestRateLimitWithUserStrategy_Coverage(t *testing.T) {
	// Create a logger
	logger := zap.NewNop()

	// Create a rate limiter
	limiter := NewUberRateLimiter()

	// Create a rate limit config with user strategy
	config := &common.RateLimitConfig[string, string]{ // Use common.RateLimitConfig
		BucketName:     "test-bucket",
		Limit:          10,
		Window:         time.Minute,
		Strategy:       common.StrategyUser, // Use common.StrategyUser
		UserIDFromUser: func(user string) string { return user },
		UserIDToString: func(userID string) string { return userID },
	}

	// Create a test handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	// Create the middleware
	middleware := RateLimit(config, limiter, logger)

	// Wrap the handler with the middleware
	wrappedHandler := middleware(handler)

	// Create a test request with user in context
	req := httptest.NewRequest("GET", "/", nil)
	ctx := scontext.WithUserID[string, string](req.Context(), "user123") // Use scontext
	req = req.WithContext(ctx)

	// Test the middleware with a simple case
	w := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(w, req)

	// Verify that the request was allowed
	if w.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
	}
}

// --- New Coverage Tests ---

// TestUberRateLimiter_RemainingNegativeCoverage tests the case where remaining calculation might go negative
func TestUberRateLimiter_RemainingNegativeCoverage(t *testing.T) {
	limiter := NewUberRateLimiter()
	key := "remaining-neg-test"
	limit := 1            // Very low limit
	window := time.Second // 1s window (RPS = 1)

	// First call should succeed
	allowed, remaining, _ := limiter.Allow(key, limit, window)
	assert.True(t, allowed, "First call should be allowed")
	assert.Equal(t, 0, remaining, "Remaining should be 0 after first call with limit 1") // Uber limiter might return 0 immediately

	// Second call immediately after should be denied, and waitTime > window
	// We need to simulate time passing slightly for Take() to potentially return a future time
	time.Sleep(10 * time.Millisecond) // Small sleep
	allowed, remaining, reset := limiter.Allow(key, limit, window)
	assert.False(t, allowed, "Second call should be denied")
	// Assert that remaining is capped at 0, covering the `if remaining < 0` block
	assert.Equal(t, 0, remaining, "Remaining should be 0 when denied")
	assert.Greater(t, reset.Nanoseconds(), int64(0), "Reset time should be positive when denied")
}

// TestRateLimit_NilLimiterPanic tests that RateLimit panics if the limiter is nil
func TestRateLimit_NilLimiterPanic(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Errorf("Expected RateLimit to panic with nil limiter, but it did not")
		} else {
			// Optionally check the panic message
			assert.Contains(t, fmt.Sprintf("%v", r), "RateLimit middleware requires a non-nil RateLimiter")
		}
	}()

	config := &common.RateLimitConfig[string, string]{
		BucketName: "test",
		Limit:      10,
		Window:     time.Minute,
		Strategy:   common.StrategyIP,
	}
	logger := zap.NewNop()

	// Call RateLimit with nil limiter - this should panic
	_ = RateLimit[string, string](config, nil, logger)
}

// TestRateLimit_NilLogger tests that RateLimit uses a Nop logger if nil is provided
func TestRateLimit_NilLogger(t *testing.T) {
	config := &common.RateLimitConfig[string, string]{
		BucketName: "test",
		Limit:      10,
		Window:     time.Minute,
		Strategy:   common.StrategyIP,
	}
	limiter := NewUberRateLimiter() // Use a real limiter

	// Call RateLimit with nil logger - should not panic
	middleware := RateLimit[string, string](config, limiter, nil) // Pass nil logger

	// Create a simple handler and apply the middleware
	handlerCalled := false
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})
	wrappedHandler := middleware(testHandler)

	// Make a request
	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "127.0.0.1:1234" // Provide RemoteAddr for fallback
	rr := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(rr, req)

	// Assert that the request was processed successfully
	assert.True(t, handlerCalled, "Handler should have been called")
	assert.Equal(t, http.StatusOK, rr.Code, "Expected status OK")
}

// TestRateLimit_UserStrategyFallback tests the fallback logic for StrategyUser when no user key is found
func TestRateLimit_UserStrategyFallback(t *testing.T) {
	limiter := NewUberRateLimiter()

	// Sub-test 1: Fallback to ClientIP when available
	t.Run("Fallback to ClientIP", func(t *testing.T) {
		core, observedLogs := observer.New(zapcore.InfoLevel) // Capture Info level logs
		logger := zap.New(core)

		config := &common.RateLimitConfig[string, string]{
			BucketName:     "user-fallback",
			Limit:          10,
			Window:         time.Minute,
			Strategy:       common.StrategyUser,
			UserIDToString: func(id string) string { return id }, // Need this for extractUserKey
		}
		middleware := RateLimit(config, limiter, logger)
		handlerCalled := false
		testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true
			w.WriteHeader(http.StatusOK)
		})
		wrappedHandler := middleware(testHandler)

		// Request without user context, but with ClientIP context
		req := httptest.NewRequest("GET", "/", nil)
		ctx := scontext.WithClientIP[string, string](req.Context(), "192.168.1.100")
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()
		wrappedHandler.ServeHTTP(rr, req)

		assert.True(t, handlerCalled, "Handler should have been called")
		assert.Equal(t, http.StatusOK, rr.Code, "Expected status OK")

		// Check logs for fallback message
		logEntries := observedLogs.FilterMessage("User key not found, falling back to ClientIP from context for rate limiting.").All()
		assert.Equal(t, 1, len(logEntries), "Expected one log entry for ClientIP fallback")
		if len(logEntries) > 0 {
			foundIP := false
			for _, field := range logEntries[0].Context {
				if field.Key == "client_ip" && field.String == "192.168.1.100" {
					foundIP = true
					break
				}
			}
			assert.True(t, foundIP, "Expected log context to contain correct client_ip")
		}
	})

	// Sub-test 2: Fallback to RemoteAddr when ClientIP is not available
	t.Run("Fallback to RemoteAddr", func(t *testing.T) {
		core, observedLogs := observer.New(zapcore.WarnLevel) // Capture Warn level logs
		logger := zap.New(core)

		config := &common.RateLimitConfig[string, string]{
			BucketName:     "user-fallback",
			Limit:          10,
			Window:         time.Minute,
			Strategy:       common.StrategyUser,
			UserIDToString: func(id string) string { return id }, // Need this for extractUserKey
		}
		middleware := RateLimit(config, limiter, logger)
		handlerCalled := false
		testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true
			w.WriteHeader(http.StatusOK)
		})
		wrappedHandler := middleware(testHandler)

		// Request without user context and without ClientIP context
		req := httptest.NewRequest("GET", "/", nil)
		req.RemoteAddr = "10.0.0.1:54321" // Set RemoteAddr
		rr := httptest.NewRecorder()
		wrappedHandler.ServeHTTP(rr, req)

		assert.True(t, handlerCalled, "Handler should have been called")
		assert.Equal(t, http.StatusOK, rr.Code, "Expected status OK")

		// Check logs for fallback message
		logEntries := observedLogs.FilterMessage("User key not found, falling back to RemoteAddr for rate limiting.").All()
		assert.Equal(t, 1, len(logEntries), "Expected one log entry for RemoteAddr fallback")
		if len(logEntries) > 0 {
			foundAddr := false
			for _, field := range logEntries[0].Context {
				if field.Key == "remote_addr" && field.String == "10.0.0.1:54321" {
					foundAddr = true
					break
				}
			}
			assert.True(t, foundAddr, "Expected log context to contain correct remote_addr")
		}
	})
}

// TestRateLimit_CustomStrategyEmptyKey tests the case where the custom key extractor returns an empty string
func TestRateLimit_CustomStrategyEmptyKey(t *testing.T) {
	core, observedLogs := observer.New(zapcore.ErrorLevel) // Capture Error level logs
	logger := zap.New(core)
	limiter := NewUberRateLimiter() // Use a real limiter

	config := &common.RateLimitConfig[string, string]{
		BucketName: "custom-empty",
		Limit:      10,
		Window:     time.Minute,
		Strategy:   common.StrategyCustom,
		KeyExtractor: func(r *http.Request) (string, error) {
			return "", nil // Return empty key
		},
	}
	middleware := RateLimit(config, limiter, logger)
	handlerCalled := false
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})
	wrappedHandler := middleware(testHandler)

	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(rr, req)

	assert.False(t, handlerCalled, "Handler should not have been called")
	assert.Equal(t, http.StatusInternalServerError, rr.Code, "Expected status 500 for empty custom key")

	// Check logs for the specific error message
	logEntries := observedLogs.FilterMessage("Custom KeyExtractor returned an empty key.").All()
	assert.Equal(t, 1, len(logEntries), "Expected one log entry for empty custom key error")
}

// MockRateLimiterWithReset is a mock implementation allowing control over the reset duration
type MockRateLimiterWithReset struct {
	allowFunc func(key string, limit int, window time.Duration) (bool, int, time.Duration)
}

// Allow implements the RateLimiter interface
func (m *MockRateLimiterWithReset) Allow(key string, limit int, window time.Duration) (bool, int, time.Duration) {
	return m.allowFunc(key, limit, window)
}

// TestRateLimit_RetryAfterMinimum tests the minimum value for the Retry-After header
func TestRateLimit_RetryAfterMinimum(t *testing.T) {
	logger := zap.NewNop()
	// Mock limiter that denies the request and returns a reset duration less than 1 second
	mockLimiter := &MockRateLimiterWithReset{
		allowFunc: func(key string, limit int, window time.Duration) (bool, int, time.Duration) {
			return false, 0, 500 * time.Millisecond // Return reset duration < 1s
		},
	}

	config := &common.RateLimitConfig[string, string]{
		BucketName: "retry-test",
		Limit:      1,
		Window:     time.Minute,
		Strategy:   common.StrategyIP,
	}
	middleware := RateLimit(config, mockLimiter, logger)
	handlerCalled := false
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})
	wrappedHandler := middleware(testHandler)

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "127.0.0.1:9999" // Use RemoteAddr as fallback
	rr := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(rr, req)

	assert.False(t, handlerCalled, "Handler should not have been called")
	assert.Equal(t, http.StatusTooManyRequests, rr.Code, "Expected status 429")

	// Check that Retry-After header is set to the minimum of 1 second
	retryAfter := rr.Header().Get("Retry-After")
	assert.Equal(t, "1", retryAfter, "Expected Retry-After header to be 1")
}
