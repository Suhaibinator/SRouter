package router

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/codec"
	"github.com/Suhaibinator/SRouter/pkg/middleware"
	"go.uber.org/zap"
)

// TestSubRouterIntegration tests that sub-routers work correctly
func TestSubRouterIntegration(t *testing.T) {
	// Create a logger
	logger, _ := zap.NewDevelopment()

	// Create a router with sub-routers and string as both the user ID and user type
	r := NewRouter(RouterConfig{
		Logger:            logger,
		GlobalTimeout:     1 * time.Second,
		GlobalMaxBodySize: 1024, // 1 KB
		SubRouters: []SubRouterConfig{
			{
				PathPrefix:          "/api/v1",
				TimeoutOverride:     2 * time.Second,
				MaxBodySizeOverride: 2048, // 2 KB
				Routes: []any{ // Changed to []any
					RouteConfigBase{
						Path:    "/users",
						Methods: []string{"GET"},
						Handler: func(w http.ResponseWriter, r *http.Request) {
							w.Header().Set("Content-Type", "application/json")
							_, _ = w.Write([]byte(`{"users":["user1","user2"]}`))
						},
					},
				},
			},
			{
				PathPrefix: "/api/v2",
				Routes: []any{ // Changed to []any
					RouteConfigBase{
						Path:    "/users",
						Methods: []string{"GET"},
						Handler: func(w http.ResponseWriter, r *http.Request) {
							w.Header().Set("Content-Type", "application/json")
							_, _ = w.Write([]byte(`{"users":["user3","user4"]}`))
						},
					},
				},
			},
		},
	},
		// Mock auth function that always returns invalid
		func(ctx context.Context, token string) (string, bool) {
			return "", false
		},
		// Mock user ID function that returns the string itself
		func(user string) string {
			return user
		})

	// Test the first sub-router
	req, _ := http.NewRequest("GET", "/api/v1/users", nil)
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

	// Test the second sub-router
	req, _ = http.NewRequest("GET", "/api/v2/users", nil)
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	// Check status code
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rr.Code)
	}

	// Check response body
	expected = `{"users":["user3","user4"]}`
	if rr.Body.String() != expected {
		t.Errorf("Expected response body %q, got %q", expected, rr.Body.String())
	}
}

// TestPathParameters tests that path parameters are correctly extracted
func TestPathParameters(t *testing.T) {
	// Create a logger
	logger, _ := zap.NewDevelopment()

	// Create a router with a route that has path parameters and string as both the user ID and user type
	r := NewRouter(RouterConfig{
		Logger: logger,
		SubRouters: []SubRouterConfig{
			{
				PathPrefix: "/api",
				Routes: []any{ // Changed to []any
					RouteConfigBase{
						Path:    "/users/:id",
						Methods: []string{"GET"},
						Handler: func(w http.ResponseWriter, r *http.Request) {
							id := GetParam(r, "id")
							w.Header().Set("Content-Type", "application/json")
							_, _ = w.Write([]byte(`{"id":"` + id + `"}`))
						},
					},
					RouteConfigBase{ // Add explicit type
						Path:    "/posts/:postId/comments/:commentId",
						Methods: []string{"GET"},
						Handler: func(w http.ResponseWriter, r *http.Request) {
							postId := GetParam(r, "postId")
							commentId := GetParam(r, "commentId")
							w.Header().Set("Content-Type", "application/json")
							_, _ = w.Write([]byte(`{"postId":"` + postId + `","commentId":"` + commentId + `"}`))
						},
					},
				},
			},
		},
	},
		// Mock auth function that always returns invalid
		func(ctx context.Context, token string) (string, bool) {
			return "", false
		},
		// Mock user ID function that returns the string itself
		func(user string) string {
			return user
		})

	// Test the first route
	req, _ := http.NewRequest("GET", "/api/users/123", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	// Check status code
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rr.Code)
	}

	// Check response body
	expected := `{"id":"123"}`
	if rr.Body.String() != expected {
		t.Errorf("Expected response body %q, got %q", expected, rr.Body.String())
	}

	// Test the second route
	req, _ = http.NewRequest("GET", "/api/posts/456/comments/789", nil)
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	// Check status code
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rr.Code)
	}

	// Check response body
	expected = `{"postId":"456","commentId":"789"}`
	if rr.Body.String() != expected {
		t.Errorf("Expected response body %q, got %q", expected, rr.Body.String())
	}
}

// TestTimeoutOverrides tests that timeout overrides work correctly
func TestTimeoutOverrides(t *testing.T) {
	// Create a logger
	logger, _ := zap.NewDevelopment()

	// Create a router with timeout overrides and string as both the user ID and user type
	r := NewRouter(RouterConfig{
		Logger:        logger,
		GlobalTimeout: 100 * time.Millisecond,
		SubRouters: []SubRouterConfig{
			{
				PathPrefix:      "/api/v1",
				TimeoutOverride: 50 * time.Millisecond,
				Routes: []any{ // Changed to []any
					RouteConfigBase{
						Path:    "/fast",
						Methods: []string{"GET"},
						Handler: func(w http.ResponseWriter, r *http.Request) {
							// This handler returns immediately
							w.WriteHeader(http.StatusOK)
						},
					},
					RouteConfigBase{ // Add explicit type
						Path:    "/slow",
						Methods: []string{"GET"},
						Handler: func(w http.ResponseWriter, r *http.Request) {
							// This handler sleeps for 200ms, which is longer than the timeout
							time.Sleep(200 * time.Millisecond)
							w.WriteHeader(http.StatusOK)
						},
					},
					RouteConfigBase{ // Add explicit type
						Path:    "/custom-timeout",
						Methods: []string{"GET"},
						Timeout: 300 * time.Millisecond,
						Handler: func(w http.ResponseWriter, r *http.Request) {
							// This handler sleeps for 200ms, which is shorter than the custom timeout
							time.Sleep(200 * time.Millisecond)
							w.WriteHeader(http.StatusOK)
						},
					},
				},
			},
		},
	},
		// Mock auth function that always returns invalid
		func(ctx context.Context, token string) (string, bool) {
			return "", false
		},
		// Mock user ID function that returns the string itself
		func(user string) string {
			return user
		})

	// Test the fast route
	req, _ := http.NewRequest("GET", "/api/v1/fast", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	// Check status code
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rr.Code)
	}

	// Test the slow route
	req, _ = http.NewRequest("GET", "/api/v1/slow", nil)
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	// Check status code (should be timeout)
	if rr.Code != http.StatusRequestTimeout {
		t.Errorf("Expected status code %d, got %d", http.StatusRequestTimeout, rr.Code)
	}

	// Test the custom timeout route
	req, _ = http.NewRequest("GET", "/api/v1/custom-timeout", nil)
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	// Check status code (should be OK because the custom timeout is longer than the sleep)
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rr.Code)
	}
}

// TestMaxBodySizeOverrides tests that max body size overrides work correctly
func TestMaxBodySizeOverrides(t *testing.T) {
	// Create a logger
	logger, _ := zap.NewDevelopment()

	// Create a router with max body size overrides and string as both the user ID and user type
	r := NewRouter(RouterConfig{
		Logger:            logger,
		GlobalMaxBodySize: 10, // 10 bytes
		SubRouters: []SubRouterConfig{
			{
				PathPrefix:          "/api/v1",
				MaxBodySizeOverride: 20, // 20 bytes
				Routes: []any{ // Changed to []any
					RouteConfigBase{
						Path:    "/small",
						Methods: []string{"POST"},
						Handler: func(w http.ResponseWriter, r *http.Request) {
							// Read the body
							_, err := io.ReadAll(r.Body)
							if err != nil {
								http.Error(w, err.Error(), http.StatusInternalServerError)
								return
							}
							w.WriteHeader(http.StatusOK)
						},
					},
					RouteConfigBase{ // Add explicit type
						Path:        "/large",
						Methods:     []string{"POST"},
						MaxBodySize: 100, // 100 bytes
						Handler: func(w http.ResponseWriter, r *http.Request) {
							// Read the body
							_, err := io.ReadAll(r.Body)
							if err != nil {
								http.Error(w, err.Error(), http.StatusInternalServerError)
								return
							}
							w.WriteHeader(http.StatusOK)
						},
					},
				},
			},
		},
	},
		// Mock auth function that always returns invalid
		func(ctx context.Context, token string) (string, bool) {
			return "", false
		},
		// Mock user ID function that returns the string itself
		func(user string) string {
			return user
		})

	// Test the small route with a small body
	req, _ := http.NewRequest("POST", "/api/v1/small", strings.NewReader("small"))
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	// Check status code
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rr.Code)
	}

	// Test the small route with a large body
	req, _ = http.NewRequest("POST", "/api/v1/small", strings.NewReader("this is a large body that exceeds the limit"))
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	// Check status code (should be error)
	if rr.Code == http.StatusOK {
		t.Errorf("Expected error status code, got %d", rr.Code)
	}

	// Test the large route with a large body
	req, _ = http.NewRequest("POST", "/api/v1/large", strings.NewReader("this is a large body but within the custom limit"))
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	// Check status code
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rr.Code)
	}
}

// TestGenericRouteIntegration tests that generic routes work correctly
type TestRequest struct {
	Name string `json:"name"`
	Age  int    `json:"age"`
}

type TestResponse struct {
	Greeting string `json:"greeting"`
	Age      int    `json:"age"`
}

func TestGenericRouteIntegration(t *testing.T) {
	// Create a logger
	logger, _ := zap.NewDevelopment()

	// Create a router with string as both the user ID and user type
	r := NewRouter(RouterConfig{
		Logger: logger,
	},
		// Mock auth function that always returns invalid
		func(ctx context.Context, token string) (string, bool) {
			return "", false
		},
		// Mock user ID function that returns the string itself
		func(user string) string {
			return user
		})

	// Register a generic route
	RegisterGenericRoute(r, RouteConfig[TestRequest, TestResponse]{
		Path:    "/greet",
		Methods: []string{"POST"},
		Codec:   codec.NewJSONCodec[TestRequest, TestResponse](),
		Handler: func(req *http.Request, data TestRequest) (TestResponse, error) {
			return TestResponse{
				Greeting: "Hello, " + data.Name,
				Age:      data.Age,
			}, nil
		},
	}, time.Duration(0), int64(0), nil) // Added effective settings

	// Create a request
	reqBody := `{"name":"John","age":30}`
	req, _ := http.NewRequest("POST", "/greet", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()

	// Serve the request
	r.ServeHTTP(rr, req)

	// Check status code
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rr.Code)
	}

	// Check response body
	var resp TestResponse
	err := json.Unmarshal(rr.Body.Bytes(), &resp)
	if err != nil {
		t.Errorf("Failed to unmarshal response: %v", err)
	}

	if resp.Greeting != "Hello, John" {
		t.Errorf("Expected greeting %q, got %q", "Hello, John", resp.Greeting)
	}

	if resp.Age != 30 {
		t.Errorf("Expected age %d, got %d", 30, resp.Age)
	}
}

// TestMiddlewareIntegration tests that middleware integration works correctly
func TestMiddlewareIntegration(t *testing.T) {
	// Create a logger
	logger, _ := zap.NewDevelopment()

	// Create a router with middleware and string as both the user ID and user type
	r := NewRouter(RouterConfig{
		Logger: logger,
		Middlewares: []Middleware{
			middleware.CORS([]string{"*"}, []string{"GET", "POST"}, []string{"Content-Type"}),
		},
		SubRouters: []SubRouterConfig{
			{
				PathPrefix: "/api",
				Middlewares: []Middleware{
					func(next http.Handler) http.Handler {
						return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							// Add a custom header
							w.Header().Set("X-API-Version", "1.0")
							next.ServeHTTP(w, r)
						})
					},
				},
				Routes: []any{ // Changed to []any
					RouteConfigBase{
						Path:    "/test",
						Methods: []string{"GET"},
						Middlewares: []Middleware{
							func(next http.Handler) http.Handler {
								return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
									// Add another custom header
									w.Header().Set("X-Route", "test")
									next.ServeHTTP(w, r)
								})
							},
						},
						Handler: func(w http.ResponseWriter, r *http.Request) {
							w.WriteHeader(http.StatusOK)
							_, _ = w.Write([]byte("OK"))
						},
					},
				},
			},
		},
	},
		// Mock auth function that always returns invalid
		func(ctx context.Context, token string) (string, bool) {
			return "", false
		},
		// Mock user ID function that returns the string itself
		func(user string) string {
			return user
		})

	// Create a request
	req, _ := http.NewRequest("GET", "/api/test", nil)
	rr := httptest.NewRecorder()

	// Serve the request
	r.ServeHTTP(rr, req)

	// Check status code
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rr.Code)
	}

	// Check response body
	if rr.Body.String() != "OK" {
		t.Errorf("Expected response body %q, got %q", "OK", rr.Body.String())
	}

	// Check headers
	if rr.Header().Get("Access-Control-Allow-Origin") != "*" {
		t.Errorf("Expected Access-Control-Allow-Origin header %q, got %q", "*", rr.Header().Get("Access-Control-Allow-Origin"))
	}

	if rr.Header().Get("X-API-Version") != "1.0" {
		t.Errorf("Expected X-API-Version header %q, got %q", "1.0", rr.Header().Get("X-API-Version"))
	}

	if rr.Header().Get("X-Route") != "test" {
		t.Errorf("Expected X-Route header %q, got %q", "test", rr.Header().Get("X-Route"))
	}
}

// TestGracefulShutdown tests that graceful shutdown works correctly
func TestGracefulShutdown(t *testing.T) {
	// Create a logger
	logger, _ := zap.NewDevelopment()

	// Create a router with string as both the user ID and user type
	r := NewRouter(RouterConfig{
		Logger: logger,
	},
		// Mock auth function that always returns invalid
		func(ctx context.Context, token string) (string, bool) {
			return "", false
		},
		// Mock user ID function that returns the string itself
		func(user string) string {
			return user
		})

	// Register a route that sleeps
	r.RegisterRoute(RouteConfigBase{
		Path:    "/sleep",
		Methods: []string{"GET"},
		Handler: func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(100 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("OK"))
		},
	})

	// Start a goroutine to send a request
	done := make(chan struct{})
	go func() {
		// Wait a bit to ensure the server is ready
		time.Sleep(10 * time.Millisecond)

		// Send a request
		req, _ := http.NewRequest("GET", "/sleep", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)

		// Check status code
		if rr.Code != http.StatusOK {
			t.Errorf("Expected status code %d, got %d", http.StatusOK, rr.Code)
		}

		// Check response body
		if rr.Body.String() != "OK" {
			t.Errorf("Expected response body %q, got %q", "OK", rr.Body.String())
		}

		close(done)
	}()

	// Wait a bit to ensure the request is in progress
	time.Sleep(50 * time.Millisecond)

	// Initiate shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 200*time.Millisecond)
	defer cancel()
	err := r.Shutdown(ctx)
	if err != nil {
		t.Errorf("Shutdown returned error: %v", err)
	}

	// Wait for the request to complete
	select {
	case <-done:
		// Request completed successfully
	case <-time.After(300 * time.Millisecond):
		t.Errorf("Request did not complete within timeout")
	}

	// Try to send another request after shutdown
	req, _ := http.NewRequest("GET", "/sleep", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	// Check status code (should be service unavailable)
	if rr.Code != http.StatusServiceUnavailable {
		t.Errorf("Expected status code %d, got %d", http.StatusServiceUnavailable, rr.Code)
	}
}

// TestEdgeCases tests various edge cases
func TestEdgeCases(t *testing.T) {
	// Create a logger
	logger, _ := zap.NewDevelopment()

	// Create a router with string as both the user ID and user type
	r := NewRouter(RouterConfig{
		Logger: logger,
	},
		// Mock auth function that always returns invalid
		func(ctx context.Context, token string) (string, bool) {
			return "", false
		},
		// Mock user ID function that returns the string itself
		func(user string) string {
			return user
		})

	// Register a route with a root path
	r.RegisterRoute(RouteConfigBase{
		Path:    "/",
		Methods: []string{"GET"},
		Handler: func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("Root"))
		},
	})

	// Register a route with a trailing slash
	r.RegisterRoute(RouteConfigBase{
		Path:    "/trailing/",
		Methods: []string{"GET"},
		Handler: func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("Trailing"))
		},
	})

	// Register a route with special characters
	r.RegisterRoute(RouteConfigBase{
		Path:    "/special/:param",
		Methods: []string{"GET"},
		Handler: func(w http.ResponseWriter, r *http.Request) {
			param := GetParam(r, "param")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(param))
		},
	})

	// Register a route with multiple methods
	r.RegisterRoute(RouteConfigBase{
		Path:    "/methods",
		Methods: []string{"GET", "POST", "PUT", "DELETE"},
		Handler: func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(r.Method))
		},
	})

	// Test the root route
	req, _ := http.NewRequest("GET", "/", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	// Check status code
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rr.Code)
	}

	// Check response body
	if rr.Body.String() != "Root" {
		t.Errorf("Expected response body %q, got %q", "Root", rr.Body.String())
	}

	// Test the trailing slash route
	req, _ = http.NewRequest("GET", "/trailing/", nil)
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	// Check status code
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rr.Code)
	}

	// Check response body
	if rr.Body.String() != "Trailing" {
		t.Errorf("Expected response body %q, got %q", "Trailing", rr.Body.String())
	}

	// Test the special characters route
	req, _ = http.NewRequest("GET", "/special/hello%20world", nil)
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	// Check status code
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rr.Code)
	}

	// Check response body
	if rr.Body.String() != "hello world" {
		t.Errorf("Expected response body %q, got %q", "hello world", rr.Body.String())
	}

	// Test the multiple methods route with GET
	req, _ = http.NewRequest("GET", "/methods", nil)
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	// Check status code
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rr.Code)
	}

	// Check response body
	if rr.Body.String() != "GET" {
		t.Errorf("Expected response body %q, got %q", "GET", rr.Body.String())
	}

	// Test the multiple methods route with POST
	req, _ = http.NewRequest("POST", "/methods", nil)
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	// Check status code
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rr.Code)
	}

	// Check response body
	if rr.Body.String() != "POST" {
		t.Errorf("Expected response body %q, got %q", "POST", rr.Body.String())
	}
}
