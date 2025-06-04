package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"go.uber.org/zap"
)

// --- Test Types and Codec for Generic CORS Test ---

type genericCORSTestRequest struct {
	Value string `json:"value"`
}

type genericCORSTestResponse struct {
	Result string `json:"result"`
}

// genericCORSTestCodec implements the Codec interface for testing generic routes with CORS.
type genericCORSTestCodec struct{}

func (c *genericCORSTestCodec) NewRequest() genericCORSTestRequest {
	return genericCORSTestRequest{}
}

// Decode method signature must match the Codec interface
func (c *genericCORSTestCodec) Decode(r *http.Request) (genericCORSTestRequest, error) {
	// In a real test, you might decode the actual request body
	// For simplicity here, we just return a fixed value
	return genericCORSTestRequest{Value: "decoded"}, nil
}

func (c *genericCORSTestCodec) DecodeBytes(data []byte) (genericCORSTestRequest, error) {
	// Similar simplification for DecodeBytes
	return genericCORSTestRequest{Value: "decoded_bytes"}, nil
}

func (c *genericCORSTestCodec) Encode(w http.ResponseWriter, resp genericCORSTestResponse) error {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	// Ensure genericCORSTestResponse has the Result field
	_, err := w.Write([]byte(`{"result":"` + resp.Result + `"}`))
	return err
}

// TestCORSBasic tests basic CORS functionality with various configurations
func TestCORSBasic(t *testing.T) {
	// Create a logger
	logger, _ := zap.NewDevelopment()

	// Create a simple handler for testing
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	// Test cases
	testCases := []struct {
		name          string
		corsConfig    *CORSConfig
		requestOrigin string
		expectOrigin  string // Expected Access-Control-Allow-Origin header
		expectCreds   bool   // Expect Access-Control-Allow-Credentials header
		expectVary    bool   // Expect Vary: Origin header
		expectExpose  string // Expected Access-Control-Expose-Headers
		requestMethod string // HTTP method to use
		expectStatus  int    // Expected HTTP status code
	}{
		{
			name: "Specific origin allowed",
			corsConfig: &CORSConfig{
				Origins:          []string{"http://example.com"},
				Methods:          []string{"GET", "POST"},
				Headers:          []string{"Content-Type"},
				ExposeHeaders:    []string{"X-Custom-Header"},
				AllowCredentials: true,
				MaxAge:           time.Hour,
			},
			requestOrigin: "http://example.com",
			expectOrigin:  "http://example.com",
			expectCreds:   true,
			expectVary:    true,
			expectExpose:  "X-Custom-Header",
			requestMethod: "GET",
			expectStatus:  http.StatusOK,
		},
		{
			name: "Wildcard origin",
			corsConfig: &CORSConfig{
				Origins:       []string{"*"},
				Methods:       []string{"GET", "POST"},
				Headers:       []string{"Content-Type"},
				ExposeHeaders: []string{"X-Custom-Header"},
				MaxAge:        time.Hour,
			},
			requestOrigin: "http://example.com",
			expectOrigin:  "*",
			expectCreds:   false, // Credentials not allowed with wildcard
			expectVary:    false, // No Vary with wildcard
			expectExpose:  "X-Custom-Header",
			requestMethod: "GET",
			expectStatus:  http.StatusOK,
		},
		{
			name: "Origin not allowed",
			corsConfig: &CORSConfig{
				Origins:       []string{"http://allowed.com"},
				Methods:       []string{"GET", "POST"},
				Headers:       []string{"Content-Type"},
				ExposeHeaders: []string{"X-Custom-Header"},
				MaxAge:        time.Hour,
			},
			requestOrigin: "http://example.com",
			expectOrigin:  "", // No CORS headers for disallowed origin
			expectCreds:   false,
			expectVary:    false,
			expectExpose:  "",
			requestMethod: "GET",
			expectStatus:  http.StatusOK, // Still returns OK, just without CORS headers
		},
		{
			name: "No origin header",
			corsConfig: &CORSConfig{
				Origins:       []string{"http://allowed.com"},
				Methods:       []string{"GET", "POST"},
				Headers:       []string{"Content-Type"},
				ExposeHeaders: []string{"X-Custom-Header"},
				MaxAge:        time.Hour,
			},
			requestOrigin: "", // No origin header
			expectOrigin:  "", // No CORS headers
			expectCreds:   false,
			expectVary:    false,
			expectExpose:  "",
			requestMethod: "GET",
			expectStatus:  http.StatusOK,
		},
		{
			name:          "No CORS config",
			corsConfig:    nil, // No CORS config
			requestOrigin: "http://example.com",
			expectOrigin:  "", // No CORS headers
			expectCreds:   false,
			expectVary:    false,
			expectExpose:  "",
			requestMethod: "GET",
			expectStatus:  http.StatusOK,
		},
		{
			name: "Multiple allowed origins - match first",
			corsConfig: &CORSConfig{
				Origins:          []string{"http://first.com", "http://second.com"},
				Methods:          []string{"GET", "POST"},
				Headers:          []string{"Content-Type"},
				ExposeHeaders:    []string{"X-Custom-Header"},
				AllowCredentials: true,
				MaxAge:           time.Hour,
			},
			requestOrigin: "http://first.com",
			expectOrigin:  "http://first.com",
			expectCreds:   true,
			expectVary:    true,
			expectExpose:  "X-Custom-Header",
			requestMethod: "GET",
			expectStatus:  http.StatusOK,
		},
		{
			name: "Multiple allowed origins - match second",
			corsConfig: &CORSConfig{
				Origins:          []string{"http://first.com", "http://second.com"},
				Methods:          []string{"GET", "POST"},
				Headers:          []string{"Content-Type"},
				ExposeHeaders:    []string{"X-Custom-Header"},
				AllowCredentials: true,
				MaxAge:           time.Hour,
			},
			requestOrigin: "http://second.com",
			expectOrigin:  "http://second.com",
			expectCreds:   true,
			expectVary:    true,
			expectExpose:  "X-Custom-Header",
			requestMethod: "GET",
			expectStatus:  http.StatusOK,
		},
		{
			name: "Wildcard with credentials (should not set credentials)",
			corsConfig: &CORSConfig{
				Origins:          []string{"*"},
				Methods:          []string{"GET", "POST"},
				Headers:          []string{"Content-Type"},
				ExposeHeaders:    []string{"X-Custom-Header"},
				AllowCredentials: true, // This is set but should be ignored for wildcard
				MaxAge:           time.Hour,
			},
			requestOrigin: "http://example.com",
			expectOrigin:  "*",
			expectCreds:   false, // Should not set credentials with wildcard origin
			expectVary:    false,
			expectExpose:  "X-Custom-Header",
			requestMethod: "GET",
			expectStatus:  http.StatusOK,
		},
	}

	// Run test cases
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a router with the test case's CORS config
			r := NewRouter(RouterConfig{
				Logger:     logger,
				CORSConfig: tc.corsConfig,
			},
				// Mock auth function that always returns invalid
				func(ctx context.Context, token string) (*string, bool) {
					return nil, false
				},
				// Mock user ID function
				func(user *string) string {
					if user == nil {
						return ""
					}
					return *user
				})

			// Register a test route
			r.RegisterRoute(RouteConfigBase{
				Path:    "/test",
				Methods: []HttpMethod{MethodGet, MethodPost, MethodOptions},
				Handler: handler,
			})

			// Create a request
			req, _ := http.NewRequest(tc.requestMethod, "/test", nil)
			if tc.requestOrigin != "" {
				req.Header.Set("Origin", tc.requestOrigin)
			}
			rec := httptest.NewRecorder()

			// Serve the request
			r.ServeHTTP(rec, req)

			// Check status code
			if rec.Code != tc.expectStatus {
				t.Errorf("Expected status code %d, got %d", tc.expectStatus, rec.Code)
			}

			// Check CORS headers
			if got := rec.Header().Get("Access-Control-Allow-Origin"); got != tc.expectOrigin {
				t.Errorf("Expected Access-Control-Allow-Origin %q, got %q", tc.expectOrigin, got)
			}

			if tc.expectCreds {
				if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
					t.Errorf("Expected Access-Control-Allow-Credentials to be 'true', got %q", got)
				}
			} else {
				if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "" {
					t.Errorf("Expected no Access-Control-Allow-Credentials, got %q", got)
				}
			}

			if tc.expectVary {
				if got := rec.Header().Get("Vary"); !strings.Contains(got, "Origin") {
					t.Errorf("Expected Vary header to contain 'Origin', got %q", got)
				}
			} else {
				// Only check if we're not expecting Vary and the config exists
				// (because other middleware might set Vary for other reasons)
				if tc.corsConfig != nil && tc.expectOrigin != "" {
					if got := rec.Header().Get("Vary"); strings.Contains(got, "Origin") {
						t.Errorf("Expected Vary header not to contain 'Origin', got %q", got)
					}
				}
			}

			if tc.expectExpose != "" {
				if got := rec.Header().Get("Access-Control-Expose-Headers"); got != tc.expectExpose {
					t.Errorf("Expected Access-Control-Expose-Headers %q, got %q", tc.expectExpose, got)
				}
			} else {
				if got := rec.Header().Get("Access-Control-Expose-Headers"); got != "" {
					t.Errorf("Expected no Access-Control-Expose-Headers, got %q", got)
				}
			}
		})
	}
}

// TestCORSPreflight tests CORS preflight (OPTIONS) requests
func TestCORSPreflight(t *testing.T) {
	// Create a logger
	logger, _ := zap.NewDevelopment()

	// Create a simple handler for testing
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	// Test cases
	testCases := []struct {
		name           string
		corsConfig     *CORSConfig
		requestOrigin  string
		requestMethod  string // Access-Control-Request-Method
		requestHeaders string // Access-Control-Request-Headers
		expectOrigin   string // Expected Access-Control-Allow-Origin header
		expectCreds    bool   // Expect Access-Control-Allow-Credentials header
		expectMethods  string // Expected Access-Control-Allow-Methods header
		expectHeaders  string // Expected Access-Control-Allow-Headers header
		expectMaxAge   string // Expected Access-Control-Max-Age header
		expectStatus   int    // Expected HTTP status code
	}{
		{
			name: "Valid preflight - all fields",
			corsConfig: &CORSConfig{
				Origins:          []string{"http://example.com"},
				Methods:          []string{"GET", "POST", "PUT"},
				Headers:          []string{"Content-Type", "Authorization", "X-Requested-With"},
				AllowCredentials: true,
				MaxAge:           time.Hour,
			},
			requestOrigin:  "http://example.com",
			requestMethod:  "POST",
			requestHeaders: "Content-Type, Authorization",
			expectOrigin:   "http://example.com",
			expectCreds:    true,
			expectMethods:  "GET, POST, PUT",
			expectHeaders:  "Content-Type, Authorization, X-Requested-With",
			expectMaxAge:   "3600",
			expectStatus:   http.StatusNoContent,
		},
		{
			name: "Valid preflight - wildcard origin",
			corsConfig: &CORSConfig{
				Origins: []string{"*"},
				Methods: []string{"GET", "POST", "PUT"},
				Headers: []string{"Content-Type", "Authorization"},
				MaxAge:  time.Hour,
			},
			requestOrigin:  "http://example.com",
			requestMethod:  "POST",
			requestHeaders: "Content-Type",
			expectOrigin:   "*",
			expectCreds:    false,
			expectMethods:  "GET, POST, PUT",
			expectHeaders:  "Content-Type, Authorization",
			expectMaxAge:   "3600",
			expectStatus:   http.StatusNoContent,
		},
		{
			name: "Preflight - method not allowed",
			corsConfig: &CORSConfig{
				Origins: []string{"http://example.com"},
				Methods: []string{"GET", "POST"},
				Headers: []string{"Content-Type"},
				MaxAge:  time.Hour,
			},
			requestOrigin:  "http://example.com",
			requestMethod:  "DELETE", // Not in allowed methods
			requestHeaders: "Content-Type",
			expectOrigin:   "http://example.com", // Origin is still set
			expectCreds:    false,
			expectMethods:  "",                   // No methods header when method not allowed
			expectHeaders:  "",                   // No headers header when method not allowed
			expectMaxAge:   "",                   // No max-age header when method not allowed
			expectStatus:   http.StatusNoContent, // Still returns 204
		},
		{
			name: "Preflight - header not allowed",
			corsConfig: &CORSConfig{
				Origins: []string{"http://example.com"},
				Methods: []string{"GET", "POST"},
				Headers: []string{"Content-Type"},
				MaxAge:  time.Hour,
			},
			requestOrigin:  "http://example.com",
			requestMethod:  "POST",
			requestHeaders: "Content-Type, X-Custom-Header", // X-Custom-Header not allowed
			expectOrigin:   "http://example.com",            // Origin is still set
			expectCreds:    false,
			expectMethods:  "",                   // No methods header when header not allowed
			expectHeaders:  "",                   // No headers header when header not allowed
			expectMaxAge:   "",                   // No max-age header when header not allowed
			expectStatus:   http.StatusNoContent, // Still returns 204
		},
		{
			name: "Preflight - origin not allowed",
			corsConfig: &CORSConfig{
				Origins: []string{"http://allowed.com"},
				Methods: []string{"GET", "POST"},
				Headers: []string{"Content-Type"},
				MaxAge:  time.Hour,
			},
			requestOrigin:  "http://example.com", // Not allowed
			requestMethod:  "POST",
			requestHeaders: "Content-Type",
			expectOrigin:   "", // No CORS headers for disallowed origin
			expectCreds:    false,
			expectMethods:  "",
			expectHeaders:  "",
			expectMaxAge:   "",
			expectStatus:   http.StatusNoContent, // Still returns 204
		},
		{
			name: "Preflight - case insensitive header matching",
			corsConfig: &CORSConfig{
				Origins: []string{"http://example.com"},
				Methods: []string{"GET", "POST"},
				Headers: []string{"Content-Type", "Authorization"},
				MaxAge:  time.Hour,
			},
			requestOrigin:  "http://example.com",
			requestMethod:  "POST",
			requestHeaders: "content-type, authorization", // Lowercase
			expectOrigin:   "http://example.com",
			expectCreds:    false,
			expectMethods:  "GET, POST",
			expectHeaders:  "Content-Type, Authorization", // Original case preserved in response
			expectMaxAge:   "3600",
			expectStatus:   http.StatusNoContent,
		},
		{
			name: "Preflight - no request method",
			corsConfig: &CORSConfig{
				Origins: []string{"http://example.com"},
				Methods: []string{"GET", "POST"},
				Headers: []string{"Content-Type"},
				MaxAge:  time.Hour,
			},
			requestOrigin:  "http://example.com",
			requestMethod:  "", // No request method
			requestHeaders: "Content-Type",
			expectOrigin:   "http://example.com",
			expectCreds:    false,
			expectMethods:  "GET, POST", // Methods still returned
			expectHeaders:  "Content-Type",
			expectMaxAge:   "3600",
			expectStatus:   http.StatusNoContent,
		},
		{
			name: "Preflight - no request headers",
			corsConfig: &CORSConfig{
				Origins: []string{"http://example.com"},
				Methods: []string{"GET", "POST"},
				Headers: []string{"Content-Type"},
				MaxAge:  time.Hour,
			},
			requestOrigin:  "http://example.com",
			requestMethod:  "POST",
			requestHeaders: "", // No request headers
			expectOrigin:   "http://example.com",
			expectCreds:    false,
			expectMethods:  "GET, POST",
			expectHeaders:  "Content-Type", // Headers still returned
			expectMaxAge:   "3600",
			expectStatus:   http.StatusNoContent,
		},
		{
			name: "Preflight - empty headers in request",
			corsConfig: &CORSConfig{
				Origins: []string{"http://example.com"},
				Methods: []string{"GET", "POST"},
				Headers: []string{"Content-Type"},
				MaxAge:  time.Hour,
			},
			requestOrigin:  "http://example.com",
			requestMethod:  "POST",
			requestHeaders: ",,, ,", // Empty headers
			expectOrigin:   "http://example.com",
			expectCreds:    false,
			expectMethods:  "GET, POST",
			expectHeaders:  "Content-Type",
			expectMaxAge:   "3600",
			expectStatus:   http.StatusNoContent,
		},
		{
			name: "Preflight - wildcard headers",
			corsConfig: &CORSConfig{
				Origins: []string{"http://example.com"},
				Methods: []string{"GET", "POST"},
				Headers: []string{"*"}, // Wildcard headers
				MaxAge:  time.Hour,
			},
			requestOrigin:  "http://example.com",
			requestMethod:  "POST",
			requestHeaders: "Content-Type, X-Custom-Header, x-faro-session-id", // Custom headers not explicitly listed
			expectOrigin:   "http://example.com",
			expectCreds:    false,
			expectMethods:  "GET, POST",
			expectHeaders:  "Content-Type, X-Custom-Header, x-faro-session-id", // Echo back the exact requested headers
			expectMaxAge:   "3600",
			expectStatus:   http.StatusNoContent,
		},
	}

	// Run test cases
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a router with the test case's CORS config
			r := NewRouter(RouterConfig{
				Logger:     logger,
				CORSConfig: tc.corsConfig,
			},
				// Mock auth function that always returns invalid
				func(ctx context.Context, token string) (*string, bool) {
					return nil, false
				},
				// Mock user ID function
				func(user *string) string {
					if user == nil {
						return ""
					}
					return *user
				})

			// Register a test route
			r.RegisterRoute(RouteConfigBase{
				Path:    "/test",
				Methods: []HttpMethod{MethodGet, MethodPost, MethodOptions},
				Handler: handler,
			})

			// Create a preflight request
			req, _ := http.NewRequest("OPTIONS", "/test", nil)
			if tc.requestOrigin != "" {
				req.Header.Set("Origin", tc.requestOrigin)
			}
			if tc.requestMethod != "" {
				req.Header.Set("Access-Control-Request-Method", tc.requestMethod)
			}
			if tc.requestHeaders != "" {
				req.Header.Set("Access-Control-Request-Headers", tc.requestHeaders)
			}
			rec := httptest.NewRecorder()

			// Serve the request
			r.ServeHTTP(rec, req)

			// Check status code
			if rec.Code != tc.expectStatus {
				t.Errorf("Expected status code %d, got %d", tc.expectStatus, rec.Code)
			}

			// Check CORS headers
			if got := rec.Header().Get("Access-Control-Allow-Origin"); got != tc.expectOrigin {
				t.Errorf("Expected Access-Control-Allow-Origin %q, got %q", tc.expectOrigin, got)
			}

			if tc.expectCreds {
				if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
					t.Errorf("Expected Access-Control-Allow-Credentials to be 'true', got %q", got)
				}
			} else {
				if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "" {
					t.Errorf("Expected no Access-Control-Allow-Credentials, got %q", got)
				}
			}

			if tc.expectMethods != "" {
				if got := rec.Header().Get("Access-Control-Allow-Methods"); got != tc.expectMethods {
					t.Errorf("Expected Access-Control-Allow-Methods %q, got %q", tc.expectMethods, got)
				}
			} else {
				if got := rec.Header().Get("Access-Control-Allow-Methods"); got != "" {
					t.Errorf("Expected no Access-Control-Allow-Methods, got %q", got)
				}
			}

			if tc.expectHeaders != "" {
				if got := rec.Header().Get("Access-Control-Allow-Headers"); got != tc.expectHeaders {
					t.Errorf("Expected Access-Control-Allow-Headers %q, got %q", tc.expectHeaders, got)
				}
			} else {
				if got := rec.Header().Get("Access-Control-Allow-Headers"); got != "" {
					t.Errorf("Expected no Access-Control-Allow-Headers, got %q", got)
				}
			}

			if tc.expectMaxAge != "" {
				if got := rec.Header().Get("Access-Control-Max-Age"); got != tc.expectMaxAge {
					t.Errorf("Expected Access-Control-Max-Age %q, got %q", tc.expectMaxAge, got)
				}
			} else {
				if got := rec.Header().Get("Access-Control-Max-Age"); got != "" {
					t.Errorf("Expected no Access-Control-Max-Age, got %q", got)
				}
			}
		})
	}
}

// TestCORSErrorResponses tests that CORS headers are correctly applied to error responses
func TestCORSErrorResponses(t *testing.T) {
	// Create a logger
	logger, _ := zap.NewDevelopment()

	// Create a handler that returns an error
	errorHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Error", http.StatusBadRequest)
	})

	// Create a handler that panics
	panicHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		panic("test panic")
	})

	// Test cases
	testCases := []struct {
		name          string
		corsConfig    *CORSConfig
		requestOrigin string
		expectOrigin  string // Expected Access-Control-Allow-Origin header
		expectCreds   bool   // Expect Access-Control-Allow-Credentials header
		handler       http.HandlerFunc
		expectStatus  int // Expected HTTP status code
	}{
		{
			name: "Error response with CORS",
			corsConfig: &CORSConfig{
				Origins:          []string{"http://example.com"},
				Methods:          []string{"GET", "POST"},
				Headers:          []string{"Content-Type"},
				AllowCredentials: true,
				MaxAge:           time.Hour,
			},
			requestOrigin: "http://example.com",
			expectOrigin:  "http://example.com",
			expectCreds:   true,
			handler:       errorHandler,
			expectStatus:  http.StatusBadRequest,
		},
		{
			name: "Error response with wildcard origin",
			corsConfig: &CORSConfig{
				Origins: []string{"*"},
				Methods: []string{"GET", "POST"},
				Headers: []string{"Content-Type"},
				MaxAge:  time.Hour,
			},
			requestOrigin: "http://example.com",
			expectOrigin:  "*",
			expectCreds:   false,
			handler:       errorHandler,
			expectStatus:  http.StatusBadRequest,
		},
		{
			name: "Error response with disallowed origin",
			corsConfig: &CORSConfig{
				Origins: []string{"http://allowed.com"},
				Methods: []string{"GET", "POST"},
				Headers: []string{"Content-Type"},
				MaxAge:  time.Hour,
			},
			requestOrigin: "http://example.com", // Not allowed
			expectOrigin:  "",                   // No CORS headers
			expectCreds:   false,
			handler:       errorHandler,
			expectStatus:  http.StatusBadRequest,
		},
		{
			name: "Panic with CORS",
			corsConfig: &CORSConfig{
				Origins:          []string{"http://example.com"},
				Methods:          []string{"GET", "POST"},
				Headers:          []string{"Content-Type"},
				AllowCredentials: true,
				MaxAge:           time.Hour,
			},
			requestOrigin: "http://example.com",
			expectOrigin:  "http://example.com",
			expectCreds:   true,
			handler:       panicHandler,
			expectStatus:  http.StatusInternalServerError, // Panic results in 500
		},
	}

	// Run test cases
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a router with the test case's CORS config
			r := NewRouter(RouterConfig{
				Logger:     logger,
				CORSConfig: tc.corsConfig,
			},
				// Mock auth function that always returns invalid
				func(ctx context.Context, token string) (*string, bool) {
					return nil, false
				},
				// Mock user ID function
				func(user *string) string {
					if user == nil {
						return ""
					}
					return *user
				})

			// Register a test route
			r.RegisterRoute(RouteConfigBase{
				Path:    "/test",
				Methods: []HttpMethod{MethodGet},
				Handler: tc.handler,
			})

			// Create a request
			req, _ := http.NewRequest("GET", "/test", nil)
			if tc.requestOrigin != "" {
				req.Header.Set("Origin", tc.requestOrigin)
			}
			rec := httptest.NewRecorder()

			// Serve the request
			r.ServeHTTP(rec, req)

			// Check status code
			if rec.Code != tc.expectStatus {
				t.Errorf("Expected status code %d, got %d", tc.expectStatus, rec.Code)
			}

			// Check CORS headers
			if got := rec.Header().Get("Access-Control-Allow-Origin"); got != tc.expectOrigin {
				t.Errorf("Expected Access-Control-Allow-Origin %q, got %q", tc.expectOrigin, got)
			}

			if tc.expectCreds {
				if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
					t.Errorf("Expected Access-Control-Allow-Credentials to be 'true', got %q", got)
				}
			} else {
				if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "" {
					t.Errorf("Expected no Access-Control-Allow-Credentials, got %q", got)
				}
			}
		})
	}
}

// TestCORSWithGenericRoutes tests CORS with generic routes
func TestCORSWithGenericRoutes(t *testing.T) {
	// Create a logger
	logger, _ := zap.NewDevelopment()

	// Simple request/response types (ensure these match the usage in testCodec)
	// MOVED TO PACKAGE LEVEL

	// Test cases
	testCases := []struct {
		name          string
		corsConfig    *CORSConfig
		requestOrigin string
		expectOrigin  string // Expected Access-Control-Allow-Origin header
		expectCreds   bool   // Expect Access-Control-Allow-Credentials header
		expectStatus  int    // Expected HTTP status code
	}{
		{
			name: "Generic route with CORS",
			corsConfig: &CORSConfig{
				Origins:          []string{"http://example.com"},
				Methods:          []string{"GET", "POST"},
				Headers:          []string{"Content-Type"},
				AllowCredentials: true,
				MaxAge:           time.Hour,
			},
			requestOrigin: "http://example.com",
			expectOrigin:  "http://example.com",
			expectCreds:   true,
			expectStatus:  http.StatusOK,
		},
		{
			name: "Generic route with wildcard origin",
			corsConfig: &CORSConfig{
				Origins: []string{"*"},
				Methods: []string{"GET", "POST"},
				Headers: []string{"Content-Type"},
				MaxAge:  time.Hour,
			},
			requestOrigin: "http://example.com",
			expectOrigin:  "*",
			expectCreds:   false,
			expectStatus:  http.StatusOK,
		},
		{
			name: "Generic route with disallowed origin",
			corsConfig: &CORSConfig{
				Origins: []string{"http://allowed.com"},
				Methods: []string{"GET", "POST"},
				Headers: []string{"Content-Type"},
				MaxAge:  time.Hour,
			},
			requestOrigin: "http://example.com", // Not allowed
			expectOrigin:  "",                   // No CORS headers
			expectCreds:   false,
			expectStatus:  http.StatusOK,
		},
	}

	// Run test cases
	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			// Create a router with the test case's CORS config
			r := NewRouter(RouterConfig{
				Logger:     logger,
				CORSConfig: tc.corsConfig,
			},
				// Mock auth function that always returns invalid
				func(ctx context.Context, token string) (*string, bool) {
					return nil, false
				},
				// Mock user ID function
				func(user *string) string {
					if user == nil {
						return ""
					}
					return *user
				})

			// Register a generic route using the package-level types
			RegisterGenericRoute(r, RouteConfig[genericCORSTestRequest, genericCORSTestResponse]{
				Path:    "/test",
				Methods: []HttpMethod{MethodPost},
				Handler: func(req *http.Request, data genericCORSTestRequest) (genericCORSTestResponse, error) {
					return genericCORSTestResponse{Result: "Success: " + data.Value}, nil
				},
				Codec: &genericCORSTestCodec{}, // Use the package-level test codec
			}, 0, 0, nil)

			// Create a request
			reqBody := `{"value":"test"}`
			req, _ := http.NewRequest("POST", "/test", strings.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")
			if tc.requestOrigin != "" {
				req.Header.Set("Origin", tc.requestOrigin)
			}
			rec := httptest.NewRecorder()

			// Serve the request
			r.ServeHTTP(rec, req)

			// Check status code
			if rec.Code != tc.expectStatus {
				t.Errorf("Expected status code %d, got %d", tc.expectStatus, rec.Code)
			}

			// Check CORS headers
			if got := rec.Header().Get("Access-Control-Allow-Origin"); got != tc.expectOrigin {
				t.Errorf("Expected Access-Control-Allow-Origin %q, got %q", tc.expectOrigin, got)
			}

			if tc.expectCreds {
				if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "true" {
					t.Errorf("Expected Access-Control-Allow-Credentials to be 'true', got %q", got)
				}
			} else {
				if got := rec.Header().Get("Access-Control-Allow-Credentials"); got != "" {
					t.Errorf("Expected no Access-Control-Allow-Credentials, got %q", got)
				}
			}
		})
	}
}

// Note: genericCORSTestCodec implementation moved to package level above TestCORSBasic
