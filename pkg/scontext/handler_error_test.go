package scontext

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestHandlerError(t *testing.T) {
	t.Run("WithHandlerError and GetHandlerError", func(t *testing.T) {
		ctx := context.Background()
		testErr := errors.New("test handler error")

		// Test setting and getting handler error
		ctx = WithHandlerError[int, string](ctx, testErr)

		err, ok := GetHandlerError[int, string](ctx)
		if !ok {
			t.Error("Expected handler error to be set")
		}
		if err != testErr {
			t.Errorf("Expected error %v, got %v", testErr, err)
		}
	})

	t.Run("GetHandlerError with no error set", func(t *testing.T) {
		ctx := context.Background()

		err, ok := GetHandlerError[int, string](ctx)
		if ok {
			t.Error("Expected no handler error to be found")
		}
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}
	})

	t.Run("GetHandlerError with nil error", func(t *testing.T) {
		ctx := context.Background()

		// Explicitly set nil error
		ctx = WithHandlerError[int, string](ctx, nil)

		err, ok := GetHandlerError[int, string](ctx)
		if !ok {
			t.Error("Expected handler error to be set (even if nil)")
		}
		if err != nil {
			t.Errorf("Expected nil error, got %v", err)
		}
	})

	t.Run("GetHandlerErrorFromRequest", func(t *testing.T) {
		testErr := errors.New("request handler error")

		// Create request with context containing error
		req := httptest.NewRequest("GET", "/test", nil)
		ctx := WithHandlerError[int, string](req.Context(), testErr)
		req = req.WithContext(ctx)

		err, ok := GetHandlerErrorFromRequest[int, string](req)
		if !ok {
			t.Error("Expected handler error to be set in request")
		}
		if err != testErr {
			t.Errorf("Expected error %v, got %v", testErr, err)
		}
	})

	t.Run("Multiple errors overwrites previous", func(t *testing.T) {
		ctx := context.Background()
		err1 := errors.New("first error")
		err2 := errors.New("second error")

		ctx = WithHandlerError[int, string](ctx, err1)
		ctx = WithHandlerError[int, string](ctx, err2)

		err, ok := GetHandlerError[int, string](ctx)
		if !ok {
			t.Error("Expected handler error to be set")
		}
		if err != err2 {
			t.Errorf("Expected second error %v, got %v", err2, err)
		}
	})

	t.Run("Type safety with different type parameters", func(t *testing.T) {
		ctx := context.Background()
		testErr := errors.New("type-specific error")

		// Set error with one type parameter set
		ctx = WithHandlerError[string, interface{}](ctx, testErr)

		// Try to get with different type parameters - should not find it
		_, ok := GetHandlerError[int, string](ctx)
		if ok {
			t.Error("Expected no error when using different type parameters")
		}

		// Get with correct type parameters
		err, ok := GetHandlerError[string, interface{}](ctx)
		if !ok {
			t.Error("Expected handler error to be found with correct type parameters")
		}
		if err != testErr {
			t.Errorf("Expected error %v, got %v", testErr, err)
		}
	})
}

func TestHandlerErrorInMiddleware(t *testing.T) {
	// Simulate a middleware that checks for handler errors
	errorCheckingMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Call the next handler
			next.ServeHTTP(w, r)

			// After handler execution, check for errors
			if err, ok := GetHandlerErrorFromRequest[int, string](r); ok && err != nil {
				// In real middleware, you might rollback a transaction here
				w.Header().Set("X-Handler-Error", err.Error())
			}
		})
	}

	t.Run("Middleware detects handler error", func(t *testing.T) {
		testErr := errors.New("handler failed")

		// Simulate a handler that sets an error in context
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// This simulates what RegisterGenericRoute does when handler returns error
			ctx := WithHandlerError[int, string](r.Context(), testErr)
			*r = *r.WithContext(ctx)

			// Write response (normally handleError would do this)
			w.WriteHeader(http.StatusInternalServerError)
		})

		// Wrap handler with middleware
		wrapped := errorCheckingMiddleware(handler)

		// Make request
		req := httptest.NewRequest("GET", "/test", nil)
		rec := httptest.NewRecorder()

		wrapped.ServeHTTP(rec, req)

		// Check that middleware detected the error
		if rec.Header().Get("X-Handler-Error") != testErr.Error() {
			t.Errorf("Expected middleware to detect error '%s', got '%s'",
				testErr.Error(), rec.Header().Get("X-Handler-Error"))
		}
	})

	t.Run("Middleware with no handler error", func(t *testing.T) {
		// Handler that succeeds (no error in context)
		handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		// Wrap handler with middleware
		wrapped := errorCheckingMiddleware(handler)

		// Make request
		req := httptest.NewRequest("GET", "/test", nil)
		rec := httptest.NewRecorder()

		wrapped.ServeHTTP(rec, req)

		// Check that middleware didn't set error header
		if rec.Header().Get("X-Handler-Error") != "" {
			t.Errorf("Expected no error header, got '%s'", rec.Header().Get("X-Handler-Error"))
		}
	})
}
