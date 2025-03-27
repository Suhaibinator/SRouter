package router

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/middleware"
	"github.com/Suhaibinator/SRouter/pkg/router/internal/mocks" // Use centralized mocks
	"go.uber.org/zap"
)

// --- Tests from register_subrouter_cache_test.go ---

// TestRegisterSubRouterWithCaching tests registerSubRouter with caching enabled
func TestRegisterSubRouterWithCaching(t *testing.T) {

	// Create a router with caching enabled
	r := NewRouter[string, string](RouterConfig{
		Logger: zap.NewNop(),
	}, mocks.MockAuthFunction, mocks.MockUserIDFromUser) // Use mock functions

	// Create a handler that returns a simple response
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("Hello, World!"))
	})

	// Create a sub-router with caching enabled
	sr := SubRouterConfig{
		PathPrefix: "/api",
		Routes: []RouteConfigBase{
			{
				Path:      "/hello",
				Methods:   []string{"GET"},
				Handler:   handler,
				AuthLevel: NoAuth,
			},
		},
	}

	// Register the sub-router
	r.registerSubRouter(sr)

	// Create a request
	req := httptest.NewRequest("GET", "/api/hello", nil)

	// Create a response recorder
	rr := httptest.NewRecorder()

	// Serve the request
	r.ServeHTTP(rr, req)

	// Check status code
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rr.Code)
	}

	// Check the response body
	if rr.Body.String() != "Hello, World!" {
		t.Errorf("Expected body %q, got %q", "Hello, World!", rr.Body.String())
	}

	// Make the same request again (should use cache)
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	// Check status code
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rr.Code)
	}

	// Check the response body
	if rr.Body.String() != "Hello, World!" {
		t.Errorf("Expected body %q, got %q", "Hello, World!", rr.Body.String())
	}
}

// TestRegisterSubRouterWithCachingNonGetMethod tests registerSubRouter with caching enabled and non-GET method
func TestRegisterSubRouterWithCachingNonGetMethod(t *testing.T) {

	// Create a router with caching enabled
	r := NewRouter[string, string](RouterConfig{
		Logger: zap.NewNop(),
	}, mocks.MockAuthFunction, mocks.MockUserIDFromUser) // Use mock functions

	// Create a handler that returns a simple response
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("Hello, World!"))
	})

	// Create a sub-router with caching enabled
	sr := SubRouterConfig{
		PathPrefix: "/api",
		Routes: []RouteConfigBase{
			{
				Path:      "/hello",
				Methods:   []string{"POST"}, // Non-GET method
				Handler:   handler,
				AuthLevel: NoAuth,
			},
		},
	}

	// Register the sub-router
	r.registerSubRouter(sr)

	// Create a request
	req := httptest.NewRequest("POST", "/api/hello", nil)

	// Create a response recorder
	rr := httptest.NewRecorder()

	// Serve the request
	r.ServeHTTP(rr, req)

	// Check status code
	if rr.Code != http.StatusOK { // Note: The handler runs, but caching logic is skipped for non-GET
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rr.Code)
	}

	// Check the response body
	if rr.Body.String() != "Hello, World!" {
		t.Errorf("Expected body %q, got %q", "Hello, World!", rr.Body.String())
	}
}

// TestRegisterSubRouterWithCachingError tests registerSubRouter with caching enabled and cache error
func TestRegisterSubRouterWithCachingErrorCoverage(t *testing.T) { // Renamed to avoid conflict

	// Create a router with caching enabled
	r := NewRouter[string, string](RouterConfig{
		Logger: zap.NewNop(),
	}, mocks.MockAuthFunction, mocks.MockUserIDFromUser) // Use mock functions

	// Create a handler that returns a simple response
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("Hello, World!"))
	})

	// Create a sub-router with caching enabled
	sr := SubRouterConfig{
		PathPrefix: "/api",
		Routes: []RouteConfigBase{
			{
				Path:      "/hello",
				Methods:   []string{"GET"},
				Handler:   handler,
				AuthLevel: NoAuth,
			},
		},
	}

	// Register the sub-router
	r.registerSubRouter(sr)

	// Create a request
	req := httptest.NewRequest("GET", "/api/hello", nil)

	// Create a response recorder
	rr := httptest.NewRecorder()

	// Serve the request
	r.ServeHTTP(rr, req)

	// Check status code
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rr.Code)
	}

	// Check the response body
	if rr.Body.String() != "Hello, World!" {
		t.Errorf("Expected body %q, got %q", "Hello, World!", rr.Body.String())
	}
}

// --- Original tests from subrouter_test.go ---

// TestRegisterSubRouter tests the registerSubRouter function with various configurations
func TestRegisterSubRouter(t *testing.T) {
	// Create a logger
	logger := zap.NewNop()

	// Create a router with caching enabled
	r := NewRouter[string, string](RouterConfig{
		Logger:        logger,
		GlobalTimeout: 5 * time.Second,
	},
		mocks.MockAuthFunction,   // Use mock function
		mocks.MockUserIDFromUser) // Use mock function

	// Create a middleware that adds a header
	headerMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Test-Middleware", "true")
			next.ServeHTTP(w, r)
		})
	}

	// Create a rate limit config
	rateLimitConfig := &middleware.RateLimitConfig[any, any]{
		Limit:  10,
		Window: time.Minute,
	}

	// Register a sub-router with various configurations
	r.registerSubRouter(SubRouterConfig{
		PathPrefix:          "/api",
		TimeoutOverride:     2 * time.Second,
		MaxBodySizeOverride: 1024,
		RateLimitOverride:   rateLimitConfig,
		Middlewares:         []Middleware{headerMiddleware},
		Routes: []RouteConfigBase{
			{
				Path:      "/users",
				Methods:   []string{"GET"},
				AuthLevel: NoAuth,
				Handler: func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{"users":["user1","user2"]}`))
				},
			},
			{
				Path:      "/protected",
				Methods:   []string{"GET"},
				AuthLevel: AuthRequired,
				Handler: func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{"message":"protected resource"}`))
				},
			},
			{
				Path:      "/custom-timeout",
				Methods:   []string{"GET"},
				AuthLevel: NoAuth,
				Timeout:   1 * time.Second, // Override sub-router timeout
				Handler: func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{"message":"custom timeout"}`))
				},
			},
			{
				Path:        "/custom-body-size",
				Methods:     []string{"POST"},
				AuthLevel:   NoAuth,
				MaxBodySize: 512, // Override sub-router max body size
				Handler: func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{"message":"custom body size"}`))
				},
			},
			{
				Path:      "/custom-rate-limit",
				Methods:   []string{"GET"},
				AuthLevel: NoAuth,
				RateLimit: &middleware.RateLimitConfig[any, any]{
					Limit:  5,
					Window: 30 * time.Second,
				}, // Override sub-router rate limit
				Handler: func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{"message":"custom rate limit"}`))
				},
			},
			{
				Path:      "/custom-middleware",
				Methods:   []string{"GET"},
				AuthLevel: NoAuth,
				Middlewares: []Middleware{
					func(next http.Handler) http.Handler {
						return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							w.Header().Set("X-Custom-Middleware", "true")
							next.ServeHTTP(w, r)
						})
					},
				},
				Handler: func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{"message":"custom middleware"}`))
				},
			},
		},
	})

	// Test the regular route
	req, _ := http.NewRequest("GET", "/api/users", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	// Check status code
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rr.Code)
	}

	// Check middleware was applied
	if rr.Header().Get("X-Test-Middleware") != "true" {
		t.Errorf("Expected X-Test-Middleware header to be set")
	}

	// Test the protected route without auth
	req, _ = http.NewRequest("GET", "/api/protected", nil)
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	// Check status code (should be unauthorized)
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Expected status code %d, got %d", http.StatusUnauthorized, rr.Code)
	}

	// Test the protected route with auth
	req, _ = http.NewRequest("GET", "/api/protected", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	// Check status code
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rr.Code)
	}

	// Test the custom timeout route
	req, _ = http.NewRequest("GET", "/api/custom-timeout", nil)
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	// Check status code
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rr.Code)
	}

	// Test the custom middleware route
	req, _ = http.NewRequest("GET", "/api/custom-middleware", nil)
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	// Check status code
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rr.Code)
	}

	// Check both middlewares were applied
	if rr.Header().Get("X-Test-Middleware") != "true" {
		t.Errorf("Expected X-Test-Middleware header to be set")
	}
	if rr.Header().Get("X-Custom-Middleware") != "true" {
		t.Errorf("Expected X-Custom-Middleware header to be set")
	}

	// Test non-GET method (should not be cached)
	req, _ = http.NewRequest("POST", "/api/users", nil)
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	// Check status code (should be method not allowed)
	if rr.Code != http.StatusMethodNotAllowed {
		t.Errorf("Expected status code %d, got %d", http.StatusMethodNotAllowed, rr.Code)
	}
}

// TestRegisterSubRouterWithoutCaching tests the registerSubRouter function without caching
func TestRegisterSubRouterWithoutCaching(t *testing.T) {
	// Create a logger
	logger := zap.NewNop()

	// Create a router without caching
	r := NewRouter[string, string](RouterConfig{
		Logger: logger,
	},
		mocks.MockAuthFunction,   // Use mock function
		mocks.MockUserIDFromUser) // Use mock function

	// Register a sub-router with caching enabled (but router doesn't support it)
	r.registerSubRouter(SubRouterConfig{
		PathPrefix: "/api",
		Routes: []RouteConfigBase{
			{
				Path:      "/users",
				Methods:   []string{"GET"},
				AuthLevel: NoAuth,
				Handler: func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.WriteHeader(http.StatusOK)
					_, _ = w.Write([]byte(`{"users":["user1","user2"]}`))
				},
			},
		},
	})

	// Test the route
	req, _ := http.NewRequest("GET", "/api/users", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	// Check status code
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rr.Code)
	}

	// Check response body
	expected := `{"users":["user1","user2"]}`
	if rr.Body.String() != expected {
		t.Errorf("Expected response body %q, got %q", expected, rr.Body.String())
	}
}
