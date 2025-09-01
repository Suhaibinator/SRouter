package middleware

import (
	"errors" // Need to import errors package
	"fmt"
	"io"
	"net/http" // Import http
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/common"   // Import common for RateLimitConfig etc.
	"github.com/Suhaibinator/SRouter/pkg/scontext" // Import scontext
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestUberRateLimiter(t *testing.T) {
	// Create a new UberRateLimiter
	limiter := NewUberRateLimiter()

	// Test parameters
	key := "test-key"
	limit := 3
	window := 100 * time.Millisecond

	// Test basic functionality
	allowed, remaining, _ := limiter.Allow(key, limit, window)
	if !allowed {
		t.Errorf("Expected request to be allowed, but it was denied")
	}
	if remaining <= 0 {
		t.Errorf("Expected remaining to be positive, got %d", remaining)
	}

	// Test that the limiter is reusing the same limiter for the same key and rps
	limiter.Allow(key, limit, window)
	// Calculate expected RPS and composite key
	rps1 := int(float64(limit) / window.Seconds())
	if rps1 < 1 {
		rps1 = 1
	}
	compositeKey1 := fmt.Sprintf("%s-%d", key, rps1)
	_, exists := limiter.limiters.Load(compositeKey1)
	if !exists {
		t.Errorf("Expected limiter to be stored for composite key %s", compositeKey1)
	}

	// Test with a different key
	otherKey := "other-key"
	allowed, remaining, _ = limiter.Allow(otherKey, limit, window)
	if !allowed {
		t.Errorf("Expected request with different key to be allowed, but it was denied")
	}
	if remaining <= 0 {
		t.Errorf("Expected remaining to be positive, got %d", remaining)
	}

	// Test that the limiter is storing different limiters for different keys (with the same rps)
	// Calculate expected RPS and composite key for the other key
	rps2 := int(float64(limit) / window.Seconds()) // Same limit/window as first test
	if rps2 < 1 {
		rps2 = 1
	}
	compositeKey2 := fmt.Sprintf("%s-%d", otherKey, rps2)
	_, exists = limiter.limiters.Load(compositeKey2)
	if !exists {
		t.Errorf("Expected limiter to be stored for composite key %s", compositeKey2)
	}

	// Test with different limit and window
	differentLimit := 10
	differentWindow := 500 * time.Millisecond
	allowed, remaining, _ = limiter.Allow(key, differentLimit, differentWindow)
	if !allowed {
		t.Errorf("Expected request with different limit/window to be allowed, but it was denied")
	}
	if remaining <= 0 {
		t.Errorf("Expected remaining to be positive, got %d", remaining)
	}

	// Also test that the new limiter with different rps is stored
	rps3 := int(float64(differentLimit) / differentWindow.Seconds())
	if rps3 < 1 {
		rps3 = 1
	}
	compositeKey3 := fmt.Sprintf("%s-%d", key, rps3)
	_, exists = limiter.limiters.Load(compositeKey3)
	if !exists {
		t.Errorf("Expected limiter to be stored for composite key %s (different limit/window)", compositeKey3)
	}
}

type captureLimiter struct {
	lastKey string
}

func (c *captureLimiter) Allow(key string, limit int, window time.Duration) (bool, int, time.Duration) {
	c.lastKey = key
	return true, limit, 0
}

func TestRateLimitExtractIP(t *testing.T) {
	logger := zap.NewNop()

	t.Run("uses IP from context", func(t *testing.T) {
		limiter := &captureLimiter{}
		config := &common.RateLimitConfig[string, any]{
			BucketName: "test-bucket",
			Limit:      1,
			Window:     time.Second,
			Strategy:   common.StrategyIP,
		}

		middleware := RateLimit(config, limiter, logger)
		handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)
		req.RemoteAddr = "10.0.0.1:1234" // ensure remote addr differs from context IP
		ctx := scontext.WithClientIP[string, any](req.Context(), "203.0.113.9")

		handler.ServeHTTP(httptest.NewRecorder(), req.WithContext(ctx))

		expectedKey := "test-bucket:203.0.113.9"
		if limiter.lastKey != expectedKey {
			t.Fatalf("expected limiter key %s, got %s", expectedKey, limiter.lastKey)
		}
	})

	t.Run("falls back to RemoteAddr", func(t *testing.T) {
		limiter := &captureLimiter{}
		config := &common.RateLimitConfig[string, any]{
			BucketName: "test-bucket",
			Limit:      1,
			Window:     time.Second,
			Strategy:   common.StrategyIP,
		}

		middleware := RateLimit(config, limiter, logger)
		handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}))

		req := httptest.NewRequest(http.MethodGet, "http://example.com", nil)
		req.RemoteAddr = "10.0.0.2:4321"

		handler.ServeHTTP(httptest.NewRecorder(), req)

		expectedKey := "test-bucket:10.0.0.2:4321"
		if limiter.lastKey != expectedKey {
			t.Fatalf("expected limiter key %s, got %s", expectedKey, limiter.lastKey)
		}
	})
}

// TestConvertUserIDToString tests the internal helper function (still present in ratelimit.go)
func TestConvertUserIDToString(t *testing.T) {
	// Test with string
	str := convertUserIDToString("user123")
	if str != "user123" {
		t.Errorf("Expected string to be user123, got %s", str)
	}

	// Test with int
	str = convertUserIDToString(123)
	if str != "123" {
		t.Errorf("Expected string to be 123, got %s", str)
	}

	// Test with int64
	str = convertUserIDToString(int64(123))
	if str != "123" {
		t.Errorf("Expected string to be 123, got %s", str)
	}

	// Test with float64
	str = convertUserIDToString(123.45)
	if str != "123.45" {
		t.Errorf("Expected string to be 123.45, got %s", str)
	}

	// Test with bool
	str = convertUserIDToString(true)
	if str != "true" {
		t.Errorf("Expected string to be true, got %s", str)
	}

	// Test with a custom type that implements String()
	type CustomType struct{}
	str = convertUserIDToString(CustomType{})
	if str != "{}" {
		t.Errorf("Expected string to be {}, got %s", str)
	}
}

// TestRateLimiter is a mock implementation of the RateLimiter interface for testing
type TestRateLimiter struct {
	allowCount int
}

func (m *TestRateLimiter) Allow(key string, limit int, window time.Duration) (bool, int, time.Duration) {
	m.allowCount++
	// Allow the first two requests, deny the rest
	if m.allowCount <= 2 {
		return true, limit - m.allowCount, window
	}
	return false, 0, window
}

func TestRateLimitMiddleware(t *testing.T) {
	// Create a logger
	logger, _ := zap.NewDevelopment()

	// Create a mock rate limiter
	mockLimiter := &TestRateLimiter{}

	// Create a rate limit config with IP strategy (using common types)
	config := &common.RateLimitConfig[string, any]{
		BucketName: "test-bucket",
		Limit:      2, // Set a low limit for testing
		Window:     time.Second,
		Strategy:   common.StrategyIP, // Use common.StrategyIP
	}

	// Create the middleware
	middleware := RateLimit(config, mockLimiter, logger)

	// Create a test handler that returns 200 OK
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Wrap the test handler with the middleware
	handler := middleware(testHandler)

	// Create a test server
	server := httptest.NewServer(handler)
	defer server.Close()

	// Make requests to test rate limiting
	client := &http.Client{}

	// First request should succeed
	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, resp.StatusCode)
	}

	// Check rate limit headers
	limitHeader := resp.Header.Get("X-RateLimit-Limit")
	if limitHeader != "2" {
		t.Errorf("Expected X-RateLimit-Limit header to be 2, got %s", limitHeader)
	}

	// Second request should succeed
	resp, err = client.Get(server.URL)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, resp.StatusCode)
	}

	// Third request should fail with 429 Too Many Requests
	resp, err = client.Get(server.URL)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Errorf("Expected status code %d, got %d", http.StatusTooManyRequests, resp.StatusCode)
	}

	// Check retry-after header
	retryAfterHeader := resp.Header.Get("Retry-After")
	if retryAfterHeader == "" {
		t.Errorf("Expected Retry-After header to be set")
	}
}

func TestRateLimitMiddlewareCustomStrategySuccess(t *testing.T) {
	// Create a logger
	logger, _ := zap.NewDevelopment()

	// Create a mock rate limiter
	mockLimiter := &TestRateLimiter{} // Reuse mock

	// Define the custom key to be returned
	customKey := "my-custom-key"

	// Create a rate limit config with Custom strategy and a successful KeyExtractor (using common types)
	config := &common.RateLimitConfig[string, any]{
		BucketName: "test-bucket-custom-success",
		Limit:      2, // Low limit for testing
		Window:     time.Second,
		Strategy:   common.StrategyCustom, // Use common.StrategyCustom
		KeyExtractor: func(r *http.Request) (string, error) {
			return customKey, nil // Return the custom key successfully
		},
	}

	// Create the middleware
	middleware := RateLimit(config, mockLimiter, logger)

	// Create a test handler
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Wrap the test handler
	handler := middleware(testHandler)

	// Create a test server
	server := httptest.NewServer(handler)
	defer server.Close()

	// Make requests
	client := &http.Client{}

	// First request should succeed
	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, resp.StatusCode)
	}
	resp.Body.Close()

	// Second request should succeed
	resp, err = client.Get(server.URL)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, resp.StatusCode)
	}
	resp.Body.Close()

	// Third request should fail (based on the custom key)
	resp, err = client.Get(server.URL)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Errorf("Expected status code %d, got %d", http.StatusTooManyRequests, resp.StatusCode)
	}
	resp.Body.Close()

	// Verify the mock limiter was called 3 times (indicating the custom key was used)
	if mockLimiter.allowCount != 3 {
		t.Errorf("Expected Allow to be called 3 times, got %d", mockLimiter.allowCount)
	}
}

func TestRateLimitMiddlewareCustomStrategyNilExtractor(t *testing.T) {
	// Create a logger with an observer to capture logs
	core, observed := observer.New(zap.ErrorLevel) // Capture Error level logs
	logger := zap.New(core)

	// Create a mock rate limiter (it shouldn't be called)
	mockLimiter := &TestRateLimiter{}

	// Create a rate limit config with Custom strategy but nil KeyExtractor (using common types)
	config := &common.RateLimitConfig[string, any]{
		BucketName:   "test-bucket-custom-nil",
		Limit:        2, // Low limit for testing
		Window:       time.Second,
		Strategy:     common.StrategyCustom, // Use common.StrategyCustom
		KeyExtractor: nil,                   // Explicitly nil
	}

	// Create the middleware
	middleware := RateLimit(config, mockLimiter, logger)

	// Create a test handler
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Wrap the test handler
	handler := middleware(testHandler)

	// Create a test server
	server := httptest.NewServer(handler)
	defer server.Close()

	// Make a request (should fail immediately with 500)
	client := &http.Client{}
	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	// Verify the status code is Internal Server Error
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("Expected status code %d, got %d", http.StatusInternalServerError, resp.StatusCode)
	}

	// Verify the mock limiter was NOT called
	if mockLimiter.allowCount != 0 {
		t.Errorf("Expected Allow to be called 0 times, got %d", mockLimiter.allowCount)
	}

	// Verify that the error was logged
	logs := observed.FilterMessage("KeyExtractor function is required for StrategyCustom rate limiting.").All()
	if len(logs) == 0 {
		t.Errorf("Expected log message 'KeyExtractor function is required...' was not found")
	}
}

func TestRateLimitMiddlewareCustomStrategyError(t *testing.T) {
	// Create a logger with an observer to capture logs
	core, observed := observer.New(zap.ErrorLevel) // Capture Error level logs
	logger := zap.New(core)

	// Create a mock rate limiter (it won't be hit in this error case)
	mockLimiter := &TestRateLimiter{}

	// Define the error to be returned by the key extractor
	extractorError := errors.New("failed to extract key")

	// Create a rate limit config with Custom strategy and an erroring KeyExtractor (using common types)
	config := &common.RateLimitConfig[string, any]{
		BucketName: "test-bucket-custom-error",
		Limit:      5,
		Window:     time.Minute,
		Strategy:   common.StrategyCustom, // Use common.StrategyCustom
		KeyExtractor: func(r *http.Request) (string, error) {
			return "", extractorError // Always return an error
		},
	}

	// Create the middleware
	middleware := RateLimit(config, mockLimiter, logger)

	// Create a test handler (it won't be reached)
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Error("Test handler should not be called when KeyExtractor errors")
		w.WriteHeader(http.StatusOK)
	})

	// Wrap the test handler with the middleware
	handler := middleware(testHandler)

	// Create a test server
	server := httptest.NewServer(handler)
	defer server.Close()

	// Make a request
	client := &http.Client{}
	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	defer resp.Body.Close()

	// Verify the status code is Internal Server Error
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("Expected status code %d, got %d", http.StatusInternalServerError, resp.StatusCode)
	}

	// Verify that the error was logged
	logs := observed.FilterMessage("Custom KeyExtractor failed").All() // Corrected log message
	if len(logs) == 0 {
		t.Errorf("Expected log message 'Custom KeyExtractor failed' was not found")
	} else {
		// Check if the logged error matches the expected error
		foundError := false
		for _, field := range logs[0].Context {
			if field.Key == "error" {
				if loggedErr, ok := field.Interface.(error); ok {
					if errors.Is(loggedErr, extractorError) {
						foundError = true
						break
					}
				}
			}
		}
		if !foundError {
			t.Errorf("Logged error does not match the expected extractor error. Got context: %v", logs[0].Context)
		}
	}
}

func TestRateLimitMiddlewareWithCustomHandler(t *testing.T) {
	// Create a logger
	logger, _ := zap.NewDevelopment()

	// Create a mock rate limiter
	mockLimiter := &TestRateLimiter{}

	// Create a custom handler for rate limit exceeded
	customHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTooManyRequests)
		_, _ = w.Write([]byte(`{"error": "custom rate limit exceeded"}`))
	})

	// Create a rate limit config with IP strategy and custom handler (using common types)
	config := &common.RateLimitConfig[string, any]{
		BucketName:      "test-bucket-custom",
		Limit:           2, // Set a low limit for testing
		Window:          time.Second,
		Strategy:        common.StrategyIP, // Use common.StrategyIP
		ExceededHandler: customHandler,
	}

	// Create the middleware
	middleware := RateLimit(config, mockLimiter, logger)

	// Create a test handler that returns 200 OK
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Wrap the test handler with the middleware
	handler := middleware(testHandler)

	// Create a test server
	server := httptest.NewServer(handler)
	defer server.Close()

	// Make requests to test rate limiting
	client := &http.Client{}

	// First request should succeed
	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, resp.StatusCode)
	}

	// Second request should succeed
	resp, err = client.Get(server.URL)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, resp.StatusCode)
	}

	// Third request should fail with custom handler
	resp, err = client.Get(server.URL)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Errorf("Expected status code %d, got %d", http.StatusTooManyRequests, resp.StatusCode)
	}

	// Check content type header
	contentTypeHeader := resp.Header.Get("Content-Type")
	if contentTypeHeader != "application/json" {
		t.Errorf("Expected Content-Type header to be application/json, got %s", contentTypeHeader)
	}

	// Check response body
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}
	expectedBody := `{"error": "custom rate limit exceeded"}`
	if string(body) != expectedBody {
		t.Errorf("Expected response body to be %s, got %s", expectedBody, string(body))
	}
}

// TestUser type for testing user strategy
type TestUser struct {
	ID   string
	Name string
}

func TestRateLimitMiddlewareWithUserStrategy(t *testing.T) {
	// Create a logger
	logger, _ := zap.NewDevelopment()

	// Create a mock rate limiter
	mockLimiter := &TestRateLimiter{}

	// Create a rate limit config with User strategy (using common types)
	config := &common.RateLimitConfig[string, TestUser]{
		BucketName:     "test-bucket-user",
		Limit:          2, // Set a low limit for testing
		Window:         time.Second,
		Strategy:       common.StrategyUser, // Use common.StrategyUser
		UserIDFromUser: func(u TestUser) string { return u.ID },
		UserIDToString: func(id string) string { return id }, // Need UserIDToString for StrategyUser
	}

	// Create the middleware
	middleware := RateLimit(config, mockLimiter, logger)

	// Create a test handler that returns 200 OK
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Wrap the test handler with the middleware
	handler := middleware(testHandler)

	// Create a test server that adds the user to the context before calling the handler
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user := TestUser{ID: "test-user", Name: "Test User"}
		ctx := scontext.WithUser[string](r.Context(), &user) // Use scontext
		handler.ServeHTTP(w, r.WithContext(ctx))
	}))
	defer server.Close()

	// Make requests to test rate limiting
	client := &http.Client{}

	// First request should succeed
	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, resp.StatusCode)
	}

	// Second request should succeed
	resp, err = client.Get(server.URL)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, resp.StatusCode)
	}

	// Third request should fail with 429 Too Many Requests
	resp, err = client.Get(server.URL)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Errorf("Expected status code %d, got %d", http.StatusTooManyRequests, resp.StatusCode)
	}
}

// TestRateLimitWithIPMiddleware tests that rate limiting by IP works correctly
// when the IP middleware is applied before the rate limiting middleware
func TestRateLimitWithIPMiddleware(t *testing.T) {
	// Create a logger with a test observer to capture logs
	core, observed := observer.New(zap.WarnLevel)
	logger := zap.New(core)

	// Create a mock rate limiter
	mockLimiter := &TestRateLimiter{}

	// Create a rate limit config with IP strategy (using common types)
	config := &common.RateLimitConfig[uint64, any]{
		BucketName: "test-bucket-ip",
		Limit:      2, // Set a low limit for testing
		Window:     time.Second,
		Strategy:   common.StrategyIP, // Use common.StrategyIP
	}

	// Create the rate limit middleware
	rateLimitMiddleware := RateLimit(config, mockLimiter, logger)

	// Create a test handler that returns 200 OK
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Simulate IP Middleware by adding IP to context directly
	simulatedIPMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Manually add a dummy IP to the context using scontext
			// In a real scenario, router.ClientIPMiddleware would extract the real IP
			ctx := scontext.WithClientIP[uint64, any](r.Context(), "192.168.1.100")
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}

	// Apply the middleware chain: Simulated IP middleware -> Rate limit middleware -> Handler
	handler := simulatedIPMiddleware(rateLimitMiddleware(testHandler))

	// Create a test server
	server := httptest.NewServer(handler)
	defer server.Close()

	// Make requests to test rate limiting
	client := &http.Client{}

	// First request should succeed
	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, resp.StatusCode)
	}

	// Second request should succeed
	resp, err = client.Get(server.URL)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, resp.StatusCode)
	}

	// Third request should fail with 429 Too Many Requests
	resp, err = client.Get(server.URL)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Errorf("Expected status code %d, got %d", http.StatusTooManyRequests, resp.StatusCode)
	}

	// Verify that no warning logs were generated about IP middleware not being configured
	logs := observed.All()
	for _, log := range logs {
		if log.Message == "IP middleware not properly configured or applied before rate limiting" {
			t.Errorf("Unexpected warning log: %s", log.Message)
		}
	}

	// Now test without IP middleware to verify the error is logged
	// Reset observer and mock limiter for the second part
	observed.TakeAll()
	mockLimiter.allowCount = 0

	// Create a new test server with only rate limiting middleware
	serverWithoutIP := httptest.NewServer(rateLimitMiddleware(testHandler))
	defer serverWithoutIP.Close()

	// Make a request
	resp, err = client.Get(serverWithoutIP.URL) // Reuse resp variable
	if err != nil {
		t.Fatalf("Failed to make request to serverWithoutIP: %v", err)
	}
	// The request should still pass the limiter initially, but log an error
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status OK even without IP middleware (limiter allows first), got %d", resp.StatusCode)
	}
	resp.Body.Close() // Close body

	// Verify that an error log was generated about IP middleware not being configured
	errorLogs := observed.FilterMessage("Client IP not found in context for StrategyIP rate limiting. Ensure router.ClientIPMiddleware is applied first.").All()
	if len(errorLogs) == 0 {
		t.Errorf("Expected error log about missing Client IP in context")
	}
}

func TestRateLimitMiddlewareDefaultStrategy(t *testing.T) {
	// Create a logger
	logger, _ := zap.NewDevelopment()

	// Create a mock rate limiter
	mockLimiter := &TestRateLimiter{} // Reuse the mock from other tests

	// Create a rate limit config with an invalid strategy to trigger the default case (using common types)
	invalidStrategy := common.RateLimitStrategy(99) // Use a value not defined in the enum
	config := &common.RateLimitConfig[string, any]{
		BucketName: "test-bucket-default",
		Limit:      2, // Set a low limit for testing
		Window:     time.Second,
		Strategy:   invalidStrategy, // This should trigger the default case
	}

	// Create the middleware
	middleware := RateLimit(config, mockLimiter, logger)

	// Create a test handler that returns 200 OK
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Wrap the test handler with the middleware
	handler := middleware(testHandler)

	// Create a test server
	server := httptest.NewServer(handler)
	defer server.Close()

	// Make requests to test rate limiting (should behave like IP strategy)
	client := &http.Client{}

	// First request should succeed
	resp, err := client.Get(server.URL)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, resp.StatusCode)
	}
	resp.Body.Close() // Close the response body

	// Second request should succeed
	resp, err = client.Get(server.URL)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, resp.StatusCode)
	}
	resp.Body.Close() // Close the response body

	// Third request should fail with 429 Too Many Requests
	resp, err = client.Get(server.URL)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Errorf("Expected status code %d, got %d", http.StatusTooManyRequests, resp.StatusCode)
	}
	resp.Body.Close() // Close the response body

	// Check rate limit headers on the last response
	limitHeader := resp.Header.Get("X-RateLimit-Limit")
	if limitHeader != "2" {
		t.Errorf("Expected X-RateLimit-Limit header to be 2, got %s", limitHeader)
	}
	retryAfterHeader := resp.Header.Get("Retry-After")
	if retryAfterHeader == "" {
		t.Errorf("Expected Retry-After header to be set")
	}
}
