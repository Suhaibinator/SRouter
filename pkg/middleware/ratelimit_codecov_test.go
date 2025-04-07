package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/common"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// TestUberRateLimiter_AllowedCondition specifically tests line 89 in ratelimit.go
// where the allowed condition is determined by waitTime <= time.Millisecond
func TestUberRateLimiter_AllowedCondition(t *testing.T) {
	// Create a real UberRateLimiter
	limiter := NewUberRateLimiter()

	// Test case 1: Create a scenario where waitTime is less than 1ms (should be allowed)
	t.Run("wait time less than 1ms", func(t *testing.T) {
		// Use a high limit and large window to ensure the request is allowed
		// This should result in waitTime <= time.Millisecond
		allowed, remaining, reset := limiter.Allow("test-key-allowed", 1000, 1*time.Second)

		// Verify the request is allowed (line 89 condition is true)
		assert.True(t, allowed, "Request should be allowed with high limit")
		assert.Greater(t, remaining, 0, "Remaining should be positive")
		assert.LessOrEqual(t, reset, time.Millisecond, "Reset time should be negligible")
	})

	// Test case 2: Create a scenario where waitTime is greater than 1ms (should be denied)
	t.Run("wait time greater than 1ms", func(t *testing.T) {
		// First, exhaust the limiter by making multiple requests with a very low limit
		key := "test-key-denied"
		limit := 1
		window := 1 * time.Second

		// Make the first request to consume the only token
		limiter.Allow(key, limit, window)

		// Make a second request immediately - this should be denied
		// The waitTime should be > 1ms, triggering the condition on line 89
		allowed, remaining, reset := limiter.Allow(key, limit, window)

		// Verify the request is denied (line 89 condition is false)
		assert.False(t, allowed, "Request should be denied after limit is reached")
		assert.Equal(t, 0, remaining, "Remaining should be 0")
		assert.Greater(t, reset, time.Millisecond, "Reset time should be greater than 1ms")
	})
}

// TestExtractUserKey_NilUserIDToString specifically tests lines 126-127 in ratelimit.go
// where it checks if UserIDToString is nil and returns an error
func TestExtractUserKey_NilUserIDToString_Codecov(t *testing.T) {
	// Create a request
	req := httptest.NewRequest("GET", "/", nil)

	// Create a config with nil UserIDToString
	config := &common.RateLimitConfig[string, string]{
		Strategy:       common.StrategyUser,
		UserIDFromUser: func(u string) string { return u },
		UserIDToString: nil, // Explicitly set to nil to test the nil check
	}

	// Call extractUserKey
	key, err := extractUserKey(req, config)

	// Verify that an error is returned
	assert.Error(t, err, "extractUserKey should return an error when UserIDToString is nil")
	assert.Equal(t, "", key, "Key should be empty when error is returned")
	assert.Contains(t, err.Error(), "UserIDToString function is required",
		"Error message should indicate UserIDToString is required")
}

// TestRateLimit_CustomStrategyEmptyKey_Codecov specifically tests lines 258-259 in ratelimit.go
// where it checks if the custom key extractor returned an empty key
func TestRateLimit_CustomStrategyEmptyKey_Codecov(t *testing.T) {
	// Create a logger with an observer to capture logs
	core, observedLogs := observer.New(zapcore.ErrorLevel)
	logger := zap.New(core)

	// Create a rate limiter
	limiter := NewUberRateLimiter()

	// Create a config with a custom key extractor that returns an empty string
	config := &common.RateLimitConfig[string, string]{
		BucketName: "test-bucket",
		Limit:      10,
		Window:     time.Second,
		Strategy:   common.StrategyCustom,
		KeyExtractor: func(r *http.Request) (string, error) {
			return "", nil // Return empty key with no error
		},
	}

	// Create the middleware
	middleware := RateLimit(config, limiter, logger)

	// Create a test handler
	handlerCalled := false
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	// Apply the middleware
	handler := middleware(testHandler)

	// Create a request
	req := httptest.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()

	// Call the handler
	handler.ServeHTTP(rr, req)

	// Verify that the handler was not called
	assert.False(t, handlerCalled, "Handler should not have been called")

	// Verify that the response status is 500 Internal Server Error
	assert.Equal(t, http.StatusInternalServerError, rr.Code, "Expected status 500 for empty custom key")

	// Verify that the error was logged
	logs := observedLogs.FilterMessage("Custom KeyExtractor returned an empty key.").All()
	assert.Equal(t, 1, len(logs), "Expected one log entry for empty custom key error")

	// Verify the response contains the expected error message
	assert.Contains(t, rr.Body.String(), "Internal Server Error: Rate limit key error",
		"Response should contain the expected error message")
}
