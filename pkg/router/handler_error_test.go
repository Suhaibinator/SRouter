package router

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Suhaibinator/SRouter/pkg/codec"
	"github.com/Suhaibinator/SRouter/pkg/common"
	"github.com/Suhaibinator/SRouter/pkg/scontext"
	"go.uber.org/zap"
)

func TestGenericRouteHandlerError(t *testing.T) {
	// Create a test router with dummy auth functions
	getUserByID := func(ctx context.Context, userID string) (*interface{}, bool) {
		return nil, false
	}
	getUserID := func(user *interface{}) int {
		return 0
	}
	
	router := NewRouter[int, interface{}](RouterConfig{
		Logger: zap.NewNop(),
	}, getUserByID, getUserID)

	type TestRequest struct {
		Value string `json:"value"`
	}
	type TestResponse struct {
		Result string `json:"result"`
	}

	// Track if middleware saw the error
	var middlewareSawError error
	var middlewareExecuted bool

	// Create middleware that checks for handler errors
	errorCheckingMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Let the handler run first
			next.ServeHTTP(w, r)
			
			// Check if handler set an error
			middlewareExecuted = true
			middlewareSawError, _ = scontext.GetHandlerErrorFromRequest[int, interface{}](r)
		})
	}

	t.Run("Handler error is stored in context", func(t *testing.T) {
		// Reset tracking variables
		middlewareExecuted = false
		middlewareSawError = nil
		
		expectedErr := errors.New("handler error")
		
		// Register a generic route that returns an error
		RegisterGenericRoute(router, RouteConfig[TestRequest, TestResponse]{
			Path:     "/error",
			Methods:  []HttpMethod{MethodGet},
			AuthLevel: Ptr(NoAuth),
			Middlewares: []common.Middleware{errorCheckingMiddleware},
			Handler: func(r *http.Request, req TestRequest) (TestResponse, error) {
				return TestResponse{}, expectedErr
			},
			Codec: &codec.JSONCodec[TestRequest, TestResponse]{},
		}, 0, 0, nil)

		// Make request with valid JSON body
		req := httptest.NewRequest("GET", "/error", strings.NewReader(`{"value":"test"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		
		router.ServeHTTP(rec, req)
		
		// Verify middleware was executed
		if !middlewareExecuted {
			t.Error("Expected middleware to be executed")
		}
		
		// Verify middleware saw the error
		if middlewareSawError != expectedErr {
			t.Errorf("Expected middleware to see error %v, got %v", expectedErr, middlewareSawError)
		}
		
		// Verify error response
		if rec.Code != http.StatusInternalServerError {
			t.Errorf("Expected status %d, got %d", http.StatusInternalServerError, rec.Code)
		}
	})

	t.Run("Successful handler has no error in context", func(t *testing.T) {
		// Reset tracking variables
		middlewareExecuted = false
		middlewareSawError = nil
		
		// Register a generic route that succeeds
		RegisterGenericRoute(router, RouteConfig[TestRequest, TestResponse]{
			Path:     "/success",
			Methods:  []HttpMethod{MethodGet},
			AuthLevel: Ptr(NoAuth),
			Middlewares: []common.Middleware{errorCheckingMiddleware},
			Handler: func(r *http.Request, req TestRequest) (TestResponse, error) {
				return TestResponse{Result: "success"}, nil
			},
			Codec: &codec.JSONCodec[TestRequest, TestResponse]{},
		}, 0, 0, nil)

		// Make request with valid JSON body
		req := httptest.NewRequest("GET", "/success", strings.NewReader(`{"value":"test"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		
		router.ServeHTTP(rec, req)
		
		// Verify middleware was executed
		if !middlewareExecuted {
			t.Error("Expected middleware to be executed")
		}
		
		// Verify middleware saw no error
		if middlewareSawError != nil {
			t.Errorf("Expected no error in context, got %v", middlewareSawError)
		}
		
		// Verify success response
		if rec.Code != http.StatusOK {
			t.Errorf("Expected status %d, got %d", http.StatusOK, rec.Code)
		}
	})

	t.Run("Custom HTTPError is stored in context", func(t *testing.T) {
		// Reset tracking variables
		middlewareExecuted = false
		middlewareSawError = nil
		
		customErr := &HTTPError{
			StatusCode: http.StatusBadRequest,
			Message:    "Custom error",
		}
		
		// Register a generic route that returns a custom HTTP error
		RegisterGenericRoute(router, RouteConfig[TestRequest, TestResponse]{
			Path:     "/custom-error",
			Methods:  []HttpMethod{MethodGet},
			AuthLevel: Ptr(NoAuth),
			Middlewares: []common.Middleware{errorCheckingMiddleware},
			Handler: func(r *http.Request, req TestRequest) (TestResponse, error) {
				return TestResponse{}, customErr
			},
			Codec: &codec.JSONCodec[TestRequest, TestResponse]{},
		}, 0, 0, nil)

		// Make request with valid JSON body
		req := httptest.NewRequest("GET", "/custom-error", strings.NewReader(`{"value":"test"}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		
		router.ServeHTTP(rec, req)
		
		// Verify middleware saw the custom error
		if !errors.Is(middlewareSawError, customErr) {
			t.Errorf("Expected middleware to see custom error %v, got %v", customErr, middlewareSawError)
		}
		
		// Verify custom error response
		if rec.Code != http.StatusBadRequest {
			t.Errorf("Expected status %d, got %d", http.StatusBadRequest, rec.Code)
		}
	})
}

func TestHandlerErrorWithMultipleMiddleware(t *testing.T) {
	getUserByID := func(ctx context.Context, userID string) (*interface{}, bool) {
		return nil, false
	}
	getUserID := func(user *interface{}) int {
		return 0
	}
	
	router := NewRouter[int, interface{}](RouterConfig{
		Logger: zap.NewNop(),
	}, getUserByID, getUserID)

	type TestRequest struct{}
	type TestResponse struct{}

	// Track middleware execution order and error visibility
	var executionOrder []string
	var errorsSeen []error

	createMiddleware := func(name string) common.Middleware {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				executionOrder = append(executionOrder, name+"-before")
				
				next.ServeHTTP(w, r)
				
				executionOrder = append(executionOrder, name+"-after")
				if err, ok := scontext.GetHandlerErrorFromRequest[int, interface{}](r); ok {
					errorsSeen = append(errorsSeen, err)
				}
			})
		}
	}

	t.Run("Multiple middleware can see handler error", func(t *testing.T) {
		// Reset tracking
		executionOrder = []string{}
		errorsSeen = []error{}
		
		expectedErr := errors.New("multi-middleware error")
		
		RegisterGenericRoute(router, RouteConfig[TestRequest, TestResponse]{
			Path:     "/multi-middleware",
			Methods:  []HttpMethod{MethodGet},
			AuthLevel: Ptr(NoAuth),
			Middlewares: []common.Middleware{
				createMiddleware("outer"),
				createMiddleware("inner"),
			},
			Handler: func(r *http.Request, req TestRequest) (TestResponse, error) {
				executionOrder = append(executionOrder, "handler")
				return TestResponse{}, expectedErr
			},
			Codec: &codec.JSONCodec[TestRequest, TestResponse]{},
		}, 0, 0, nil)

		// Make request with valid JSON body
		req := httptest.NewRequest("GET", "/multi-middleware", strings.NewReader(`{}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		
		router.ServeHTTP(rec, req)
		
		// Verify execution order
		expectedOrder := []string{
			"outer-before",
			"inner-before", 
			"handler",
			"inner-after",
			"outer-after",
		}
		
		if len(executionOrder) != len(expectedOrder) {
			t.Fatalf("Expected %d execution steps, got %d", len(expectedOrder), len(executionOrder))
		}
		
		for i, expected := range expectedOrder {
			if executionOrder[i] != expected {
				t.Errorf("Expected execution step %d to be '%s', got '%s'", i, expected, executionOrder[i])
			}
		}
		
		// Verify both middleware saw the error
		if len(errorsSeen) != 2 {
			t.Fatalf("Expected 2 middleware to see error, got %d", len(errorsSeen))
		}
		
		for i, err := range errorsSeen {
			if err != expectedErr {
				t.Errorf("Middleware %d saw wrong error: expected %v, got %v", i, expectedErr, err)
			}
		}
	})
}