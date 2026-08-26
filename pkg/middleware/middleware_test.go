package middleware

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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
		_, _ = fmt.Fprintf(w, "Body size: %d", len(body))
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
