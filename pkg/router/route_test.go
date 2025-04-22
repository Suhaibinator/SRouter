package router_test

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/Suhaibinator/SRouter/pkg/codec"
	// "github.com/Suhaibinator/SRouter/pkg/common" // No longer needed here
	"github.com/Suhaibinator/SRouter/pkg/router"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Mock Codec for testing decoding errors
type mockErrorCodec struct{}

func (m *mockErrorCodec) Encode(w http.ResponseWriter, v testResponse) error { // Correct signature
	// Not needed for these tests, but must match interface
	return nil
}

// Add missing NewRequest method to satisfy the Codec interface
func (m *mockErrorCodec) NewRequest() testRequest {
	// Return zero value, not needed for these tests
	var zeroReq testRequest
	return zeroReq
}

func (m *mockErrorCodec) Decode(r *http.Request) (testRequest, error) {
	// Simulate a decoding error - return zero value for Req
	var zeroReq testRequest
	return zeroReq, assert.AnError
}

func (m *mockErrorCodec) DecodeBytes(b []byte) (testRequest, error) {
	// Simulate a decoding error - return zero value for Req
	var zeroReq testRequest
	return zeroReq, assert.AnError
}

func (m *mockErrorCodec) ContentType() string {
	return "application/json" // Or appropriate type
}

// Test structure for generic routes
type testRequest struct {
	Value string `json:"value"`
}

type testResponse struct {
	Message string `json:"message"`
}

// Basic handler for testing
func basicHandler(req *http.Request, data testRequest) (testResponse, error) {
	return testResponse{Message: "Success: " + data.Value}, nil
}

// --- Test Cases ---

// Test case for scenario 1: Failed to decode query parameter data
func TestRegisterGenericRoute_QueryParamDecodeError(t *testing.T) {
	r := router.NewRouter[string, string](router.RouterConfig{}, nil, nil) // Use value receiver
	mockCodec := &mockErrorCodec{}                                         // Codec that forces DecodeBytes error

	routeConfig := router.RouteConfig[testRequest, testResponse]{
		// RouteConfigBase fields are embedded
		Methods:    []router.HttpMethod{router.MethodGet}, // Use router types
		Path:       "/test",
		Handler:    basicHandler,
		Codec:      mockCodec,
		SourceType: router.Base64QueryParameter, // Test with Base64, Base62 is similar
		SourceKey:  "data",
	}

	// Encode some valid base64 data, the error happens *after* base64 decoding
	validBase64 := base64.StdEncoding.EncodeToString([]byte("trigger decode error"))
	targetURL := "/test?data=" + url.QueryEscape(validBase64)

	// Use r.ServeHTTP for this test as it involves query params
	req := httptest.NewRequest("GET", targetURL, nil)
	rr := httptest.NewRecorder()
	router.RegisterGenericRoute(r, routeConfig, 0, 0, nil) // Register the route
	r.ServeHTTP(rr, req)                                   // Serve the request

	assert.Equal(t, http.StatusBadRequest, rr.Code, "Expected status Bad Request")
	// Optionally check response body for specific message
	// require.Contains(t, rr.Body.String(), "Failed to decode query parameter data")
}

// Test case for scenario 2: Missing required path parameter
func TestRegisterGenericRoute_MissingPathParam(t *testing.T) {
	r := router.NewRouter[string, string](router.RouterConfig{}, nil, nil) // Use value receiver
	// Use a standard codec, the error is missing param, not decoding
	jsonCodec := codec.NewJSONCodec[testRequest, testResponse]()

	// Define path with param "actualParam", but look for "missingKey" in SourceKey
	routeConfig := router.RouteConfig[testRequest, testResponse]{
		Methods:    []router.HttpMethod{router.MethodGet}, // Use router types
		Path:       "/test/:actualParam",                  // Actual param name in path
		Handler:    basicHandler,
		Codec:      jsonCodec,
		SourceType: router.Base64PathParameter,
		SourceKey:  "missingKey", // Key we are looking for, which won't match 'actualParam'
	}

	// Register the route
	router.RegisterGenericRoute(r, routeConfig, 0, 0, nil)

	// Create request that matches the path pattern
	req := httptest.NewRequest("GET", "/test/someValue", nil) // Request matches /test/:actualParam
	rr := httptest.NewRecorder()

	// Serve the request using the router
	// httprouter will populate context with {"actualParam": "someValue"}
	// Inside RegisterGenericRoute, GetParam(req, "missingKey") will be called.
	// Since "missingKey" is not in the context, GetParam returns "".
	r.ServeHTTP(rr, req)

	// Check the result
	assert.Equal(t, http.StatusBadRequest, rr.Code, "Expected status Bad Request")
	// The error message should refer to the SourceKey we were looking for
	require.Contains(t, rr.Body.String(), "Missing required path parameter: missingKey")
}

// Test case for scenario 3: Failed to decode path parameter data
func TestRegisterGenericRoute_PathParamDecodeError(t *testing.T) {
	r := router.NewRouter[string, string](router.RouterConfig{}, nil, nil) // Use value receiver
	mockCodec := &mockErrorCodec{}                                         // Codec that forces DecodeBytes error

	routeConfig := router.RouteConfig[testRequest, testResponse]{
		// RouteConfigBase fields are embedded
		Methods:    []router.HttpMethod{router.MethodGet}, // Use router types
		Path:       "/test/:data",                         // Path expects a parameter
		Handler:    basicHandler,
		Codec:      mockCodec,
		SourceType: router.Base64PathParameter, // Test with Base64, Base62 is similar
		SourceKey:  "data",                     // Look for 'data' param
	}

	// Encode some valid base64 data, the error happens *after* base64 decoding in DecodeBytes
	validBase64 := base64.StdEncoding.EncodeToString([]byte("trigger decode error"))
	targetURL := "/test/" + validBase64 // Path with the parameter value

	// Use r.ServeHTTP for this test as it involves path params processed by httprouter
	req := httptest.NewRequest("GET", targetURL, nil)
	rr := httptest.NewRecorder()
	router.RegisterGenericRoute(r, routeConfig, 0, 0, nil) // Register the route
	r.ServeHTTP(rr, req)                                   // Serve the request

	assert.Equal(t, http.StatusBadRequest, rr.Code, "Expected status Bad Request")
	// Optionally check response body for specific message
	// require.Contains(t, rr.Body.String(), "Failed to decode path parameter data")
}
