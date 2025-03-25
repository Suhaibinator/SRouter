package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap"
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

// TestGetLimiter tests the getLimiter function indirectly through the Allow method
func TestGetLimiter_Coverage(t *testing.T) {
	// Create a new UberRateLimiter
	limiter := NewUberRateLimiter()

	// Call Allow which will internally call getLimiter
	allowed, remaining, _ := limiter.Allow("test-key", 10, time.Millisecond)

	// Verify that the request was allowed
	if !allowed {
		t.Errorf("Expected request to be allowed, but it was denied")
	}

	// Verify that the remaining count is reasonable
	if remaining < 0 {
		t.Errorf("Expected remaining to be non-negative, got %d", remaining)
	}
}

// TestCreateRateLimitMiddleware_Coverage tests the CreateRateLimitMiddleware function
func TestCreateRateLimitMiddleware_Coverage(t *testing.T) {
	// Create a logger
	logger := zap.NewNop()

	// Create a rate limit middleware
	middleware := CreateRateLimitMiddleware[string, string](
		"test-bucket",
		10,
		time.Minute,
		StrategyIP,
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
	config := &RateLimitConfig[string, string]{
		BucketName:   "test-bucket",
		Limit:        10,
		Window:       time.Minute,
		Strategy:     StrategyCustom,
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
	config := &RateLimitConfig[string, string]{
		BucketName:     "test-bucket",
		Limit:          10,
		Window:         time.Minute,
		Strategy:       StrategyUser,
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
	ctx := WithUserID[string, string](req.Context(), "user123")
	req = req.WithContext(ctx)

	// Test the middleware with a simple case
	w := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(w, req)

	// Verify that the request was allowed
	if w.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
	}
}
