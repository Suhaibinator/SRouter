package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	mw "github.com/Suhaibinator/SRouter/pkg/middleware"
)

// TestAuthOptionalMiddlewareWithValidToken tests the authOptionalMiddleware function
// with a valid token and AddUserObjectToCtx set to true, which are cases not covered
// by the existing test in auth_test.go
func TestAuthOptionalMiddlewareWithValidToken(t *testing.T) {
	// Create a router with a simple auth function
	config := RouterConfig{
		AddUserObjectToCtx: true,
	}

	// Auth function that accepts "valid-token" and returns user ID 123
	authFunction := func(ctx context.Context, token string) (string, bool) {
		if token == "valid-token" {
			return "user123", true
		}
		return "", false
	}

	getUserIDFromUser := func(user string) string {
		return user
	}

	router := NewRouter[string, string](config, authFunction, getUserIDFromUser)

	// Create a test handler that checks if user ID is in context
	handlerCalled := false
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerCalled = true

		// For the valid token test case, we'll check the context separately
		w.WriteHeader(http.StatusOK)
	})

	// Apply the authOptionalMiddleware to the test handler
	middleware := router.authOptionalMiddleware(testHandler)

	// Test case 1: Request with valid auth header
	t.Run("with valid auth header", func(t *testing.T) {
		handlerCalled = false
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rr := httptest.NewRecorder()

		// Create a special handler for this test case that checks the context
		validTokenHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true

			// Check if user ID is in context using the router instance's method
			userID, ok := mw.GetUserIDFromRequest[string, string](r)
			if !ok {
				t.Error("Expected user ID in context, but not found")
			} else if userID != "user123" {
				t.Errorf("Expected user ID 'user123', got '%s'", userID)
			}

			w.WriteHeader(http.StatusOK)
		})

		// Apply middleware to this special handler
		validTokenMiddleware := router.authOptionalMiddleware(validTokenHandler)
		validTokenMiddleware.ServeHTTP(rr, req)

		if !handlerCalled {
			t.Error("Handler was not called")
		}

		if status := rr.Code; status != http.StatusOK {
			t.Errorf("Handler returned wrong status code: got %v want %v", status, http.StatusOK)
		}
	})

	// Test case 2: Request without auth header
	t.Run("without auth header", func(t *testing.T) {
		handlerCalled = false
		req := httptest.NewRequest("GET", "/", nil)
		rr := httptest.NewRecorder()

		middleware.ServeHTTP(rr, req)

		if !handlerCalled {
			t.Error("Handler was not called")
		}

		if status := rr.Code; status != http.StatusOK {
			t.Errorf("Handler returned wrong status code: got %v want %v", status, http.StatusOK)
		}
	})

	// Test case 3: Request with invalid auth header
	t.Run("with invalid auth header", func(t *testing.T) {
		handlerCalled = false
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Authorization", "Bearer invalid-token")
		rr := httptest.NewRecorder()

		middleware.ServeHTTP(rr, req)

		if !handlerCalled {
			t.Error("Handler was not called")
		}

		if status := rr.Code; status != http.StatusOK {
			t.Errorf("Handler returned wrong status code: got %v want %v", status, http.StatusOK)
		}
	})
}
