package middleware

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync" // Ensure sync is imported
	"testing"
	"time" // Ensure time is imported

	"github.com/Suhaibinator/SRouter/pkg/scontext" // Added import
	"github.com/stretchr/testify/assert"           // Using testify for assertions
)

// AddTraceIDToRequest adds a trace ID to the request context.
// This is useful for testing or for manually setting a trace ID.
// Copied here temporarily as it was removed from trace.go but needed for tests.
// TODO: Consider if tests should directly use scontext.WithTraceID instead.
func AddTraceIDToRequest[T comparable, U any](r *http.Request, traceID string) *http.Request {
	ctx := scontext.WithTraceID[T, U](r.Context(), traceID)
	return r.WithContext(ctx)
}

// TestAddTraceIDToRequest tests that AddTraceIDToRequest adds a trace ID to the request context
func TestAddTraceIDToRequest(t *testing.T) {
	// Create a request
	req, err := http.NewRequest("GET", "/test", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	// Verify no trace ID initially
	if traceID := scontext.GetTraceIDFromRequest[uint64, string](req); traceID != "" { // Use scontext
		t.Errorf("Expected trace ID to be empty initially, got %q", traceID)
	}

	// Add a trace ID to the request using the local helper
	expectedTraceID := "test-trace-id-123"
	req = AddTraceIDToRequest[uint64, string](req, expectedTraceID) // Use local helper

	// Verify the trace ID was added
	if traceID := scontext.GetTraceIDFromRequest[uint64, string](req); traceID != expectedTraceID { // Use scontext
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

// TestGetTraceID tests that GetTraceIDFromRequest returns the trace ID from the request context
func TestGetTraceID(t *testing.T) {
	// Create a request
	req, err := http.NewRequest("GET", "/test", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	// Test with no trace ID
	if traceID := scontext.GetTraceIDFromRequest[uint64, string](req); traceID != "" { // Use scontext
		t.Errorf("Expected trace ID to be empty, got %q", traceID)
	}

	// Add a trace ID to the context using the helper
	expectedTraceID := "test-trace-id"
	req = AddTraceIDToRequest[uint64, string](req, expectedTraceID) // Use local helper

	// Test with trace ID
	if traceID := scontext.GetTraceIDFromRequest[uint64, string](req); traceID != expectedTraceID { // Use scontext
		t.Errorf("Expected trace ID to be %q, got %q", expectedTraceID, traceID)
	}
}

// TestWithTraceID_AlreadySet tests that scontext.WithTraceID doesn't overwrite an existing trace ID
func TestWithTraceID_AlreadySet(t *testing.T) {
	ctx := context.Background()
	initialTraceID := "initial-trace-id"
	secondTraceID := "second-trace-id"

	// Set the first trace ID
	ctx = scontext.WithTraceID[uint64, string](ctx, initialTraceID) // Use scontext

	// Attempt to set a second trace ID
	ctx = scontext.WithTraceID[uint64, string](ctx, secondTraceID) // Use scontext

	// Verify that the initial trace ID is still the one present
	finalTraceID := scontext.GetTraceIDFromContext[uint64, string](ctx) // Use scontext
	if finalTraceID != initialTraceID {
		t.Errorf("Expected trace ID to remain %q after second call, but got %q", initialTraceID, finalTraceID)
	}

	// Double-check the SRouterContext directly
	rc, ok := scontext.GetSRouterContext[uint64, string](ctx) // Use scontext
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
	// Provide default types for the generic middleware call in the test
	middleware := CreateTraceMiddleware[string, any](generator)

	// Create a handler that checks the trace ID in the context
	var handlerTraceID string
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Use the same generic types as the middleware [string, any]
		handlerTraceID = scontext.GetTraceIDFromRequest[string, any](r) // Use scontext
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

// TestGetTraceIDFromContext tests that scontext.GetTraceIDFromContext returns the trace ID from the context
func TestGetTraceIDFromContext(t *testing.T) {
	// Create a context
	ctx := context.Background()

	// Test with no trace ID
	if traceID := scontext.GetTraceIDFromContext[uint64, string](ctx); traceID != "" { // Use scontext
		t.Errorf("Expected trace ID to be empty, got %q", traceID)
	}

	// Add a trace ID to the context using the new approach
	expectedTraceID := "test-trace-id"
	ctx = scontext.WithTraceID[uint64, string](ctx, expectedTraceID) // Use scontext

	// Test with trace ID
	if traceID := scontext.GetTraceIDFromContext[uint64, string](ctx); traceID != expectedTraceID { // Use scontext
		t.Errorf("Expected trace ID to be %q, got %q", expectedTraceID, traceID)
	}
}
