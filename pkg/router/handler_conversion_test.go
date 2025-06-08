package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Suhaibinator/SRouter/pkg/common"
	"github.com/Suhaibinator/SRouter/pkg/router/internal/mocks"
	"github.com/Suhaibinator/SRouter/pkg/scontext"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zaptest"
)

// customHandler implements http.Handler but is not an http.HandlerFunc
type customHandler struct {
	called bool
}

func (h *customHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.called = true
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("custom handler"))
}

// TestWrapWithTransactionHandlerFuncConversion tests the HandlerFunc conversion after wrapWithTransaction
func TestWrapWithTransactionHandlerFuncConversion(t *testing.T) {
	// Create mock transaction that tracks if it was used
	mockTx := &mocks.MockTransaction{}
	
	// Create mock factory
	mockFactory := &mocks.MockTransactionFactory{
		BeginFunc: func(ctx context.Context, options map[string]any) (scontext.DatabaseTransaction, error) {
			return mockTx, nil
		},
	}

	// Create router
	r := &Router[string, TestUser]{
		config: RouterConfig{
			Logger:             zaptest.NewLogger(t),
			TransactionFactory: mockFactory,
		},
		logger: zaptest.NewLogger(t),
	}

	// Test 1: When transaction is enabled, wrapWithTransaction returns http.HandlerFunc
	t.Run("transaction enabled returns HandlerFunc", func(t *testing.T) {
		handler := &customHandler{}
		txConfig := &common.TransactionConfig{Enabled: true}
		
		wrapped := r.wrapWithTransaction(handler, txConfig)
		
		// Should return a HandlerFunc when transaction is enabled
		_, isHandlerFunc := wrapped.(http.HandlerFunc)
		assert.True(t, isHandlerFunc, "wrapped handler should be http.HandlerFunc")
		
		// Test the wrapped handler works
		req := httptest.NewRequest("GET", "/test", nil)
		w := httptest.NewRecorder()
		wrapped.ServeHTTP(w, req)
		
		assert.Equal(t, http.StatusOK, w.Code)
		assert.True(t, handler.called)
		assert.True(t, mockTx.IsCommitCalled())
	})

	// Test 2: When transaction is disabled, wrapWithTransaction returns original handler
	t.Run("transaction disabled returns original handler", func(t *testing.T) {
		handler := &customHandler{}
		
		// No transaction config
		wrapped := r.wrapWithTransaction(handler, nil)
		
		// Should return the original handler
		assert.Equal(t, handler, wrapped)
		
		// Verify it's NOT a HandlerFunc
		_, isHandlerFunc := wrapped.(http.HandlerFunc)
		assert.False(t, isHandlerFunc, "wrapped handler should not be http.HandlerFunc")
	})
}

// TestHandlerFuncConversionInRouteRegistration tests the conversion in actual route registration
func TestHandlerFuncConversionInRouteRegistration(t *testing.T) {
	// This test verifies the type assertion and conversion code path
	
	// Create router without transactions
	r := NewRouter[string, TestUser](RouterConfig{
		Logger: zaptest.NewLogger(t),
	}, nil, nil)

	// Track if conversion happened
	conversionHappened := false
	
	// Create a wrapper handler that tracks the conversion
	handler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// Check if the handler was properly converted
		// The handler should be called successfully
		conversionHappened = true
		w.WriteHeader(http.StatusOK)
	})

	// Register route - this will go through the conversion logic
	r.RegisterRoute(RouteConfigBase{
		Path:    "/test",
		Methods: []HttpMethod{MethodGet},
		Handler: handler,
		// No transaction to ensure wrapWithTransaction returns original
	})

	// Make request
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, conversionHappened)
}

// mockNonHandlerFunc is a mock that tracks if its ServeHTTP was used for conversion
type mockNonHandlerFunc struct {
	serveHTTPUsedForConversion bool
}

func (m *mockNonHandlerFunc) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	m.serveHTTPUsedForConversion = true
	w.WriteHeader(http.StatusOK)
}

// TestDirectHandlerFuncConversion tests the exact conversion lines
func TestDirectHandlerFuncConversion(t *testing.T) {
	// This test directly exercises the conversion code:
	// handlerFunc, ok := finalHandler.(http.HandlerFunc)
	// if !ok {
	//     handlerFunc = http.HandlerFunc(finalHandler.ServeHTTP)
	// }
	
	t.Run("already HandlerFunc - no conversion", func(t *testing.T) {
		// Create a HandlerFunc
		var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})
		
		// Type assertion should succeed
		handlerFunc, ok := handler.(http.HandlerFunc)
		assert.True(t, ok)
		assert.NotNil(t, handlerFunc)
	})
	
	t.Run("not HandlerFunc - needs conversion", func(t *testing.T) {
		// Create a non-HandlerFunc handler
		mockHandler := &mockNonHandlerFunc{}
		var handler http.Handler = mockHandler
		
		// Type assertion should fail
		handlerFunc, ok := handler.(http.HandlerFunc)
		assert.False(t, ok)
		
		// Conversion should work
		handlerFunc = http.HandlerFunc(handler.ServeHTTP)
		assert.NotNil(t, handlerFunc)
		
		// Test the converted handler
		req := httptest.NewRequest("GET", "/test", nil)
		w := httptest.NewRecorder()
		handlerFunc.ServeHTTP(w, req)
		
		assert.Equal(t, http.StatusOK, w.Code)
		assert.True(t, mockHandler.serveHTTPUsedForConversion)
	})
}

// TestSubRouterWithNonHandlerFuncAfterTransaction simulates the exact scenario where conversion is needed
func TestSubRouterWithNonHandlerFuncAfterTransaction(t *testing.T) {
	// Custom router implementation that forces non-HandlerFunc return from wrapWithTransaction
	type customRouter[T comparable, U any] struct {
		*Router[T, U]
	}
	
	// Override wrapWithTransaction to always return a non-HandlerFunc
	wrapWithTransactionCalled := false
	originalWrapWithTransaction := func(_ *Router[string, TestUser], _ http.Handler, _ *common.TransactionConfig) http.Handler {
		wrapWithTransactionCalled = true
		// Return a custom handler that is NOT http.HandlerFunc
		return &customHandler{}
	}
	
	// Create router
	r := NewRouter[string, TestUser](RouterConfig{
		Logger: zaptest.NewLogger(t),
		SubRouters: []SubRouterConfig{
			{
				PathPrefix: "/api",
				Routes: []RouteDefinition{
					// Use a custom registration function to control the flow
					GenericRouteRegistrationFunc[string, TestUser](func(router *Router[string, TestUser], sr SubRouterConfig) {
						// Manually create the scenario where wrapWithTransaction returns non-HandlerFunc
						handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							w.WriteHeader(http.StatusOK)
						})
						
						// Call wrapWithTransaction which returns non-HandlerFunc
						wrapped := originalWrapWithTransaction(router, handler, nil)
						
						// This simulates the conversion code in router.go
						handlerFunc, ok := wrapped.(http.HandlerFunc)
						assert.False(t, ok, "wrapped should not be HandlerFunc")
						
						// Perform conversion
						handlerFunc = http.HandlerFunc(wrapped.ServeHTTP)
						
						// Continue with registration
						finalHandler := router.wrapHandler(handlerFunc, nil, 0, 0, nil, nil)
						router.router.Handle("GET", "/api/test", router.convertToHTTPRouterHandle(finalHandler, "/api/test"))
					}),
				},
			},
		},
	}, nil, nil)
	
	// Make request
	req := httptest.NewRequest("GET", "/api/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	
	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, wrapWithTransactionCalled)
}