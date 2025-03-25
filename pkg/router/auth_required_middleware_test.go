package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Suhaibinator/SRouter/pkg/middleware"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// TestAuthRequiredMiddleware tests the authRequiredMiddleware function
func TestAuthRequiredMiddleware(t *testing.T) {
	// Create a logger with an observer to capture logs
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)

	// Create a router with a specific auth function
	r := NewRouter[string, string](RouterConfig{
		Logger: logger,
	},
		// Mock auth function that validates "valid-token"
		func(ctx context.Context, token string) (string, bool) {
			if token == "valid-token" {
				return "user123", true
			}
			return "", false
		},
		// Mock user ID function that returns the string itself
		func(user string) string {
			return user
		})

	// Create a test handler that checks if the user ID is in the context
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID, ok := middleware.GetUserIDFromRequest[string, string](r)
		if !ok {
			t.Error("Expected user ID to be in context")
		}
		if userID != "user123" {
			t.Errorf("Expected user ID %q, got %q", "user123", userID)
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	// Wrap the handler with the authRequiredMiddleware
	wrappedHandler := r.authRequiredMiddleware(handler)

	// Test with no Authorization header
	req, _ := http.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(rr, req)

	// Check status code (should be unauthorized)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Expected status code %d, got %d", http.StatusUnauthorized, rr.Code)
	}

	// Check that a warning was logged
	logEntries := logs.All()
	if len(logEntries) == 0 {
		t.Errorf("Expected warning to be logged")
	}

	// Check that the log contains the expected message
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

	// Reset logs
	logs.TakeAll()

	// Test with invalid Authorization header
	req, _ = http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer invalid-token")
	rr = httptest.NewRecorder()
	wrappedHandler.ServeHTTP(rr, req)

	// Check status code (should be unauthorized)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Expected status code %d, got %d", http.StatusUnauthorized, rr.Code)
	}

	// Check that a warning was logged
	logEntries = logs.All()
	if len(logEntries) == 0 {
		t.Errorf("Expected warning to be logged")
	}

	// Check that the log contains the expected message
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

	// Reset logs
	logs.TakeAll()

	// Create a new logger to capture debug logs
	debugCore, debugLogs := observer.New(zap.DebugLevel)
	debugLogger := zap.New(debugCore)

	// Create a new router with the debug logger
	r = NewRouter[string, string](RouterConfig{
		Logger: debugLogger,
	},
		// Mock auth function that validates "valid-token"
		func(ctx context.Context, token string) (string, bool) {
			if token == "valid-token" {
				return "user123", true
			}
			return "", false
		},
		// Mock user ID function that returns the string itself
		func(user string) string {
			return user
		})

	// Wrap the handler with the authRequiredMiddleware
	wrappedHandler = r.authRequiredMiddleware(handler)

	// Test with valid Authorization header
	req, _ = http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	rr = httptest.NewRecorder()
	wrappedHandler.ServeHTTP(rr, req)

	// Check status code (should be OK)
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rr.Code)
	}

	// Check response body
	if rr.Body.String() != "OK" {
		t.Errorf("Expected response body %q, got %q", "OK", rr.Body.String())
	}

	// Check that a debug log was recorded
	debugLogEntries := debugLogs.All()
	if len(debugLogEntries) == 0 {
		t.Errorf("Expected debug log to be recorded")
	}

	// Check that the log contains the expected message
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

	// Test with Authorization header without Bearer prefix
	req, _ = http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "valid-token")
	rr = httptest.NewRecorder()
	wrappedHandler.ServeHTTP(rr, req)

	// Check status code (should be OK)
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rr.Code)
	}

	// Check response body
	if rr.Body.String() != "OK" {
		t.Errorf("Expected response body %q, got %q", "OK", rr.Body.String())
	}
}

// TestAuthRequiredMiddlewareWithTraceID tests the authRequiredMiddleware function with trace ID
func TestAuthRequiredMiddlewareWithTraceID(t *testing.T) {
	// Create a logger with an observer to capture logs
	core, logs := observer.New(zap.DebugLevel)
	logger := zap.New(core)

	// Create a router with a specific auth function and trace ID enabled
	r := NewRouter[string, string](RouterConfig{
		Logger:        logger,
		EnableTraceID: true,
	},
		// Mock auth function that validates "valid-token"
		func(ctx context.Context, token string) (string, bool) {
			if token == "valid-token" {
				return "user123", true
			}
			return "", false
		},
		// Mock user ID function that returns the string itself
		func(user string) string {
			return user
		})

	// Create a test handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	// Wrap the handler with the authRequiredMiddleware
	wrappedHandler := r.authRequiredMiddleware(handler)

	// Test with valid Authorization header and trace ID
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	req = middleware.AddTraceIDToRequest(req, "test-trace-id")

	// Verify trace ID was set in the request context
	traceID := middleware.GetTraceID(req)
	t.Logf("Initial trace ID: %s", traceID)

	rr := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(rr, req)

	// Check status code (should be OK)
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rr.Code)
	}

	// Check that a debug log was recorded with trace ID
	logEntries := logs.All()
	if len(logEntries) == 0 {
		t.Errorf("Expected debug log to be recorded")
	}

	// Print out all log entries for debugging
	t.Logf("Found %d log entries", len(logEntries))
	for i, log := range logEntries {
		t.Logf("Log entry %d: Message: %s", i, log.Message)
		for _, field := range log.Context {
			t.Logf("  Field: %s = %v", field.Key, field.String)
		}
	}

	// Check that the log contains the expected message and trace ID
	found := false
	for _, log := range logEntries {
		if log.Message == "Authentication successful" {
			// Check if trace ID is in the log fields
			for _, field := range log.Context {
				if field.Key == "trace_id" && field.String == "test-trace-id" {
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
		t.Errorf("Expected 'Authentication successful' log message with trace ID")
	}
}
