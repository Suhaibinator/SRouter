package codec

import (
	"bytes"
	"errors"
	"net/http/httptest"
	"testing"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/reflect/protoreflect"
)

// TestProtoMessage is a minimal implementation of proto.Message for testing
type TestProtoMessage struct {
	Data []byte
}

// Implement the proto.Message interface
func (m *TestProtoMessage) Reset()                             { *m = TestProtoMessage{} }
func (m *TestProtoMessage) String() string                     { return string(m.Data) }
func (m *TestProtoMessage) ProtoMessage()                      {}
func (m *TestProtoMessage) ProtoReflect() protoreflect.Message { return nil }

// TestNewProtoCodec tests the NewProtoCodec function
func TestNewProtoCodec(t *testing.T) {
	codec := NewProtoCodec[*TestProtoMessage, *TestProtoMessage]()
	if codec == nil {
		t.Error("Expected non-nil codec")
	}
}

// TestProtoCodecDecode tests the Decode method of ProtoCodec
func TestProtoCodecDecode(t *testing.T) {
	// Create a codec
	codec := NewProtoCodec[*TestProtoMessage, *TestProtoMessage]()

	// Create a request with test data
	reqBody := []byte("test data")
	req := httptest.NewRequest("POST", "/test", bytes.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/x-protobuf")

	// Create a mock implementation of proto.Unmarshal
	originalUnmarshal := protoUnmarshal
	defer func() { protoUnmarshal = originalUnmarshal }()

	unmarshalCalled := false
	protoUnmarshal = func(b []byte, m proto.Message) error {
		unmarshalCalled = true
		if msg, ok := m.(*TestProtoMessage); ok {
			msg.Data = b
			return nil
		}
		return errors.New("not a TestProtoMessage")
	}

	// Decode the request
	decoded, err := codec.Decode(req)
	if err != nil {
		t.Fatalf("Decode() returned error: %v", err)
	}

	// Verify Unmarshal was called
	if !unmarshalCalled {
		t.Error("Expected Unmarshal to be called")
	}

	// Verify the decoded data
	if string(decoded.Data) != "test data" {
		t.Errorf("Decode() got Data = %q, want %q", string(decoded.Data), "test data")
	}
}

// TestProtoCodecEncode tests the Encode method of ProtoCodec
func TestProtoCodecEncode(t *testing.T) {
	// Create a codec
	codec := NewProtoCodec[*TestProtoMessage, *TestProtoMessage]()

	// Create a mock implementation of proto.Marshal
	originalMarshal := protoMarshal
	defer func() { protoMarshal = originalMarshal }()

	marshalCalled := false
	protoMarshal = func(m proto.Message) ([]byte, error) {
		marshalCalled = true
		if msg, ok := m.(*TestProtoMessage); ok {
			return msg.Data, nil
		}
		return nil, errors.New("not a TestProtoMessage")
	}

	// Create a response
	resp := &TestProtoMessage{Data: []byte("response data")}
	rr := httptest.NewRecorder()

	// Encode the response
	err := codec.Encode(rr, resp)
	if err != nil {
		t.Fatalf("Encode() returned error: %v", err)
	}

	// Verify Marshal was called
	if !marshalCalled {
		t.Error("Expected Marshal to be called")
	}

	// Verify the Content-Type header
	if got := rr.Header().Get("Content-Type"); got != "application/x-protobuf" {
		t.Errorf("Content-Type = %q, want %q", got, "application/x-protobuf")
	}

	// Verify the response body
	if got := rr.Body.String(); got != "response data" {
		t.Errorf("Encode() response body = %q, want %q", got, "response data")
	}
}

// TestProtoCodecDecodeErrors tests error handling in the Decode method of ProtoCodec
func TestProtoCodecDecodeErrors(t *testing.T) {
	// Create a codec
	codec := NewProtoCodec[*TestProtoMessage, *TestProtoMessage]()

	// Test Decode with read error
	req := httptest.NewRequest("POST", "/test", &errorReader{})
	req.Header.Set("Content-Type", "application/x-protobuf")

	_, err := codec.Decode(req)
	if err == nil {
		t.Error("Expected error when reading body fails")
	}

	// Test Decode with unmarshal error
	req = httptest.NewRequest("POST", "/test", bytes.NewReader([]byte("test data")))
	req.Header.Set("Content-Type", "application/x-protobuf")

	// Create a mock implementation of proto.Unmarshal that returns an error
	originalUnmarshal := protoUnmarshal
	defer func() { protoUnmarshal = originalUnmarshal }()

	protoUnmarshal = func([]byte, proto.Message) error {
		return errors.New("mock unmarshal error")
	}

	_, err = codec.Decode(req)
	if err == nil {
		t.Error("Expected error when unmarshaling fails")
	}
}

// TestProtoCodecEncodeErrors tests error handling in the Encode method of ProtoCodec
func TestProtoCodecEncodeErrors(t *testing.T) {
	// Create a codec
	codec := NewProtoCodec[*TestProtoMessage, *TestProtoMessage]()

	// Test Encode with marshal error
	rr := httptest.NewRecorder()

	// Create a mock implementation of proto.Marshal that returns an error
	originalMarshal := protoMarshal
	defer func() { protoMarshal = originalMarshal }()

	protoMarshal = func(proto.Message) ([]byte, error) {
		return nil, errors.New("mock marshal error")
	}

	err := codec.Encode(rr, &TestProtoMessage{})
	if err == nil {
		t.Error("Expected error when marshaling fails")
	}

	// Test Encode with write error
	// Restore the original proto.Marshal function
	protoMarshal = originalMarshal

	// Create a mock implementation of proto.Marshal that returns test data
	protoMarshal = func(proto.Message) ([]byte, error) {
		return []byte("test data"), nil
	}

	err = codec.Encode(&errorResponseWriter{}, &TestProtoMessage{})
	if err == nil {
		t.Error("Expected error when writing response fails")
	}
}

// TestNewMessage tests the newMessage helper function
func TestNewMessage(t *testing.T) {
	// Test creating a new instance
	msg := newMessage[*TestProtoMessage]()

	// Verify the instance is properly initialized
	if msg == nil {
		t.Error("Expected non-nil message")
	}
}
