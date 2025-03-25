package middleware

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap"
)

// Define a custom type with String method for testing
type CustomID struct {
	id string
}

// String implements the fmt.Stringer interface
func (c CustomID) String() string {
	return c.id
}

// TestExtractUser tests the extractUser function with various scenarios
func TestExtractUser(t *testing.T) {

	// Test case 1: User object in context with UserIDFromUser and UserIDToString
	t.Run("with user object in context and both conversion functions", func(t *testing.T) {
		// Create a request with a user object in the context
		req := httptest.NewRequest("GET", "/", nil)
		user := "testUser"
		// Use the new context wrapper
		ctx := WithUser[string, string](req.Context(), &user)
		req = req.WithContext(ctx)

		// Create a config with UserIDFromUser function
		config := &RateLimitConfig[string, string]{
			UserIDFromUser: func(u string) string {
				return u
			},
			UserIDToString: func(id string) string {
				return id
			},
		}

		// Extract the user
		userID := extractUser(req, config)
		if userID != "testUser" {
			t.Errorf("Expected user ID 'testUser', got '%s'", userID)
		}
	})

	// Test case 1b: User object in context with only UserIDFromUser
	t.Run("with user object in context and only UserIDFromUser", func(t *testing.T) {
		// Create a request with a user object in the context
		req := httptest.NewRequest("GET", "/", nil)
		user := "testUser"
		// Use the new context wrapper
		ctx := WithUser[string, string](req.Context(), &user)
		req = req.WithContext(ctx)

		// Create a config with UserIDFromUser function but no UserIDToString
		config := &RateLimitConfig[string, string]{
			UserIDFromUser: func(u string) string {
				return u + "-modified"
			},
		}

		// Extract the user
		userID := extractUser(req, config)
		if userID != "testUser-modified" {
			t.Errorf("Expected user ID 'testUser-modified', got '%s'", userID)
		}
	})

	// Test case 1c: User object in context with UserIDFromUser returning int
	t.Run("with user object in context and UserIDFromUser returning int", func(t *testing.T) {
		// Create a request with a user object in the context
		req := httptest.NewRequest("GET", "/", nil)
		user := "testUser"
		// Use the new context wrapper
		ctx := WithUser[int, string](req.Context(), &user)
		req = req.WithContext(ctx)

		// Create a config with UserIDFromUser function returning int
		config := &RateLimitConfig[int, string]{
			UserIDFromUser: func(u string) int {
				return 42
			},
		}

		// Extract the user
		userID := extractUser(req, config)
		if userID != "42" {
			t.Errorf("Expected user ID '42', got '%s'", userID)
		}
	})

	// Test case 1d: User object in context with UserIDFromUser returning custom type with String method
	t.Run("with user object in context and UserIDFromUser returning custom type", func(t *testing.T) {
		// Create a request with a user object in the context
		req := httptest.NewRequest("GET", "/", nil)
		user := "testUser"
		// Use the new context wrapper
		ctx := WithUser[CustomID, string](req.Context(), &user)
		req = req.WithContext(ctx)

		// Create a CustomID instance
		customID := CustomID{id: "custom-id"}

		// Create a config with UserIDFromUser function returning CustomID
		config := &RateLimitConfig[CustomID, string]{
			UserIDFromUser: func(u string) CustomID {
				return customID
			},
		}

		// Extract the user
		userID := extractUser(req, config)
		// Since CustomID doesn't have a String method, it should use fmt.Sprint
		if userID != fmt.Sprint(customID) {
			t.Errorf("Expected user ID '%v', got '%s'", customID, userID)
		}
	})

	// Test case 2: User ID in context
	t.Run("with user ID in context", func(t *testing.T) {
		// Create a request with a user ID in the context
		req := httptest.NewRequest("GET", "/", nil)
		userID := "user123"
		ctx := WithUserID[string, string](req.Context(), userID)
		req = req.WithContext(ctx)

		// Create a config with UserIDToString function
		config := &RateLimitConfig[string, string]{
			UserIDToString: func(id string) string {
				return id + "-suffix"
			},
		}

		// Extract the user
		extractedID := extractUser(req, config)
		if extractedID != "user123-suffix" {
			t.Errorf("Expected user ID 'user123-suffix', got '%s'", extractedID)
		}
	})

	// Test case 3: User ID in context with default string conversion
	t.Run("with user ID in context and default string conversion", func(t *testing.T) {
		// Create a request with a user ID in the context
		req := httptest.NewRequest("GET", "/", nil)
		userID := "user123"
		ctx := WithUserID[string, string](req.Context(), userID)
		req = req.WithContext(ctx)

		// Create a config without UserIDToString function
		config := &RateLimitConfig[string, string]{}

		// Extract the user
		extractedID := extractUser(req, config)
		if extractedID != "user123" {
			t.Errorf("Expected user ID 'user123', got '%s'", extractedID)
		}
	})

	// Test case 4: Int user ID in context
	t.Run("with int user ID in context", func(t *testing.T) {
		// Create a request with an int user ID in the context
		req := httptest.NewRequest("GET", "/", nil)
		userID := 42
		ctx := WithUserID[int, string](req.Context(), userID)
		req = req.WithContext(ctx)

		// Create a config without UserIDToString function
		config := &RateLimitConfig[int, string]{}

		// Extract the user
		extractedID := extractUser(req, config)
		if extractedID != "42" {
			t.Errorf("Expected user ID '42', got '%s'", extractedID)
		}
	})

	// Test case 5: Bool user ID in context
	t.Run("with bool user ID in context", func(t *testing.T) {
		// Create a request with a bool user ID in the context
		req := httptest.NewRequest("GET", "/", nil)
		userID := true
		ctx := WithUserID[bool, string](req.Context(), userID)
		req = req.WithContext(ctx)

		// Create a config without UserIDToString function
		config := &RateLimitConfig[bool, string]{}

		// Extract the user
		extractedID := extractUser(req, config)
		if extractedID != "true" {
			t.Errorf("Expected user ID 'true', got '%s'", extractedID)
		}
	})

	// Test case 6: No user in context
	t.Run("with no user in context", func(t *testing.T) {
		// Create a request without a user in the context
		req := httptest.NewRequest("GET", "/", nil)

		// Create a config
		config := &RateLimitConfig[string, string]{}

		// Extract the user
		extractedID := extractUser(req, config)
		if extractedID != "" {
			t.Errorf("Expected empty user ID, got '%s'", extractedID)
		}
	})
}

// TestRateLimit tests the RateLimit function with various scenarios
func TestRateLimit(t *testing.T) {
	// Create a test logger
	logger := zap.NewNop()

	// Create a mock rate limiter
	mockLimiter := &MockRateLimiter{
		allowFunc: func(key string, limit int, window time.Duration) (bool, int, time.Duration) {
			// Allow all requests except those with key "bucket:blocked"
			if key == "bucket:blocked" {
				return false, 0, time.Duration(1000) * time.Millisecond
			}
			return true, 10, time.Duration(1000) * time.Millisecond
		},
	}

	// Test case 1: IP strategy
	t.Run("with IP strategy", func(t *testing.T) {
		// Create a config with IP strategy
		config := &RateLimitConfig[string, string]{
			BucketName: "bucket",
			Limit:      10,
			Window:     time.Duration(1000) * time.Millisecond,
			Strategy:   StrategyIP,
		}

		// Create the middleware
		middleware := RateLimit(config, mockLimiter, logger)

		// Create a test handler
		handlerCalled := false
		testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true
			w.WriteHeader(http.StatusOK)
		})

		// Apply the middleware
		handler := middleware(testHandler)

		// Create a request with X-Forwarded-For header
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("X-Forwarded-For", "192.168.1.1")
		rr := httptest.NewRecorder()

		// Call the handler
		handler.ServeHTTP(rr, req)

		// Check that the handler was called
		if !handlerCalled {
			t.Error("Handler was not called")
		}

		// Check the response headers
		if rr.Header().Get("X-RateLimit-Limit") != "10" {
			t.Errorf("Expected X-RateLimit-Limit header to be '10', got '%s'", rr.Header().Get("X-RateLimit-Limit"))
		}
	})

	// Test case 2: User strategy with user ID in context
	t.Run("with User strategy and user ID in context", func(t *testing.T) {
		// Create a config with User strategy
		config := &RateLimitConfig[string, string]{
			BucketName: "bucket",
			Limit:      10,
			Window:     time.Duration(1000) * time.Millisecond,
			Strategy:   StrategyUser,
			UserIDToString: func(id string) string {
				return id
			},
		}

		// Create the middleware
		middleware := RateLimit(config, mockLimiter, logger)

		// Create a test handler
		handlerCalled := false
		testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true
			w.WriteHeader(http.StatusOK)
		})

		// Apply the middleware
		handler := middleware(testHandler)

		// Create a request with a user ID in the context
		req := httptest.NewRequest("GET", "/", nil)
		userID := "user123"
		ctx := WithUserID[string, string](req.Context(), userID)
		req = req.WithContext(ctx)
		rr := httptest.NewRecorder()

		// Call the handler
		handler.ServeHTTP(rr, req)

		// Check that the handler was called
		if !handlerCalled {
			t.Error("Handler was not called")
		}
	})

	// Test case 3: User strategy with no user ID in context (falls back to IP)
	t.Run("with User strategy and no user ID in context", func(t *testing.T) {
		// Create a config with User strategy
		config := &RateLimitConfig[string, string]{
			BucketName: "bucket",
			Limit:      10,
			Window:     time.Duration(1000) * time.Millisecond,
			Strategy:   StrategyUser,
		}

		// Create the middleware
		middleware := RateLimit(config, mockLimiter, logger)

		// Create a test handler
		handlerCalled := false
		testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true
			w.WriteHeader(http.StatusOK)
		})

		// Apply the middleware
		handler := middleware(testHandler)

		// Create a request without a user ID in the context
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("X-Forwarded-For", "192.168.1.1")
		rr := httptest.NewRecorder()

		// Call the handler
		handler.ServeHTTP(rr, req)

		// Check that the handler was called
		if !handlerCalled {
			t.Error("Handler was not called")
		}
	})

	// Test case 4: Custom strategy with key extractor
	t.Run("with Custom strategy and key extractor", func(t *testing.T) {
		// Create a config with Custom strategy
		config := &RateLimitConfig[string, string]{
			BucketName: "bucket",
			Limit:      10,
			Window:     time.Duration(1000) * time.Millisecond,
			Strategy:   StrategyCustom,
			KeyExtractor: func(r *http.Request) (string, error) {
				return "custom-key", nil
			},
		}

		// Create the middleware
		middleware := RateLimit(config, mockLimiter, logger)

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

		// Check that the handler was called
		if !handlerCalled {
			t.Error("Handler was not called")
		}
	})

	// Test case 5: Custom strategy without key extractor (falls back to IP)
	t.Run("with Custom strategy and no key extractor", func(t *testing.T) {
		// Create a config with Custom strategy but no key extractor
		config := &RateLimitConfig[string, string]{
			BucketName: "bucket",
			Limit:      10,
			Window:     time.Duration(1000) * time.Millisecond,
			Strategy:   StrategyCustom,
		}

		// Create the middleware
		middleware := RateLimit(config, mockLimiter, logger)

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
		req.Header.Set("X-Forwarded-For", "192.168.1.1")
		rr := httptest.NewRecorder()

		// Call the handler
		handler.ServeHTTP(rr, req)

		// Check that the handler was called
		if !handlerCalled {
			t.Error("Handler was not called")
		}
	})

	// Test case 6: Rate limit exceeded
	t.Run("with rate limit exceeded", func(t *testing.T) {
		// Create a config that will trigger the rate limit
		config := &RateLimitConfig[string, string]{
			BucketName: "bucket",
			Limit:      10,
			Window:     time.Duration(1000) * time.Millisecond,
			Strategy:   StrategyIP,
		}

		// Create the middleware
		middleware := RateLimit(config, mockLimiter, logger)

		// Create a test handler
		handlerCalled := false
		testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true
			w.WriteHeader(http.StatusOK)
		})

		// Apply the middleware
		handler := middleware(testHandler)

		// Create a request that will be blocked
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("X-Forwarded-For", "blocked")
		rr := httptest.NewRecorder()

		// Call the handler
		handler.ServeHTTP(rr, req)

		// Check that the handler was not called
		if handlerCalled {
			t.Error("Handler was called when it should have been blocked")
		}

		// Check the response status code
		if rr.Code != http.StatusTooManyRequests {
			t.Errorf("Expected status code %d, got %d", http.StatusTooManyRequests, rr.Code)
		}

		// Check the Retry-After header
		if rr.Header().Get("Retry-After") == "" {
			t.Error("Expected Retry-After header to be set")
		}
	})

	// Test case 7: Rate limit exceeded with custom handler
	t.Run("with rate limit exceeded and custom handler", func(t *testing.T) {
		// Create a custom handler for rate limit exceeded
		customHandlerCalled := false
		customHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			customHandlerCalled = true
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte("Custom rate limit exceeded"))
		})

		// Create a config that will trigger the rate limit
		config := &RateLimitConfig[string, string]{
			BucketName:      "bucket",
			Limit:           10,
			Window:          time.Duration(1000) * time.Millisecond,
			Strategy:        StrategyIP,
			ExceededHandler: customHandler,
		}

		// Create the middleware
		middleware := RateLimit(config, mockLimiter, logger)

		// Create a test handler
		handlerCalled := false
		testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true
			w.WriteHeader(http.StatusOK)
		})

		// Apply the middleware
		handler := middleware(testHandler)

		// Create a request that will be blocked
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("X-Forwarded-For", "blocked")
		rr := httptest.NewRecorder()

		// Call the handler
		handler.ServeHTTP(rr, req)

		// Check that the handler was not called
		if handlerCalled {
			t.Error("Handler was called when it should have been blocked")
		}

		// Check that the custom handler was called
		if !customHandlerCalled {
			t.Error("Custom handler was not called")
		}

		// Check the response status code
		if rr.Code != http.StatusForbidden {
			t.Errorf("Expected status code %d, got %d", http.StatusForbidden, rr.Code)
		}

		// Check the response body
		if rr.Body.String() != "Custom rate limit exceeded" {
			t.Errorf("Expected response body '%s', got '%s'", "Custom rate limit exceeded", rr.Body.String())
		}
	})

	// Test case 8: Nil config
	t.Run("with nil config", func(t *testing.T) {
		// Create the middleware with nil config
		middleware := RateLimit[string, string](nil, mockLimiter, logger)

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

		// Check that the handler was called
		if !handlerCalled {
			t.Error("Handler was not called")
		}

		// Check the response status code
		if rr.Code != http.StatusOK {
			t.Errorf("Expected status code %d, got %d", http.StatusOK, rr.Code)
		}
	})
}

// MockRateLimiter is a mock implementation of the RateLimiter interface for testing
type MockRateLimiter struct {
	allowFunc func(key string, limit int, window time.Duration) (bool, int, time.Duration)
}

// Allow implements the RateLimiter interface
func (m *MockRateLimiter) Allow(key string, limit int, window time.Duration) (bool, int, time.Duration) {
	return m.allowFunc(key, limit, window)
}
