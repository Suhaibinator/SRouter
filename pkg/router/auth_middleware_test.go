package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	// Keep middleware alias if needed for other types
	"github.com/Suhaibinator/SRouter/pkg/router/internal/mocks" // Use centralized mocks
	"github.com/Suhaibinator/SRouter/pkg/scontext"              // Added scontext import
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// TestAuthOptionalMiddleware tests the authOptionalMiddleware function
// (Combined from auth_test.go and auth_optional_middleware_test.go)
func TestAuthOptionalMiddleware(t *testing.T) {
	logger := zap.NewNop()

	// Create a router with a simple auth function
	config := RouterConfig{
		Logger:             logger,
		AddUserObjectToCtx: true, // Test case from auth_optional_middleware_test.go
	}

	// Auth function that accepts "valid-token" and returns user ID 123
	authFunction := func(ctx context.Context, token string) (*string, bool) {
		if token == "valid-token" {
			user := "user123"
			return &user, true
		}
		return nil, false
	}

	getUserIDFromUser := func(user *string) string {
		if user == nil {
			return "nil_user_pointer" // Or handle appropriately
		}
		return *user
	}

	router := NewRouter(config, authFunction, getUserIDFromUser)

	// Create a base test handler
	baseHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	// Apply the authOptionalMiddleware to the test handler
	middleware := router.authOptionalMiddleware(baseHandler)

	// Test case 1: Request with valid auth header (checks context)
	t.Run("with valid auth header", func(t *testing.T) {
		handlerCalled := false
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Authorization", "Bearer valid-token")
		rr := httptest.NewRecorder()

		// Special handler to check context
		validTokenHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			handlerCalled = true
			userID, ok := scontext.GetUserIDFromRequest[string, string](r) // Use scontext
			if !ok {
				t.Error("Expected user ID in context, but not found")
			} else if userID != "user123" {
				t.Errorf("Expected user ID 'user123', got '%s'", userID)
			}
			// Check for user object as well since AddUserObjectToCtx is true
			userObjPtr, ok := scontext.GetUserFromRequest[string, string](r) // Use scontext, gets a pointer
			if !ok {
				t.Error("Expected user object pointer in context, but not found")
			} else if userObjPtr == nil {
				t.Error("Expected non-nil user object pointer, got nil")
			} else if *userObjPtr != "user123" { // Dereference the pointer to check the value
				t.Errorf("Expected user object 'user123', got '%v'", *userObjPtr)
			}

			w.WriteHeader(http.StatusOK)
		})

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
		req := httptest.NewRequest("GET", "/", nil)
		rr := httptest.NewRecorder()
		middleware.ServeHTTP(rr, req)

		if status := rr.Code; status != http.StatusOK {
			t.Errorf("Handler returned wrong status code: got %v want %v", status, http.StatusOK)
		}
		if rr.Body.String() != "OK" {
			t.Errorf("Expected response body %q, got %q", "OK", rr.Body.String())
		}
		// Check context - should not have user ID
		_, ok := scontext.GetUserIDFromRequest[string, string](req) // Use scontext
		if ok {
			t.Error("Expected user ID not to be in context, but found")
		}
	})

	// Test case 3: Request with invalid auth header
	t.Run("with invalid auth header", func(t *testing.T) {
		req := httptest.NewRequest("GET", "/", nil)
		req.Header.Set("Authorization", "Bearer invalid-token")
		rr := httptest.NewRecorder()
		middleware.ServeHTTP(rr, req)

		if status := rr.Code; status != http.StatusOK {
			t.Errorf("Handler returned wrong status code: got %v want %v", status, http.StatusOK)
		}
		if rr.Body.String() != "OK" {
			t.Errorf("Expected response body %q, got %q", "OK", rr.Body.String())
		}
		// Check context - should not have user ID
		_, ok := scontext.GetUserIDFromRequest[string, string](req) // Use scontext
		if ok {
			t.Error("Expected user ID not to be in context, but found")
		}
	})
}

// TestAuthRequiredMiddleware tests the authRequiredMiddleware function
// (from auth_required_middleware_test.go)
func TestAuthRequiredMiddleware(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)

	r := NewRouter(RouterConfig{Logger: logger}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, ok := scontext.GetUserIDFromRequest[string, string](r) // Use scontext
		if !ok {
			t.Error("Expected user ID to be in context")
		}
		if userID != "user123" {
			t.Errorf("Expected user ID %q, got %q", "user123", userID)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	wrappedHandler := r.authRequiredMiddleware(handler)

	// Test with no Authorization header
	req, _ := http.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Expected status code %d, got %d", http.StatusUnauthorized, rr.Code)
	}
	logEntries := logs.All()
	if len(logEntries) == 0 {
		t.Errorf("Expected warning to be logged")
	}
	found := false
	for _, log := range logEntries {
		if log.Message == "Authentication failed" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected 'Authentication failed' log message")
	}
	logs.TakeAll() // Reset logs

	// Test with invalid Authorization header
	req, _ = http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	rr = httptest.NewRecorder()
	wrappedHandler.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Expected status code %d, got %d", http.StatusUnauthorized, rr.Code)
	}
	logEntries = logs.All()
	if len(logEntries) == 0 {
		t.Errorf("Expected warning to be logged")
	}
	found = false
	for _, log := range logEntries {
		if log.Message == "Authentication failed" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected 'Authentication failed' log message")
	}
	logs.TakeAll() // Reset logs

	// Test with valid Authorization header (using debug logger)
	debugCore, debugLogs := observer.New(zap.DebugLevel)
	debugLogger := zap.New(debugCore)
	r = NewRouter(RouterConfig{Logger: debugLogger}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)
	wrappedHandler = r.authRequiredMiddleware(handler) // Re-wrap with new router instance

	req, _ = http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	rr = httptest.NewRecorder()
	wrappedHandler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rr.Code)
	}
	if rr.Body.String() != "OK" {
		t.Errorf("Expected response body %q, got %q", "OK", rr.Body.String())
	}
	debugLogEntries := debugLogs.All()
	if len(debugLogEntries) == 0 {
		t.Errorf("Expected debug log to be recorded")
	}
	found = false
	for _, log := range debugLogEntries {
		if log.Message == "Authentication successful" {
			found = true
			break
		}
	}
	if !found {
		t.Errorf("Expected 'Authentication successful' log message")
	}

	// Test with Authorization header without Bearer prefix (should still work if auth func handles it)
	// Note: The default auth func in mocks.MockAuthFunction expects the token directly.
	req, _ = http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "valid-token") // No Bearer prefix
	rr = httptest.NewRecorder()
	wrappedHandler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code %d without Bearer prefix, got %d", http.StatusOK, rr.Code)
	}
	if rr.Body.String() != "OK" {
		t.Errorf("Expected response body %q without Bearer prefix, got %q", "OK", rr.Body.String())
	}
}

// TestAuthRequiredMiddlewareWithUserObject tests that authRequired middleware properly adds user object to context when AddUserObjectToCtx is enabled
func TestAuthRequiredMiddlewareWithUserObject(t *testing.T) {
	logger := zap.NewNop()

	// Create router with AddUserObjectToCtx enabled
	config := RouterConfig{
		Logger:             logger,
		AddUserObjectToCtx: true,
	}

	authFunction := func(ctx context.Context, token string) (*string, bool) {
		if token == "valid-token" {
			user := "user123"
			return &user, true
		}
		return nil, false
	}

	getUserIDFromUser := func(user *string) string {
		if user == nil {
			return "nil_user_pointer"
		}
		return *user
	}

	router := NewRouter(config, authFunction, getUserIDFromUser)

	// Handler that checks both user ID and user object in context
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Check user ID
		userID, ok := scontext.GetUserIDFromRequest[string, string](r)
		if !ok {
			t.Error("Expected user ID to be in context")
		}
		if userID != "user123" {
			t.Errorf("Expected user ID %q, got %q", "user123", userID)
		}

		// Check user object (this would fail with the original bug)
		userObjPtr, ok := scontext.GetUserFromRequest[string, string](r)
		if !ok {
			t.Error("Expected user object pointer in context, but not found")
		} else if userObjPtr == nil {
			t.Error("Expected non-nil user object pointer, got nil")
		} else if *userObjPtr != "user123" {
			t.Errorf("Expected user object 'user123', got '%v'", *userObjPtr)
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	wrappedHandler := router.authRequiredMiddleware(handler)

	// Test with valid Authorization header
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	rr := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rr.Code)
	}
	if rr.Body.String() != "OK" {
		t.Errorf("Expected response body %q, got %q", "OK", rr.Body.String())
	}
}

// TestAuthRequiredMiddlewareWithTraceID tests the authRequiredMiddleware function with trace ID
// (from auth_required_middleware_test.go)
func TestAuthRequiredMiddlewareWithTraceID(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)
	logger := zap.New(core)

	r := NewRouter(RouterConfig{Logger: logger, TraceIDBufferSize: 1000}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	wrappedHandler := r.authRequiredMiddleware(handler)

	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	traceID := "test-trace-id-required"
	// Replace mw.AddTraceIDToRequest with scontext.WithTraceID
	ctxWithTrace := scontext.WithTraceID[string, string](req.Context(), traceID) // Use scontext
	req = req.WithContext(ctxWithTrace)                                          // Apply the context with trace ID

	rr := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rr.Code)
	}

	logEntries := logs.All()
	if len(logEntries) == 0 {
		t.Errorf("Expected debug log to be recorded")
	}

	found := false
	for _, log := range logEntries {
		if log.Message == "Authentication successful" {
			for _, field := range log.Context {
				if field.Key == "trace_id" && field.String == traceID {
					found = true
					break
				}
			}
			if found {
				break
			}
		}
	}
	if !found {
		t.Errorf("Expected 'Authentication successful' log message with trace ID %q", traceID)
	}
}

// TestAuthMiddleware tests the authentication middleware (integration style)
// (from advanced_features_test.go)
func TestAuthMiddlewareIntegration(t *testing.T) {
	logger := zap.NewNop()
	r := NewRouter(RouterConfig{Logger: logger}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	r.RegisterRoute(RouteConfigBase{
		Path:      "/protected",
		Methods:   []HttpMethod{MethodGet}, // Use HttpMethod enum
		AuthLevel: Ptr(AuthRequired),
		Handler: func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("Protected"))
		},
	})

	// Test without Authorization header
	req, _ := http.NewRequest("GET", "/protected", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Expected status code %d, got %d", http.StatusUnauthorized, rr.Code)
	}

	// Test with valid Authorization header
	req, _ = http.NewRequest("GET", "/protected", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rr.Code)
	}
	if rr.Body.String() != "Protected" {
		t.Errorf("Expected response body %q, got %q", "Protected", rr.Body.String())
	}
}
