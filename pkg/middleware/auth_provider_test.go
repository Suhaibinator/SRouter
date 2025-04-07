package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Suhaibinator/SRouter/pkg/scontext" // Added import
	"go.uber.org/zap"
)

// TestBearerTokenProvider_Authenticate tests the Authenticate method of BearerTokenProvider
func TestBearerTokenProvider_Authenticate(t *testing.T) {
	// Create a BearerTokenProvider with a map of valid tokens
	validTokens := map[string]string{
		"token1": "user1",
		"token2": "user2",
	}
	provider := &BearerTokenProvider[string]{
		ValidTokens: validTokens,
	}

	// Test with valid token
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer token1")
	userID, ok := provider.Authenticate(req)
	if !ok {
		t.Error("Expected authentication to succeed, but it failed")
	}
	if userID != "user1" {
		t.Errorf("Expected user ID 'user1', got '%s'", userID)
	}

	// Test with invalid token
	req = httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	userID, ok = provider.Authenticate(req)
	if ok {
		t.Error("Expected authentication to fail, but it succeeded")
	}
	if userID != "" {
		t.Errorf("Expected empty user ID, got '%s'", userID)
	}

	// Test with no Authorization header
	req = httptest.NewRequest("GET", "/test", nil)
	userID, ok = provider.Authenticate(req)
	if ok {
		t.Error("Expected authentication to fail, but it succeeded")
	}
	if userID != "" {
		t.Errorf("Expected empty user ID, got '%s'", userID)
	}

	// Test with invalid Authorization header format
	req = httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Basic dXNlcjpwYXNz")
	userID, ok = provider.Authenticate(req)
	if ok {
		t.Error("Expected authentication to fail, but it succeeded")
	}
	if userID != "" {
		t.Errorf("Expected empty user ID, got '%s'", userID)
	}

	// Test with validator function
	provider = &BearerTokenProvider[string]{
		Validator: func(token string) (string, bool) {
			if token == "valid-token" {
				return "user123", true
			}
			return "", false
		},
	}

	req = httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	userID, ok = provider.Authenticate(req)
	if !ok {
		t.Error("Expected authentication to succeed, but it failed")
	}
	if userID != "user123" {
		t.Errorf("Expected user ID 'user123', got '%s'", userID)
	}
}

// TestAPIKeyProvider_Authenticate tests the Authenticate method of APIKeyProvider
func TestAPIKeyProvider_Authenticate(t *testing.T) {
	// Create an APIKeyProvider with a map of valid keys
	validKeys := map[string]string{
		"key1": "user1",
		"key2": "user2",
	}
	provider := &APIKeyProvider[string]{
		ValidKeys: validKeys,
		Header:    "X-API-Key",
		Query:     "api_key",
	}

	// Test with valid key in header
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-API-Key", "key1")
	userID, ok := provider.Authenticate(req)
	if !ok {
		t.Error("Expected authentication to succeed, but it failed")
	}
	if userID != "user1" {
		t.Errorf("Expected user ID 'user1', got '%s'", userID)
	}

	// Test with valid key in query parameter
	req = httptest.NewRequest("GET", "/test?api_key=key2", nil)
	userID, ok = provider.Authenticate(req)
	if !ok {
		t.Error("Expected authentication to succeed, but it failed")
	}
	if userID != "user2" {
		t.Errorf("Expected user ID 'user2', got '%s'", userID)
	}

	// Test with invalid key
	req = httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-API-Key", "invalid-key")
	userID, ok = provider.Authenticate(req)
	if ok {
		t.Error("Expected authentication to fail, but it succeeded")
	}
	if userID != "" {
		t.Errorf("Expected empty user ID, got '%s'", userID)
	}

	// Test with no key
	req = httptest.NewRequest("GET", "/test", nil)
	userID, ok = provider.Authenticate(req)
	if ok {
		t.Error("Expected authentication to fail, but it succeeded")
	}
	if userID != "" {
		t.Errorf("Expected empty user ID, got '%s'", userID)
	}

	// Test with empty header name
	provider = &APIKeyProvider[string]{
		ValidKeys: validKeys,
		Header:    "",
		Query:     "api_key",
	}
	req = httptest.NewRequest("GET", "/test?api_key=key2", nil)
	userID, ok = provider.Authenticate(req)
	if !ok {
		t.Error("Expected authentication to succeed, but it failed")
	}
	if userID != "user2" {
		t.Errorf("Expected user ID 'user2', got '%s'", userID)
	}

	// Test with empty query parameter name
	provider = &APIKeyProvider[string]{
		ValidKeys: validKeys,
		Header:    "X-API-Key",
		Query:     "",
	}
	req = httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-API-Key", "key1")
	userID, ok = provider.Authenticate(req)
	if !ok {
		t.Error("Expected authentication to succeed, but it failed")
	}
	if userID != "user1" {
		t.Errorf("Expected user ID 'user1', got '%s'", userID)
	}
}

// TestAuthenticationWithProvider tests the AuthenticationWithProvider middleware
func TestAuthenticationWithProvider(t *testing.T) {
	// Create a logger
	logger, _ := zap.NewProduction()

	// Create a test handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get the user ID from the context
		userID, ok := scontext.GetUserIDFromRequest[string, any](r) // Use scontext
		if !ok {
			t.Error("Expected user ID in context, but not found")
		}
		if userID != "user1" {
			t.Errorf("Expected user ID 'user1', got '%s'", userID)
		}
		w.WriteHeader(http.StatusOK)
	})

	// Create a provider
	validTokens := map[string]string{
		"token1": "user1",
		"token2": "user2",
	}
	provider := &BearerTokenProvider[string]{
		ValidTokens: validTokens,
	}

	// Apply the AuthenticationWithProvider middleware
	middleware := AuthenticationWithProvider[string, any](provider, logger)
	wrappedHandler := middleware(handler)

	// Test with valid authentication
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer token1")
	rec := httptest.NewRecorder()

	// Call the handler
	wrappedHandler.ServeHTTP(rec, req)

	// Check that the response status code is 200 (OK)
	if rec.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rec.Code)
	}

	// Test with invalid authentication
	req = httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	rec = httptest.NewRecorder()

	// Call the handler
	wrappedHandler.ServeHTTP(rec, req)

	// Check that the response status code is 401 (Unauthorized)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Expected status code %d, got %d", http.StatusUnauthorized, rec.Code)
	}
}

// TestNewBearerTokenMiddleware tests the NewBearerTokenMiddleware function
func TestNewBearerTokenMiddleware(t *testing.T) {
	// Create a logger
	logger, _ := zap.NewProduction()

	// Create a test handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get the user ID from the context
		userID, ok := scontext.GetUserIDFromRequest[string, any](r) // Use scontext
		if !ok {
			t.Error("Expected user ID in context, but not found")
		}
		if userID != "user1" {
			t.Errorf("Expected user ID 'user1', got '%s'", userID)
		}
		w.WriteHeader(http.StatusOK)
	})

	// Create a map of valid tokens
	validTokens := map[string]string{
		"token1": "user1",
		"token2": "user2",
	}

	// Apply the NewBearerTokenMiddleware
	middleware := NewBearerTokenMiddleware[string, any](validTokens, logger)
	wrappedHandler := middleware(handler)

	// Test with valid authentication
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer token1")
	rec := httptest.NewRecorder()

	// Call the handler
	wrappedHandler.ServeHTTP(rec, req)

	// Check that the response status code is 200 (OK)
	if rec.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rec.Code)
	}
}

// TestNewBearerTokenValidatorMiddleware tests the NewBearerTokenValidatorMiddleware function
func TestNewBearerTokenValidatorMiddleware(t *testing.T) {
	// Create a logger
	logger, _ := zap.NewProduction()

	// Create a test handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get the user ID from the context
		userID, ok := scontext.GetUserIDFromRequest[string, any](r) // Use scontext
		if !ok {
			t.Error("Expected user ID in context, but not found")
		}
		if userID != "user123" {
			t.Errorf("Expected user ID 'user123', got '%s'", userID)
		}
		w.WriteHeader(http.StatusOK)
	})

	// Create a validator function
	validator := func(token string) (string, bool) {
		if token == "valid-token" {
			return "user123", true
		}
		return "", false
	}

	// Apply the NewBearerTokenValidatorMiddleware
	middleware := NewBearerTokenValidatorMiddleware[string, any](validator, logger)
	wrappedHandler := middleware(handler)

	// Test with valid authentication
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	rec := httptest.NewRecorder()

	// Call the handler
	wrappedHandler.ServeHTTP(rec, req)

	// Check that the response status code is 200 (OK)
	if rec.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rec.Code)
	}
}

// TestNewAPIKeyMiddleware tests the NewAPIKeyMiddleware function
func TestNewAPIKeyMiddleware(t *testing.T) {
	// Create a logger
	logger, _ := zap.NewProduction()

	// Create a test handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get the user ID from the context
		userID, ok := scontext.GetUserIDFromRequest[string, any](r) // Use scontext
		if !ok {
			t.Error("Expected user ID in context, but not found")
		}
		if userID != "user1" {
			t.Errorf("Expected user ID 'user1', got '%s'", userID)
		}
		w.WriteHeader(http.StatusOK)
	})

	// Create a map of valid keys
	validKeys := map[string]string{
		"key1": "user1",
		"key2": "user2",
	}

	// Apply the NewAPIKeyMiddleware
	middleware := NewAPIKeyMiddleware[string, any](validKeys, "X-API-Key", "api_key", logger)
	wrappedHandler := middleware(handler)

	// Test with valid authentication in header
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-API-Key", "key1")
	rec := httptest.NewRecorder()

	// Call the handler
	wrappedHandler.ServeHTTP(rec, req)

	// Check that the response status code is 200 (OK)
	if rec.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rec.Code)
	}

	// Test with valid authentication in query parameter
	req = httptest.NewRequest("GET", "/test?api_key=key1", nil)
	rec = httptest.NewRecorder()

	// Call the handler
	wrappedHandler.ServeHTTP(rec, req)

	// Check that the response status code is 200 (OK)
	if rec.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rec.Code)
	}
}
