package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync" // Ensure sync is imported
	"testing"
	"time" // Ensure time is imported
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

// TestIDGeneratorBatchFill tests the batch-filling logic under high contention.
func TestIDGeneratorBatchFill(t *testing.T) {
	bufferSize := 100 // Small buffer to trigger depletion easily
	generator := NewIDGenerator(bufferSize)

	// Wait for the generator to initialize and fill the buffer initially
	time.Sleep(100 * time.Millisecond) // Give some time for initial fill

	// Check initial fill
	if len(generator.idChan) != bufferSize {
		t.Fatalf("Expected initial buffer size %d, got %d", bufferSize, len(generator.idChan))
	}

	// Simulate high contention by rapidly consuming IDs
	numConsumers := 10
	consumeCount := bufferSize * 2 // Consume more than the buffer size
	var wg sync.WaitGroup
	wg.Add(numConsumers)

	for i := 0; i < numConsumers; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < consumeCount/numConsumers; j++ {
				_ = generator.GetIDNonBlocking() // Consume IDs
				// Small sleep to yield and allow background filler to run
				time.Sleep(time.Microsecond)
			}
		}()
	}

	wg.Wait() // Wait for consumers to finish

	// Give the background filler time to potentially batch-fill
	time.Sleep(50 * time.Millisecond)

	// Check if the buffer has been refilled (at least partially)
	// It's hard to guarantee it's *full* due to timing, but it shouldn't be empty
	// and should ideally be close to full if batching worked.
	finalLen := len(generator.idChan)
	if finalLen < bufferSize/2 { // Check if it's at least half full
		t.Errorf("Expected buffer to be significantly refilled after contention, got length %d (buffer size %d)", finalLen, bufferSize)
	}

	// Consume one more ID to ensure generator is still working
	id := generator.GetIDNonBlocking()
	if id == "" {
		t.Error("Generator failed to produce ID after contention test")
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
	traceMiddleware := Trace() // Use the variable

	// Create a wrapped handler
	wrappedHandler := traceMiddleware(handler) // Use the correct variable name

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

// TestTraceMiddlewareWithConfig tests that TraceMiddlewareWithConfig adds a trace ID
func TestTraceMiddlewareWithConfig(t *testing.T) {
	// Create a handler that checks for the trace ID
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		traceID := GetTraceID(r)
		if traceID == "" {
			t.Error("Expected trace ID to be set, but it was empty")
		}
		_, _ = w.Write([]byte(traceID))
	})

	// Create a middleware with a specific buffer size
	bufferSize := 50                               // Use a different size than the default
	traceMiddleware := TraceWithConfig(bufferSize) // Use the variable

	// Create a wrapped handler
	wrappedHandler := traceMiddleware(handler) // Use the correct variable name

	// Create a request
	req, err := http.NewRequest("GET", "/test-config", nil)
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

	// Optional: Verify a generator with the specific buffer size was created/used
	// This might require exposing internal state or using more complex testing techniques.
	// For now, just verifying functionality is sufficient.
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
