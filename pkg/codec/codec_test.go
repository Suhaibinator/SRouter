package codec

import (
	"bytes"
	json "encoding/json/v2"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// TestJSONCodec tests the JSONCodec
func TestJSONCodec(t *testing.T) {
	// Define test types
	type TestRequest struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	type TestResponse struct {
		Greeting string `json:"greeting"`
		Age      int    `json:"age"`
	}

	// Create a codec
	codec := NewJSONCodec[TestRequest, TestResponse]()

	// Test Decode
	reqBody := `{"name":"John","age":30}`
	req := httptest.NewRequest("POST", "/test", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	// Decode the request
	data, err := codec.Decode(req)
	if err != nil {
		t.Fatalf("Failed to decode request: %v", err)
	}

	// Check the decoded data
	if data.Name != "John" {
		t.Errorf("Expected name to be %q, got %q", "John", data.Name)
	}
	if data.Age != 30 {
		t.Errorf("Expected age to be %d, got %d", 30, data.Age)
	}

	// Test Encode
	resp := TestResponse{
		Greeting: "Hello, John!",
		Age:      30,
	}
	rr := httptest.NewRecorder()

	// Encode the response
	err = codec.Encode(rr, resp)
	if err != nil {
		t.Fatalf("Failed to encode response: %v", err)
	}

	// Check the encoded response
	if rr.Header().Get("Content-Type") != "application/json" {
		t.Errorf("Expected Content-Type to be %q, got %q", "application/json", rr.Header().Get("Content-Type"))
	}

	// Decode the response body
	var decodedResp TestResponse
	err = json.UnmarshalRead(rr.Body, &decodedResp)
	if err != nil {
		t.Fatalf("Failed to decode response body: %v", err)
	}

	// Check the decoded response
	if decodedResp.Greeting != "Hello, John!" {
		t.Errorf("Expected greeting to be %q, got %q", "Hello, John!", decodedResp.Greeting)
	}
	if decodedResp.Age != 30 {
		t.Errorf("Expected age to be %d, got %d", 30, decodedResp.Age)
	}
}

func TestJSONCodecV2Defaults(t *testing.T) {
	type TestRequest struct {
		Name string `json:"name"`
	}

	codec := NewJSONCodec[TestRequest, struct{}]()
	tests := []struct {
		name string
		body []byte
	}{
		{
			name: "duplicate object name",
			body: []byte(`{"name":"first","name":"second"}`),
		},
		{
			name: "invalid UTF-8",
			body: []byte{'{', '"', 'n', 'a', 'm', 'e', '"', ':', '"', 0xff, '"', '}'},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if _, err := codec.DecodeBytes(tt.body); err == nil {
				t.Fatal("DecodeBytes() unexpectedly accepted invalid JSON")
			}

			req := httptest.NewRequest(http.MethodPost, "/test", bytes.NewReader(tt.body))
			if _, err := codec.Decode(req); err == nil {
				t.Fatal("Decode() unexpectedly accepted invalid JSON")
			}
		})
	}
}

func TestJSONCodecOptions(t *testing.T) {
	type TestRequest struct {
		Name string `json:"name"`
	}

	codec := NewJSONCodec[TestRequest, struct{}](
		json.MatchCaseInsensitiveNames(true),
		json.RejectUnknownMembers(true),
	)

	decoded, err := codec.DecodeBytes([]byte(`{"NAME":"John"}`))
	if err != nil {
		t.Fatalf("DecodeBytes() returned error: %v", err)
	}
	if decoded.Name != "John" {
		t.Fatalf("DecodeBytes() Name = %q, want %q", decoded.Name, "John")
	}

	if _, err := codec.DecodeBytes([]byte(`{"name":"John","extra":true}`)); err == nil {
		t.Fatal("DecodeBytes() unexpectedly accepted an unknown object member")
	}
}

// TestJSONCodecErrors tests error handling in the JSONCodec
func TestJSONCodecErrors(t *testing.T) {
	// Define test types
	type TestRequest struct {
		Name string `json:"name"`
		Age  int    `json:"age"`
	}
	type TestResponse struct {
		Greeting string `json:"greeting"`
		Age      int    `json:"age"`
	}

	// Create a codec
	codec := NewJSONCodec[TestRequest, TestResponse]()

	// Test Decode with invalid JSON
	reqBody := `{"name":"John","age":invalid}`
	req := httptest.NewRequest("POST", "/test", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")

	// Decode the request
	_, err := codec.Decode(req)
	if err == nil {
		t.Errorf("Expected error when decoding invalid JSON")
	}

	// Test Decode with empty body
	req = httptest.NewRequest("POST", "/test", strings.NewReader(""))
	req.Header.Set("Content-Type", "application/json")

	// Decode the request
	_, err = codec.Decode(req)
	if err == nil {
		t.Errorf("Expected error when decoding empty body")
	}

	// Test Decode with read error
	req = httptest.NewRequest("POST", "/test", &errorReader{})
	req.Header.Set("Content-Type", "application/json")

	// Decode the request
	_, err = codec.Decode(req)
	if err == nil {
		t.Errorf("Expected error when reading body fails")
	}

	// Test Encode with write error
	resp := TestResponse{
		Greeting: "Hello, John!",
		Age:      30,
	}
	rw := &errorResponseWriter{}

	// Encode the response
	err = codec.Encode(rw, resp)
	if err == nil {
		t.Errorf("Expected error when writing response fails")
	}

	// Test Encode with marshal error
	type UnmarshalableResponse struct {
		Channel chan int `json:"channel"` // channels cannot be marshaled to JSON
	}
	codec2 := NewJSONCodec[TestRequest, UnmarshalableResponse]()
	resp2 := UnmarshalableResponse{
		Channel: make(chan int),
	}
	rr := httptest.NewRecorder()

	// Encode the response
	err = codec2.Encode(rr, resp2)
	if err == nil {
		t.Errorf("Expected error when marshaling fails")
	}
}

// CustomProtoCodec is a simplified version of ProtoCodec for testing
type CustomProtoCodec struct{}

// Decode reads the request body and returns it as a MockProtoMessage
func (c *CustomProtoCodec) Decode(r *http.Request) (*MockProtoMessage, error) {
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	defer r.Body.Close()

	return &MockProtoMessage{Data: body}, nil
}

// Encode writes the MockProtoMessage's Data field to the response
func (c *CustomProtoCodec) Encode(w http.ResponseWriter, resp *MockProtoMessage) error {
	w.Header().Set("Content-Type", "application/x-protobuf")
	_, err := w.Write(resp.Data)
	return err
}

// MockProtoMessage is a simple struct for testing
type MockProtoMessage struct {
	Data []byte
}

// MockErrorReader is a reader that always returns an error
type MockErrorReader struct{}

func (r *MockErrorReader) Read(p []byte) (n int, err error) {
	return 0, errors.New("mock read error")
}

// TestCustomProtoCodec tests a simplified version of ProtoCodec for demonstration
func TestCustomProtoCodec(t *testing.T) {
	// Create a custom codec
	c := &CustomProtoCodec{}

	//
	// 1) Test Decode
	//
	incomingBytes := []byte("test data")
	req := httptest.NewRequest("POST", "/test", bytes.NewReader(incomingBytes))
	req.Header.Set("Content-Type", "application/x-protobuf")

	decoded, err := c.Decode(req)
	if err != nil {
		t.Fatalf("Decode() returned error: %v", err)
	}

	if string(decoded.Data) != "test data" {
		t.Errorf("Decode() got Data = %q, want %q", decoded.Data, "test data")
	}

	//
	// 2) Test Encode
	//
	resp := &MockProtoMessage{Data: []byte("response data")}
	rr := httptest.NewRecorder()

	err = c.Encode(rr, resp)
	if err != nil {
		t.Fatalf("Encode() returned error: %v", err)
	}

	// Check headers
	if got := rr.Header().Get("Content-Type"); got != "application/x-protobuf" {
		t.Errorf("Content-Type = %q, want %q", got, "application/x-protobuf")
	}

	// Check body
	if got := rr.Body.String(); got != "response data" {
		t.Errorf("Encode() response body = %q, want %q", got, "response data")
	}
}

// TestCustomProtoCodecErrors tests error handling in the CustomProtoCodec
func TestCustomProtoCodecErrors(t *testing.T) {
	// Create a custom codec
	c := &CustomProtoCodec{}

	// Test Decode with read error
	req := httptest.NewRequest("POST", "/test", &errorReader{})
	req.Header.Set("Content-Type", "application/x-protobuf")

	// Decode the request
	_, err := c.Decode(req)
	if err == nil {
		t.Errorf("Expected error when reading body fails")
	}

	// Test Encode with write error
	resp := &MockProtoMessage{Data: []byte("test data")}
	rw := &errorResponseWriter{}

	// Encode the response
	err = c.Encode(rw, resp)
	if err == nil {
		t.Errorf("Expected error when writing response fails")
	}
}

// errorReader is a reader that always returns an error
type errorReader struct{}

func (r *errorReader) Read(p []byte) (n int, err error) {
	return 0, io.ErrUnexpectedEOF
}

// errorResponseWriter is a response writer that always returns an error
type errorResponseWriter struct {
	http.ResponseWriter
}

func (w *errorResponseWriter) Write(p []byte) (n int, err error) {
	return 0, io.ErrUnexpectedEOF
}

func (w *errorResponseWriter) Header() http.Header {
	return http.Header{}
}

func (w *errorResponseWriter) WriteHeader(statusCode int) {
	// Do nothing
}
