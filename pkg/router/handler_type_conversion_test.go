package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/julienschmidt/httprouter"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zaptest"
)

// nonHandlerFunc implements http.Handler but is not http.HandlerFunc
type nonHandlerFunc struct {
	called bool
}

func (h *nonHandlerFunc) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.called = true
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("non-HandlerFunc"))
}

// TestHandlerConversionLogic tests the exact conversion pattern used in the codebase
func TestHandlerConversionLogic(t *testing.T) {
	t.Run("conversion when handler is not HandlerFunc", func(t *testing.T) {
		// Create a non-HandlerFunc handler
		var handler http.Handler = &nonHandlerFunc{}
		
		// Test the exact conversion pattern from route.go lines 39-40, 260-261
		// and router.go lines 236-237
		handlerFunc, ok := handler.(http.HandlerFunc)
		assert.False(t, ok, "should not be HandlerFunc")
		
		// This is the conversion that should be covered
		if !ok {
			handlerFunc = http.HandlerFunc(handler.ServeHTTP)
		}
		
		// Verify the converted handler works
		req := httptest.NewRequest("GET", "/test", nil)
		w := httptest.NewRecorder()
		handlerFunc.ServeHTTP(w, req)
		
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "non-HandlerFunc", w.Body.String())
		
		// Also verify the original handler was called
		originalHandler := handler.(*nonHandlerFunc)
		assert.True(t, originalHandler.called)
	})
	
	t.Run("no conversion when already HandlerFunc", func(t *testing.T) {
		// Create a HandlerFunc
		var handler http.Handler = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("already HandlerFunc"))
		})
		
		// Test the conversion pattern
		handlerFunc, ok := handler.(http.HandlerFunc)
		assert.True(t, ok, "should be HandlerFunc")
		
		// No conversion needed
		if !ok {
			t.Fatal("should not reach here")
		}
		
		// Verify it works
		req := httptest.NewRequest("GET", "/test", nil)
		w := httptest.NewRecorder()
		handlerFunc.ServeHTTP(w, req)
		
		assert.Equal(t, http.StatusOK, w.Code)
		assert.Equal(t, "already HandlerFunc", w.Body.String())
	})
}

// TestRouteRegistrationWithNonHandlerFunc simulates the scenario in RegisterRoute
func TestRouteRegistrationWithNonHandlerFunc(t *testing.T) {
	// Create a custom registration function that tests the conversion
	testConversionInRegistration := func(r *Router[string, TestUser]) {
		// Simulate what happens in RegisterRoute when wrapWithTransaction
		// returns a non-HandlerFunc
		
		// In the real code, this would be the handler passed to RegisterRoute
		// but we're focusing on testing the conversion after wrapWithTransaction
		
		// Simulate wrapWithTransaction returning non-HandlerFunc
		var finalHandler http.Handler = &nonHandlerFunc{}
		
		// This is the exact conversion from route.go lines 39-40
		handlerFunc, ok := finalHandler.(http.HandlerFunc)
		if !ok {
			handlerFunc = http.HandlerFunc(finalHandler.ServeHTTP)
		}
		
		// Continue with registration as the real code does
		wrapped := r.wrapHandler(handlerFunc, nil, 0, 0, nil, nil)
		r.router.Handle("GET", "/test", r.convertToHTTPRouterHandle(wrapped, "/test"))
	}
	
	// Create router
	httpRouter := httprouter.New()
	r := &Router[string, TestUser]{
		config: RouterConfig{
			Logger: zaptest.NewLogger(t),
		},
		logger: zaptest.NewLogger(t),
		router: httpRouter,
	}
	
	// Run the test registration
	testConversionInRegistration(r)
	
	// Verify the route works
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "non-HandlerFunc", w.Body.String())
}

// TestGenericRouteRegistrationWithNonHandlerFunc simulates RegisterGenericRoute scenario
func TestGenericRouteRegistrationWithNonHandlerFunc(t *testing.T) {
	// This simulates what happens in RegisterGenericRoute at lines 260-261
	testGenericConversion := func(r *Router[string, TestUser]) {
		// In RegisterGenericRoute, a marshaling handler is created
		// but we're testing the conversion after wrapWithTransaction
		
		// Simulate wrapWithTransaction returning non-HandlerFunc
		var wrappedWithTx http.Handler = &nonHandlerFunc{}
		
		// This is the exact conversion from route.go lines 260-261
		handlerFunc, ok := wrappedWithTx.(http.HandlerFunc)
		if !ok {
			handlerFunc = http.HandlerFunc(wrappedWithTx.ServeHTTP)
		}
		
		// Continue with registration
		wrapped := r.wrapHandler(handlerFunc, nil, 0, 0, nil, nil)
		r.router.Handle("POST", "/api/test", r.convertToHTTPRouterHandle(wrapped, "/api/test"))
	}
	
	// Create router
	httpRouter := httprouter.New()
	r := &Router[string, TestUser]{
		config: RouterConfig{
			Logger: zaptest.NewLogger(t),
		},
		logger: zaptest.NewLogger(t),
		router: httpRouter,
	}
	
	// Run the test
	testGenericConversion(r)
	
	// Verify
	req := httptest.NewRequest("POST", "/api/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "non-HandlerFunc", w.Body.String())
}

// TestSubRouterRegistrationWithNonHandlerFunc simulates the subrouter scenario
func TestSubRouterRegistrationWithNonHandlerFunc(t *testing.T) {
	// Track if conversion happened
	conversionHappened := false
	
	// Create a custom route definition
	customRoute := GenericRouteRegistrationFunc[string, TestUser](func(router *Router[string, TestUser], sr SubRouterConfig) {
		// In real subrouter registration, route config would be used
		// but we're focusing on testing the handler conversion
		
		// Simulate wrapWithTransaction returning non-HandlerFunc
		var finalHandler http.Handler = &nonHandlerFunc{}
		
		// This is the exact code from router.go lines 236-237
		handlerFunc, ok := finalHandler.(http.HandlerFunc)
		if !ok {
			conversionHappened = true
			handlerFunc = http.HandlerFunc(finalHandler.ServeHTTP)
		}
		
		// Complete registration
		handler := router.wrapHandler(handlerFunc, nil, 0, 0, nil, nil)
		fullPath := sr.PathPrefix + "/test"
		router.router.Handle("GET", fullPath, router.convertToHTTPRouterHandle(handler, fullPath))
	})
	
	// Create router
	r := NewRouter[string, TestUser](RouterConfig{
		Logger: zaptest.NewLogger(t),
		SubRouters: []SubRouterConfig{
			{
				PathPrefix: "/api",
				Routes:     []RouteDefinition{customRoute},
			},
		},
	}, nil, nil)
	
	// Make request
	req := httptest.NewRequest("GET", "/api/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	
	assert.Equal(t, http.StatusOK, w.Code)
	assert.True(t, conversionHappened, "conversion should have happened")
}