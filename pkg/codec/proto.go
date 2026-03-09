// Package codec provides encoding and decoding functionality for different data formats.
package codec

import (
	"io"
	"net/http"

	"google.golang.org/protobuf/proto"
)

// ProtoCodec implements the Codec interface for Protocol Buffers.
// It handles marshaling and unmarshaling of protobuf messages for use with generic routes.
// Both T and U must be pointer types that implement proto.Message (e.g., *MyRequest, *MyResponse).
type ProtoCodec[T proto.Message, U proto.Message] struct {
	newRequest func() T
}

// NewProtoCodec creates a new ProtoCodec instance for protobuf request/response types.
// It infers the underlying message type from T and allocates fresh zero-value messages
// without reflection by using Go's new(expr) support.
//
// Example:
//
//	codec := NewProtoCodec[*pb.CreateUserReq, *pb.CreateUserResp]()
func NewProtoCodec[T interface {
	proto.Message
	*M
}, U proto.Message, M any]() *ProtoCodec[T, U] {

	var zero M
	return &ProtoCodec[T, U]{
		newRequest: func() T {
			return new(zero)
		},
	}
}

// For testing purposes, we expose these variables so they can be overridden in tests
var protoUnmarshal = proto.Unmarshal
var protoMarshal = proto.Marshal

// NewRequest creates a new instance of the request protobuf message.
// It implements the Codec interface.
func (c *ProtoCodec[T, U]) NewRequest() T {
	return c.newRequest()
}

// Decode reads and unmarshals protobuf data from the HTTP request body into type T.
// It implements the Codec interface. The entire request body is read and the
// body is closed after reading. Returns an error if the data is not valid protobuf
// or doesn't match the expected message type.
func (c *ProtoCodec[T, U]) Decode(r *http.Request) (T, error) {
	msg := c.NewRequest()

	body, err := io.ReadAll(r.Body)
	if err != nil {
		var zero T
		return zero, err
	}
	defer r.Body.Close()

	if err := protoUnmarshal(body, msg); err != nil {
		var zero T
		return zero, err
	}
	return msg, nil
}

// DecodeBytes unmarshals protobuf data from a byte slice into type T.
// It implements the Codec interface. This method is used when the request
// data comes from sources other than the request body (e.g., base64-encoded
// query parameters). Returns an error if the data is invalid.
func (c *ProtoCodec[T, U]) DecodeBytes(data []byte) (T, error) {
	msg := c.NewRequest()

	// Unmarshal directly from the provided data
	if err := protoUnmarshal(data, msg); err != nil {
		var zero T
		return zero, err
	}
	return msg, nil
}

// Encode marshals the response protobuf message to binary format and writes it to the HTTP response.
// It implements the Codec interface. Sets the Content-Type header to "application/x-protobuf"
// before writing the response body. Returns an error if marshaling fails or if
// writing to the response writer fails.
func (c *ProtoCodec[T, U]) Encode(w http.ResponseWriter, resp U) error {
	w.Header().Set("Content-Type", "application/x-protobuf")
	bytes, err := protoMarshal(resp)
	if err != nil {
		return err
	}
	_, err = w.Write(bytes)
	return err
}
