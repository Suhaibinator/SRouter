package router

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"math/big" // Needed for Base62 test helper
	"net/http"
	"net/http/httptest"
	"strings" // Add missing import
	"sync"    // Needed for TestMutexResponseWriterFlush
	"testing"
	"time"

	"encoding/base64"

	"github.com/Suhaibinator/SRouter/pkg/codec"
	"github.com/Suhaibinator/SRouter/pkg/common"
	"github.com/Suhaibinator/SRouter/pkg/router/internal/mocks"
	"github.com/Suhaibinator/SRouter/pkg/scontext" // Ensure scontext is imported
	"github.com/stretchr/testify/assert"           // Using testify for assertions
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"          // Import zapcore
	"go.uber.org/zap/zaptest/observer" // Import observer
)

// --- Test Data Struct ---
type TestData struct {
	Value string `json:"value"`
}

// --- Tests from original router_test.go ---

// TestRouteMatching tests that routes are matched correctly
func TestRouteMatching(t *testing.T) {
	logger, _ := zap.NewProduction()
	r := NewRouter(RouterConfig{Logger: logger, SubRouters: []SubRouterConfig{{PathPrefix: "/api", Routes: []any{RouteConfigBase{Path: "/users/:id", Methods: []HttpMethod{MethodGet}, Handler: func(w http.ResponseWriter, r *http.Request) { // Changed to []any{RouteConfigBase{...}}
		id := GetParam(r, "id")
		_, err := w.Write([]byte("User ID: " + id))
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to write response: %v", err), http.StatusInternalServerError)
			return
		}
	}}}}}}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)
	server := httptest.NewServer(r)
	defer server.Close()
	resp, err := http.Get(server.URL + "/api/users/123")
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}
	if string(body) != "User ID: 123" {
		t.Errorf("Expected response body %q, got %q", "User ID: 123", string(body))
	}
}

// TestSubRouterOverrides tests that sub-router overrides work correctly
func TestSubRouterOverrides(t *testing.T) {
	logger, _ := zap.NewProduction()
	r := NewRouter(RouterConfig{Logger: logger, GlobalTimeout: 1 * time.Second, SubRouters: []SubRouterConfig{{PathPrefix: "/api", TimeoutOverride: 2 * time.Second, Routes: []any{ // Changed to []any{...}
		RouteConfigBase{Path: "/slow", Methods: []HttpMethod{MethodGet}, Handler: func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(1500 * time.Millisecond)
			_, err := w.Write([]byte("Slow response"))
			if err != nil {
				http.Error(w, fmt.Sprintf("Failed to write response: %v", err), http.StatusInternalServerError)
				return
			}
		}},
		RouteConfigBase{Path: "/fast", Methods: []HttpMethod{MethodGet}, Timeout: 500 * time.Millisecond, Handler: func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(750 * time.Millisecond)
			_, err := w.Write([]byte("Fast response"))
			if err != nil {
				http.Error(w, fmt.Sprintf("Failed to write response: %v", err), http.StatusInternalServerError)
				return
			}
		}},
	}}}}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)
	server := httptest.NewServer(r)
	defer server.Close()
	respSlow, errSlow := http.Get(server.URL + "/api/slow")
	if errSlow != nil {
		t.Fatalf("Failed to send request to /api/slow: %v", errSlow)
	}
	defer respSlow.Body.Close()
	if respSlow.StatusCode != http.StatusOK {
		t.Errorf("Expected status code %d for /api/slow, got %d", http.StatusOK, respSlow.StatusCode)
	}
	respFast, errFast := http.Get(server.URL + "/api/fast")
	if errFast != nil {
		t.Fatalf("Failed to send request to /api/fast: %v", errFast)
	}
	defer respFast.Body.Close()
	if respFast.StatusCode != http.StatusRequestTimeout {
		t.Errorf("Expected status code %d for /api/fast, got %d", http.StatusRequestTimeout, respFast.StatusCode)
	}
}

// TestBodySizeLimits tests that body size limits are enforced
func TestBodySizeLimits(t *testing.T) {
	logger := zap.NewNop()
	r := NewRouter(RouterConfig{Logger: logger, GlobalMaxBodySize: 10, SubRouters: []SubRouterConfig{{PathPrefix: "/api", MaxBodySizeOverride: 20, Routes: []any{ // Changed to []any{...}
		RouteConfigBase{Path: "/small", Methods: []HttpMethod{MethodPost}, MaxBodySize: 5, Handler: func(w http.ResponseWriter, r *http.Request) {
			_, err := io.ReadAll(r.Body)
			if err != nil {
				// Check if the error is due to body size limit
				if err.Error() == "http: request body too large" {
					http.Error(w, "Request Entity Too Large", http.StatusRequestEntityTooLarge)
				} else {
					http.Error(w, fmt.Sprintf("Failed to read body: %v", err), http.StatusInternalServerError)
				}
				return
			}
			_, err = w.Write([]byte("OK"))
			if err != nil {
				http.Error(w, fmt.Sprintf("Failed to write response: %v", err), http.StatusInternalServerError)
				return
			}
		}},
		RouteConfigBase{Path: "/medium", Methods: []HttpMethod{MethodPost}, Handler: func(w http.ResponseWriter, r *http.Request) {
			_, err := io.ReadAll(r.Body)
			if err != nil {
				// Check if the error is due to body size limit
				if err.Error() == "http: request body too large" {
					http.Error(w, "Request Entity Too Large", http.StatusRequestEntityTooLarge)
				} else {
					http.Error(w, fmt.Sprintf("Failed to read body: %v", err), http.StatusInternalServerError)
				}
				return
			}
			_, err = w.Write([]byte("OK"))
			if err != nil {
				http.Error(w, fmt.Sprintf("Failed to write response: %v", err), http.StatusInternalServerError)
				return
			}
		}},
	}}}}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)
	server := httptest.NewServer(r)
	defer server.Close()

	// Test /api/small (limit 5)
	smallBody := bytes.NewBufferString(string(make([]byte, 4)))
	respSmallOK, errSmallOK := http.Post(server.URL+"/api/small", "text/plain", smallBody)
	if errSmallOK != nil {
		t.Fatalf("Failed to send small request to /api/small: %v", errSmallOK)
	}
	defer respSmallOK.Body.Close()
	if respSmallOK.StatusCode != http.StatusOK {
		t.Errorf("Expected status code %d for /api/small with small body, got %d", http.StatusOK, respSmallOK.StatusCode)
	}

	largeBodySmall := bytes.NewBufferString(string(make([]byte, 6)))
	respSmallLarge, errSmallLarge := http.Post(server.URL+"/api/small", "text/plain", largeBodySmall)
	if errSmallLarge != nil {
		t.Fatalf("Failed to send large request to /api/small: %v", errSmallLarge)
	}
	defer respSmallLarge.Body.Close()
	if respSmallLarge.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("Expected status code %d for /api/small with large body, got %d", http.StatusRequestEntityTooLarge, respSmallLarge.StatusCode)
	}

	// Test /api/medium (limit 20)
	mediumBody := bytes.NewBufferString(string(make([]byte, 15)))
	respMediumOK, errMediumOK := http.Post(server.URL+"/api/medium", "text/plain", mediumBody)
	if errMediumOK != nil {
		t.Fatalf("Failed to send medium request to /api/medium: %v", errMediumOK)
	}
	defer respMediumOK.Body.Close()
	if respMediumOK.StatusCode != http.StatusOK {
		t.Errorf("Expected status code %d for /api/medium with medium body, got %d", http.StatusOK, respMediumOK.StatusCode)
	}

	largeBodyMedium := bytes.NewBufferString(string(make([]byte, 25)))
	respMediumLarge, errMediumLarge := http.Post(server.URL+"/api/medium", "text/plain", largeBodyMedium)
	if errMediumLarge != nil {
		t.Fatalf("Failed to send large request to /api/medium: %v", errMediumLarge)
	}
	defer respMediumLarge.Body.Close()
	if respMediumLarge.StatusCode != http.StatusRequestEntityTooLarge {
		t.Errorf("Expected status code %d for /api/medium with large body, got %d", http.StatusRequestEntityTooLarge, respMediumLarge.StatusCode)
	}
}

// TestJSONCodec tests that JSON marshaling and unmarshaling works correctly
func TestJSONCodec(t *testing.T) {
	type RouterTestRequest struct {
		Name string `json:"name"`
	}
	type RouterTestResponse struct {
		Greeting string `json:"greeting"`
	}
	logger, _ := zap.NewProduction()
	r := NewRouter(RouterConfig{Logger: logger}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)
	// Pass zero values for effective settings as this test doesn't involve sub-routers
	RegisterGenericRoute(r, RouteConfig[RouterTestRequest, RouterTestResponse]{Path: "/greet", Methods: []HttpMethod{MethodPost}, Codec: codec.NewJSONCodec[RouterTestRequest, RouterTestResponse](), Handler: func(r *http.Request, req RouterTestRequest) (RouterTestResponse, error) {
		return RouterTestResponse{Greeting: "Hello, " + req.Name + "!"}, nil
	}}, time.Duration(0), int64(0), nil) // Added effective settings
	server := httptest.NewServer(r)
	defer server.Close()
	reqBody, _ := json.Marshal(RouterTestRequest{Name: "John"})
	resp, err := http.Post(server.URL+"/greet", "application/json", bytes.NewBuffer(reqBody))
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, resp.StatusCode)
	}
	var respBody RouterTestResponse
	err = json.NewDecoder(resp.Body).Decode(&respBody)
	if err != nil {
		t.Fatalf("Failed to decode response body: %v", err)
	}
	if respBody.Greeting != "Hello, John!" {
		t.Errorf("Expected greeting %q, got %q", "Hello, John!", respBody.Greeting)
	}
}

// TestMiddlewareChaining tests that middleware chaining works correctly
func TestMiddlewareChaining(t *testing.T) {
	logger, _ := zap.NewProduction()

	// Define middleware helper
	addHeaderMiddleware := func(name, value string) common.Middleware {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Add(name, value)
				next.ServeHTTP(w, r)
			})
		}
	}

	// Define route configuration
	testRoute := RouteConfigBase{
		Path:    "/test",
		Methods: []HttpMethod{MethodGet},
		Middlewares: []common.Middleware{
			addHeaderMiddleware("Route", "true"),
		},
		Handler: func(w http.ResponseWriter, r *http.Request) {
			_, err := w.Write([]byte("OK"))
			if err != nil {
				http.Error(w, fmt.Sprintf("Failed to write response: %v", err), http.StatusInternalServerError)
				return
			}
		},
	}

	// Define sub-router configuration
	apiSubRouter := SubRouterConfig{
		PathPrefix: "/api",
		Middlewares: []common.Middleware{
			addHeaderMiddleware("SubRouter", "true"),
		},
		Routes: []any{testRoute}, // Changed to []any{...}
	}

	// Define global router configuration
	routerConfig := RouterConfig{
		Logger: logger,
		Middlewares: []common.Middleware{
			addHeaderMiddleware("Global", "true"),
		},
		SubRouters: []SubRouterConfig{apiSubRouter},
	}

	// Create the router
	r := NewRouter(routerConfig, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	// Create a test server
	server := httptest.NewServer(r)
	defer server.Close()

	// Test middleware chaining
	resp, err := http.Get(server.URL + "/api/test")
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, resp.StatusCode)
	}
	if resp.Header.Get("Global") != "true" {
		t.Errorf("Expected Global header to be %q, got %q", "true", resp.Header.Get("Global"))
	}
	if resp.Header.Get("SubRouter") != "true" {
		t.Errorf("Expected SubRouter header to be %q, got %q", "true", resp.Header.Get("SubRouter"))
	}
	if resp.Header.Get("Route") != "true" {
		t.Errorf("Expected Route header to be %q, got %q", "true", resp.Header.Get("Route"))
	}
}

// TestShutdown tests that the router can be gracefully shut down
func TestShutdown(t *testing.T) {
	logger, _ := zap.NewProduction()
	r := NewRouter(RouterConfig{Logger: logger}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)
	r.RegisterRoute(RouteConfigBase{Path: "/slow", Methods: []HttpMethod{MethodGet}, Handler: func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(500 * time.Millisecond)
		_, err := w.Write([]byte("OK"))
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to write response: %v", err), http.StatusInternalServerError)
			return
		}
	}})
	server := httptest.NewServer(r)
	defer server.Close()
	done := make(chan struct{})
	go func() {
		resp, err := http.Get(server.URL + "/slow")
		if err != nil {
			// This error is expected if the server shuts down before the request completes fully
			if !strings.Contains(err.Error(), "connection refused") && !strings.Contains(err.Error(), "server closed") {
				t.Errorf("Unexpected error sending request: %v", err)
			}
			close(done)
			return
		}
		defer resp.Body.Close()
		// If the request completes, check the status code
		if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusServiceUnavailable {
			t.Errorf("Expected status code %d or %d, got %d", http.StatusOK, http.StatusServiceUnavailable, resp.StatusCode)
		}
		close(done)
	}()
	time.Sleep(100 * time.Millisecond) // Give the request a chance to start
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Second)
	defer cancel()
	err := r.Shutdown(ctx)
	if err != nil {
		t.Fatalf("Failed to shut down router: %v", err)
	}
	select {
	case <-done: // Request goroutine finished
	case <-time.After(2 * time.Second): // Increased timeout
		t.Fatalf("Request goroutine did not complete within timeout")
	}
}

// --- Tests from router_additional_coverage_test.go ---

// TestRegisterRoute tests the RegisterRoute function
func TestRegisterRouteCoverage(t *testing.T) { // Renamed to avoid conflict
	logger := zap.NewNop()
	r := NewRouter(RouterConfig{Logger: logger}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)
	r.RegisterRoute(RouteConfigBase{Path: "/direct", Methods: []HttpMethod{MethodGet}, Handler: func(w http.ResponseWriter, r *http.Request) {
		_, err := w.Write([]byte("Direct route"))
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to write response: %v", err), http.StatusInternalServerError)
			return
		}
	}})
	server := httptest.NewServer(r)
	defer server.Close()
	resp, err := http.Get(server.URL + "/direct")
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}
	if string(body) != "Direct route" {
		t.Errorf("Expected response body %q, got %q", "Direct route", string(body))
	}
}

// TestGetParams tests the GetParams and GetParam functions
func TestGetParamsCoverage(t *testing.T) { // Renamed to avoid conflict
	logger := zap.NewNop()
	r := NewRouter(RouterConfig{Logger: logger}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)
	r.RegisterRoute(RouteConfigBase{Path: "/users/:id/posts/:postId", Methods: []HttpMethod{MethodGet}, Handler: func(w http.ResponseWriter, r *http.Request) {
		params := GetParams(r)
		if len(params) != 2 {
			t.Errorf("Expected 2 params, got %d", len(params))
		}
		userId := GetParam(r, "id")
		postId := GetParam(r, "postId")
		_, err := w.Write([]byte(fmt.Sprintf("User: %s, Post: %s", userId, postId)))
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to write response: %v", err), http.StatusInternalServerError)
			return
		}
	}})
	server := httptest.NewServer(r)
	defer server.Close()
	resp, err := http.Get(server.URL + "/users/123/posts/456")
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}
	if string(body) != "User: 123, Post: 456" {
		t.Errorf("Expected response body %q, got %q", "User: 123, Post: 456", string(body))
	}
}

// TestUserAuth tests authentication functionality (using custom context key)
type testUserIDKey struct{} // Define custom key type locally

func TestUserAuthCoverage(t *testing.T) { // Renamed to avoid conflict
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		userID := r.Context().Value(testUserIDKey{}).(string)
		if userID != "user123" {
			t.Errorf("Expected user ID %q, got %q", "user123", userID)
		}
		_, err := w.Write([]byte(fmt.Sprintf("User ID: %s", userID)))
		if err != nil {
			http.Error(w, fmt.Sprintf("Failed to write response: %v", err), http.StatusInternalServerError)
			return
		}
	})
	authMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), testUserIDKey{}, "user123")
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
	handler := authMiddleware(testHandler)
	server := httptest.NewServer(handler)
	defer server.Close()
	resp, err := http.Get(server.URL)
	if err != nil {
		t.Fatalf("Failed to send request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, resp.StatusCode)
	}
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatalf("Failed to read response body: %v", err)
	}
	if string(body) != "User ID: user123" {
		t.Errorf("Expected response body %q, got %q", "User ID: user123", string(body))
	}
}

// TestSimpleError tests returning errors from handlers
func TestSimpleErrorCoverage(t *testing.T) { // Renamed to avoid conflict
	logger := zap.NewNop()
	r := NewRouter(RouterConfig{Logger: logger}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)
	r.RegisterRoute(RouteConfigBase{Path: "/error", Methods: []HttpMethod{MethodGet}, Handler: func(w http.ResponseWriter, req *http.Request) { http.Error(w, "Bad request", http.StatusBadRequest) }})
	r.RegisterRoute(RouteConfigBase{Path: "/regular-error", Methods: []HttpMethod{MethodGet}, Handler: func(w http.ResponseWriter, req *http.Request) {
		http.Error(w, "Internal error", http.StatusInternalServerError)
	}})
	server := httptest.NewServer(r)
	defer server.Close()
	respErr, errErr := http.Get(server.URL + "/error")
	if errErr != nil {
		t.Fatalf("Failed to send request to /error: %v", errErr)
	}
	defer respErr.Body.Close()
	if respErr.StatusCode != http.StatusBadRequest {
		t.Errorf("Expected status code %d for /error, got %d", http.StatusBadRequest, respErr.StatusCode)
	}
	bodyErr, _ := io.ReadAll(respErr.Body)
	if string(bodyErr) != "Bad request\n" {
		t.Errorf("Expected response body %q for /error, got %q", "Bad request\n", string(bodyErr))
	}
	respReg, errReg := http.Get(server.URL + "/regular-error")
	if errReg != nil {
		t.Fatalf("Failed to send request to /regular-error: %v", errReg)
	}
	defer respReg.Body.Close()
	if respReg.StatusCode != http.StatusInternalServerError {
		t.Errorf("Expected status code %d for /regular-error, got %d", http.StatusInternalServerError, respReg.StatusCode)
	}
	bodyReg, _ := io.ReadAll(respReg.Body)
	if string(bodyReg) != "Internal error\n" {
		t.Errorf("Expected response body %q for /regular-error, got %q", "Internal error\n", string(bodyReg))
	}
}

// TestEffectiveSettings tests the getEffectiveTimeout, getEffectiveMaxBodySize, and getEffectiveRateLimit functions
func TestEffectiveSettingsCoverage(t *testing.T) { // Renamed to avoid conflict
	logger := zap.NewNop()
	globalRateLimit := &common.RateLimitConfig[any, any]{BucketName: "global", Limit: 10, Window: time.Minute} // Use common.RateLimitConfig
	r := NewRouter(RouterConfig{Logger: logger, GlobalTimeout: 5 * time.Second, GlobalMaxBodySize: 1024, GlobalRateLimit: globalRateLimit}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)
	timeout := r.getEffectiveTimeout(0, 0)
	if timeout != 5*time.Second {
		t.Errorf("Expected timeout %v, got %v", 5*time.Second, timeout)
	}
	timeout = r.getEffectiveTimeout(0, 3*time.Second)
	if timeout != 3*time.Second {
		t.Errorf("Expected timeout %v, got %v", 3*time.Second, timeout)
	}
	timeout = r.getEffectiveTimeout(2*time.Second, 3*time.Second)
	if timeout != 2*time.Second {
		t.Errorf("Expected timeout %v, got %v", 2*time.Second, timeout)
	}
	maxBodySize := r.getEffectiveMaxBodySize(0, 0)
	if maxBodySize != 1024 {
		t.Errorf("Expected max body size %d, got %d", 1024, maxBodySize)
	}
	maxBodySize = r.getEffectiveMaxBodySize(0, 2048)
	if maxBodySize != 2048 {
		t.Errorf("Expected max body size %d, got %d", 2048, maxBodySize)
	}
	maxBodySize = r.getEffectiveMaxBodySize(4096, 2048)
	if maxBodySize != 4096 {
		t.Errorf("Expected max body size %d, got %d", 4096, maxBodySize)
	}
	rateLimit := r.getEffectiveRateLimit(nil, nil)
	if rateLimit == nil || rateLimit.BucketName != "global" {
		t.Errorf("Expected global rate limit, got %v", rateLimit)
	}
	subRouterRateLimit := &common.RateLimitConfig[any, any]{BucketName: "subrouter", Limit: 20, Window: time.Minute} // Use common.RateLimitConfig
	rateLimit = r.getEffectiveRateLimit(nil, subRouterRateLimit)
	if rateLimit == nil || rateLimit.BucketName != "subrouter" {
		t.Errorf("Expected subrouter rate limit, got %v", rateLimit)
	}
	routeRateLimit := &common.RateLimitConfig[any, any]{BucketName: "route", Limit: 30, Window: time.Minute} // Use common.RateLimitConfig
	rateLimit = r.getEffectiveRateLimit(routeRateLimit, subRouterRateLimit)
	if rateLimit == nil || rateLimit.BucketName != "route" {
		t.Errorf("Expected route rate limit, got %v", rateLimit)
	}
}

// --- Tests from router_additional_test.go ---

// TestNewRouterWithNilLogger tests creating a router with a nil logger
func TestNewRouterWithNilLogger(t *testing.T) {
	r := NewRouter(RouterConfig{Logger: nil}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)
	if r == nil {
		t.Errorf("Expected router to be created with nil logger")
		return
	}
	if r.logger == nil {
		t.Errorf("Expected logger to be set to a default logger")
	}
}

// TestRegisterGenericRoute tests registering a generic route
func TestRegisterGenericRouteCoverage(t *testing.T) { // Renamed to avoid conflict
	// Corrected struct tag syntax (removed semicolons, ensured proper spacing)
	type TestRequest struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	type TestResponse struct {
		Greeting string `json:"greeting"`
		Age      int    `json:"age"`
	}
	r := NewRouter(RouterConfig{}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)
	// Pass zero values for effective settings
	RegisterGenericRoute(r, RouteConfig[TestRequest, TestResponse]{Path: "/greet", Methods: []HttpMethod{MethodPost}, Codec: codec.NewJSONCodec[TestRequest, TestResponse](), Handler: func(req *http.Request, data TestRequest) (TestResponse, error) {
		return TestResponse{Greeting: "Hello, " + data.Name, Age: data.Age}, nil
	}}, time.Duration(0), int64(0), nil) // Added effective settings
	req, _ := http.NewRequest("POST", "/greet", strings.NewReader(`{"name":"John","age":30}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rr.Code)
	}
}

// TestHandleErrorWithHTTPError tests handling an error with an HTTPError, expecting JSON output
func TestHandleErrorWithHTTPError(t *testing.T) {
	logger := zap.NewNop() // Use Nop logger for testing
	r := NewRouter(RouterConfig{Logger: logger}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)
	httpErr := NewHTTPError(http.StatusNotFound, "Not Found") // Use the actual message, not pre-formatted JSON
	req, _ := http.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()

	// Call handleError which should now write JSON
	r.handleError(rr, req, httpErr, http.StatusInternalServerError, "Internal Server Error") // Default status/message are overridden by httpErr

	// Check status code
	assert.Equal(t, http.StatusNotFound, rr.Code, "Expected status code to be StatusNotFound")

	// Check Content-Type header
	assert.Equal(t, "application/json; charset=utf-8", rr.Header().Get("Content-Type"), "Expected Content-Type header to be application/json")

	// Unmarshal the JSON body
	var respBody map[string]map[string]string
	err := json.Unmarshal(rr.Body.Bytes(), &respBody)
	assert.NoError(t, err, "Failed to unmarshal JSON response body")

	// Check the structure and content of the JSON body
	expectedBody := map[string]map[string]string{
		"error": {
			"message": "Not Found",
		},
	}
	assert.Equal(t, expectedBody, respBody, "Expected JSON body content mismatch")
}

// TestRegisterGenericRouteWithError tests registering a generic route with an error
func TestRegisterGenericRouteWithErrorCoverage(t *testing.T) { // Renamed to avoid conflict
	// Corrected struct tag syntax
	type TestRequest struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	type TestResponse struct {
		Greeting string `json:"greeting"`
		Age      int    `json:"age"`
	}
	r := NewRouter(RouterConfig{}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)
	// Pass zero values for effective settings
	RegisterGenericRoute(r, RouteConfig[TestRequest, TestResponse]{Path: "/greet-error", Methods: []HttpMethod{MethodPost}, Codec: codec.NewJSONCodec[TestRequest, TestResponse](), Handler: func(req *http.Request, data TestRequest) (TestResponse, error) {
		return TestResponse{}, errors.New("handler error")
	}}, time.Duration(0), int64(0), nil) // Added effective settings
	req, _ := http.NewRequest("POST", "/greet-error", strings.NewReader(`{"name":"John","age":30}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusInternalServerError {
		t.Errorf("Expected status code %d, got %d", http.StatusInternalServerError, rr.Code)
	}
}

// TestResponseWriter tests the responseWriter type
func TestResponseWriterCoverage(t *testing.T) { // Renamed to avoid conflict
	rr := httptest.NewRecorder()
	rw := &responseWriter{ResponseWriter: rr, statusCode: http.StatusOK}
	rw.WriteHeader(http.StatusNotFound)
	if rw.statusCode != http.StatusNotFound {
		t.Errorf("Expected statusCode to be %d, got %d", http.StatusNotFound, rw.statusCode)
	}
	_, err := rw.Write([]byte("Hello, World!"))
	if err != nil {
		t.Fatalf("Failed to write response: %v", err)
	}
	if rr.Body.String() != "Hello, World!" {
		t.Errorf("Expected response body %q, got %q", "Hello, World!", rr.Body.String())
	}
	if rr.Code != http.StatusNotFound {
		t.Errorf("Expected response code to be %d, got %d", http.StatusNotFound, rr.Code)
	}
	rw.Flush() // Test Flush method
}

// TestShutdownWithCancel tests the Shutdown method with a canceled context
func TestShutdownWithCancel(t *testing.T) {
	r := NewRouter(RouterConfig{}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	err := r.Shutdown(ctx)
	if err != context.Canceled {
		t.Errorf("Expected error to be %v, got %v", context.Canceled, err)
	}
}

// --- Tests for Refactored Generic SubRouter Registration ---

// TestFindSubRouterConfig tests the helper function for finding sub-router configs
func TestFindSubRouterConfig(t *testing.T) {
	nestedSR := SubRouterConfig{PathPrefix: "/api/v1/users"}
	v1SR := SubRouterConfig{PathPrefix: "/api/v1", SubRouters: []SubRouterConfig{nestedSR}}
	v2SR := SubRouterConfig{PathPrefix: "/api/v2"}
	subRouters := []SubRouterConfig{v1SR, v2SR}

	// Test finding top-level
	foundSR, found := findSubRouterConfig(subRouters, "/api/v1")
	if !found {
		t.Errorf("Expected to find sub-router with prefix /api/v1")
	}
	if foundSR == nil || foundSR.PathPrefix != "/api/v1" {
		t.Errorf("Found incorrect sub-router or nil for /api/v1")
	}

	// Test finding nested
	foundSR, found = findSubRouterConfig(subRouters, "/api/v1/users")
	if !found {
		t.Errorf("Expected to find nested sub-router with prefix /api/v1/users")
	}
	if foundSR == nil || foundSR.PathPrefix != "/api/v1/users" {
		t.Errorf("Found incorrect sub-router or nil for /api/v1/users")
	}

	// Test finding non-existent
	_, found = findSubRouterConfig(subRouters, "/api/v3")
	if found {
		t.Errorf("Did not expect to find sub-router with prefix /api/v3")
	}

	// Test finding non-existent nested
	_, found = findSubRouterConfig(subRouters, "/api/v1/posts")
	if found {
		t.Errorf("Did not expect to find sub-router with prefix /api/v1/posts")
	}
}

// TestRegisterGenericRouteOnSubRouter tests the functional registration method
func TestRegisterGenericRouteOnSubRouter(t *testing.T) {
	logger := zap.NewNop()
	authFunc := func(ctx context.Context, token string) (*string, bool) { user := "user"; return &user, true }
	userIDFunc := func(user *string) string {
		if user == nil {
			return ""
		}
		return *user
	}

	// Define sub-routers first
	usersV1SR := SubRouterConfig{PathPrefix: "/api/v1/users"}
	apiV1SR := SubRouterConfig{PathPrefix: "/api/v1", SubRouters: []SubRouterConfig{usersV1SR}}

	// Create router with sub-router structure
	r := NewRouter(RouterConfig{
		Logger:     logger,
		SubRouters: []SubRouterConfig{apiV1SR},
	}, authFunc, userIDFunc)

	// Define generic route config
	routeCfg := RouteConfig[TestRequest, TestResponse]{
		Path:    "/info", // Relative path
		Methods: []HttpMethod{MethodPost},
		Codec:   codec.NewJSONCodec[TestRequest, TestResponse](),
		Handler: func(req *http.Request, data TestRequest) (TestResponse, error) {
			return TestResponse{Greeting: "Info for " + data.Name, Age: data.Age}, nil
		},
	}

	// Register the route on the nested sub-router
	err := RegisterGenericRouteOnSubRouter(r, "/api/v1/users", routeCfg)
	if err != nil {
		t.Fatalf("RegisterGenericRouteOnSubRouter failed: %v", err)
	}

	// Test the registered route
	reqBody := `{"name":"Test","age":42}`
	req := httptest.NewRequest("POST", "/api/v1/users/info", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status OK (200), got %d", rr.Code)
	}
	var resp TestResponse
	if err := json.Unmarshal(rr.Body.Bytes(), &resp); err != nil {
		t.Fatalf("Failed to unmarshal response: %v", err)
	}
	expectedGreeting := "Info for Test"
	if resp.Greeting != expectedGreeting {
		t.Errorf("Expected greeting %q, got %q", expectedGreeting, resp.Greeting)
	}
	if resp.Age != 42 {
		t.Errorf("Expected age %d, got %d", 42, resp.Age)
	}

	// Test registering on non-existent prefix
	err = RegisterGenericRouteOnSubRouter(r, "/api/v2", routeCfg)
	if err == nil {
		t.Errorf("Expected an error when registering on non-existent prefix, but got nil")
	}
}

// TestRegisterSubRouterWithSubRouter tests the helper for nesting configs
func TestRegisterSubRouterWithSubRouterCoverage(t *testing.T) { // Renamed
	parent := SubRouterConfig{PathPrefix: "/parent"}
	child := SubRouterConfig{PathPrefix: "/child"}

	RegisterSubRouterWithSubRouter(&parent, child)

	if len(parent.SubRouters) != 1 {
		t.Fatalf("Expected 1 sub-router in parent, got %d", len(parent.SubRouters))
	}
	if parent.SubRouters[0].PathPrefix != "/child" {
		t.Errorf("Expected child prefix to be '/child', got %q", parent.SubRouters[0].PathPrefix)
	}
}

// --- Test from deleted auth_test.go ---

// TestMutexResponseWriterFlush tests the Flush method of mutexResponseWriter
func TestMutexResponseWriterFlush(t *testing.T) {
	rr := mocks.NewFlusherRecorder() // Use mock FlusherRecorder
	mu := &sync.Mutex{}
	mrw := &mutexResponseWriter{ResponseWriter: rr, mu: mu}
	mrw.Flush()
	if !rr.Flushed {
		t.Errorf("Expected Flush to be called on the underlying response writer")
	}
}

// Helper function to encode bytes to Base62 string using math/big logic,
// mirroring the DecodeBase62 implementation in pkg/codec/encoding.go
func encodeBase62ForTest(data []byte) string {
	if len(data) == 0 {
		return ""
	}

	var num big.Int
	num.SetBytes(data)

	if num.Sign() == 0 {
		return "0" // Special case for zero
	}

	base := big.NewInt(62)
	zero := big.NewInt(0)
	chars := "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"
	var result []byte

	// Use a temporary variable for calculations to avoid modifying num directly in the loop condition
	tempNum := new(big.Int).Set(&num)
	mod := new(big.Int)

	for tempNum.Cmp(zero) > 0 {
		tempNum.DivMod(tempNum, base, mod) // tempNum = tempNum / base; mod = tempNum % base
		result = append(result, chars[mod.Int64()])
	}

	// Reverse the result
	for i, j := 0, len(result)-1; i < j; i, j = i+1, j-1 {
		result[i], result[j] = result[j], result[i]
	}

	return string(result)
}

// --- New Test for Path Parameter Fallback ---

// TestGenericRoutePathParameterFallback tests the fallback logic for path parameters
// when SourceKey is empty for Base64PathParameter and Base62PathParameter.
func TestGenericRoutePathParameterFallback(t *testing.T) {
	logger := zap.NewNop()
	r := NewRouter(RouterConfig{Logger: logger}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	// Define test data
	testPayload := TestData{Value: "hello world"}
	jsonData, err := json.Marshal(testPayload)
	if err != nil {
		t.Fatalf("Failed to marshal test data: %v", err)
	}

	// Base64 encode
	encodedBase64 := base64.URLEncoding.EncodeToString(jsonData)

	// Base62 encode using the test helper that mirrors the decoder logic
	encodedBase62 := encodeBase62ForTest(jsonData)

	// Handler function to verify decoded data
	verifyHandler := func(expectedValue string) func(req *http.Request, data TestData) (string, error) {
		return func(req *http.Request, data TestData) (string, error) {
			if data.Value != expectedValue {
				return "", fmt.Errorf("expected value %q, got %q", expectedValue, data.Value)
			}
			// Return simple "OK" string, which will be JSON encoded by the codec
			return "OK", nil
		}
	}

	// Register Base64 route with empty SourceKey
	RegisterGenericRoute(r, RouteConfig[TestData, string]{
		Path:       "/base64/:dataParam", // Path parameter name is 'dataParam'
		Methods:    []HttpMethod{MethodGet},
		SourceType: Base64PathParameter,
		SourceKey:  "",                                     // Empty SourceKey, should use 'dataParam'
		Codec:      codec.NewJSONCodec[TestData, string](), // Use JSON codec for request and response
		Handler:    verifyHandler(testPayload.Value),
	}, time.Duration(0), int64(0), nil)

	// Register Base62 route with empty SourceKey
	RegisterGenericRoute(r, RouteConfig[TestData, string]{
		Path:       "/base62/:valueParam", // Path parameter name is 'valueParam'
		Methods:    []HttpMethod{MethodGet},
		SourceType: Base62PathParameter,
		SourceKey:  "",                                     // Empty SourceKey, should use 'valueParam'
		Codec:      codec.NewJSONCodec[TestData, string](), // Use JSON codec for request and response
		Handler:    verifyHandler(testPayload.Value),
	}, time.Duration(0), int64(0), nil)

	// Register routes to test "no path parameters found" error
	RegisterGenericRoute(r, RouteConfig[TestData, string]{
		Path:       "/no-params-base64", // No path parameters
		Methods:    []HttpMethod{MethodGet},
		SourceType: Base64PathParameter,
		SourceKey:  "", // Empty SourceKey
		Codec:      codec.NewJSONCodec[TestData, string](),
		Handler:    verifyHandler(testPayload.Value), // Handler shouldn't be reached
	}, time.Duration(0), int64(0), nil)

	RegisterGenericRoute(r, RouteConfig[TestData, string]{
		Path:       "/no-params-base62", // No path parameters
		Methods:    []HttpMethod{MethodGet},
		SourceType: Base62PathParameter,
		SourceKey:  "", // Empty SourceKey
		Codec:      codec.NewJSONCodec[TestData, string](),
		Handler:    verifyHandler(testPayload.Value), // Handler shouldn't be reached
	}, time.Duration(0), int64(0), nil)

	// Create test server
	server := httptest.NewServer(r)
	defer server.Close()

	// --- Test Cases ---
	tests := []struct {
		name           string
		path           string
		expectedStatus int
		expectedBody   string // Expected body content if status is OK or specific error message part
	}{
		{
			name:           "Base64 Fallback OK",
			path:           fmt.Sprintf("/base64/%s", encodedBase64),
			expectedStatus: http.StatusOK,
			expectedBody:   `"OK"`, // JSON encoded string "OK"
		},
		{
			name:           "Base62 Fallback OK",
			path:           fmt.Sprintf("/base62/%s", encodedBase62),
			expectedStatus: http.StatusOK,
			expectedBody:   `"OK"`, // JSON encoded string "OK"
		},
		{
			name:           "Base64 No Params Error",
			path:           "/no-params-base64",
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "No path parameters found", // Check for specific error message part
		},
		{
			name:           "Base62 No Params Error",
			path:           "/no-params-base62",
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "No path parameters found", // Check for specific error message part
		},
		// Add edge cases if necessary (e.g., multiple path params, invalid encoding)
		{
			name:           "Base64 Invalid Encoding",
			path:           "/base64/invalid-base64-$$",
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Failed to decode base64 path parameter",
		},
		{
			name:           "Base62 Invalid Encoding",
			path:           "/base62/invalid-base62-$$", // Assuming '$$' is invalid for base58/62
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Failed to decode base62 path parameter", // Message from route.go
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			resp, err := http.Get(server.URL + tc.path)
			if err != nil {
				t.Fatalf("Failed to send request: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tc.expectedStatus {
				bodyBytes, _ := io.ReadAll(resp.Body)
				t.Errorf("Expected status code %d, got %d. Body: %s", tc.expectedStatus, resp.StatusCode, string(bodyBytes))
				return // Stop further checks if status is wrong
			}

			if tc.expectedBody != "" {
				bodyBytes, err := io.ReadAll(resp.Body)
				if err != nil {
					t.Fatalf("Failed to read response body: %v", err)
				}
				bodyString := strings.TrimSpace(string(bodyBytes))

				// For error messages, check if the body contains the expected text
				if tc.expectedStatus >= 400 {
					// Extract the "message" field from the JSON error response if possible
					var errResp struct {
						Message string `json:"message"`
					}
					jsonErr := json.Unmarshal(bodyBytes, &errResp)
					if jsonErr == nil && errResp.Message != "" {
						bodyString = errResp.Message // Use the extracted message for comparison
					}
					// Fallback to raw body string if JSON parsing fails or message is empty

					if !strings.Contains(bodyString, tc.expectedBody) {
						t.Errorf("Expected response body to contain %q, got %q", tc.expectedBody, bodyString)
					}
				} else { // For success messages, check for exact match
					if bodyString != tc.expectedBody {
						t.Errorf("Expected response body %q, got %q", tc.expectedBody, bodyString)
					}
				}
			}
		})
	}
}

// TestWriteJSONError_CORSHeaders tests that writeJSONError adds CORS headers from context.
func TestWriteJSONError_CORSHeaders(t *testing.T) {
	assert := assert.New(t)
	logger := zap.NewNop()
	r := NewRouter(RouterConfig{Logger: logger}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	tests := []struct {
		name                 string
		originInContext      string
		credentialsInContext bool
		expectedOriginHeader string
		expectCredentials    bool
		expectVary           bool
	}{
		{
			name:                 "Specific Origin with Credentials",
			originInContext:      "http://allowed.com",
			credentialsInContext: true,
			expectedOriginHeader: "http://allowed.com",
			expectCredentials:    true,
			expectVary:           true,
		},
		{
			name:                 "Specific Origin without Credentials",
			originInContext:      "http://allowed.com",
			credentialsInContext: false,
			expectedOriginHeader: "http://allowed.com",
			expectCredentials:    false,
			expectVary:           true,
		},
		{
			name:                 "Wildcard Origin",
			originInContext:      "*",
			credentialsInContext: false, // Credentials cannot be true with wildcard
			expectedOriginHeader: "*",
			expectCredentials:    false,
			expectVary:           false, // Vary should not be set for wildcard
		},
		{
			name:                 "No CORS Info in Context",
			originInContext:      "", // Simulate no info set
			credentialsInContext: false,
			expectedOriginHeader: "", // Expect no header
			expectCredentials:    false,
			expectVary:           false,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			rr := httptest.NewRecorder()
			req, _ := http.NewRequest("GET", "/error", nil) // Method/path don't matter much here

			// Prepare context
			ctx := req.Context()
			if tc.originInContext != "" || tc.credentialsInContext { // Only add if simulating set context
				ctx = scontext.WithCORSInfo[string, string](ctx, tc.originInContext, tc.credentialsInContext)
			}
			req = req.WithContext(ctx)

			// Call writeJSONError
			r.writeJSONError(rr, req, http.StatusInternalServerError, "Test Error", "trace-123")

			// Assertions
			assert.Equal(tc.expectedOriginHeader, rr.Header().Get("Access-Control-Allow-Origin"), "Origin header mismatch")

			if tc.expectCredentials {
				assert.Equal("true", rr.Header().Get("Access-Control-Allow-Credentials"), "Credentials header expected")
			} else {
				assert.Empty(rr.Header().Get("Access-Control-Allow-Credentials"), "Credentials header not expected")
			}

			if tc.expectVary {
				assert.Contains(rr.Header().Get("Vary"), "Origin", "Vary header should contain Origin")
			} else {
				assert.NotContains(rr.Header().Get("Vary"), "Origin", "Vary header should not contain Origin")
			}

			// Check other standard error response parts
			assert.Equal(http.StatusInternalServerError, rr.Code, "Status code mismatch")
			assert.Equal("application/json; charset=utf-8", rr.Header().Get("Content-Type"), "Content-Type mismatch")
			// Optionally check body content
			var body map[string]map[string]string
			err := json.Unmarshal(rr.Body.Bytes(), &body)
			assert.NoError(err, "Failed to unmarshal error body")
			assert.Equal("Test Error", body["error"]["message"], "Error message mismatch")
			// Trace ID check depends on TraceIDBufferSize > 0 in config, which is 0 here, so skip
			// assert.Equal("trace-123", body["error"]["trace_id"], "Trace ID mismatch")
		})
	}
}

// --- New Test for Generic Route Error Paths ---

// TestRegisterGenericRouteErrorPaths covers various error scenarios in RegisterGenericRoute.
func TestRegisterGenericRouteErrorPaths(t *testing.T) {
	// Use an observer logger to capture logs for specific error messages
	core, observedLogs := observer.New(zapcore.ErrorLevel) // Capture Error level logs
	logger := zap.New(core)
	// logger := zap.NewNop() // Original Nop logger
	r := NewRouter(RouterConfig{Logger: logger}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	// --- Route Definitions for Error Cases ---

	// Unmarshal Path Param Error (Base64)
	invalidJSONBase64 := base64.StdEncoding.EncodeToString([]byte("{invalid json"))
	RegisterGenericRoute(r, RouteConfig[TestData, string]{
		Path:       "/err/unmarshal-path/:data",
		Methods:    []HttpMethod{MethodGet},
		SourceType: Base64PathParameter,
		SourceKey:  "data",
		Codec:      codec.NewJSONCodec[TestData, string](),
		Handler: func(req *http.Request, data TestData) (string, error) {
			t.Error("Handler should not be called on unmarshal error")
			return "", errors.New("handler should not be called")
		},
	}, time.Duration(0), int64(0), nil)

	// Unmarshal Query Param Error (Base64)
	RegisterGenericRoute(r, RouteConfig[TestData, string]{
		Path:       "/err/unmarshal-query",
		Methods:    []HttpMethod{MethodGet},
		SourceType: Base64QueryParameter,
		SourceKey:  "qdata",
		Codec:      codec.NewJSONCodec[TestData, string](),
		Handler: func(req *http.Request, data TestData) (string, error) {
			t.Error("Handler should not be called on unmarshal error")
			return "", errors.New("handler should not be called")
		},
	}, time.Duration(0), int64(0), nil)

	// Missing Query Param Error
	RegisterGenericRoute(r, RouteConfig[TestData, string]{
		Path:       "/err/missing-query",
		Methods:    []HttpMethod{MethodGet},
		SourceType: Base64QueryParameter, // Type doesn't matter much here
		SourceKey:  "required_param",
		Codec:      codec.NewJSONCodec[TestData, string](),
		Handler: func(req *http.Request, data TestData) (string, error) {
			t.Error("Handler should not be called on missing query param error")
			return "", errors.New("handler should not be called")
		},
	}, time.Duration(0), int64(0), nil)

	// Body Decode Error
	RegisterGenericRoute(r, RouteConfig[TestData, string]{
		Path:       "/err/body-decode",
		Methods:    []HttpMethod{MethodPost},
		SourceType: Body,
		Codec:      codec.NewJSONCodec[TestData, string](),
		Handler: func(req *http.Request, data TestData) (string, error) {
			t.Error("Handler should not be called on body decode error")
			return "", errors.New("handler should not be called")
		},
	}, time.Duration(0), int64(0), nil)

	// Unsupported SourceType Error
	RegisterGenericRoute(r, RouteConfig[TestData, string]{
		Path:       "/err/unsupported-source",
		Methods:    []HttpMethod{MethodGet},
		SourceType: 99, // Invalid source type
		Codec:      codec.NewJSONCodec[TestData, string](),
		Handler: func(req *http.Request, data TestData) (string, error) {
			t.Error("Handler should not be called on unsupported source type error")
			return "", errors.New("handler should not be called")
		},
	}, time.Duration(0), int64(0), nil)

	// Handler Error
	RegisterGenericRoute(r, RouteConfig[TestData, string]{
		Path:       "/err/handler",
		Methods:    []HttpMethod{MethodPost},
		SourceType: Body,
		Codec:      codec.NewJSONCodec[TestData, string](),
		Handler: func(req *http.Request, data TestData) (string, error) {
			return "", errors.New("internal handler error") // Explicitly return error
		},
	}, time.Duration(0), int64(0), nil)

	// Response Encode Error
	type UnencodableResponse struct {
		Ch chan int `json:"ch"` // Channels cannot be JSON encoded
	}
	RegisterGenericRoute(r, RouteConfig[TestData, UnencodableResponse]{
		Path:       "/err/resp-encode",
		Methods:    []HttpMethod{MethodPost},
		SourceType: Body,
		Codec:      codec.NewJSONCodec[TestData, UnencodableResponse](),
		Handler: func(req *http.Request, data TestData) (UnencodableResponse, error) {
			return UnencodableResponse{Ch: make(chan int)}, nil // Return unencodable type
		},
	}, time.Duration(0), int64(0), nil)

	// Base64 Query Decode Error
	RegisterGenericRoute(r, RouteConfig[TestData, string]{
		Path:       "/err/base64-query-decode",
		Methods:    []HttpMethod{MethodGet},
		SourceType: Base64QueryParameter,
		SourceKey:  "b64data",
		Codec:      codec.NewJSONCodec[TestData, string](),
		Handler: func(req *http.Request, data TestData) (string, error) {
			t.Error("Handler should not be called on base64 decode error")
			return "", errors.New("handler should not be called")
		},
	}, time.Duration(0), int64(0), nil)

	// Base62 Query Decode Error
	RegisterGenericRoute(r, RouteConfig[TestData, string]{
		Path:       "/err/base62-query-decode",
		Methods:    []HttpMethod{MethodGet},
		SourceType: Base62QueryParameter,
		SourceKey:  "b62data",
		Codec:      codec.NewJSONCodec[TestData, string](),
		Handler: func(req *http.Request, data TestData) (string, error) {
			t.Error("Handler should not be called on base62 decode error")
			return "", errors.New("handler should not be called")
		},
	}, time.Duration(0), int64(0), nil)

	// Base64 Path Decode Error (already covered in TestGenericRoutePathParameterFallback, but good to have here too)
	RegisterGenericRoute(r, RouteConfig[TestData, string]{
		Path:       "/err/base64-path-decode/:data",
		Methods:    []HttpMethod{MethodGet},
		SourceType: Base64PathParameter,
		SourceKey:  "data",
		Codec:      codec.NewJSONCodec[TestData, string](),
		Handler: func(req *http.Request, data TestData) (string, error) {
			t.Error("Handler should not be called on base64 decode error")
			return "", errors.New("handler should not be called")
		},
	}, time.Duration(0), int64(0), nil)

	// Base62 Path Decode Error (already covered in TestGenericRoutePathParameterFallback, but good to have here too)
	RegisterGenericRoute(r, RouteConfig[TestData, string]{
		Path:       "/err/base62-path-decode/:data",
		Methods:    []HttpMethod{MethodGet},
		SourceType: Base62PathParameter,
		SourceKey:  "data",
		Codec:      codec.NewJSONCodec[TestData, string](),
		Handler: func(req *http.Request, data TestData) (string, error) {
			t.Error("Handler should not be called on base62 decode error")
			return "", errors.New("handler should not be called")
		},
	}, time.Duration(0), int64(0), nil)

	// Context Deadline Exceeded Error
	RegisterGenericRoute(r, RouteConfig[TestData, string]{
		Path:       "/err/deadline-exceeded",
		Methods:    []HttpMethod{MethodPost},
		SourceType: Body, // Source type doesn't matter much here
		Codec:      codec.NewJSONCodec[TestData, string](),
		Handler: func(req *http.Request, data TestData) (string, error) {
			return "", context.DeadlineExceeded // Explicitly return this error
		},
	}, time.Duration(0), int64(0), nil)

	// --- Test Server ---
	server := httptest.NewServer(r)
	defer server.Close()

	// --- Test Cases ---
	tests := []struct {
		name           string
		method         string
		path           string
		body           io.Reader // For POST requests
		expectedStatus int
		expectedBody   string // Expected substring in error message
	}{
		{
			name:           "Unmarshal Path Param Error",
			method:         "GET",
			path:           fmt.Sprintf("/err/unmarshal-path/%s", invalidJSONBase64),
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Failed to decode path parameter data",
		},
		{
			name:           "Unmarshal Query Param Error",
			method:         "GET",
			path:           fmt.Sprintf("/err/unmarshal-query?qdata=%s", invalidJSONBase64),
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Failed to decode query parameter data",
		},
		{
			name:           "Missing Query Param Error",
			method:         "GET",
			path:           "/err/missing-query", // Missing required_param
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Missing required query parameter: required_param",
		},
		{
			name:           "Body Decode Error",
			method:         "POST",
			path:           "/err/body-decode",
			body:           strings.NewReader("{invalid json"),
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Failed to decode request body",
		},
		{
			name:           "Unsupported SourceType Error",
			method:         "GET",
			path:           "/err/unsupported-source",
			expectedStatus: http.StatusInternalServerError, // Or maybe Bad Request? Let's check route.go... it uses InternalServerError
			expectedBody:   "Unsupported source type",
		},
		{
			name:           "Handler Error",
			method:         "POST",
			path:           "/err/handler",
			body:           strings.NewReader(`{"value":"test"}`), // Valid body
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   "Handler error", // Generic message from route.go
		},
		{
			name:           "Response Encode Error",
			method:         "POST",
			path:           "/err/resp-encode",
			body:           strings.NewReader(`{"value":"test"}`), // Valid body
			expectedStatus: http.StatusInternalServerError,
			expectedBody:   "Failed to encode response",
		},
		{
			name:           "Base64 Query Decode Error",
			method:         "GET",
			path:           "/err/base64-query-decode?b64data=invalid-$$",
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Failed to decode base64 query parameter",
		},
		{
			name:           "Base62 Query Decode Error",
			method:         "GET",
			path:           "/err/base62-query-decode?b62data=invalid-$$",
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Failed to decode base62 query parameter",
		},
		{
			name:           "Base64 Path Decode Error",
			method:         "GET",
			path:           "/err/base64-path-decode/invalid-$$",
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Failed to decode base64 path parameter",
		},
		{
			name:           "Base62 Path Decode Error",
			method:         "GET",
			path:           "/err/base62-path-decode/invalid-$$",
			expectedStatus: http.StatusBadRequest,
			expectedBody:   "Failed to decode base62 path parameter",
		},
		{
			name:           "Context Deadline Exceeded",
			method:         "POST",
			path:           "/err/deadline-exceeded",
			body:           strings.NewReader(`{"value":"test"}`), // Valid body
			expectedStatus: http.StatusRequestTimeout,
			expectedBody:   "Request Timeout", // Specific message for deadline exceeded
		},
	}

	// --- Run Tests ---
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req, err := http.NewRequest(tc.method, server.URL+tc.path, tc.body)
			if err != nil {
				t.Fatalf("Failed to create request: %v", err)
			}
			if tc.method == "POST" {
				req.Header.Set("Content-Type", "application/json")
			}

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("Failed to send request: %v", err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != tc.expectedStatus {
				bodyBytes, _ := io.ReadAll(resp.Body)
				t.Errorf("Expected status code %d, got %d. Body: %s", tc.expectedStatus, resp.StatusCode, string(bodyBytes))
				return
			}

			if tc.expectedBody != "" {
				bodyBytes, err := io.ReadAll(resp.Body)
				if err != nil {
					t.Fatalf("Failed to read response body: %v", err)
				}
				bodyString := strings.TrimSpace(string(bodyBytes))

				// Extract the "message" field from the JSON error response
				var errResp struct {
					Message string `json:"message"`
				}
				jsonErr := json.Unmarshal(bodyBytes, &errResp)
				if jsonErr == nil && errResp.Message != "" {
					bodyString = errResp.Message // Use the extracted message for comparison
				}
				// Fallback to raw body string if JSON parsing fails or message is empty

				if !strings.Contains(bodyString, tc.expectedBody) {
					t.Errorf("Expected response body to contain %q, got %q", tc.expectedBody, bodyString)
				}
			}
		})
	}

	// --- Verify Specific Log for Deadline Exceeded ---
	deadlineLogs := observedLogs.FilterMessage("Request timed out (detected in handler)").AllUntimed()
	assert.Equal(t, 1, len(deadlineLogs), "Expected exactly one 'Request timed out (detected in handler)' log entry")
	if len(deadlineLogs) > 0 {
		// Optionally check context fields if needed
		foundErrField := false
		for _, field := range deadlineLogs[0].Context {
			if field.Key == "error" {
				if errVal, ok := field.Interface.(error); ok {
					assert.True(t, errors.Is(errVal, context.DeadlineExceeded), "Expected logged error to be context.DeadlineExceeded")
					foundErrField = true
					break
				}
			}
		}
		assert.True(t, foundErrField, "Expected 'error' field in deadline exceeded log context")
	}
}

// TestNewGenericRouteDefinition tests the helper function for creating declarative generic route definitions.
func TestNewGenericRouteDefinition(t *testing.T) {
	logger := zap.NewNop()

	// Define types and handler
	type DefReq struct{ Val string }
	type DefResp struct{ Res string }
	defHandler := func(r *http.Request, data DefReq) (DefResp, error) {
		// Check if middleware was applied
		if r.Header.Get("X-Sub-Mw") != "sub" || r.Header.Get("X-Route-Mw") != "route" {
			return DefResp{}, errors.New("middleware not applied correctly")
		}
		return DefResp{Res: "Processed: " + data.Val}, nil
	}
	defCodec := codec.NewJSONCodec[DefReq, DefResp]()

	// Define route config
	routeCfg := RouteConfig[DefReq, DefResp]{
		Path:    "/data",
		Methods: []HttpMethod{MethodPost},
		Codec:   defCodec,
		Handler: defHandler,
		Middlewares: []Middleware{
			func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					r.Header.Set("X-Route-Mw", "route") // Use Header().Set for request modification
					next.ServeHTTP(w, r)
				})
			},
		},
		Timeout: 500 * time.Millisecond, // Specific timeout
	}

	// Create the registration function using the helper
	regFunc := NewGenericRouteDefinition[DefReq, DefResp, string, string](routeCfg)

	// Define sub-router config with overrides and middleware
	subRouterCfg := SubRouterConfig{
		PathPrefix:      "/sub",
		TimeoutOverride: 1 * time.Second, // Different from route timeout
		Middlewares: []Middleware{
			func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					r.Header.Set("X-Sub-Mw", "sub") // Use Header().Set for request modification
					next.ServeHTTP(w, r)
				})
			},
		},
		Routes: []any{regFunc}, // Add the registration function here
	}

	// Create router with the sub-router
	r := NewRouter(RouterConfig{
		Logger:     logger,
		SubRouters: []SubRouterConfig{subRouterCfg},
	}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	// Test the registered route
	server := httptest.NewServer(r)
	defer server.Close()

	reqBody := `{"Val":"test-value"}`
	req, _ := http.NewRequest("POST", server.URL+"/sub/data", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	client := &http.Client{
		Timeout: 2 * time.Second, // Client timeout > route timeout
	}
	resp, err := client.Do(req)
	if err != nil {
		t.Fatalf("Request failed: %v", err)
	}
	defer resp.Body.Close()

	// Check status
	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		t.Fatalf("Expected status OK (200), got %d. Body: %s", resp.StatusCode, string(bodyBytes))
	}

	// Check response body
	var respBody DefResp
	if err := json.NewDecoder(resp.Body).Decode(&respBody); err != nil {
		t.Fatalf("Failed to decode response body: %v", err)
	}
	expectedResp := "Processed: test-value"
	if respBody.Res != expectedResp {
		t.Errorf("Expected response %q, got %q", expectedResp, respBody.Res)
	}

	// --- Test Timeout Override ---
	// Define a slow handler
	slowHandler := func(r *http.Request, data DefReq) (DefResp, error) {
		time.Sleep(750 * time.Millisecond) // Longer than route timeout (500ms), shorter than sub-router (1s)
		return DefResp{Res: "Slow"}, nil
	}
	slowRouteCfg := RouteConfig[DefReq, DefResp]{
		Path:    "/slow",
		Methods: []HttpMethod{MethodPost},
		Codec:   defCodec,
		Handler: slowHandler,
		Timeout: 500 * time.Millisecond, // Route timeout
	}
	slowRegFunc := NewGenericRouteDefinition[DefReq, DefResp, string, string](slowRouteCfg)

	// Create new router with this route
	slowSubRouterCfg := SubRouterConfig{
		PathPrefix:      "/sub-slow",
		TimeoutOverride: 1 * time.Second, // Sub-router timeout
		Routes:          []any{slowRegFunc},
	}
	slowRouter := NewRouter(RouterConfig{
		Logger:     logger,
		SubRouters: []SubRouterConfig{slowSubRouterCfg},
	}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	slowServer := httptest.NewServer(slowRouter)
	defer slowServer.Close()

	slowReq, _ := http.NewRequest("POST", slowServer.URL+"/sub-slow/slow", strings.NewReader(`{"Val":"slow"}`))
	slowReq.Header.Set("Content-Type", "application/json")

	slowResp, err := client.Do(slowReq) // Use client with timeout
	if err != nil {
		// Check if the error is a timeout error
		if !errors.Is(err, context.DeadlineExceeded) && !strings.Contains(err.Error(), "Timeout exceeded") {
			t.Fatalf("Expected timeout error, but got: %v", err)
		}
		// If it's a timeout error, the test passes for this part.
		// However, the server might still send a 504. Let's check the response if available.
		if slowResp != nil {
			defer slowResp.Body.Close()
			if slowResp.StatusCode != http.StatusRequestTimeout { // Check for 504 Gateway Timeout
				bodyBytes, _ := io.ReadAll(slowResp.Body)
				t.Errorf("Expected status %d after timeout, got %d. Body: %s", http.StatusRequestTimeout, slowResp.StatusCode, string(bodyBytes))
			}
		}
	} else {
		// If no error, check the status code directly
		defer slowResp.Body.Close()
		if slowResp.StatusCode != http.StatusRequestTimeout {
			bodyBytes, _ := io.ReadAll(slowResp.Body)
			t.Errorf("Expected status %d due to route timeout, got %d. Body: %s", http.StatusRequestTimeout, slowResp.StatusCode, string(bodyBytes))
		}
	}
}

// TestRegisterSubRouter_UnsupportedRouteType tests the default case in registerSubRouter's switch statement.
func TestRegisterSubRouter_UnsupportedRouteType(t *testing.T) {
	// Create an observer logger
	observedZapCore, observedLogs := observer.New(zapcore.WarnLevel)
	observedLogger := zap.New(observedZapCore)

	// Define sub-router config with an invalid route type (int)
	subRouterCfg := SubRouterConfig{
		PathPrefix: "/invalid",
		Routes:     []any{123}, // Add an integer, which is an unsupported type
	}

	// Create router config
	routerConfig := RouterConfig{
		Logger:     observedLogger,
		SubRouters: []SubRouterConfig{subRouterCfg},
	}

	// Create the router (this will trigger registerSubRouter)
	_ = NewRouter(routerConfig, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	// Assert that a warning was logged
	assert.Equal(t, 1, observedLogs.Len(), "Expected exactly one log entry")
	if observedLogs.Len() > 0 {
		logEntry := observedLogs.AllUntimed()[0]
		assert.Equal(t, zapcore.WarnLevel, logEntry.Level, "Expected log level to be Warn")
		assert.Equal(t, "Unsupported type found in SubRouterConfig.Routes", logEntry.Message, "Expected specific warning message")

		// Check context fields
		expectedContext := map[string]any{
			"pathPrefix": "/invalid",
			"type":       "int", // The type of the invalid item
		}
		// Convert zapcore.Field to map for easier comparison
		actualContext := make(map[string]any)
		for _, field := range logEntry.Context {
			switch field.Type {
			case zapcore.StringType:
				actualContext[field.Key] = field.String
			case zapcore.Int64Type: // Handle other types if necessary
				actualContext[field.Key] = field.Integer
			default:
				actualContext[field.Key] = field.Interface // Use Interface for Any type
			}
		}

		assert.Equal(t, expectedContext["pathPrefix"], actualContext["pathPrefix"], "Expected pathPrefix field to match")
		assert.Equal(t, expectedContext["type"], actualContext["type"], "Expected type field to match")
	}
}

// TestAuthRequiredMiddleware_OptionsBypass tests that OPTIONS requests bypass authRequiredMiddleware
func TestAuthRequiredMiddleware_OptionsBypass(t *testing.T) {
	logger := zap.NewNop()
	mockAuth := func(ctx context.Context, token string) (*string, bool) {
		if token == "valid-token" {
			user := "user123"
			return &user, true
		}
		return nil, false
	}
	mockUserID := func(user *string) string {
		if user == nil {
			return ""
		}
		return *user
	}

	r := NewRouter(RouterConfig{Logger: logger}, mockAuth, mockUserID)

	// Simple handler that checks for user ID if not OPTIONS
	handler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodOptions {
			userID, ok := scontext.GetUserIDFromRequest[string, string](req) // Use scontext
			if !ok {
				t.Error("Expected user ID in context for non-OPTIONS request")
				return // Return immediately after reporting the test error
			}
			if userID != "user123" {
				t.Errorf("Expected user ID 'user123', got '%s'", userID)
				return // Return immediately after reporting the test error
			}
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	// Apply the middleware directly for testing its behavior
	wrappedHandler := r.authRequiredMiddleware(handler)
	server := httptest.NewServer(wrappedHandler)
	defer server.Close()

	// Test OPTIONS request (should pass regardless of auth)
	reqOptions, _ := http.NewRequest(http.MethodOptions, server.URL+"/test", nil)
	reqOptions.Header.Set("Authorization", "Bearer invalid-token") // Invalid token
	respOptions, err := http.DefaultClient.Do(reqOptions)
	if err != nil {
		t.Fatalf("OPTIONS request failed: %v", err)
	}
	defer respOptions.Body.Close()
	assert.Equal(t, http.StatusOK, respOptions.StatusCode, "OPTIONS request should return OK")

	// Test GET request with invalid auth (should fail)
	reqInvalid, _ := http.NewRequest(http.MethodGet, server.URL+"/test", nil)
	reqInvalid.Header.Set("Authorization", "Bearer invalid-token")
	respInvalid, err := http.DefaultClient.Do(reqInvalid)
	if err != nil {
		t.Fatalf("Invalid GET request failed: %v", err)
	}
	defer respInvalid.Body.Close()
	assert.Equal(t, http.StatusUnauthorized, respInvalid.StatusCode, "Invalid GET request should return Unauthorized")

	// Test GET request with valid auth (should pass)
	reqValid, _ := http.NewRequest(http.MethodGet, server.URL+"/test", nil)
	reqValid.Header.Set("Authorization", "Bearer valid-token")
	respValid, err := http.DefaultClient.Do(reqValid)
	if err != nil {
		t.Fatalf("Valid GET request failed: %v", err)
	}
	defer respValid.Body.Close()
	assert.Equal(t, http.StatusOK, respValid.StatusCode, "Valid GET request should return OK")
	bodyBytes, _ := io.ReadAll(respValid.Body)
	assert.Equal(t, "OK", string(bodyBytes), "Valid GET request body mismatch")
}

// TestAuthOptionalMiddleware_OptionsBypass tests that OPTIONS requests bypass authOptionalMiddleware
func TestAuthOptionalMiddleware_OptionsBypass(t *testing.T) {
	logger := zap.NewNop()
	mockAuth := func(ctx context.Context, token string) (*string, bool) {
		if token == "valid-token" {
			user := "user123"
			return &user, true
		}
		return nil, false
	}
	mockUserID := func(user *string) string {
		if user == nil {
			return ""
		}
		return *user
	}

	r := NewRouter(RouterConfig{Logger: logger}, mockAuth, mockUserID)

	// Simple handler that checks for user ID if present (and method is not OPTIONS)
	handler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		if req.Method != http.MethodOptions {
			userID, ok := scontext.GetUserIDFromRequest[string, string](req) // Use scontext
			authHeader := req.Header.Get("Authorization")
			token := strings.TrimPrefix(authHeader, "Bearer ")

			if token == "valid-token" {
				if !ok {
					t.Error("Expected user ID in context for valid non-OPTIONS request")
					http.Error(w, "Missing User ID", http.StatusInternalServerError)
					return
				}
				if userID != "user123" {
					t.Errorf("Expected user ID 'user123', got '%s'", userID)
					http.Error(w, "Incorrect User ID", http.StatusInternalServerError)
					return
				}
			} else {
				// For invalid or missing token, user ID should NOT be present
				if ok {
					t.Errorf("Did not expect user ID in context for invalid/missing token, but found '%s'", userID)
					http.Error(w, "Unexpected User ID", http.StatusInternalServerError)
					return
				}
			}
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	// Apply the middleware directly
	wrappedHandler := r.authOptionalMiddleware(handler)
	server := httptest.NewServer(wrappedHandler)
	defer server.Close()

	// Test OPTIONS request (should pass regardless of auth)
	reqOptions, _ := http.NewRequest(http.MethodOptions, server.URL+"/test", nil)
	reqOptions.Header.Set("Authorization", "Bearer invalid-token") // Invalid token
	respOptions, err := http.DefaultClient.Do(reqOptions)
	if err != nil {
		t.Fatalf("OPTIONS request failed: %v", err)
	}
	defer respOptions.Body.Close()
	assert.Equal(t, http.StatusOK, respOptions.StatusCode, "OPTIONS request should return OK")

	// Test GET request with invalid auth (should pass, no user context)
	reqInvalid, _ := http.NewRequest(http.MethodGet, server.URL+"/test", nil)
	reqInvalid.Header.Set("Authorization", "Bearer invalid-token")
	respInvalid, err := http.DefaultClient.Do(reqInvalid)
	if err != nil {
		t.Fatalf("Invalid GET request failed: %v", err)
	}
	defer respInvalid.Body.Close()
	assert.Equal(t, http.StatusOK, respInvalid.StatusCode, "Invalid GET request should return OK for optional auth")
	bodyBytesInv, _ := io.ReadAll(respInvalid.Body)
	assert.Equal(t, "OK", string(bodyBytesInv), "Invalid GET request body mismatch")

	// Test GET request with no auth (should pass, no user context)
	reqNoAuth, _ := http.NewRequest(http.MethodGet, server.URL+"/test", nil)
	respNoAuth, err := http.DefaultClient.Do(reqNoAuth)
	if err != nil {
		t.Fatalf("No auth GET request failed: %v", err)
	}
	defer respNoAuth.Body.Close()
	assert.Equal(t, http.StatusOK, respNoAuth.StatusCode, "No auth GET request should return OK for optional auth")
	bodyBytesNo, _ := io.ReadAll(respNoAuth.Body)
	assert.Equal(t, "OK", string(bodyBytesNo), "No auth GET request body mismatch")

	// Test GET request with valid auth (should pass, with user context)
	reqValid, _ := http.NewRequest(http.MethodGet, server.URL+"/test", nil)
	reqValid.Header.Set("Authorization", "Bearer valid-token")
	respValid, err := http.DefaultClient.Do(reqValid)
	if err != nil {
		t.Fatalf("Valid GET request failed: %v", err)
	}
	defer respValid.Body.Close()
	assert.Equal(t, http.StatusOK, respValid.StatusCode, "Valid GET request should return OK for optional auth")
	bodyBytesVal, _ := io.ReadAll(respValid.Body)
	assert.Equal(t, "OK", string(bodyBytesVal), "Valid GET request body mismatch")
}

// --- Phase 2: Concurrency Testing ---

// TestConcurrentRequests simulates multiple clients hitting various endpoints concurrently.
func TestConcurrentRequests(t *testing.T) {
	t.Parallel() // Allow this test to run in parallel with others

	logger := zap.NewNop() // Use Nop logger for less noise during concurrent test
	authFunc := func(ctx context.Context, token string) (*string, bool) { user := "user"; return &user, true }
	userIDFunc := func(user *string) string {
		if user == nil {
			return ""
		}
		return *user
	}

	// Middleware for testing
	addHeaderMiddleware := func(name, value string) common.Middleware {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Add(name, value)
				next.ServeHTTP(w, r)
			})
		}
	}

	// Router setup with a mix of routes
	r := NewRouter(RouterConfig{
		Logger: logger,
		Middlewares: []common.Middleware{
			addHeaderMiddleware("X-Global-Test", "true"),
		},
	}, authFunc, userIDFunc)

	// 1. Simple GET route
	r.RegisterRoute(RouteConfigBase{
		Path:    "/simple",
		Methods: []HttpMethod{MethodGet},
		Handler: func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("Simple OK"))
		},
	})

	// 2. GET route with params
	r.RegisterRoute(RouteConfigBase{
		Path:    "/params/:id",
		Methods: []HttpMethod{MethodGet},
		Handler: func(w http.ResponseWriter, r *http.Request) {
			id := GetParam(r, "id")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("Param OK: " + id))
		},
	})

	// 3. Generic POST route
	type ConcurrentReq struct{ Data string }
	type ConcurrentResp struct{ Res string }
	RegisterGenericRoute(r, RouteConfig[ConcurrentReq, ConcurrentResp]{
		Path:    "/generic",
		Methods: []HttpMethod{MethodPost},
		Codec:   codec.NewJSONCodec[ConcurrentReq, ConcurrentResp](),
		Handler: func(req *http.Request, data ConcurrentReq) (ConcurrentResp, error) {
			return ConcurrentResp{Res: "Generic OK: " + data.Data}, nil
		},
	}, time.Duration(0), int64(0), nil)

	// 4. Route with middleware
	r.RegisterRoute(RouteConfigBase{
		Path:    "/middleware",
		Methods: []HttpMethod{http.MethodGet},
		Middlewares: []common.Middleware{
			addHeaderMiddleware("X-Route-Test", "true"),
		},
		Handler: func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("Middleware OK"))
		},
	})

	// Create test server
	server := httptest.NewServer(r)
	defer server.Close()

	// Concurrency parameters
	numGoroutines := 50
	requestsPerGoroutine := 20
	paths := []string{
		"/simple",
		"/params/1", "/params/2", "/params/3", // Mix params
		"/middleware",
		// POST requests need special handling
	}
	postPath := "/generic"
	postBodyTemplate := `{"data":"req-%d-%d"}` // Goroutine index, request index

	var wg sync.WaitGroup
	var mu sync.Mutex    // Mutex to protect errors slice
	errors := []string{} // Slice to collect errors from goroutines

	wg.Add(numGoroutines)

	client := &http.Client{
		Timeout: 5 * time.Second, // Reasonable timeout for local requests
	}

	// Launch goroutines
	for i := 0; i < numGoroutines; i++ {
		go func(goroutineIndex int) {
			defer wg.Done()
			for j := 0; j < requestsPerGoroutine; j++ {
				var req *http.Request
				var err error
				targetURL := ""

				// Alternate between GET and POST requests
				if j%2 == 0 { // GET requests
					pathIndex := (goroutineIndex*requestsPerGoroutine + j) % len(paths)
					targetURL = server.URL + paths[pathIndex]
					req, err = http.NewRequest(http.MethodGet, targetURL, nil)
					if err != nil {
						// Collect error instead of calling t.Fatalf
						mu.Lock()
						errors = append(errors, fmt.Sprintf("Goroutine %d: Failed to create GET request %d: %v", goroutineIndex, j, err))
						mu.Unlock()
						return // Stop this goroutine if request creation fails
					}
				} else { // POST requests
					targetURL = server.URL + postPath
					body := fmt.Sprintf(postBodyTemplate, goroutineIndex, j)
					req, err = http.NewRequest(http.MethodPost, targetURL, strings.NewReader(body))
					if err != nil {
						// Collect error instead of calling t.Fatalf
						mu.Lock()
						errors = append(errors, fmt.Sprintf("Goroutine %d: Failed to create POST request %d: %v", goroutineIndex, j, err))
						mu.Unlock()
						return // Stop this goroutine if request creation fails
					}
					req.Header.Set("Content-Type", "application/json")
				}

				resp, err := client.Do(req)
				if err != nil {
					// Collect error instead of calling t.Logf
					mu.Lock()
					errors = append(errors, fmt.Sprintf("Goroutine %d: Request %d to %s failed: %v", goroutineIndex, j, targetURL, err))
					mu.Unlock()
					continue // Continue to next request
				}

				// Check for unexpected status codes (primarily non-2xx)
				if resp.StatusCode < 200 || resp.StatusCode >= 300 {
					bodyBytes, _ := io.ReadAll(resp.Body)
					// Collect error instead of calling t.Errorf
					mu.Lock()
					errors = append(errors, fmt.Sprintf("Goroutine %d: Request %d to %s got unexpected status %d. Body: %s",
						goroutineIndex, j, targetURL, resp.StatusCode, string(bodyBytes)))
					mu.Unlock()
				}

				// Ensure body is read and closed to reuse connection
				_, _ = io.Copy(io.Discard, resp.Body)
				resp.Body.Close()
			}
		}(i)
	}

	// Wait for all goroutines to complete
	wg.Wait()

	// Report collected errors in the main test goroutine
	mu.Lock() // Lock while reading the errors slice
	if len(errors) > 0 {
		for _, errMsg := range errors {
			t.Error(errMsg)
		}
		t.FailNow() // Fail the test explicitly if errors were collected
	}
	mu.Unlock()
}

// TestServeHTTP_MetricsLoggingWithTraceID verifies that the trace_id field is included
// in the "Request metrics" log when tracing and metrics are enabled.
func TestServeHTTP_MetricsLoggingWithTraceID(t *testing.T) {
	assert := assert.New(t)

	// 1. Create an observer logger
	observedZapCore, observedLogs := observer.New(zapcore.DebugLevel) // Use DebugLevel for "Request metrics"
	observedLogger := zap.New(observedZapCore)

	// 2. Configure router with tracing, metrics, and observer logger
	routerConfig := RouterConfig{
		Logger:             observedLogger,
		TraceIDBufferSize:  10,   // Enable tracing
		EnableMetrics:      true, // Enable metrics logging
		EnableTraceLogging: true, // <<< ADD THIS LINE to enable the "Request metrics" log block
	}
	r := NewRouter(routerConfig, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	// 3. Register a simple route
	r.RegisterRoute(RouteConfigBase{
		Path:    "/ping",
		Methods: []HttpMethod{MethodGet},
		Handler: func(w http.ResponseWriter, req *http.Request) {
			// Simulate middleware adding trace ID to context (for testing the logger)
			// In real execution, the trace middleware does this.
			// We need to ensure the context passed *down* has the ID.
			// However, the defer in ServeHTTP captures the *initial* req.
			// The test should reflect the actual implementation's behavior.
			// The trace middleware *does* set the response header.
			traceIDFromHeader := req.Header.Get("X-Trace-ID") // Get ID potentially set by middleware
			if traceIDFromHeader == "" {
				// If middleware didn't run/set it (like in this direct ServeHTTP call),
				// generate one to simulate it being available *somewhere*.
				// The key is testing if the defer *logs* it if found.
				traceIDFromHeader = "simulated-trace-id-for-test"
			}
			w.Header().Set("X-Trace-ID", traceIDFromHeader) // Ensure header is set for the defer logic

			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("pong"))
		},
	})

	// 4. Make a request
	rr := httptest.NewRecorder()
	req, _ := http.NewRequest(http.MethodGet, "/ping", nil)

	// Simulate the trace middleware adding the trace ID to the request context
	// before ServeHTTP is called internally by the server.
	// This mimics the state *before* the defer captures `req`.
	initialTraceID := "initial-context-trace-id"                                        // This is what the defer will capture
	ctxWithTrace := scontext.WithTraceID[string, string](req.Context(), initialTraceID) // Use scontext
	req = req.WithContext(ctxWithTrace)                                                 // Apply the context with trace ID

	r.ServeHTTP(rr, req) // Call ServeHTTP directly

	// 5. Assert response status
	assert.Equal(http.StatusOK, rr.Code, "Expected status OK")

	// 6. Filter logs for "Request metrics"
	metricsLogs := observedLogs.FilterMessage("Request metrics").AllUntimed()
	assert.GreaterOrEqual(len(metricsLogs), 1, "Expected at least one 'Request metrics' log entry")

	// 7. Check the first matching log entry for trace_id field
	// IMPORTANT: Based on the current implementation of ServeHTTP's defer,
	// it uses `middleware.GetTraceID(req)` which captures the *initial* request context.
	// Therefore, we expect the logged trace_id to be `initialTraceID`.
	if len(metricsLogs) > 0 {
		logEntry := metricsLogs[0]
		foundTraceID := false
		var loggedTraceID string
		for _, field := range logEntry.Context {
			if field.Key == "trace_id" {
				foundTraceID = true
				assert.Equal(zapcore.StringType, field.Type, "Expected trace_id field type to be String")
				loggedTraceID = field.String
				break
			}
		}
		assert.True(foundTraceID, "Expected to find 'trace_id' field in log context")
		// Assert against the trace ID added to the initial request context
		assert.Equal(initialTraceID, loggedTraceID, "Expected logged trace_id to match the one from the initial request context captured by defer")
		assert.NotEmpty(loggedTraceID, "Expected trace_id field value to be non-empty")
	}
}
