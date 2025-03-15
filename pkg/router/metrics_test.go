package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// TestMetrics tests that metrics are collected correctly
func TestMetrics(t *testing.T) {
	// Create an observed zap logger to capture logs at Debug level
	core, logs := observer.New(zap.DebugLevel)
	logger := zap.New(core)

	// Define the auth function that takes a token and returns a string and a boolean
	authFunction := func(ctx context.Context, token string) (string, bool) {
		// This is a simple example, so we'll just validate that the token is not empty
		if token != "" {
			return token, true
		}
		return "", false
	}

	// Define the function to get the user ID from a string
	userIdFromUserFunction := func(user string) string {
		// In this example, we're using the string itself as the ID
		return user
	}

	// Create a router with string as both the user ID and user type
	r := NewRouter(RouterConfig{
		Logger:        logger,
		EnableMetrics: true,
	}, authFunction, userIdFromUserFunction)

	// Register a route
	r.RegisterRoute(RouteConfigBase{
		Path:    "/test",
		Methods: []string{"GET"},
		Handler: func(w http.ResponseWriter, r *http.Request) {
			_, err := w.Write([]byte("Hello, World!"))
			if err != nil {
				t.Fatalf("Failed to write response: %v", err)
			}
		},
	})

	// Create a test request
	req, err := http.NewRequest("GET", "/test", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	// Create a test response recorder
	rr := httptest.NewRecorder()

	// Serve the request
	r.ServeHTTP(rr, req)

	// Check status code
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rr.Code)
	}

	// Check response body
	if rr.Body.String() != "Hello, World!" {
		t.Errorf("Expected response body %q, got %q", "Hello, World!", rr.Body.String())
	}

	// Check that metrics were logged
	logEntries := logs.All()
	if len(logEntries) == 0 {
		t.Errorf("Expected metrics to be logged")
	}

	// Check that the log contains the expected fields
	found := false
	for _, log := range logEntries {
		if log.Message == "Request metrics" {
			found = true
			// Check that the log contains the expected fields
			if log.Context[0].Key != "method" || log.Context[0].String != "GET" {
				t.Errorf("Expected method field to be %q, got %q", "GET", log.Context[0].String)
			}
			if log.Context[1].Key != "path" || log.Context[1].String != "/test" {
				t.Errorf("Expected path field to be %q, got %q", "/test", log.Context[1].String)
			}
			if log.Context[2].Key != "status" || log.Context[2].Integer != int64(http.StatusOK) {
				t.Errorf("Expected status field to be %d, got %d", http.StatusOK, log.Context[2].Integer)
			}
			// Duration and bytes fields are also logged, but we don't check them here
			break
		}
	}
	if !found {
		t.Errorf("Expected 'Request metrics' log message")
	}
}

// TestTracing tests that tracing information is collected correctly
func TestTracing(t *testing.T) {
	// Create an observed zap logger to capture logs at Debug level
	core, logs := observer.New(zap.DebugLevel)
	logger := zap.New(core)

	// Define the auth function that takes a token and returns a string and a boolean
	authFunction := func(ctx context.Context, token string) (string, bool) {
		// This is a simple example, so we'll just validate that the token is not empty
		if token != "" {
			return token, true
		}
		return "", false
	}

	// Define the function to get the user ID from a string
	userIdFromUserFunction := func(user string) string {
		// In this example, we're using the string itself as the ID
		return user
	}

	// Create a router with string as both the user ID and user type
	r := NewRouter(RouterConfig{
		Logger:        logger,
		EnableTracing: true,
	}, authFunction, userIdFromUserFunction)

	// Register a route
	r.RegisterRoute(RouteConfigBase{
		Path:    "/test",
		Methods: []string{"GET"},
		Handler: func(w http.ResponseWriter, r *http.Request) {
			_, err := w.Write([]byte("Hello, World!"))
			if err != nil {
				t.Fatalf("Failed to write response: %v", err)
			}
		},
	})

	// Create a test request
	req, err := http.NewRequest("GET", "/test", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	req.Header.Set("User-Agent", "test-agent")
	req.RemoteAddr = "127.0.0.1:1234"

	// Create a test response recorder
	rr := httptest.NewRecorder()

	// Serve the request
	r.ServeHTTP(rr, req)

	// Check status code
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rr.Code)
	}

	// Check response body
	if rr.Body.String() != "Hello, World!" {
		t.Errorf("Expected response body %q, got %q", "Hello, World!", rr.Body.String())
	}

	// Check that tracing information was logged
	logEntries := logs.All()
	if len(logEntries) == 0 {
		t.Errorf("Expected tracing information to be logged")
	}

	// Check that the log contains the expected fields
	found := false
	for _, log := range logEntries {
		if log.Message == "Request trace" {
			found = true
			// Check that the log contains the expected fields
			if log.Context[0].Key != "method" || log.Context[0].String != "GET" {
				t.Errorf("Expected method field to be %q, got %q", "GET", log.Context[0].String)
			}
			if log.Context[1].Key != "path" || log.Context[1].String != "/test" {
				t.Errorf("Expected path field to be %q, got %q", "/test", log.Context[1].String)
			}
			if log.Context[2].Key != "remote_addr" || log.Context[2].String != "127.0.0.1:1234" {
				t.Errorf("Expected remote_addr field to be %q, got %q", "127.0.0.1:1234", log.Context[2].String)
			}
			if log.Context[3].Key != "user_agent" || log.Context[3].String != "test-agent" {
				t.Errorf("Expected user_agent field to be %q, got %q", "test-agent", log.Context[3].String)
			}
			if log.Context[4].Key != "status" || log.Context[4].Integer != int64(http.StatusOK) {
				t.Errorf("Expected status field to be %d, got %d", http.StatusOK, log.Context[4].Integer)
			}
			// Duration field is also logged, but we don't check it here
			break
		}
	}
	if !found {
		t.Errorf("Expected 'Request trace' log message")
	}
}

// TestMetricsResponseWriter tests the metricsResponseWriter
func TestMetricsResponseWriter(t *testing.T) {
	// Create a test response recorder
	rr := httptest.NewRecorder()

	// Create a metrics response writer with string as both the user ID and user type
	mrw := &metricsResponseWriter[string, string]{
		ResponseWriter: rr,
		statusCode:     http.StatusOK,
	}

	// Set a different status code
	mrw.WriteHeader(http.StatusNotFound)

	// Check that the status code was set
	if mrw.statusCode != http.StatusNotFound {
		t.Errorf("Expected statusCode to be %d, got %d", http.StatusNotFound, mrw.statusCode)
	}

	// Write a response
	_, err := mrw.Write([]byte("Hello, World!"))
	if err != nil {
		t.Fatalf("Failed to write response: %v", err)
	}

	// Check that the response was written
	if rr.Body.String() != "Hello, World!" {
		t.Errorf("Expected response body %q, got %q", "Hello, World!", rr.Body.String())
	}

	// Check that the bytes written were counted
	if mrw.bytesWritten != 13 {
		t.Errorf("Expected bytesWritten to be %d, got %d", 13, mrw.bytesWritten)
	}

	// Check that the status code was written to the response
	if rr.Code != http.StatusNotFound {
		t.Errorf("Expected response code to be %d, got %d", http.StatusNotFound, rr.Code)
	}
}
