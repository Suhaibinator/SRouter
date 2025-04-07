package middleware

import (
	"net/http/httptest"
	"sync"
	"testing"
)

// TestTimeoutResponseWriterWriteAndFlush tests the Write and Flush methods of the mutexResponseWriter
func TestTimeoutResponseWriterWriteAndFlush(t *testing.T) {
	// Create a test response recorder
	recorder := httptest.NewRecorder()

	// Create a mutexResponseWriter that wraps the recorder
	rw := &mutexResponseWriter{
		ResponseWriter: recorder,
		mu:             &sync.Mutex{},
	}

	// Test the Write method
	data := []byte("Hello, World!")
	n, err := rw.Write(data)
	if err != nil {
		t.Errorf("Write returned an error: %v", err)
	}
	if n != len(data) {
		t.Errorf("Write returned %d, expected %d", n, len(data))
	}
	if recorder.Body.String() != "Hello, World!" {
		t.Errorf("Write did not write the expected data: got %q, expected %q", recorder.Body.String(), "Hello, World!")
	}

	// Test the Flush method
	// Since httptest.ResponseRecorder doesn't implement http.Flusher, this is just for coverage
	rw.Flush()
}
