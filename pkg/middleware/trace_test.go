package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync" // Ensure sync is imported
	"testing"
	"time" // Ensure time is imported

	"github.com/stretchr/testify/assert" // Using testify for assertions
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

	for range numConsumers {
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

// TestWithTraceID_AlreadySet tests that WithTraceID doesn't overwrite an existing trace ID
func TestWithTraceID_AlreadySet(t *testing.T) {
	ctx := context.Background()
	initialTraceID := "initial-trace-id"
	secondTraceID := "second-trace-id"

	// Set the first trace ID
	ctx = WithTraceID[string, any](ctx, initialTraceID)

	// Attempt to set a second trace ID
	ctx = WithTraceID[string, any](ctx, secondTraceID)

	// Verify that the initial trace ID is still the one present
	finalTraceID := GetTraceIDFromContext(ctx)
	if finalTraceID != initialTraceID {
		t.Errorf("Expected trace ID to remain %q after second call, but got %q", initialTraceID, finalTraceID)
	}

	// Double-check the SRouterContext directly
	rc, ok := GetSRouterContext[string, any](ctx)
	if !ok {
		t.Fatal("SRouterContext not found in context")
	}
	if !rc.TraceIDSet {
		t.Error("Expected TraceIDSet flag to be true")
	}
	if rc.TraceID != initialTraceID {
		t.Errorf("Expected SRouterContext.TraceID to be %q, got %q", initialTraceID, rc.TraceID)
	}
}

// TestCreateTraceMiddleware_WithExistingHeader tests that the middleware uses an existing X-Trace-ID header
func TestCreateTraceMiddleware_WithExistingHeader(t *testing.T) {
	generator := NewIDGenerator(10) // Generator needed, but shouldn't be used in this path
	middleware := CreateTraceMiddleware(generator)

	// Create a handler that checks the trace ID in the context
	var handlerTraceID string
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		handlerTraceID = GetTraceID(r) // Get trace ID from context
		w.WriteHeader(http.StatusOK)
	})

	// Apply the middleware
	wrappedHandler := middleware(testHandler)

	// Create a request recorder and a request with an existing header
	rr := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	existingTraceID := "existing-trace-id-from-header"
	req.Header.Set("X-Trace-ID", existingTraceID)

	// Serve the request
	wrappedHandler.ServeHTTP(rr, req)

	// Assertions
	assert.Equal(t, http.StatusOK, rr.Code, "Expected status OK")
	assert.Equal(t, existingTraceID, handlerTraceID, "Expected handler to see the existing trace ID from header")
	assert.Equal(t, existingTraceID, rr.Header().Get("X-Trace-ID"), "Expected response header to contain the existing trace ID")

	// Ensure the generator wasn't drained (though GetIDNonBlocking has a fallback)
	// This is a bit indirect, but confirms GetIDNonBlocking wasn't the primary source
	select {
	case <-generator.idChan:
		// Good, an ID was still available
	default:
		t.Error("Generator channel seems empty, GetIDNonBlocking might have been called unexpectedly")
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
