package middleware

import (
	"errors" // Need to import errors package
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

	// Test that the limiter is reusing the same limiter for the same key
	limiter.Allow(key, limit, window)
	_, exists := limiter.limiters.Load(key)
	if !exists {
		t.Errorf("Expected limiter to be stored for key %s", key)
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

	// Test that the limiter is storing different limiters for different keys
	_, exists = limiter.limiters.Load(otherKey)
	if !exists {
		t.Errorf("Expected limiter to be stored for key %s", otherKey)
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
}

func TestRateLimitExtractIP(t *testing.T) {
	// Create a nil logger for testing
	var logger *zap.Logger = nil

	// Test with X-Forwarded-For header
	req := httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Forwarded-For", "192.168.1.1, 10.0.0.1")
	ip := extractIP[uint64, string](req, logger)
	if ip != "192.168.1.1" {
		t.Errorf("Expected IP to be 192.168.1.1, got %s", ip)
	}

	// Test with X-Real-IP header
	req = httptest.NewRequest("GET", "/", nil)
	req.Header.Set("X-Real-IP", "192.168.1.2")
	ip = extractIP[uint64, string](req, logger)
	if ip != "192.168.1.2" {
		t.Errorf("Expected IP to be 192.168.1.2, got %s", ip)
	}

	// Test with RemoteAddr
	req = httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "192.168.1.3:1234"
	ip = extractIP[uint64, string](req, logger)
	if ip != "192.168.1.3:1234" {
		t.Errorf("Expected IP to be 192.168.1.3:1234, got %s", ip)
	}
}

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

	// Create a rate limit config with IP strategy
	config := &RateLimitConfig[string, any]{
		BucketName: "test-bucket",
		Limit:      2, // Set a low limit for testing
		Window:     time.Second,
		Strategy:   StrategyIP,
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

	// Create a rate limit config with Custom strategy and a successful KeyExtractor
	config := &RateLimitConfig[string, any]{
		BucketName: "test-bucket-custom-success",
		Limit:      2, // Low limit for testing
		Window:     time.Second,
		Strategy:   StrategyCustom,
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
	// Create a logger
	logger, _ := zap.NewDevelopment()

	// Create a mock rate limiter
	mockLimiter := &TestRateLimiter{} // Reuse mock

	// Create a rate limit config with Custom strategy but nil KeyExtractor
	config := &RateLimitConfig[string, any]{
		BucketName:   "test-bucket-custom-nil",
		Limit:        2, // Low limit for testing
		Window:       time.Second,
		Strategy:     StrategyCustom,
		KeyExtractor: nil, // Explicitly nil
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

	// Make requests (should behave like IP strategy)
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

	// Third request should fail
	resp, err = client.Get(server.URL)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}
	if resp.StatusCode != http.StatusTooManyRequests {
		t.Errorf("Expected status code %d, got %d", http.StatusTooManyRequests, resp.StatusCode)
	}
	resp.Body.Close()

	// Verify the mock limiter was called 3 times
	if mockLimiter.allowCount != 3 {
		t.Errorf("Expected Allow to be called 3 times, got %d", mockLimiter.allowCount)
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

	// Create a rate limit config with Custom strategy and an erroring KeyExtractor
	config := &RateLimitConfig[string, any]{
		BucketName: "test-bucket-custom-error",
		Limit:      5,
		Window:     time.Minute,
		Strategy:   StrategyCustom,
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
	logs := observed.FilterMessage("Failed to extract rate limit key").All()
	if len(logs) == 0 {
		t.Errorf("Expected log message 'Failed to extract rate limit key' was not found")
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

	// Create a rate limit config with IP strategy and custom handler
	config := &RateLimitConfig[string, any]{
		BucketName:      "test-bucket-custom",
		Limit:           2, // Set a low limit for testing
		Window:          time.Second,
		Strategy:        StrategyIP,
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

// TestUser type for testing
type TestUser struct {
	ID   string
	Name string
}

func TestRateLimitMiddlewareWithUserStrategy(t *testing.T) {
	// Create a logger
	logger, _ := zap.NewDevelopment()

	// Create a mock rate limiter
	mockLimiter := &TestRateLimiter{}

	// Create a rate limit config with User strategy
	config := &RateLimitConfig[string, TestUser]{
		BucketName:     "test-bucket-user",
		Limit:          2, // Set a low limit for testing
		Window:         time.Second,
		Strategy:       StrategyUser,
		UserIDFromUser: func(u TestUser) string { return u.ID },
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
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Add the user to the context using the new wrapper
		user := TestUser{ID: "test-user", Name: "Test User"}
		ctx := WithUser[string](r.Context(), &user)

		// Call the handler with the updated context
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

func TestCreateRateLimitMiddleware(t *testing.T) {
	// Create a logger
	logger, _ := zap.NewDevelopment()

	// Create a rate limit middleware using the helper function
	middleware := CreateRateLimitMiddleware(
		"test-bucket-create",
		2, // Set a low limit for testing
		time.Second,
		StrategyUser,
		func(u TestUser) string { return u.ID },
		nil, // Use default string conversion
		logger,
	)

	// Create a test handler that returns 200 OK
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Wrap the test handler with the middleware
	handler := middleware(testHandler)

	// Create a test server
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Add the user to the context using the new wrapper
		user := TestUser{ID: "test-user", Name: "Test User"}
		ctx := WithUser[string](r.Context(), &user)

		// Call the handler with the updated context
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
}

// TestRateLimitWithIPMiddleware tests that rate limiting by IP works correctly
// when the IP middleware is applied before the rate limiting middleware
func TestRateLimitWithIPMiddleware(t *testing.T) {
	// Create a logger with a test observer to capture logs
	core, observed := observer.New(zap.WarnLevel)
	logger := zap.New(core)

	// Create a mock rate limiter
	mockLimiter := &TestRateLimiter{}

	// Create a rate limit config with IP strategy
	config := &RateLimitConfig[uint64, any]{
		BucketName: "test-bucket-ip",
		Limit:      2, // Set a low limit for testing
		Window:     time.Second,
		Strategy:   StrategyIP,
	}

	// Create the rate limit middleware
	rateLimitMiddleware := RateLimit(config, mockLimiter, logger)

	// Create a test handler that returns 200 OK
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Create a chain of middleware with IP middleware first, then rate limiting
	ipConfig := DefaultIPConfig()
	ipMiddleware := ClientIPMiddleware[uint64, any](ipConfig) // Use the variable

	// Apply the middleware chain: IP middleware -> Rate limit middleware -> Handler
	handler := ipMiddleware(rateLimitMiddleware(testHandler))

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

	// Now test without IP middleware to verify the warning is logged
	// Create a new test server with only rate limiting middleware
	serverWithoutIP := httptest.NewServer(rateLimitMiddleware(testHandler))
	defer serverWithoutIP.Close()

	// Make a request
	_, err = client.Get(serverWithoutIP.URL)
	if err != nil {
		t.Fatalf("Failed to make request: %v", err)
	}

	// Verify that a warning log was generated about IP middleware not being configured
	logs = observed.All()
	found := false
	for _, log := range logs {
		if log.Message == "IP middleware not properly configured or applied before rate limiting" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected warning log about IP middleware not being configured")
	}
}

func TestRateLimitMiddlewareDefaultStrategy(t *testing.T) {
	// Create a logger
	logger, _ := zap.NewDevelopment()

	// Create a mock rate limiter
	mockLimiter := &TestRateLimiter{} // Reuse the mock from other tests

	// Create a rate limit config with an invalid strategy to trigger the default case
	invalidStrategy := RateLimitStrategy(99) // Use a value not defined in the enum
	config := &RateLimitConfig[string, any]{
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
