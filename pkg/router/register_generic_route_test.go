package router

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time" // Added for zero value

	"github.com/Suhaibinator/SRouter/pkg/codec"
	"github.com/Suhaibinator/SRouter/pkg/middleware"
	"github.com/Suhaibinator/SRouter/pkg/router/internal/mocks" // Import the new mocks package
	"go.uber.org/zap"
)

// --- Test Types ---

type RequestType struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

type ResponseType struct {
	Message string `json:"message"`
	ID      string `json:"id"`
	Name    string `json:"name"`
}

type UnmarshalableResponse struct {
	Channel chan int `json:"channel"` // channels cannot be marshaled to JSON
}

// Test request and response types for subrouter tests
type TestProfileRequest struct {
	// Empty request, we'll get the user from the context
}

type TestProfileResponse struct {
	UserID   string `json:"user_id"`
	IsAdmin  bool   `json:"is_admin"`
	LoggedIn bool   `json:"logged_in"`
}

type TestQueryRequest struct {
	ID   int    `query:"id"`
	Name string `query:"name"`
}

type TestQueryResponse struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

type TestErrorRequest struct {
	ShouldError bool `json:"should_error"`
}

type TestErrorResponse struct {
	Message string `json:"message"`
}

// SourceTestRequest is a simple request type for testing source types
type SourceTestRequest struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// SourceTestResponse is a simple response type for testing source types
type SourceTestResponse struct {
	Message string `json:"message"`
	ID      string `json:"id"`
	Name    string `json:"name"`
}

// --- Helper Functions ---

// testGenericHandler is a helper function for testing generic routes
func testGenericHandler[T any, U any](r *http.Request, data T) (U, error) {
	// Convert data to map
	dataBytes, _ := json.Marshal(data)
	var dataMap map[string]interface{}
	_ = json.Unmarshal(dataBytes, &dataMap)

	// Create response
	var respMap map[string]interface{}
	if name, ok := dataMap["name"].(string); ok {
		respMap = map[string]interface{}{
			"message": "Hello, " + name + "!",
			"id":      dataMap["id"],
			"name":    name,
		}
	} else {
		respMap = map[string]interface{}{
			"message": "Hello!",
			"id":      dataMap["id"],
			"name":    "",
		}
	}

	// Convert response to U
	respBytes, _ := json.Marshal(respMap)
	var resp U
	_ = json.Unmarshal(respBytes, &resp)

	return resp, nil
}

// testGenericHandlerWithError is a helper function for testing generic routes that returns an error
func testGenericHandlerWithError[T any, U any](r *http.Request, data T) (U, error) {
	var resp U
	return resp, errors.New("handler error")
}

// Test handler that accesses user information from the request context (for subrouter tests)
func testProfileHandler(req *http.Request, data TestProfileRequest) (TestProfileResponse, error) {
	// Get the user ID from the request context
	userID, loggedIn := middleware.GetUserIDFromRequest[string, string](req)

	// Create a response with the user information
	response := TestProfileResponse{
		LoggedIn: loggedIn,
	}

	if loggedIn {
		response.UserID = userID
		// Check if the user is an admin
		response.IsAdmin = userID == "admin"
	}

	return response, nil
}

// Test handler for query parameters (for subrouter tests)
func testQueryHandler(req *http.Request, data TestQueryRequest) (TestQueryResponse, error) {
	// Simply echo back the query parameters
	return TestQueryResponse(data), nil
}

// Test handler for error handling (for subrouter tests)
func testErrorHandler(req *http.Request, data TestErrorRequest) (TestErrorResponse, error) {
	if data.ShouldError {
		return TestErrorResponse{}, NewHTTPError(http.StatusBadRequest, "Error requested by client")
	}
	return TestErrorResponse{
		Message: "Success",
	}, nil
}

// SourceTestHandler is a simple handler for testing source types
func SourceTestHandler(r *http.Request, req SourceTestRequest) (SourceTestResponse, error) {
	return SourceTestResponse{
		Message: "Hello, " + req.Name + "!",
		ID:      req.ID,
		Name:    req.Name,
	}, nil
}

// setupTestRouter creates a router for testing source types
func setupTestRouter() *Router[string, string] {
	logger := zap.NewNop() // Use No-op logger for tests unless specific logging is needed

	// Create a router configuration
	routerConfig := RouterConfig{
		Logger: logger,
	}

	// Define the auth function
	authFunction := func(ctx context.Context, token string) (string, bool) {
		return token, true
	}

	// Define the function to get the user ID from a string
	userIdFromUserFunction := func(user string) string {
		return user
	}

	// Create a router
	return NewRouter[string, string](routerConfig, authFunction, userIdFromUserFunction)
}

// --- Tests ---

// TestRegisterGenericRouteWithBody tests RegisterGenericRoute with body source type
// (from register_generic_route_test.go)
func TestRegisterGenericRouteWithBody(t *testing.T) {
	logger := zap.NewNop()
	r := NewRouter[string, string](RouterConfig{Logger: logger}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	RegisterGenericRoute[RequestType, ResponseType, string, string](r, RouteConfig[RequestType, ResponseType]{
		Path:       "/test",
		Methods:    []string{"POST"},
		Codec:      codec.NewJSONCodec[RequestType, ResponseType](),
		Handler:    testGenericHandler[RequestType, ResponseType],
		SourceType: Body,
	}, time.Duration(0), int64(0), nil) // Added effective settings

	reqBody := RequestType{ID: "123", Name: "John"}
	reqBytes, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/test", strings.NewReader(string(reqBytes)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rr.Code)
	}
	var resp ResponseType
	err := json.Unmarshal(rr.Body.Bytes(), &resp)
	if err != nil {
		t.Errorf("Failed to unmarshal response: %v", err)
	}
	if resp.Message != "Hello, John!" {
		t.Errorf("Expected message %q, got %q", "Hello, John!", resp.Message)
	}
}

// TestRegisterGenericRouteWithUnsupportedSourceType tests RegisterGenericRoute with an unsupported source type
// (from register_generic_route_test.go)
func TestRegisterGenericRouteWithUnsupportedSourceType(t *testing.T) {
	logger := zap.NewNop()
	r := NewRouter[string, string](RouterConfig{Logger: logger}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	RegisterGenericRoute[RequestType, ResponseType, string, string](r, RouteConfig[RequestType, ResponseType]{
		Path:       "/test",
		Methods:    []string{"GET"},
		Codec:      codec.NewJSONCodec[RequestType, ResponseType](),
		Handler:    testGenericHandler[RequestType, ResponseType],
		SourceType: SourceType(999), // Unsupported source type
	}, time.Duration(0), int64(0), nil) // Added effective settings

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("Expected status code %d, got %d", http.StatusInternalServerError, rr.Code)
	}
}

// TestRegisterGenericRouteWithAuthRequired tests RegisterGenericRoute with AuthRequired
// (from register_generic_route_auth_test.go)
func TestRegisterGenericRouteWithAuthRequired(t *testing.T) {
	logger := zap.NewNop()
	// Auth function always returns true
	authFunc := func(ctx context.Context, token string) (string, bool) { return "user123", true }
	r := NewRouter[string, string](RouterConfig{Logger: logger}, authFunc, mocks.MockUserIDFromUser)

	RegisterGenericRoute[RequestType, ResponseType, string, string](r, RouteConfig[RequestType, ResponseType]{
		Path:       "/test",
		Methods:    []string{"POST"},
		Codec:      codec.NewJSONCodec[RequestType, ResponseType](),
		Handler:    testGenericHandler[RequestType, ResponseType],
		SourceType: Body,
		AuthLevel:  AuthRequired,
	}, time.Duration(0), int64(0), nil) // Added effective settings

	reqBody := RequestType{ID: "123", Name: "John"}
	reqBytes, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/test", strings.NewReader(string(reqBytes)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer valid-token")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rr.Code)
	}
	var resp ResponseType
	err := json.Unmarshal(rr.Body.Bytes(), &resp)
	if err != nil {
		t.Errorf("Failed to unmarshal response: %v", err)
	}
	if resp.Message != "Hello, John!" {
		t.Errorf("Expected message %q, got %q", "Hello, John!", resp.Message)
	}
}

// TestRegisterGenericRouteWithAuthOptional tests RegisterGenericRoute with AuthOptional
// (from register_generic_route_auth_test.go)
func TestRegisterGenericRouteWithAuthOptional(t *testing.T) {
	logger := zap.NewNop()
	// Auth function always returns true
	authFunc := func(ctx context.Context, token string) (string, bool) { return "user123", true }
	r := NewRouter[string, string](RouterConfig{Logger: logger}, authFunc, mocks.MockUserIDFromUser)

	RegisterGenericRoute[RequestType, ResponseType, string, string](r, RouteConfig[RequestType, ResponseType]{
		Path:       "/test",
		Methods:    []string{"POST"},
		Codec:      codec.NewJSONCodec[RequestType, ResponseType](),
		Handler:    testGenericHandler[RequestType, ResponseType],
		SourceType: Body,
		AuthLevel:  AuthOptional,
	}, time.Duration(0), int64(0), nil) // Added effective settings

	// With valid token
	reqBody := RequestType{ID: "123", Name: "John"}
	reqBytes, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/test", strings.NewReader(string(reqBytes)))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer valid-token")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rr.Code)
	}
	var resp ResponseType
	err := json.Unmarshal(rr.Body.Bytes(), &resp)
	if err != nil {
		t.Errorf("Failed to unmarshal response: %v", err)
	}
	if resp.Message != "Hello, John!" {
		t.Errorf("Expected message %q, got %q", "Hello, John!", resp.Message)
	}

	// Without token
	req = httptest.NewRequest("POST", "/test", strings.NewReader(string(reqBytes)))
	req.Header.Set("Content-Type", "application/json")
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rr.Code)
	}
	err = json.Unmarshal(rr.Body.Bytes(), &resp)
	if err != nil {
		t.Errorf("Failed to unmarshal response: %v", err)
	}
	if resp.Message != "Hello, John!" {
		t.Errorf("Expected message %q, got %q", "Hello, John!", resp.Message)
	}
}

// TestRegisterGenericRouteWithBase62QueryParameter tests RegisterGenericRoute with base62 query parameter source type
// (from register_generic_route_base62_test.go)
func TestRegisterGenericRouteWithBase62QueryParameter(t *testing.T) {
	logger := zap.NewNop()
	r := NewRouter[string, string](RouterConfig{Logger: logger}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	RegisterGenericRoute[RequestType, ResponseType, string, string](r, RouteConfig[RequestType, ResponseType]{
		Path:       "/test",
		Methods:    []string{"GET"},
		Codec:      codec.NewJSONCodec[RequestType, ResponseType](),
		Handler:    testGenericHandler[RequestType, ResponseType],
		SourceType: Base62QueryParameter,
		SourceKey:  "data",
	}, time.Duration(0), int64(0), nil) // Added effective settings

	// Base62 encoded {"id":"123","name":"John"}
	base62Data := "MeHBdAdIGZQif5kLNcARNp0cYy5QevNaNOX"
	req := httptest.NewRequest("GET", "/test?data="+base62Data, nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rr.Code)
	}
	var resp ResponseType
	err := json.Unmarshal(rr.Body.Bytes(), &resp)
	if err != nil {
		t.Errorf("Failed to unmarshal response: %v", err)
	}
	if resp.Message != "Hello, John!" {
		t.Errorf("Expected message %q, got %q", "Hello, John!", resp.Message)
	}
}

// TestRegisterGenericRouteWithBase62PathParameter tests RegisterGenericRoute with base62 path parameter source type
// (from register_generic_route_base62_test.go)
func TestRegisterGenericRouteWithBase62PathParameter(t *testing.T) {
	logger := zap.NewNop()
	r := NewRouter[string, string](RouterConfig{Logger: logger}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	RegisterGenericRoute[RequestType, ResponseType, string, string](r, RouteConfig[RequestType, ResponseType]{
		Path:       "/test/:data",
		Methods:    []string{"GET"},
		Codec:      codec.NewJSONCodec[RequestType, ResponseType](),
		Handler:    testGenericHandler[RequestType, ResponseType],
		SourceType: Base62PathParameter,
		SourceKey:  "data",
	}, time.Duration(0), int64(0), nil) // Added effective settings

	// Base62 encoded {"id":"123","name":"John"}
	base62Data := "MeHBdAdIGZQif5kLNcARNp0cYy5QevNaNOX"
	req := httptest.NewRequest("GET", "/test/"+base62Data, nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rr.Code)
	}
	var resp ResponseType
	err := json.Unmarshal(rr.Body.Bytes(), &resp)
	if err != nil {
		t.Errorf("Failed to unmarshal response: %v", err)
	}
	if resp.Message != "Hello, John!" {
		t.Errorf("Expected message %q, got %q", "Hello, John!", resp.Message)
	}
}

// TestRegisterGenericRouteWithBase62QueryParameterMissing tests RegisterGenericRoute with missing base62 query parameter
// (from register_generic_route_base62_test.go)
func TestRegisterGenericRouteWithBase62QueryParameterMissing(t *testing.T) {
	logger := zap.NewNop()
	r := NewRouter[string, string](RouterConfig{Logger: logger}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	RegisterGenericRoute[RequestType, ResponseType, string, string](r, RouteConfig[RequestType, ResponseType]{
		Path:       "/test",
		Methods:    []string{"GET"},
		Codec:      codec.NewJSONCodec[RequestType, ResponseType](),
		Handler:    testGenericHandler[RequestType, ResponseType],
		SourceType: Base62QueryParameter,
		SourceKey:  "data",
	}, time.Duration(0), int64(0), nil) // Added effective settings

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("Expected status code %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

// TestRegisterGenericRouteWithBase62QueryParameterInvalid tests RegisterGenericRoute with invalid base62 query parameter
// (from register_generic_route_base62_test.go)
func TestRegisterGenericRouteWithBase62QueryParameterInvalid(t *testing.T) {
	logger := zap.NewNop()
	r := NewRouter[string, string](RouterConfig{Logger: logger}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	RegisterGenericRoute[RequestType, ResponseType, string, string](r, RouteConfig[RequestType, ResponseType]{
		Path:       "/test",
		Methods:    []string{"GET"},
		Codec:      codec.NewJSONCodec[RequestType, ResponseType](),
		Handler:    testGenericHandler[RequestType, ResponseType],
		SourceType: Base62QueryParameter,
		SourceKey:  "data",
	}, time.Duration(0), int64(0), nil) // Added effective settings

	req := httptest.NewRequest("GET", "/test?data=invalid!@#$", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("Expected status code %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

// TestRegisterGenericRouteWithBase62PathParameterMissing tests RegisterGenericRoute with missing base62 path parameter
// (from register_generic_route_base62_test.go)
func TestRegisterGenericRouteWithBase62PathParameterMissing(t *testing.T) {
	logger := zap.NewNop()
	r := NewRouter[string, string](RouterConfig{Logger: logger}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	RegisterGenericRoute[RequestType, ResponseType, string, string](r, RouteConfig[RequestType, ResponseType]{
		Path:       "/test/:somevalue",
		Methods:    []string{"GET"},
		Codec:      codec.NewJSONCodec[RequestType, ResponseType](),
		Handler:    testGenericHandler[RequestType, ResponseType],
		SourceType: Base62PathParameter,
		SourceKey:  "nonexistent",
	}, time.Duration(0), int64(0), nil) // Added effective settings

	req := httptest.NewRequest("GET", "/test/somevalue", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("Expected status code %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

// TestRegisterGenericRouteWithBase62PathParameterInvalid tests RegisterGenericRoute with invalid base62 path parameter
// (from register_generic_route_base62_test.go)
func TestRegisterGenericRouteWithBase62PathParameterInvalid(t *testing.T) {
	logger := zap.NewNop()
	r := NewRouter[string, string](RouterConfig{Logger: logger}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	RegisterGenericRoute[RequestType, ResponseType, string, string](r, RouteConfig[RequestType, ResponseType]{
		Path:       "/test/:data",
		Methods:    []string{"GET"},
		Codec:      codec.NewJSONCodec[RequestType, ResponseType](),
		Handler:    testGenericHandler[RequestType, ResponseType],
		SourceType: Base62PathParameter,
		SourceKey:  "data",
	}, time.Duration(0), int64(0), nil) // Added effective settings

	req := httptest.NewRequest("GET", "/test/invalid!@#$", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("Expected status code %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

// TestRegisterGenericRouteWithEncodeError tests registering a generic route with an encode error
// (from register_generic_route_error_test.go - adapted)
func TestRegisterGenericRouteWithEncodeError(t *testing.T) {
	logger := zap.NewNop()
	r := NewRouter[string, string](RouterConfig{Logger: logger}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	RegisterGenericRoute(r, RouteConfig[RequestType, UnmarshalableResponse]{
		Path:    "/greet-encode-error",
		Methods: []string{"POST"},
		Codec:   codec.NewJSONCodec[RequestType, UnmarshalableResponse](),
		Handler: func(req *http.Request, data RequestType) (UnmarshalableResponse, error) {
			return UnmarshalableResponse{
				Channel: make(chan int),
			}, nil
		},
	}, time.Duration(0), int64(0), nil) // Added effective settings

	req, _ := http.NewRequest("POST", "/greet-encode-error", strings.NewReader(`{"name":"John","age":30}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("Expected status code %d, got %d", http.StatusInternalServerError, rr.Code)
	}
}

// TestRegisterGenericRouteWithMiddleware tests RegisterGenericRoute with middleware
// (from register_generic_route_middleware_test.go)
func TestRegisterGenericRouteWithMiddleware(t *testing.T) {
	logger := zap.NewNop()
	r := NewRouter[string, string](RouterConfig{Logger: logger}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	middleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Test", "test")
			next.ServeHTTP(w, r)
		})
	}

	RegisterGenericRoute[RequestType, ResponseType, string, string](r, RouteConfig[RequestType, ResponseType]{
		Path:        "/test",
		Methods:     []string{"POST"},
		Codec:       codec.NewJSONCodec[RequestType, ResponseType](),
		Handler:     testGenericHandler[RequestType, ResponseType],
		SourceType:  Body,
		Middlewares: []Middleware{middleware},
	}, time.Duration(0), int64(0), nil) // Added effective settings

	reqBody := RequestType{ID: "123", Name: "John"}
	reqBytes, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/test", strings.NewReader(string(reqBytes)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rr.Code)
	}
	if rr.Header().Get("X-Test") != "test" {
		t.Errorf("Expected X-Test header to be %q, got %q", "test", rr.Header().Get("X-Test"))
	}
	var resp ResponseType
	err := json.Unmarshal(rr.Body.Bytes(), &resp)
	if err != nil {
		t.Errorf("Failed to unmarshal response: %v", err)
	}
	if resp.Message != "Hello, John!" {
		t.Errorf("Expected message %q, got %q", "Hello, John!", resp.Message)
	}
}

// TestRegisterGenericRouteWithTimeout tests RegisterGenericRoute with timeout
// (from register_generic_route_middleware_test.go)
func TestRegisterGenericRouteWithTimeout(t *testing.T) {
	t.Skip("Skipping flaky test") // Kept the skip from original file
}

// TestRegisterGenericRouteWithMaxBodySize tests RegisterGenericRoute with max body size
// (from register_generic_route_middleware_test.go)
func TestRegisterGenericRouteWithMaxBodySize(t *testing.T) {
	logger := zap.NewNop()
	r := NewRouter[string, string](RouterConfig{Logger: logger}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	// Calculate size of a JSON body to ensure our test is accurate
	smallBody := RequestType{ID: "1", Name: "A"}
	smallBytes, _ := json.Marshal(smallBody)
	smallSize := len(smallBytes)

	// Set the MaxBodySize to allow only the small body (plus a bit of buffer)
	maxBodySize := int64(smallSize + 5) // Small buffer to ensure small body passes

	RegisterGenericRoute[RequestType, ResponseType, string, string](r, RouteConfig[RequestType, ResponseType]{
		Path:        "/test",
		Methods:     []string{"POST"},
		Codec:       codec.NewJSONCodec[RequestType, ResponseType](),
		Handler:     testGenericHandler[RequestType, ResponseType],
		SourceType:  Body,
		MaxBodySize: maxBodySize,
	}, time.Duration(0), maxBodySize, nil) // Use maxBodySize here, timeout 0, rate limit nil

	// Request with small body (should succeed)
	reqBodySmall := smallBody
	reqBytesSmall, _ := json.Marshal(reqBodySmall)
	reqSmall := httptest.NewRequest("POST", "/test", strings.NewReader(string(reqBytesSmall)))
	reqSmall.Header.Set("Content-Type", "application/json")
	rrSmall := httptest.NewRecorder()
	r.ServeHTTP(rrSmall, reqSmall)

	if rrSmall.Code != http.StatusOK {
		t.Errorf("Expected status code %d for small body, got %d", http.StatusOK, rrSmall.Code)
	}

	// Request with large body (should fail)
	reqBodyLarge := RequestType{ID: "123456789", Name: "This is a much longer name that will exceed the size limit"}
	reqBytesLarge, _ := json.Marshal(reqBodyLarge)

	// Verify that large body is actually larger than our limit
	if len(reqBytesLarge) <= int(maxBodySize) {
		t.Fatalf("Test setup error: 'large' body (%d bytes) is not larger than max body size (%d bytes)",
			len(reqBytesLarge), maxBodySize)
	}

	reqLarge := httptest.NewRequest("POST", "/test", strings.NewReader(string(reqBytesLarge)))
	reqLarge.Header.Set("Content-Type", "application/json")
	rrLarge := httptest.NewRecorder()
	r.ServeHTTP(rrLarge, reqLarge)

	if rrLarge.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("Expected status code %d for large body, got %d", http.StatusRequestEntityTooLarge, rrLarge.Code)
	}
}

// TestRegisterGenericRouteWithQueryParameter tests RegisterGenericRoute with query parameter source type
// (from register_generic_route_query_test.go)
func TestRegisterGenericRouteWithQueryParameter(t *testing.T) {
	logger := zap.NewNop()
	r := NewRouter[string, string](RouterConfig{Logger: logger}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	RegisterGenericRoute[RequestType, ResponseType, string, string](r, RouteConfig[RequestType, ResponseType]{
		Path:       "/test",
		Methods:    []string{"GET"},
		Codec:      codec.NewJSONCodec[RequestType, ResponseType](),
		Handler:    testGenericHandler[RequestType, ResponseType],
		SourceType: Base64QueryParameter,
		SourceKey:  "data",
	}, time.Duration(0), int64(0), nil) // Added effective settings

	base64Data := "eyJpZCI6IjEyMyIsIm5hbWUiOiJKb2huIn0=" // Base64 encoded {"id":"123","name":"John"}
	req := httptest.NewRequest("GET", "/test?data="+base64Data, nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rr.Code)
	}
	var resp ResponseType
	err := json.Unmarshal(rr.Body.Bytes(), &resp)
	if err != nil {
		t.Errorf("Failed to unmarshal response: %v", err)
	}
	if resp.Message != "Hello, John!" {
		t.Errorf("Expected message %q, got %q", "Hello, John!", resp.Message)
	}
}

// TestRegisterGenericRouteWithPathParameter tests RegisterGenericRoute with path parameter source type
// (from register_generic_route_query_test.go)
func TestRegisterGenericRouteWithPathParameter(t *testing.T) {
	logger := zap.NewNop()
	r := NewRouter[string, string](RouterConfig{Logger: logger}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	RegisterGenericRoute[RequestType, ResponseType, string, string](r, RouteConfig[RequestType, ResponseType]{
		Path:       "/test/:data",
		Methods:    []string{"GET"},
		Codec:      codec.NewJSONCodec[RequestType, ResponseType](),
		Handler:    testGenericHandler[RequestType, ResponseType],
		SourceType: Base64PathParameter,
		SourceKey:  "data",
	}, time.Duration(0), int64(0), nil) // Added effective settings

	base64Data := "eyJpZCI6IjEyMyIsIm5hbWUiOiJKb2huIn0=" // Base64 encoded {"id":"123","name":"John"}
	req := httptest.NewRequest("GET", "/test/"+base64Data, nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rr.Code)
	}
	var resp ResponseType
	err := json.Unmarshal(rr.Body.Bytes(), &resp)
	if err != nil {
		t.Errorf("Failed to unmarshal response: %v", err)
	}
	if resp.Message != "Hello, John!" {
		t.Errorf("Expected message %q, got %q", "Hello, John!", resp.Message)
	}
}

// TestRegisterGenericRouteWithBase64QueryParameter tests RegisterGenericRoute with base64 query parameter source type
// (from register_generic_route_query_test.go - duplicate name, keeping content)
func TestRegisterGenericRouteWithBase64QueryParameterAgain(t *testing.T) {
	logger := zap.NewNop()
	r := NewRouter[string, string](RouterConfig{Logger: logger}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	RegisterGenericRoute[RequestType, ResponseType, string, string](r, RouteConfig[RequestType, ResponseType]{
		Path:       "/test",
		Methods:    []string{"GET"},
		Codec:      codec.NewJSONCodec[RequestType, ResponseType](),
		Handler:    testGenericHandler[RequestType, ResponseType],
		SourceType: Base64QueryParameter,
		SourceKey:  "data",
	}, time.Duration(0), int64(0), nil) // Added effective settings

	base64Data := "eyJpZCI6IjEyMyIsIm5hbWUiOiJKb2huIn0=" // Base64 encoded {"id":"123","name":"John"}
	req := httptest.NewRequest("GET", "/test?data="+base64Data, nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rr.Code)
	}
	var resp ResponseType
	err := json.Unmarshal(rr.Body.Bytes(), &resp)
	if err != nil {
		t.Errorf("Failed to unmarshal response: %v", err)
	}
}
