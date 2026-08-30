package router

import (
	"context"
	json "encoding/json/v2"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time" // Added for zero value

	"github.com/Suhaibinator/SRouter/pkg/codec"
	"github.com/Suhaibinator/SRouter/pkg/common"
	"github.com/Suhaibinator/SRouter/pkg/router/internal/mocks" // Import the new mocks package
	"github.com/stretchr/testify/require"
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

// Test request and response types for routegroup tests
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

// TestRegisterTypedRouteWithBody tests typed route registration with body source type
// (from register_generic_route_test.go)
func TestRegisterTypedRouteWithBody(t *testing.T) {
	logger := zap.NewNop()
	r := NewRouter(RouterConfig{Logger: logger}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	r.Route(RouteConfig[RequestType, ResponseType]{
		Path:       "/test",
		Methods:    []HttpMethod{MethodPost}, // Use HttpMethod enum
		Codec:      codec.NewJSONCodec[RequestType, ResponseType](),
		Handler:    testGenericHandler[RequestType, ResponseType],
		SourceType: Body,
		// AuthLevel: nil (default NoAuth)
	})

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
func nameSanitizer(_ context.Context, req RequestType) (RequestType, error) {
	sanitized := req // Make a copy
	sanitized.Name = "Sanitized " + sanitized.Name
	return sanitized, nil
}

// Sanitizer that returns an error
func errorSanitizer(_ context.Context, req RequestType) (RequestType, error) {
	return req, errors.New("sanitizer error")
}

// TestRegisterTypedRouteWithSanitizerSuccess tests successful sanitization
func TestRegisterTypedRouteWithSanitizerSuccess(t *testing.T) {
	logger := zap.NewNop()
	r := NewRouter(RouterConfig{Logger: logger}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	r.Route(RouteConfig[RequestType, ResponseType]{
		Path:       "/test-sanitize-success",
		Methods:    []HttpMethod{MethodPost},
		Codec:      codec.NewJSONCodec[RequestType, ResponseType](),
		Handler:    testGenericHandler[RequestType, ResponseType], // Handler should receive sanitized data
		SourceType: Body,
		Sanitizer:  nameSanitizer, // Add the successful sanitizer
	})

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

// TestRegisterTypedRouteWithSanitizerError tests sanitizer returning an error
func TestRegisterTypedRouteWithSanitizerError(t *testing.T) {
	logger := zap.NewNop()
	r := NewRouter(RouterConfig{Logger: logger}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	r.Route(RouteConfig[RequestType, ResponseType]{
		Path:       "/test-sanitize-error",
		Methods:    []HttpMethod{MethodPost},
		Codec:      codec.NewJSONCodec[RequestType, ResponseType](),
		Handler:    testGenericHandler[RequestType, ResponseType],
		SourceType: Body,
		Sanitizer:  errorSanitizer, // Add the erroring sanitizer
	})

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

// TestRegisterTypedRouteSanitizerReceivesRequestContext verifies that the
// sanitizer observes the active context after route middleware has run.
func TestRegisterTypedRouteSanitizerReceivesRequestContext(t *testing.T) {
	type sanitizerContextKey struct{}

	const contextValue = "from-route-middleware"
	key := sanitizerContextKey{}
	sanitizerCalled := false

	r := NewRouter(RouterConfig{Logger: zap.NewNop()}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)
	r.Route(RouteConfig[RequestType, ResponseType]{
		Path:       "/test-sanitize-context",
		Methods:    []HttpMethod{MethodPost},
		Codec:      codec.NewJSONCodec[RequestType, ResponseType](),
		Handler:    testGenericHandler[RequestType, ResponseType],
		SourceType: Body,
		Middlewares: []common.Middleware{
			func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
					ctx := context.WithValue(req.Context(), key, contextValue)
					next.ServeHTTP(w, req.WithContext(ctx))
				})
			},
		},
		Sanitizer: func(ctx context.Context, req RequestType) (RequestType, error) {
			sanitizerCalled = true
			require.Equal(t, contextValue, ctx.Value(key))
			return req, nil
		},
	})

	req := httptest.NewRequest("POST", "/test-sanitize-context", strings.NewReader(`{"id":"ctx","name":"Context"}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	require.Equal(t, http.StatusOK, rr.Code)
	require.True(t, sanitizerCalled)
}

// TestRegisterTypedRouteWithUnsupportedSourceType tests typed route registration with an unsupported source type
// (from register_generic_route_test.go)
func TestRegisterTypedRouteWithUnsupportedSourceType(t *testing.T) {
	logger := zap.NewNop()
	r := NewRouter(RouterConfig{Logger: logger}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	r.Route(RouteConfig[RequestType, ResponseType]{
		Path:       "/test",
		Methods:    []HttpMethod{MethodGet}, // Use HttpMethod enum
		Codec:      codec.NewJSONCodec[RequestType, ResponseType](),
		Handler:    testGenericHandler[RequestType, ResponseType],
		SourceType: SourceType(999), // Unsupported source type
		// AuthLevel: nil (default NoAuth)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("Expected status code %d, got %d", http.StatusInternalServerError, rr.Code)
	}
}

// TestRegisterTypedRouteWithAuthRequired tests typed route registration with AuthRequired
// (from register_generic_route_auth_test.go)
func TestRegisterTypedRouteWithAuthRequired(t *testing.T) {
	logger := zap.NewNop()
	// Auth function always returns true
	authFunc := func(ctx context.Context, token string) (*string, bool) { user := "user123"; return &user, true }
	r := NewRouter(RouterConfig{Logger: logger}, authFunc, mocks.MockUserIDFromUser)

	r.Route(RouteConfig[RequestType, ResponseType]{
		Path:       "/test",
		Methods:    []HttpMethod{MethodPost}, // Use HttpMethod enum
		Codec:      codec.NewJSONCodec[RequestType, ResponseType](),
		Handler:    testGenericHandler[RequestType, ResponseType],
		SourceType: Body,
		AuthLevel:  new(AuthRequired), // Changed
	})

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

// TestRegisterTypedRouteWithAuthOptional tests typed route registration with AuthOptional
// (from register_generic_route_auth_test.go)
func TestRegisterTypedRouteWithAuthOptional(t *testing.T) {
	logger := zap.NewNop()
	// Auth function always returns true
	authFunc := func(ctx context.Context, token string) (*string, bool) { user := "user123"; return &user, true }
	r := NewRouter(RouterConfig{Logger: logger}, authFunc, mocks.MockUserIDFromUser)

	r.Route(RouteConfig[RequestType, ResponseType]{
		Path:       "/test",
		Methods:    []HttpMethod{MethodPost}, // Use HttpMethod enum
		Codec:      codec.NewJSONCodec[RequestType, ResponseType](),
		Handler:    testGenericHandler[RequestType, ResponseType],
		SourceType: Body,
		AuthLevel:  new(AuthOptional), // Changed
	})

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

// TestRegisterTypedRouteWithBase62QueryParameter tests typed route registration with base62 query parameter source type
// (from register_generic_route_base62_test.go)
func TestRegisterTypedRouteWithBase62QueryParameter(t *testing.T) {
	logger := zap.NewNop()
	r := NewRouter(RouterConfig{Logger: logger}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	r.Route(RouteConfig[RequestType, ResponseType]{
		Path:       "/test",
		Methods:    []HttpMethod{MethodGet}, // Use HttpMethod enum
		Codec:      codec.NewJSONCodec[RequestType, ResponseType](),
		Handler:    testGenericHandler[RequestType, ResponseType],
		SourceType: Base62QueryParameter,
		SourceKey:  "data",
		// AuthLevel: nil (default NoAuth)
	})

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

// TestRegisterTypedRouteWithBase62PathParameter tests typed route registration with base62 path parameter source type
// (from register_generic_route_base62_test.go)
func TestRegisterTypedRouteWithBase62PathParameter(t *testing.T) {
	logger := zap.NewNop()
	r := NewRouter(RouterConfig{Logger: logger}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	r.Route(RouteConfig[RequestType, ResponseType]{
		Path:       "/test/:data",
		Methods:    []HttpMethod{MethodGet}, // Use HttpMethod enum
		Codec:      codec.NewJSONCodec[RequestType, ResponseType](),
		Handler:    testGenericHandler[RequestType, ResponseType],
		SourceType: Base62PathParameter,
		SourceKey:  "data",
		// AuthLevel: nil (default NoAuth)
	})

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

// TestRegisterTypedRouteWithBase62QueryParameterMissing tests typed route registration with missing base62 query parameter
// (from register_generic_route_base62_test.go)
func TestRegisterTypedRouteWithBase62QueryParameterMissing(t *testing.T) {
	logger := zap.NewNop()
	r := NewRouter(RouterConfig{Logger: logger}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	r.Route(RouteConfig[RequestType, ResponseType]{
		Path:       "/test",
		Methods:    []HttpMethod{MethodGet}, // Use HttpMethod enum
		Codec:      codec.NewJSONCodec[RequestType, ResponseType](),
		Handler:    testGenericHandler[RequestType, ResponseType],
		SourceType: Base62QueryParameter,
		SourceKey:  "data",
		// AuthLevel: nil (default NoAuth)
	})

	req := httptest.NewRequest("GET", "/test", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("Expected status code %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

// TestRegisterTypedRouteWithBase62QueryParameterInvalid tests typed route registration with invalid base62 query parameter
// (from register_generic_route_base62_test.go)
func TestRegisterTypedRouteWithBase62QueryParameterInvalid(t *testing.T) {
	logger := zap.NewNop()
	r := NewRouter(RouterConfig{Logger: logger}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	r.Route(RouteConfig[RequestType, ResponseType]{
		Path:       "/test",
		Methods:    []HttpMethod{MethodGet}, // Use HttpMethod enum
		Codec:      codec.NewJSONCodec[RequestType, ResponseType](),
		Handler:    testGenericHandler[RequestType, ResponseType],
		SourceType: Base62QueryParameter,
		SourceKey:  "data",
		// AuthLevel: nil (default NoAuth)
	})

	req := httptest.NewRequest("GET", "/test?data=invalid!@#$", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("Expected status code %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

// TestRegisterTypedRouteWithBase62PathParameterMissing tests typed route registration with missing base62 path parameter
// (from register_generic_route_base62_test.go)
func TestRegisterTypedRouteWithBase62PathParameterMissing(t *testing.T) {
	logger := zap.NewNop()
	r := NewRouter(RouterConfig{Logger: logger}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	r.Route(RouteConfig[RequestType, ResponseType]{
		Path:       "/test/:somevalue",
		Methods:    []HttpMethod{MethodGet}, // Use HttpMethod enum
		Codec:      codec.NewJSONCodec[RequestType, ResponseType](),
		Handler:    testGenericHandler[RequestType, ResponseType],
		SourceType: Base62PathParameter,
		SourceKey:  "nonexistent",
		// AuthLevel: nil (default NoAuth)
	})

	req := httptest.NewRequest("GET", "/test/somevalue", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("Expected status code %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

// TestRegisterTypedRouteWithBase62PathParameterInvalid tests typed route registration with invalid base62 path parameter
// (from register_generic_route_base62_test.go)
func TestRegisterTypedRouteWithBase62PathParameterInvalid(t *testing.T) {
	logger := zap.NewNop()
	r := NewRouter(RouterConfig{Logger: logger}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	r.Route(RouteConfig[RequestType, ResponseType]{
		Path:       "/test/:data",
		Methods:    []HttpMethod{MethodGet}, // Use HttpMethod enum
		Codec:      codec.NewJSONCodec[RequestType, ResponseType](),
		Handler:    testGenericHandler[RequestType, ResponseType],
		SourceType: Base62PathParameter,
		SourceKey:  "data",
		// AuthLevel: nil (default NoAuth)
	})

	req := httptest.NewRequest("GET", "/test/invalid!@#$", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusBadRequest {
		t.Errorf("Expected status code %d, got %d", http.StatusBadRequest, rr.Code)
	}
}

// TestRegisterTypedRouteWithEncodeError tests registering a generic route with an encode error
// (from register_generic_route_error_test.go - adapted)
func TestRegisterTypedRouteWithEncodeError(t *testing.T) {
	logger := zap.NewNop()
	r := NewRouter(RouterConfig{Logger: logger}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	r.Route(RouteConfig[RequestType, UnmarshalableResponse]{
		Path:    "/greet-encode-error",
		Methods: []HttpMethod{MethodPost}, // Use HttpMethod enum
		Codec:   codec.NewJSONCodec[RequestType, UnmarshalableResponse](),
		Handler: func(req *http.Request, data RequestType) (UnmarshalableResponse, error) {
			return UnmarshalableResponse{
				Channel: make(chan int),
			}, nil
		},
		// AuthLevel: nil (default NoAuth)
	})

	req, _ := http.NewRequest("POST", "/greet-encode-error", strings.NewReader(`{"name":"John","age":30}`))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusInternalServerError {
		t.Errorf("Expected status code %d, got %d", http.StatusInternalServerError, rr.Code)
	}
}

// TestRegisterTypedRouteWithMiddleware tests typed route registration with middleware
// (from register_generic_route_middleware_test.go)
func TestRegisterTypedRouteWithMiddleware(t *testing.T) {
	logger := zap.NewNop()
	r := NewRouter(RouterConfig{Logger: logger}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	middleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-Test", "test")
			next.ServeHTTP(w, r)
		})
	}

	r.Route(RouteConfig[RequestType, ResponseType]{
		Path:        "/test",
		Methods:     []HttpMethod{MethodPost}, // Use HttpMethod enum
		Codec:       codec.NewJSONCodec[RequestType, ResponseType](),
		Handler:     testGenericHandler[RequestType, ResponseType],
		SourceType:  Body,
		Middlewares: []common.Middleware{middleware},
		// AuthLevel: nil (default NoAuth)
	})

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

// TestRegisterTypedRouteWithTimeout tests typed route registration with timeout
// (from register_generic_route_middleware_test.go)
func TestRegisterTypedRouteWithTimeout(t *testing.T) {
	logger := zap.NewNop()
	r := NewRouter(RouterConfig{Logger: logger}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	timeout := 1 * time.Millisecond
	ctxErrCh := make(chan error, 1)

	r.Route(RouteConfig[RequestType, ResponseType]{
		Path:    "/test",
		Methods: []HttpMethod{MethodPost},
		Codec:   codec.NewJSONCodec[RequestType, ResponseType](),
		Handler: func(r *http.Request, req RequestType) (ResponseType, error) {
			<-r.Context().Done()
			ctxErrCh <- r.Context().Err()
			return ResponseType{Message: "Should have timed out"}, r.Context().Err()
		},
		SourceType: Body,
		Overrides:  common.RouteOverrides{Timeout: timeout},
	})

	reqBody := RequestType{ID: "123", Name: "John"}
	reqBytes, err := json.Marshal(reqBody)
	require.NoError(t, err)
	req := httptest.NewRequest("POST", "/test", strings.NewReader(string(reqBytes)))
	req.Header.Set("Content-Type", "application/json")
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusRequestTimeout {
		t.Errorf("Expected status code %d, got %d", http.StatusRequestTimeout, rr.Code)
	}

	select {
	case err := <-ctxErrCh:
		if !errors.Is(err, context.DeadlineExceeded) {
			t.Errorf("Expected context deadline exceeded, got %v", err)
		}
	default:
		t.Error("Handler did not receive context cancellation")
	}
}

// TestRegisterTypedRouteWithMaxBodySize tests typed route registration with max body size
// (from register_generic_route_middleware_test.go)
func TestRegisterTypedRouteWithMaxBodySize(t *testing.T) {
	logger := zap.NewNop()
	r := NewRouter(RouterConfig{Logger: logger}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	// Calculate size of a JSON body to ensure our test is accurate
	smallBody := RequestType{ID: "1", Name: "A"}
	smallBytes, _ := json.Marshal(smallBody)
	smallSize := len(smallBytes)

	// Set the MaxBodySize to allow only the small body (plus a bit of buffer)
	maxBodySize := int64(smallSize + 5) // Small buffer to ensure small body passes

	r.Route(RouteConfig[RequestType, ResponseType]{
		Path:       "/test",
		Methods:    []HttpMethod{MethodPost}, // Use HttpMethod enum
		Codec:      codec.NewJSONCodec[RequestType, ResponseType](),
		Handler:    testGenericHandler[RequestType, ResponseType],
		SourceType: Body,
		Overrides:  common.RouteOverrides{MaxBodySize: maxBodySize},
		// AuthLevel: nil (default NoAuth)
	})

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

// TestRegisterTypedRouteWithBase64Parameters verifies base64 data from query and path sources.
func TestRegisterTypedRouteWithBase64Parameters(t *testing.T) {
	logger := zap.NewNop()
	base64Data := "eyJpZCI6IjEyMyIsIm5hbWUiOiJKb2huIn0=" // {"id":"123","name":"John"}

	tests := []struct {
		name       string
		path       string
		requestURL string
		sourceType SourceType
	}{
		{
			name:       "query parameter",
			path:       "/test",
			requestURL: "/test?data=" + base64Data,
			sourceType: Base64QueryParameter,
		},
		{
			name:       "path parameter",
			path:       "/test/:data",
			requestURL: "/test/" + base64Data,
			sourceType: Base64PathParameter,
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			r := NewRouter(RouterConfig{Logger: logger}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

			r.Route(RouteConfig[RequestType, ResponseType]{
				Path:       tc.path,
				Methods:    []HttpMethod{MethodGet},
				Codec:      codec.NewJSONCodec[RequestType, ResponseType](),
				Handler:    testGenericHandler[RequestType, ResponseType],
				SourceType: tc.sourceType,
				SourceKey:  "data",
			})

			req := httptest.NewRequest("GET", tc.requestURL, nil)
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)

			require.Equal(t, http.StatusOK, rr.Code)

			var resp ResponseType
			require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &resp))
			require.Equal(t, ResponseType{
				Message: "Hello, John!",
				ID:      "123",
				Name:    "John",
			}, resp)
		})
	}
}
