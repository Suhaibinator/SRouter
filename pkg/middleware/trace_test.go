package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

// TestAddTraceIDToRequest tests that AddTraceIDToRequest adds a trace ID to the request context
func TestAddTraceIDToRequest(t *testing.T) {
	// Create a request
	req, err := http.NewRequest("GET", "/test", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	// Verify no trace ID initially
	if traceID := GetTraceID(req); traceID != "" {
		t.Errorf("Expected trace ID to be empty initially, got %q", traceID)
	}

	// Add a trace ID to the request
	expectedTraceID := "test-trace-id-123"
	req = AddTraceIDToRequest(req, expectedTraceID)

	// Verify the trace ID was added
	if traceID := GetTraceID(req); traceID != expectedTraceID {
		t.Errorf("Expected trace ID to be %q after adding, got %q", expectedTraceID, traceID)
	}
}

// TestTraceMiddleware tests that the TraceMiddleware adds a trace ID to the request context
func TestTraceMiddleware(t *testing.T) {
	// Create a handler that checks for the trace ID
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get the trace ID from the context
		traceID := GetTraceID(r)

		// Check that the trace ID is not empty
		if traceID == "" {
			t.Error("Expected trace ID to be set, but it was empty")
		}

		// Write the trace ID to the response
		_, _ = w.Write([]byte(traceID))
	})

	// Create a middleware
	middleware := TraceMiddleware()

	// Create a wrapped handler
	wrappedHandler := middleware(handler)

	// Create a request
	req, err := http.NewRequest("GET", "/test", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	// Create a response recorder
	rr := httptest.NewRecorder()

	// Serve the request
	wrappedHandler.ServeHTTP(rr, req)

	// Check that the response contains a trace ID (non-empty string)
	if rr.Body.String() == "" {
		t.Error("Expected response to contain a trace ID, but it was empty")
	}
}

// TestGetTraceID tests that GetTraceID returns the trace ID from the request context
func TestGetTraceID(t *testing.T) {
	// Create a request
	req, err := http.NewRequest("GET", "/test", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	// Test with no trace ID
	if traceID := GetTraceID(req); traceID != "" {
		t.Errorf("Expected trace ID to be empty, got %q", traceID)
	}

	// Add a trace ID to the context using the new method
	expectedTraceID := "test-trace-id"
	req = AddTraceIDToRequest(req, expectedTraceID)

	// Test with trace ID
	if traceID := GetTraceID(req); traceID != expectedTraceID {
		t.Errorf("Expected trace ID to be %q, got %q", expectedTraceID, traceID)
	}
}

// TestGetTraceIDFromContext tests that GetTraceIDFromContext returns the trace ID from the context
func TestGetTraceIDFromContext(t *testing.T) {
	// Create a context
	ctx := context.Background()

	// Test with no trace ID
	if traceID := GetTraceIDFromContext(ctx); traceID != "" {
		t.Errorf("Expected trace ID to be empty, got %q", traceID)
	}

	// Add a trace ID to the context using the new approach
	expectedTraceID := "test-trace-id"
	ctx = WithTraceID[string, any](ctx, expectedTraceID)

	// Test with trace ID
	if traceID := GetTraceIDFromContext(ctx); traceID != expectedTraceID {
		t.Errorf("Expected trace ID to be %q, got %q", expectedTraceID, traceID)
	}
}
