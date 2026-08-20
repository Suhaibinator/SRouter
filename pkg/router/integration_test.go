package router

import (
	"context"
	json "encoding/json/v2"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/codec"
	"github.com/Suhaibinator/SRouter/pkg/common"
	"go.uber.org/zap"
)

// TestRouteGroupIntegration tests that route groups work correctly
func TestRouteGroupIntegration(t *testing.T) {
	// Create a logger
	logger, _ := zap.NewDevelopment()

	// Create a router with route groups and string as both the user ID and user type
	r := NewRouter(RouterConfig{
		Logger:            logger,
		GlobalTimeout:     1 * time.Second,
		GlobalMaxBodySize: 1024, // 1 KB
	},
		// Mock auth function that always returns invalid
		func(ctx context.Context, token string) (*string, bool) {
			return nil, false // Return nil pointer for user
		},
		// Mock user ID function that returns the string itself
		func(user *string) string {
			if user == nil {
				return "" // Handle nil pointer case
			}
			return *user // Dereference pointer
		})
	r.Group("/api/v1").Timeout(2 * time.Second).MaxBodySize(2048).Route(RouteConfigBase{
		Path:    "/users",
		Methods: []HttpMethod{MethodGet},
		Handler: func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"users":["user1","user2"]}`))
		},
	})
	r.Group("/api/v2").Route(RouteConfigBase{
		Path:    "/users",
		Methods: []HttpMethod{MethodGet},
		Handler: func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"users":["user3","user4"]}`))
		},
	})

	// Test the first route group
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

	// Test the second route group
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
	},
		// Mock auth function that always returns invalid
		func(ctx context.Context, token string) (*string, bool) {
			return nil, false // Return nil pointer for user
		},
		// Mock user ID function that returns the string itself
		func(user *string) string {
			if user == nil {
				return "" // Handle nil pointer case
			}
			return *user // Dereference pointer
		})
	r.Group("/api").Route(
		RouteConfigBase{
			Path:    "/users/:id",
			Methods: []HttpMethod{MethodGet},
			Handler: func(w http.ResponseWriter, req *http.Request) {
				id := GetParam(req, "id")
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"id":"` + id + `"}`))
			},
		},
		RouteConfigBase{
			Path:    "/posts/:postId/comments/:commentId",
			Methods: []HttpMethod{MethodGet},
			Handler: func(w http.ResponseWriter, req *http.Request) {
				postID := GetParam(req, "postId")
				commentID := GetParam(req, "commentId")
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"postId":"` + postID + `","commentId":"` + commentID + `"}`))
			},
		},
	)

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
	},
		func(ctx context.Context, token string) (*string, bool) { return nil, false },
		func(user *string) string {
			if user == nil {
				return ""
			}
			return *user
		})
	r.Group("/api/v1").Timeout(50*time.Millisecond).Route(
		RouteConfigBase{
			Path:    "/fast",
			Methods: []HttpMethod{MethodGet},
			Handler: func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusOK)
			},
		},
		RouteConfigBase{
			Path:    "/slow",
			Methods: []HttpMethod{MethodGet},
			Handler: func(w http.ResponseWriter, _ *http.Request) {
				time.Sleep(200 * time.Millisecond)
				w.WriteHeader(http.StatusOK)
			},
		},
		RouteConfigBase{
			Path:    "/custom-timeout",
			Methods: []HttpMethod{MethodGet},
			Overrides: common.RouteOverrides{
				Timeout: 300 * time.Millisecond,
			},
			Handler: func(w http.ResponseWriter, _ *http.Request) {
				time.Sleep(200 * time.Millisecond)
				w.WriteHeader(http.StatusOK)
			},
		},
	)

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
	},
		func(ctx context.Context, token string) (*string, bool) { return nil, false },
		func(user *string) string {
			if user == nil {
				return ""
			}
			return *user
		})
	r.Group("/api/v1").MaxBodySize(20).Route(
		RouteConfigBase{
			Path:    "/small",
			Methods: []HttpMethod{MethodPost},
			Handler: func(w http.ResponseWriter, req *http.Request) {
				_, err := io.ReadAll(req.Body)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				w.WriteHeader(http.StatusOK)
			},
		},
		RouteConfigBase{
			Path:    "/large",
			Methods: []HttpMethod{MethodPost},
			Overrides: common.RouteOverrides{
				MaxBodySize: 100,
			},
			Handler: func(w http.ResponseWriter, req *http.Request) {
				_, err := io.ReadAll(req.Body)
				if err != nil {
					http.Error(w, err.Error(), http.StatusInternalServerError)
					return
				}
				w.WriteHeader(http.StatusOK)
			},
		},
	)

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
		func(ctx context.Context, token string) (*string, bool) {
			return nil, false // Return nil pointer for user
		},
		// Mock user ID function that returns the string itself
		func(user *string) string {
			if user == nil {
				return "" // Handle nil pointer case
			}
			return *user // Dereference pointer
		})

	// Register a generic route
	r.Route(RouteConfig[TestRequest, TestResponse]{
		Path:    "/greet",
		Methods: []HttpMethod{MethodPost}, // Use HttpMethod enum
		Codec:   codec.NewJSONCodec[TestRequest, TestResponse](),
		Handler: func(req *http.Request, data TestRequest) (TestResponse, error) {
			return TestResponse{
				Greeting: "Hello, " + data.Name,
				Age:      data.Age,
			}, nil
		},
	})

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
		CORSConfig: &CORSConfig{ // Configure CORS directly in the router config
			Origins: []string{"*"},
			Methods: []string{"GET", "POST"},
			Headers: []string{"Content-Type"},
			MaxAge:  time.Hour,
		},
	},
		func(ctx context.Context, token string) (*string, bool) { return nil, false },
		func(user *string) string {
			if user == nil {
				return ""
			}
			return *user
		})
	r.Group("/api").Use(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			w.Header().Set("X-API-Version", "1.0")
			next.ServeHTTP(w, req)
		})
	}).Route(RouteConfigBase{
		Path:    "/test",
		Methods: []HttpMethod{MethodGet},
		Middlewares: []common.Middleware{
			func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					w.Header().Set("X-Route", "test")
					next.ServeHTTP(w, req)
				})
			},
		},
		Handler: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("OK"))
		},
	})

	// Create a request simulating a cross-origin request
	req, _ := http.NewRequest("GET", "/api/test", nil)
	req.Header.Set("Origin", "http://example.com") // Add Origin header
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
		func(ctx context.Context, token string) (*string, bool) {
			return nil, false // Return nil pointer for user
		},
		// Mock user ID function that returns the string itself
		func(user *string) string {
			if user == nil {
				return "" // Handle nil pointer case
			}
			return *user // Dereference pointer
		})

	// Register a route that sleeps
	r.Route(RouteConfigBase{
		Path:    "/sleep",
		Methods: []HttpMethod{MethodGet}, // Use HttpMethod enum
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
		func(ctx context.Context, token string) (*string, bool) {
			return nil, false // Return nil pointer for user
		},
		// Mock user ID function that returns the string itself
		func(user *string) string {
			if user == nil {
				return "" // Handle nil pointer case
			}
			return *user // Dereference pointer
		})

	// Register a route with a root path
	r.Route(RouteConfigBase{
		Path:    "/",
		Methods: []HttpMethod{MethodGet}, // Use HttpMethod enum
		Handler: func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("Root"))
		},
	})

	// Register a route with a trailing slash
	r.Route(RouteConfigBase{
		Path:    "/trailing/",
		Methods: []HttpMethod{MethodGet}, // Use HttpMethod enum
		Handler: func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("Trailing"))
		},
	})

	// Register a route with special characters
	r.Route(RouteConfigBase{
		Path:    "/special/:param",
		Methods: []HttpMethod{MethodGet}, // Use HttpMethod enum
		Handler: func(w http.ResponseWriter, r *http.Request) {
			param := GetParam(r, "param")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(param))
		},
	})

	// Register a route with multiple methods
	r.Route(RouteConfigBase{
		Path:    "/methods",
		Methods: []HttpMethod{MethodGet, MethodPost, MethodPut, MethodDelete}, // Use HttpMethod enum
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
