package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestAuthenticationGeneric tests the generic Authentication middleware
func TestAuthenticationGeneric(t *testing.T) {
	// Create a test handler that expects user ID only for non-OPTIONS requests
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodOptions {
			// Get the user ID from the context only for non-OPTIONS requests
			userID, ok := GetUserIDFromRequest[string, any](r)
			if !ok {
				t.Error("Expected user ID in context for non-OPTIONS request, but not found")
			}
			if userID != "user123" {
				t.Errorf("Expected user ID 'user123' for non-OPTIONS request, got '%s'", userID)
			}
		}
		w.WriteHeader(http.StatusOK)
	})

	// Create an authentication function that checks for a specific header
	// and returns a user ID if authentication is successful
	authFunc := func(r *http.Request) (string, bool) {
		if r.Header.Get("X-Auth-Token") == "valid-token" {
			return "user123", true
		}
		return "", false
	}

	// Apply the Authentication middleware
	authMiddleware := Authentication[string, any](authFunc) // Kept public (generic)
	wrappedHandler := authMiddleware(handler)

	// Test with valid authentication
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Auth-Token", "valid-token")
	rec := httptest.NewRecorder()

	// Call the handler
	wrappedHandler.ServeHTTP(rec, req)

	// Check that the response status code is 200 (OK)
	if rec.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rec.Code)
	}

	// Test with invalid authentication
	req = httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Auth-Token", "invalid-token")
	rec = httptest.NewRecorder()

	// Call the handler
	wrappedHandler.ServeHTTP(rec, req)

	// Check that the response status code is 401 (Unauthorized)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Expected status code %d, got %d", http.StatusUnauthorized, rec.Code)
	}

	// Test with OPTIONS request (should bypass authentication)
	req = httptest.NewRequest("OPTIONS", "/test", nil)
	req.Header.Set("X-Auth-Token", "invalid-token") // Even with invalid token
	rec = httptest.NewRecorder()

	// Call the handler
	wrappedHandler.ServeHTTP(rec, req)

	// Check that the response status code is 200 (OK) because OPTIONS should skip auth
	if rec.Code != http.StatusOK {
		t.Errorf("Expected status code %d for OPTIONS request, got %d", http.StatusOK, rec.Code)
	}
}

// TestAuthentication tests the Authentication middleware
func TestAuthentication(t *testing.T) {
	// Create a test handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Create an authentication function that checks for a specific header
	authFunc := func(r *http.Request) bool {
		return r.Header.Get("X-Auth-Token") == "valid-token"
	}

	// Apply the AuthenticationBool middleware
	authMiddleware := AuthenticationBool[string, any](authFunc, "authenticated") // Kept public (generic)
	wrappedHandler := authMiddleware(handler)

	// Test with valid authentication
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Auth-Token", "valid-token")
	rec := httptest.NewRecorder()

	// Call the handler
	wrappedHandler.ServeHTTP(rec, req)

	// Check that the response status code is 200 (OK)
	if rec.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rec.Code)
	}

	// Test with invalid authentication
	req = httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Auth-Token", "invalid-token")
	rec = httptest.NewRecorder()

	// Call the handler
	wrappedHandler.ServeHTTP(rec, req)

	// Check that the response status code is 401 (Unauthorized)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Expected status code %d, got %d", http.StatusUnauthorized, rec.Code)
	}

	// Test with no authentication
	req = httptest.NewRequest("GET", "/test", nil)
	rec = httptest.NewRecorder()

	// Call the handler
	wrappedHandler.ServeHTTP(rec, req)

	// Check that the response status code is 401 (Unauthorized)
	if rec.Code != http.StatusUnauthorized {
		t.Errorf("Expected status code %d, got %d", http.StatusUnauthorized, rec.Code)
	}

	// Test with OPTIONS request (should bypass authentication)
	req = httptest.NewRequest("OPTIONS", "/test", nil)
	req.Header.Set("X-Auth-Token", "invalid-token") // Even with invalid token
	rec = httptest.NewRecorder()

	// Call the handler
	wrappedHandler.ServeHTTP(rec, req)

	// Check that the response status code is 200 (OK) because OPTIONS should skip auth
	if rec.Code != http.StatusOK {
		t.Errorf("Expected status code %d for OPTIONS request, got %d", http.StatusOK, rec.Code)
	}
}
