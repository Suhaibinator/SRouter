package middleware

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// TestChain tests the Chain function that chains multiple middlewares together
func TestChain(t *testing.T) {
	// Create middleware functions
	middleware1 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Middleware-1", "true")
			next.ServeHTTP(w, r)
		})
	}

	middleware2 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Middleware-2", "true")
			next.ServeHTTP(w, r)
		})
	}

	middleware3 := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Middleware-3", "true")
			next.ServeHTTP(w, r)
		})
	}

	// Create a test handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Chain the middlewares
	chained := Chain(middleware1, middleware2, middleware3)
	wrappedHandler := chained(handler)

	// Create a test request
	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	// Call the handler
	wrappedHandler.ServeHTTP(rec, req)

	// Check that all middlewares were applied
	if rec.Header().Get("X-Middleware-1") != "true" {
		t.Error("Expected X-Middleware-1 header to be set")
	}
	if rec.Header().Get("X-Middleware-2") != "true" {
		t.Error("Expected X-Middleware-2 header to be set")
	}
	if rec.Header().Get("X-Middleware-3") != "true" {
		t.Error("Expected X-Middleware-3 header to be set")
	}

	// Check that the response status code is 200
	if rec.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rec.Code)
	}
}

// TestRecovery tests the Recovery middleware that recovers from panics
func TestRecovery(t *testing.T) {
	// Create a logger with an observer for testing
	core, logs := observer.New(zapcore.DebugLevel)
	logger := zap.New(core)

	// Create a test handler that panics
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	})

	// Apply the Recovery middleware
	recoveryMiddleware := Recovery(logger) // Use the variable
	wrappedHandler := recoveryMiddleware(handler)

	// Create a test request
	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	// Call the handler (should not panic)
	wrappedHandler.ServeHTTP(rec, req)

	// Check that the response status code is 500
	if rec.Code != http.StatusInternalServerError {
		t.Errorf("Expected status code %d, got %d", http.StatusInternalServerError, rec.Code)
	}

	// Check that the logger recorded the panic
	if logs.Len() == 0 {
		t.Fatal("Expected at least one log entry")
	}

	// Find the error log
	var found bool
	for _, log := range logs.All() {
		if log.Level == zapcore.ErrorLevel && log.Message == "Panic recovered" {
			found = true
			break
		}
	}

	if !found {
		t.Error("Expected to find an error log with message 'Panic recovered'")
	}
}

// TestMaxBodySize tests the MaxBodySize middleware that limits the size of the request body
func TestMaxBodySize(t *testing.T) {
	// Create a test handler that reads the request body
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			// The error message from http.MaxBytesReader contains "request body too large"
			if strings.Contains(err.Error(), "request body too large") {
				http.Error(w, err.Error(), http.StatusRequestEntityTooLarge)
				return
			}
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(fmt.Sprintf("Body size: %d", len(body))))
	})

	// Apply the MaxBodySize middleware with a limit of 10 bytes
	maxBodyMiddleware := MaxBodySize(10) // Use the variable
	wrappedHandler := maxBodyMiddleware(handler)

	// Create a test request with a body larger than the limit
	req := httptest.NewRequest("POST", "/test", strings.NewReader("This is a test body that is larger than 10 bytes"))
	rec := httptest.NewRecorder()

	// Call the handler
	wrappedHandler.ServeHTTP(rec, req)

	// Check that the response contains an error about the body being too large
	if !strings.Contains(rec.Body.String(), "request body too large") {
		t.Errorf("Expected error message about request body too large, got: %s", rec.Body.String())
	}

	// Create a test request with a body smaller than the limit
	req = httptest.NewRequest("POST", "/test", strings.NewReader("Small"))
	rec = httptest.NewRecorder()

	// Call the handler
	wrappedHandler.ServeHTTP(rec, req)

	// Check that the response status code is 200
	if rec.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rec.Code)
	}
}

// TestCORS tests the CORS middleware, verifying headers for actual and preflight requests.
func TestCORS(t *testing.T) {
	// --- Test Case 1: Standard Configuration ---
	t.Run("StandardConfig", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		corsConfig := CORSOptions{
			Origins:          []string{"http://example.com", "https://example.org"},
			Methods:          []string{"GET", "POST", "PUT"},
			Headers:          []string{"Content-Type", "Authorization"},
			ExposeHeaders:    []string{"X-Custom-Header", "Content-Length"},
			AllowCredentials: true,
			MaxAge:           time.Hour,
		}
		corsMiddleware := CORS(corsConfig) // Use the variable
		wrappedHandler := corsMiddleware(handler)

		// Test Actual Request (GET)
		reqGet := httptest.NewRequest("GET", "/test", nil)
		recGet := httptest.NewRecorder()
		wrappedHandler.ServeHTTP(recGet, reqGet)

		// Check GET Response Headers
		expectedOrigin := "http://example.com, https://example.org"
		if got := recGet.Header().Get("Access-Control-Allow-Origin"); got != expectedOrigin {
			t.Errorf("GET: Expected Allow-Origin '%s', got '%s'", expectedOrigin, got)
		}
		expectedExpose := "X-Custom-Header, Content-Length"
		if got := recGet.Header().Get("Access-Control-Expose-Headers"); got != expectedExpose {
			t.Errorf("GET: Expected Expose-Headers '%s', got '%s'", expectedExpose, got)
		}
		if got := recGet.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
			t.Errorf("GET: Expected Allow-Credentials 'true', got '%s'", got)
		}
		// These should NOT be set on non-OPTIONS requests
		if got := recGet.Header().Get("Access-Control-Allow-Methods"); got != "" {
			t.Errorf("GET: Expected Allow-Methods to be empty, got '%s'", got)
		}
		if got := recGet.Header().Get("Access-Control-Allow-Headers"); got != "" {
			t.Errorf("GET: Expected Allow-Headers to be empty, got '%s'", got)
		}
		if got := recGet.Header().Get("Access-Control-Max-Age"); got != "" {
			t.Errorf("GET: Expected Max-Age to be empty, got '%s'", got)
		}
		if recGet.Code != http.StatusOK {
			t.Errorf("GET: Expected status code %d, got %d", http.StatusOK, recGet.Code)
		}

		// Test Preflight Request (OPTIONS)
		reqOptions := httptest.NewRequest("OPTIONS", "/test", nil)
		recOptions := httptest.NewRecorder()
		wrappedHandler.ServeHTTP(recOptions, reqOptions)

		// Check OPTIONS Response Headers
		if got := recOptions.Header().Get("Access-Control-Allow-Origin"); got != expectedOrigin {
			t.Errorf("OPTIONS: Expected Allow-Origin '%s', got '%s'", expectedOrigin, got)
		}
		expectedMethods := "GET, POST, PUT"
		if got := recOptions.Header().Get("Access-Control-Allow-Methods"); got != expectedMethods {
			t.Errorf("OPTIONS: Expected Allow-Methods '%s', got '%s'", expectedMethods, got)
		}
		expectedHeaders := "Content-Type, Authorization"
		if got := recOptions.Header().Get("Access-Control-Allow-Headers"); got != expectedHeaders {
			t.Errorf("OPTIONS: Expected Allow-Headers '%s', got '%s'", expectedHeaders, got)
		}
		expectedMaxAge := "3600" // 1 hour in seconds
		if got := recOptions.Header().Get("Access-Control-Max-Age"); got != expectedMaxAge {
			t.Errorf("OPTIONS: Expected Max-Age '%s', got '%s'", expectedMaxAge, got)
		}
		if got := recOptions.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
			t.Errorf("OPTIONS: Expected Allow-Credentials 'true', got '%s'", got)
		}
		// Expose-Headers is not relevant for preflight
		if got := recOptions.Header().Get("Access-Control-Expose-Headers"); got != "" {
			t.Errorf("OPTIONS: Expected Expose-Headers to be empty, got '%s'", got)
		}
		if recOptions.Code != http.StatusOK {
			t.Errorf("OPTIONS: Expected status code %d, got %d", http.StatusOK, recOptions.Code)
		}
	})

	// --- Test Case 2: Empty Configuration ---
	t.Run("EmptyConfig", func(t *testing.T) {
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		corsMiddleware := CORS(CORSOptions{}) // Empty config // Use the variable
		wrappedHandler := corsMiddleware(handler)

		// Test Actual Request (GET)
		reqGet := httptest.NewRequest("GET", "/test", nil)
		recGet := httptest.NewRecorder()
		wrappedHandler.ServeHTTP(recGet, reqGet)

		// Check GET Response Headers (should all be empty)
		if recGet.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Errorf("Empty GET: Expected Allow-Origin empty, got '%s'", recGet.Header().Get("Access-Control-Allow-Origin"))
		}
		if recGet.Header().Get("Access-Control-Expose-Headers") != "" {
			t.Errorf("Empty GET: Expected Expose-Headers empty, got '%s'", recGet.Header().Get("Access-Control-Expose-Headers"))
		}
		if recGet.Header().Get("Access-Control-Allow-Credentials") != "" {
			t.Errorf("Empty GET: Expected Allow-Credentials empty, got '%s'", recGet.Header().Get("Access-Control-Allow-Credentials"))
		}
		if recGet.Header().Get("Access-Control-Allow-Methods") != "" {
			t.Errorf("Empty GET: Expected Allow-Methods empty, got '%s'", recGet.Header().Get("Access-Control-Allow-Methods"))
		}
		if recGet.Header().Get("Access-Control-Allow-Headers") != "" {
			t.Errorf("Empty GET: Expected Allow-Headers empty, got '%s'", recGet.Header().Get("Access-Control-Allow-Headers"))
		}
		if recGet.Header().Get("Access-Control-Max-Age") != "" {
			t.Errorf("Empty GET: Expected Max-Age empty, got '%s'", recGet.Header().Get("Access-Control-Max-Age"))
		}
		if recGet.Code != http.StatusOK {
			t.Errorf("Empty GET: Expected status code %d, got %d", http.StatusOK, recGet.Code)
		}

		// Test Preflight Request (OPTIONS)
		reqOptions := httptest.NewRequest("OPTIONS", "/test", nil)
		recOptions := httptest.NewRecorder()
		wrappedHandler.ServeHTTP(recOptions, reqOptions)

		// Check OPTIONS Response Headers (should all be empty)
		if recOptions.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Errorf("Empty OPTIONS: Expected Allow-Origin empty, got '%s'", recOptions.Header().Get("Access-Control-Allow-Origin"))
		}
		if recOptions.Header().Get("Access-Control-Allow-Methods") != "" {
			t.Errorf("Empty OPTIONS: Expected Allow-Methods empty, got '%s'", recOptions.Header().Get("Access-Control-Allow-Methods"))
		}
		if recOptions.Header().Get("Access-Control-Allow-Headers") != "" {
			t.Errorf("Empty OPTIONS: Expected Allow-Headers empty, got '%s'", recOptions.Header().Get("Access-Control-Allow-Headers"))
		}
		if recOptions.Header().Get("Access-Control-Max-Age") != "" {
			t.Errorf("Empty OPTIONS: Expected Max-Age empty, got '%s'", recOptions.Header().Get("Access-Control-Max-Age"))
		}
		if recOptions.Header().Get("Access-Control-Allow-Credentials") != "" {
			t.Errorf("Empty OPTIONS: Expected Allow-Credentials empty, got '%s'", recOptions.Header().Get("Access-Control-Allow-Credentials"))
		}
		if recOptions.Header().Get("Access-Control-Expose-Headers") != "" {
			t.Errorf("Empty OPTIONS: Expected Expose-Headers empty, got '%s'", recOptions.Header().Get("Access-Control-Expose-Headers"))
		}
		if recOptions.Code != http.StatusOK {
			t.Errorf("Empty OPTIONS: Expected status code %d, got %d", http.StatusOK, recOptions.Code)
		}
	})
}

// mockResponseWriter is a mock http.ResponseWriter that implements http.Flusher
type mockResponseWriter struct {
	http.ResponseWriter
	flushed bool
}

func (m *mockResponseWriter) Flush() {
	m.flushed = true
}

// TestMutexResponseWriter_Write tests the Write method of mutexResponseWriter
func TestMutexResponseWriter_Write(t *testing.T) {
	// Create a mock response writer
	rec := httptest.NewRecorder()

	// Create a mutex
	mu := &sync.Mutex{}

	// Create a mutexResponseWriter
	mrw := &mutexResponseWriter{
		ResponseWriter: rec,
		mu:             mu,
	}

	// Write some data
	data := []byte("test data")
	n, err := mrw.Write(data)

	// Check that the write was successful
	if err != nil {
		t.Errorf("Expected no error, got %v", err)
	}
	if n != len(data) {
		t.Errorf("Expected to write %d bytes, wrote %d", len(data), n)
	}
	if rec.Body.String() != "test data" {
		t.Errorf("Expected body to be 'test data', got '%s'", rec.Body.String())
	}
}

// TestMutexResponseWriter_Flush tests the Flush method of mutexResponseWriter
func TestMutexResponseWriter_Flush(t *testing.T) {
	// Create a mock response writer that implements http.Flusher
	rec := &mockResponseWriter{
		ResponseWriter: httptest.NewRecorder(),
	}

	// Create a mutex
	mu := &sync.Mutex{}

	// Create a mutexResponseWriter
	mrw := &mutexResponseWriter{
		ResponseWriter: rec,
		mu:             mu,
	}

	// Flush the response
	mrw.Flush()

	// Check that the flush was called
	if !rec.flushed {
		t.Error("Expected Flush to be called")
	}
}

// mockErrorWriter is a mock http.ResponseWriter that returns an error on Write
type mockErrorWriter struct {
	http.ResponseWriter
}

func (m *mockErrorWriter) Write(p []byte) (int, error) {
	return 0, errors.New("write error")
}

// TestMutexResponseWriter_WriteError tests the Write method of mutexResponseWriter with an error
func TestMutexResponseWriter_WriteError(t *testing.T) {
	// Create a mock response writer that returns an error on Write
	rec := &mockErrorWriter{
		ResponseWriter: httptest.NewRecorder(),
	}

	// Create a mutex
	mu := &sync.Mutex{}

	// Create a mutexResponseWriter
	mrw := &mutexResponseWriter{
		ResponseWriter: rec,
		mu:             mu,
	}

	// Write some data
	data := []byte("test data")
	n, err := mrw.Write(data)

	// Check that the write error was propagated
	if err == nil {
		t.Error("Expected an error, got nil")
	}
	if n != 0 {
		t.Errorf("Expected to write 0 bytes, wrote %d", n)
	}
}

// TestTimeout_ContextCancellation tests the Timeout middleware with a context cancellation
func TestTimeout_ContextCancellation(t *testing.T) {
	// Create a test handler that sleeps for 100ms
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(100 * time.Millisecond):
			w.WriteHeader(http.StatusOK)
		case <-r.Context().Done():
			// Context was cancelled, do nothing
		}
	})

	// Apply the Timeout middleware with a 50ms timeout
	timeoutMiddleware := Timeout(50 * time.Millisecond) // Use the variable
	wrappedHandler := timeoutMiddleware(handler)

	// Create a test request
	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	// Call the handler
	wrappedHandler.ServeHTTP(rec, req)

	// Check that the response status code is 408 (Request Timeout)
	if rec.Code != http.StatusRequestTimeout {
		t.Errorf("Expected status code %d, got %d", http.StatusRequestTimeout, rec.Code)
	}
}

// TestTimeout_HandlerCompletes tests the Timeout middleware with a handler that completes before the timeout
func TestTimeout_HandlerCompletes(t *testing.T) {
	// Create a test handler that returns immediately
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Apply the Timeout middleware with a 1s timeout
	timeoutMiddleware := Timeout(1 * time.Second) // Use the variable
	wrappedHandler := timeoutMiddleware(handler)

	// Create a test request
	req := httptest.NewRequest("GET", "/test", nil)
	rec := httptest.NewRecorder()

	// Call the handler
	wrappedHandler.ServeHTTP(rec, req)

	// Check that the response status code is 200 (OK)
	if rec.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rec.Code)
	}
}
