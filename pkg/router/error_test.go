package router

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"
)

// TestErrorHandling tests that errors are handled correctly
func TestErrorHandling(t *testing.T) {
	// Create a logger
	logger := zap.NewNop()

	// Define the auth function that takes a token and returns a *string and a boolean
	authFunction := func(ctx context.Context, token string) (*string, bool) {
		// This is a simple example, so we'll just validate that the token is not empty
		if token != "" {
			// Return pointer to token as user object
			return &token, true
		}
		return nil, false // Return nil pointer for user
	}

	// Define the function to get the user ID from a *string
	userIdFromUserFunction := func(user *string) string {
		// In this example, we're using the string itself as the ID
		if user == nil {
			return "" // Handle nil pointer case
		}
		return *user // Dereference pointer
	}

	// Create a router with string as both the user ID and user type
	r := NewRouter(RouterConfig{
		Logger: logger,
	}, authFunction, userIdFromUserFunction)

	// Register a route that returns an error
	r.RegisterRoute(RouteConfigBase{
		Path:    "/error",
		Methods: []HttpMethod{MethodGet}, // Use HttpMethod enum
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// This will panic
			_ = r.Context().Value(ParamsKey).(string) // Force a panic
		}),
	})

	// Register a route that returns a custom error
	r.RegisterRoute(RouteConfigBase{
		Path:    "/custom-error",
		Methods: []HttpMethod{MethodGet}, // Use HttpMethod enum
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Create a new HTTP error
			err := NewHTTPError(http.StatusBadRequest, "Bad request")
			// Write the error to the response
			http.Error(w, err.Error(), err.StatusCode)
		}),
	})

	// Create a test server
	server := httptest.NewServer(r)
	defer server.Close()

	// Test error handling
	resp, err := http.Get(server.URL + "/error")
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	defer resp.Body.Close()

	// Check status code (should be 500 Internal Server Error because of the panic)
	if resp.StatusCode != http.StatusInternalServerError {
		t.Errorf("Expected status code %d, got %d", http.StatusInternalServerError, resp.StatusCode)
	}

	// Test custom error handling
	resp, err = http.Get(server.URL + "/custom-error")
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	defer resp.Body.Close()

	// Check status code (should be 400 Bad Request)
	if resp.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status code %d, got %d", http.StatusBadRequest, resp.StatusCode)
	}
}

// TestHTTPError tests the HTTPError type
func TestHTTPError(t *testing.T) {
	// Create a new HTTP error
	err := NewHTTPError(http.StatusBadRequest, "Bad request")

	// Check the error message
	if err.Error() != "400: Bad request" {
		t.Errorf("Expected error message %q, got %q", "400: Bad request", err.Error())
	}

	// Check the status code
	if err.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status code %d, got %d", http.StatusBadRequest, err.StatusCode)
	}

	// Check the message
	if err.Message != "Bad request" {
		t.Errorf("Expected message %q, got %q", "Bad request", err.Message)
	}

	// Test error wrapping
	httpErr := &HTTPError{
		StatusCode: http.StatusInternalServerError,
		Message:    "Internal server error",
	}

	// Check if the error is an HTTPError
	var extractedErr *HTTPError
	if !errors.As(httpErr, &extractedErr) {
		t.Errorf("Expected error to be an HTTPError")
	}

	// Check if the extracted error has the correct status code
	if extractedErr.StatusCode != http.StatusInternalServerError {
		t.Errorf("Expected status code %d, got %d", http.StatusInternalServerError, extractedErr.StatusCode)
	}
}

// TestHandleError tests the handleError method
func TestHandleError(t *testing.T) {
	// Create a logger
	logger := zap.NewNop()

	// Define the auth function that takes a token and returns a *string and a boolean
	authFunction := func(ctx context.Context, token string) (*string, bool) {
		// This is a simple example, so we'll just validate that the token is not empty
		if token != "" {
			// Return pointer to token as user object
			return &token, true
		}
		return nil, false // Return nil pointer for user
	}

	// Define the function to get the user ID from a *string
	userIdFromUserFunction := func(user *string) string {
		// In this example, we're using the string itself as the ID
		if user == nil {
			return "" // Handle nil pointer case
		}
		return *user // Dereference pointer
	}

	// Create a router with string as both the user ID and user type
	r := NewRouter(RouterConfig{
		Logger: logger,
	}, authFunction, userIdFromUserFunction)

	// Create a test request
	req, err := http.NewRequest("GET", "/test", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	// Create a test response recorder
	rr := httptest.NewRecorder()

	// Test handling a regular error
	r.handleError(rr, req, errors.New("test error"), http.StatusInternalServerError, "Internal server error")

	// Check status code
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("Expected status code %d, got %d", http.StatusInternalServerError, rr.Code)
	}

	// Check response body
	if !strings.Contains(rr.Body.String(), "Internal server error") {
		t.Errorf("Expected response body to contain %q, got %q", "Internal server error", rr.Body.String())
	}

	// Create a new test response recorder
	rr = httptest.NewRecorder()

	// Test handling an HTTPError
	r.handleError(rr, req, NewHTTPError(http.StatusBadRequest, "Bad request"), http.StatusInternalServerError, "Internal server error")

	// Check status code (should be the one from the HTTPError)
	if rr.Code != http.StatusBadRequest {
		t.Errorf("Expected status code %d, got %d", http.StatusBadRequest, rr.Code)
	}

	// Check response body (should be the one from the HTTPError)
	if !strings.Contains(rr.Body.String(), "Bad request") {
		t.Errorf("Expected response body to contain %q, got %q", "Bad request", rr.Body.String())
	}
}
