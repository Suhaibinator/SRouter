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
	"github.com/Suhaibinator/SRouter/pkg/common"
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
	var dataMap map[string]any
	_ = json.Unmarshal(dataBytes, &dataMap)

	// Create response
	var respMap map[string]any
	if name, ok := dataMap["name"].(string); ok {
		respMap = map[string]any{
			"message": "Hello, " + name + "!",
			"id":      dataMap["id"],
			"name":    name,
		}
	} else {
		respMap = map[string]any{
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

// SourceTestHandler is a simple handler for testing source types
func SourceTestHandler(r *http.Request, req SourceTestRequest) (SourceTestResponse, error) {
	return SourceTestResponse{
		Message: "Hello, " + req.Name + "!",
		ID:      req.ID,
		Name:    req.Name,
	}, nil
}

// --- Tests ---

// TestRegisterGenericRouteWithBody tests RegisterGenericRoute with body source type
// (from register_generic_route_test.go)
func TestRegisterGenericRouteWithBody(t *testing.T) {
	logger := zap.NewNop()
	r := NewRouter(RouterConfig{Logger: logger}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	RegisterGenericRoute(r, RouteConfig[RequestType, ResponseType]{
		Path:       "/test",
		Methods:    []HttpMethod{MethodPost}, // Use HttpMethod enum
		Codec:      codec.NewJSONCodec[RequestType, ResponseType](),
		Handler:    testGenericHandler[RequestType, ResponseType],
		SourceType: Body,
		// AuthLevel: nil (default NoAuth)
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

// --- Sanitizer Tests ---

// Sanitizer that modifies the name
func nameSanitizer(req RequestType) (RequestType, error) {
	sanitized := req // Make a copy
	sanitized.Name = "Sanitized " + sanitized.Name
	return sanitized, nil
}

// Sanitizer that returns an error
func errorSanitizer(req RequestType) (RequestType, error) {
	return req, errors.New("sanitizer error")
}

// TestRegisterGenericRouteWithSanitizerSuccess tests successful sanitization
func TestRegisterGenericRouteWithSanitizerSuccess(t *testing.T) {
	logger := zap.NewNop()
	r := NewRouter(RouterConfig{Logger: logger}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	RegisterGenericRoute(r, RouteConfig[RequestType, ResponseType]{
		Path:       "/test-sanitize-success",
		Methods:    []HttpMethod{MethodPost},
		Codec:      codec.NewJSONCodec[RequestType, ResponseType](),
		Handler:    testGenericHandler[RequestType, ResponseType], // Handler should receive sanitized data
		SourceType: Body,
		Sanitizer:  nameSanitizer, // Add the successful sanitizer
	}, time.Duration(0), int64(0), nil)

	reqBody := RequestType{ID: "sanitize1", Name: "Original"}
	reqBytes, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/test-sanitize-success", strings.NewReader(string(reqBytes)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d. Body: %s", http.StatusOK, rr.Code, rr.Body.String())
	}
	var resp ResponseType
	err := json.Unmarshal(rr.Body.Bytes(), &resp)
	if err != nil {
		t.Errorf("Failed to unmarshal response: %v", err)
	}
	// The handler should receive the sanitized name and include it in the response
	expectedMessage := "Hello, Sanitized Original!"
	if resp.Message != expectedMessage {
		t.Errorf("Expected message %q, got %q", expectedMessage, resp.Message)
	}
	if resp.Name != "Sanitized Original" {
		t.Errorf("Expected sanitized name %q in response, got %q", "Sanitized Original", resp.Name)
	}
	if resp.ID != "sanitize1" {
		t.Errorf("Expected ID %q, got %q", "sanitize1", resp.ID)
	}
}

// TestRegisterGenericRouteWithSanitizerError tests sanitizer returning an error
func TestRegisterGenericRouteWithSanitizerError(t *testing.T) {
	logger := zap.NewNop()
	r := NewRouter(RouterConfig{Logger: logger}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	RegisterGenericRoute(r, RouteConfig[RequestType, ResponseType]{
		Path:       "/test-sanitize-error",
		Methods:    []HttpMethod{MethodPost},
		Codec:      codec.NewJSONCodec[RequestType, ResponseType](),
		Handler:    testGenericHandler[RequestType, ResponseType],
		SourceType: Body,
		Sanitizer:  errorSanitizer, // Add the erroring sanitizer
	}, time.Duration(0), int64(0), nil)

	reqBody := RequestType{ID: "sanitize2", Name: "ErrorCase"}
	reqBytes, _ := json.Marshal(reqBody)
	req := httptest.NewRequest("POST", "/test-sanitize-error", strings.NewReader(string(reqBytes)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("Expected status code %d, got %d", http.StatusBadRequest, rr.Code)
	}

	// Check error message in response body
	var errResp map[string]map[string]string
	err := json.Unmarshal(rr.Body.Bytes(), &errResp)
	if err != nil {
		t.Fatalf("Failed to unmarshal error response: %v", err)
	}
	if errMsg, ok := errResp["error"]["message"]; !ok || errMsg != "Sanitization failed" {
		t.Errorf("Expected error message 'Sanitization failed', got '%s'", errMsg)
	}
}

// TestRegisterGenericRouteWithUnsupportedSourceType tests RegisterGenericRoute with an unsupported source type
// (from register_generic_route_test.go)
func TestRegisterGenericRouteWithUnsupportedSourceType(t *testing.T) {
	logger := zap.NewNop()
	r := NewRouter(RouterConfig{Logger: logger}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	RegisterGenericRoute(r, RouteConfig[RequestType, ResponseType]{
		Path:       "/test",
		Methods:    []HttpMethod{MethodGet}, // Use HttpMethod enum
		Codec:      codec.NewJSONCodec[RequestType, ResponseType](),
		Handler:    testGenericHandler[RequestType, ResponseType],
		SourceType: SourceType(999), // Unsupported source type
		// AuthLevel: nil (default NoAuth)
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
	authFunc := func(ctx context.Context, token string) (*string, bool) { user := "user123"; return &user, true }
	r := NewRouter(RouterConfig{Logger: logger}, authFunc, mocks.MockUserIDFromUser)

	RegisterGenericRoute(r, RouteConfig[RequestType, ResponseType]{
		Path:       "/test",
		Methods:    []HttpMethod{MethodPost}, // Use HttpMethod enum
		Codec:      codec.NewJSONCodec[RequestType, ResponseType](),
		Handler:    testGenericHandler[RequestType, ResponseType],
		SourceType: Body,
		AuthLevel:  Ptr(AuthRequired), // Changed
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
	authFunc := func(ctx context.Context, token string) (*string, bool) { user := "user123"; return &user, true }
	r := NewRouter(RouterConfig{Logger: logger}, authFunc, mocks.MockUserIDFromUser)

	RegisterGenericRoute(r, RouteConfig[RequestType, ResponseType]{
		Path:       "/test",
		Methods:    []HttpMethod{MethodPost}, // Use HttpMethod enum
		Codec:      codec.NewJSONCodec[RequestType, ResponseType](),
		Handler:    testGenericHandler[RequestType, ResponseType],
		SourceType: Body,
		AuthLevel:  Ptr(AuthOptional), // Changed
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
	r := NewRouter(RouterConfig{Logger: logger}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	RegisterGenericRoute(r, RouteConfig[RequestType, ResponseType]{
		Path:       "/test",
		Methods:    []HttpMethod{MethodGet}, // Use HttpMethod enum
		Codec:      codec.NewJSONCodec[RequestType, ResponseType](),
		Handler:    testGenericHandler[RequestType, ResponseType],
		SourceType: Base62QueryParameter,
		SourceKey:  "data",
		// AuthLevel: nil (default NoAuth)
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
	r := NewRouter(RouterConfig{Logger: logger}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	RegisterGenericRoute(r, RouteConfig[RequestType, ResponseType]{
		Path:       "/test/:data",
		Methods:    []HttpMethod{MethodGet}, // Use HttpMethod enum
		Codec:      codec.NewJSONCodec[RequestType, ResponseType](),
		Handler:    testGenericHandler[RequestType, ResponseType],
		SourceType: Base62PathParameter,
		SourceKey:  "data",
		// AuthLevel: nil (default NoAuth)
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
	r := NewRouter(RouterConfig{Logger: logger}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	RegisterGenericRoute(r, RouteConfig[RequestType, ResponseType]{
		Path:       "/test",
		Methods:    []HttpMethod{MethodGet}, // Use HttpMethod enum
		Codec:      codec.NewJSONCodec[RequestType, ResponseType](),
		Handler:    testGenericHandler[RequestType, ResponseType],
		SourceType: Base62QueryParameter,
		SourceKey:  "data",
		// AuthLevel: nil (default NoAuth)
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
	r := NewRouter(RouterConfig{Logger: logger}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	RegisterGenericRoute(r, RouteConfig[RequestType, ResponseType]{
		Path:       "/test",
		Methods:    []HttpMethod{MethodGet}, // Use HttpMethod enum
		Codec:      codec.NewJSONCodec[RequestType, ResponseType](),
		Handler:    testGenericHandler[RequestType, ResponseType],
		SourceType: Base62QueryParameter,
		SourceKey:  "data",
		// AuthLevel: nil (default NoAuth)
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
	r := NewRouter(RouterConfig{Logger: logger}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	RegisterGenericRoute(r, RouteConfig[RequestType, ResponseType]{
		Path:       "/test/:somevalue",
		Methods:    []HttpMethod{MethodGet}, // Use HttpMethod enum
		Codec:      codec.NewJSONCodec[RequestType, ResponseType](),
		Handler:    testGenericHandler[RequestType, ResponseType],
		SourceType: Base62PathParameter,
		SourceKey:  "nonexistent",
		// AuthLevel: nil (default NoAuth)
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
	r := NewRouter(RouterConfig{Logger: logger}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	RegisterGenericRoute(r, RouteConfig[RequestType, ResponseType]{
		Path:       "/test/:data",
		Methods:    []HttpMethod{MethodGet}, // Use HttpMethod enum
		Codec:      codec.NewJSONCodec[RequestType, ResponseType](),
		Handler:    testGenericHandler[RequestType, ResponseType],
		SourceType: Base62PathParameter,
		SourceKey:  "data",
		// AuthLevel: nil (default NoAuth)
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
	r := NewRouter(RouterConfig{Logger: logger}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	RegisterGenericRoute(r, RouteConfig[RequestType, UnmarshalableResponse]{
		Path:    "/greet-encode-error",
		Methods: []HttpMethod{MethodPost}, // Use HttpMethod enum
		Codec:   codec.NewJSONCodec[RequestType, UnmarshalableResponse](),
		Handler: func(req *http.Request, data RequestType) (UnmarshalableResponse, error) {
			return UnmarshalableResponse{
				Channel: make(chan int),
			}, nil
		},
		// AuthLevel: nil (default NoAuth)
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
	r := NewRouter(RouterConfig{Logger: logger}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	middleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Test", "test")
			next.ServeHTTP(w, r)
		})
	}

	RegisterGenericRoute(r, RouteConfig[RequestType, ResponseType]{
		Path:        "/test",
		Methods:     []HttpMethod{MethodPost}, // Use HttpMethod enum
		Codec:       codec.NewJSONCodec[RequestType, ResponseType](),
		Handler:     testGenericHandler[RequestType, ResponseType],
		SourceType:  Body,
		Middlewares: []common.Middleware{middleware},
		// AuthLevel: nil (default NoAuth)
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
	r := NewRouter(RouterConfig{Logger: logger}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	// Calculate size of a JSON body to ensure our test is accurate
	smallBody := RequestType{ID: "1", Name: "A"}
	smallBytes, _ := json.Marshal(smallBody)
	smallSize := len(smallBytes)

	// Set the MaxBodySize to allow only the small body (plus a bit of buffer)
	maxBodySize := int64(smallSize + 5) // Small buffer to ensure small body passes

	RegisterGenericRoute(r, RouteConfig[RequestType, ResponseType]{
		Path:       "/test",
		Methods:    []HttpMethod{MethodPost}, // Use HttpMethod enum
		Codec:      codec.NewJSONCodec[RequestType, ResponseType](),
		Handler:    testGenericHandler[RequestType, ResponseType],
		SourceType: Body,
		Overrides:  common.RouteOverrides{MaxBodySize: maxBodySize},
		// AuthLevel: nil (default NoAuth)
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
	r := NewRouter(RouterConfig{Logger: logger}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	RegisterGenericRoute(r, RouteConfig[RequestType, ResponseType]{
		Path:       "/test",
		Methods:    []HttpMethod{MethodGet}, // Use HttpMethod enum
		Codec:      codec.NewJSONCodec[RequestType, ResponseType](),
		Handler:    testGenericHandler[RequestType, ResponseType],
		SourceType: Base64QueryParameter,
		SourceKey:  "data",
		// AuthLevel: nil (default NoAuth)
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
	r := NewRouter(RouterConfig{Logger: logger}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	RegisterGenericRoute(r, RouteConfig[RequestType, ResponseType]{
		Path:       "/test/:data",
		Methods:    []HttpMethod{MethodGet}, // Use HttpMethod enum
		Codec:      codec.NewJSONCodec[RequestType, ResponseType](),
		Handler:    testGenericHandler[RequestType, ResponseType],
		SourceType: Base64PathParameter,
		SourceKey:  "data",
		// AuthLevel: nil (default NoAuth)
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
	r := NewRouter(RouterConfig{Logger: logger}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	RegisterGenericRoute(r, RouteConfig[RequestType, ResponseType]{
		Path:       "/test",
		Methods:    []HttpMethod{MethodGet}, // Use HttpMethod enum
		Codec:      codec.NewJSONCodec[RequestType, ResponseType](),
		Handler:    testGenericHandler[RequestType, ResponseType],
		SourceType: Base64QueryParameter,
		SourceKey:  "data",
		// AuthLevel: nil (default NoAuth)
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
