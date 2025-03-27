package router

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings" // Add missing import
	"sync"    // Needed for TestMutexResponseWriterFlush
	"testing"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/codec"
	"github.com/Suhaibinator/SRouter/pkg/common"
	"github.com/Suhaibinator/SRouter/pkg/middleware"            // Needed for TestEffectiveSettings
	"github.com/Suhaibinator/SRouter/pkg/router/internal/mocks" // Use centralized mocks
	"go.uber.org/zap"
)

// --- Tests from original router_test.go ---

// TestRouteMatching tests that routes are matched correctly
func TestRouteMatching(t *testing.T) {
	logger, _ := zap.NewProduction()
	r := NewRouter(RouterConfig{Logger: logger, SubRouters: []SubRouterConfig{{PathPrefix: "/api", Routes: []RouteConfigBase{{Path: "/users/:id", Methods: []string{"GET"}, Handler: func(w http.ResponseWriter, r *http.Request) {
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
	r := NewRouter(RouterConfig{Logger: logger, GlobalTimeout: 1 * time.Second, SubRouters: []SubRouterConfig{{PathPrefix: "/api", TimeoutOverride: 2 * time.Second, Routes: []RouteConfigBase{
		{Path: "/slow", Methods: []string{"GET"}, Handler: func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(1500 * time.Millisecond)
			_, err := w.Write([]byte("Slow response"))
			if err != nil {
				http.Error(w, fmt.Sprintf("Failed to write response: %v", err), http.StatusInternalServerError)
				return
			}
		}},
		{Path: "/fast", Methods: []string{"GET"}, Timeout: 500 * time.Millisecond, Handler: func(w http.ResponseWriter, r *http.Request) {
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
	r := NewRouter(RouterConfig{Logger: logger, GlobalMaxBodySize: 10, SubRouters: []SubRouterConfig{{PathPrefix: "/api", MaxBodySizeOverride: 20, Routes: []RouteConfigBase{
		{Path: "/small", Methods: []string{"POST"}, MaxBodySize: 5, Handler: func(w http.ResponseWriter, r *http.Request) {
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
		{Path: "/medium", Methods: []string{"POST"}, Handler: func(w http.ResponseWriter, r *http.Request) {
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
	RegisterGenericRoute(r, RouteConfig[RouterTestRequest, RouterTestResponse]{Path: "/greet", Methods: []string{"POST"}, Codec: codec.NewJSONCodec[RouterTestRequest, RouterTestResponse](), Handler: func(r *http.Request, req RouterTestRequest) (RouterTestResponse, error) {
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
		Methods: []string{"GET"},
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
		Routes: []RouteConfigBase{testRoute},
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
	r.RegisterRoute(RouteConfigBase{Path: "/slow", Methods: []string{"GET"}, Handler: func(w http.ResponseWriter, r *http.Request) {
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
	r.RegisterRoute(RouteConfigBase{Path: "/direct", Methods: []string{"GET"}, Handler: func(w http.ResponseWriter, r *http.Request) {
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
	r.RegisterRoute(RouteConfigBase{Path: "/users/:id/posts/:postId", Methods: []string{"GET"}, Handler: func(w http.ResponseWriter, r *http.Request) {
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
	r.RegisterRoute(RouteConfigBase{Path: "/error", Methods: []string{"GET"}, Handler: func(w http.ResponseWriter, req *http.Request) { http.Error(w, "Bad request", http.StatusBadRequest) }})
	r.RegisterRoute(RouteConfigBase{Path: "/regular-error", Methods: []string{"GET"}, Handler: func(w http.ResponseWriter, req *http.Request) {
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
	globalRateLimit := &middleware.RateLimitConfig[any, any]{BucketName: "global", Limit: 10, Window: time.Minute}
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
	subRouterRateLimit := &middleware.RateLimitConfig[any, any]{BucketName: "subrouter", Limit: 20, Window: time.Minute}
	rateLimit = r.getEffectiveRateLimit(nil, subRouterRateLimit)
	if rateLimit == nil || rateLimit.BucketName != "subrouter" {
		t.Errorf("Expected subrouter rate limit, got %v", rateLimit)
	}
	routeRateLimit := &middleware.RateLimitConfig[any, any]{BucketName: "route", Limit: 30, Window: time.Minute}
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
	RegisterGenericRoute(r, RouteConfig[TestRequest, TestResponse]{Path: "/greet", Methods: []string{"POST"}, Codec: codec.NewJSONCodec[TestRequest, TestResponse](), Handler: func(req *http.Request, data TestRequest) (TestResponse, error) {
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

// TestHandleErrorWithHTTPError tests handling an error with an HTTPError
func TestHandleErrorWithHTTPError(t *testing.T) {
	r := NewRouter(RouterConfig{}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)
	httpErr := NewHTTPError(http.StatusNotFound, "Not Found")
	req, _ := http.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()
	r.handleError(rr, req, httpErr, http.StatusInternalServerError, "Internal Server Error")
	if rr.Code != http.StatusNotFound {
		t.Errorf("Expected status code %d, got %d", http.StatusNotFound, rr.Code)
	}
	if rr.Body.String() != "Not Found\n" {
		t.Errorf("Expected response body %q, got %q", "Not Found\n", rr.Body.String())
	}
}

// TestLoggingMiddleware tests the LoggingMiddleware function
func TestLoggingMiddlewareCoverage(t *testing.T) { // Renamed to avoid conflict
	r := NewRouter(RouterConfig{}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)
	wrappedHandler := LoggingMiddleware(r.logger, false)(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte("Hello, World!"))
		if err != nil {
			t.Fatalf("Failed to write response: %v", err)
		}
	}))
	req, _ := http.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rr.Code)
	}
	if rr.Body.String() != "Hello, World!" {
		t.Errorf("Expected response body %q, got %q", "Hello, World!", rr.Body.String())
	}
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
	RegisterGenericRoute(r, RouteConfig[TestRequest, TestResponse]{Path: "/greet-error", Methods: []string{"POST"}, Codec: codec.NewJSONCodec[TestRequest, TestResponse](), Handler: func(req *http.Request, data TestRequest) (TestResponse, error) {
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
