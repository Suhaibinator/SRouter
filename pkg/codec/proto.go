// Package codec provides encoding and decoding functionality for different data formats.
package codec

import (
	"io"
	"net/http"

	"google.golang.org/protobuf/proto"
)

// ProtoRequestFactory is a function type that creates a new instance of the request type T.
// T must be a pointer to a type implementing proto.Message.
type ProtoRequestFactory[T proto.Message] func() T

// Force T to be a pointer to a type implementing proto.Message.
type ProtoCodec[T proto.Message, U proto.Message] struct {
	// Factory function to create new request objects without reflection.
	newRequest ProtoRequestFactory[T]
}

// NewProtoCodec creates a new ProtoCodec instance with the provided request factory.
func NewProtoCodec[T proto.Message, U proto.Message](factory ProtoRequestFactory[T]) *ProtoCodec[T, U] {
	return &ProtoCodec[T, U]{
		newRequest: factory,
	}
}

// For testing purposes, we expose these variables so they can be overridden in tests
var protoUnmarshal = proto.Unmarshal
var protoMarshal = proto.Marshal

// NewRequest creates a new zero-value instance of the request type T using the factory.
func (c *ProtoCodec[T, U]) NewRequest() T {
	return c.newRequest()
}

// Decode reads the request body and unmarshals into T (which is a pointer).
func (c *ProtoCodec[T, U]) Decode(r *http.Request) (T, error) {
	msg := c.NewRequest() // Use the factory to create a new message instance

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

// DecodeBytes unmarshals a byte slice into T (which is a pointer).
func (c *ProtoCodec[T, U]) DecodeBytes(data []byte) (T, error) {
	msg := c.NewRequest() // Use the factory to create a new message instance

	// Unmarshal directly from the provided data
	if err := protoUnmarshal(data, msg); err != nil {
		var zero T
		return zero, err
	}
	return msg, nil
}

// Encode marshals U (also a pointer type) and writes to response.
func (c *ProtoCodec[T, U]) Encode(w http.ResponseWriter, resp U) error {
	w.Header().Set("Content-Type", "application/x-protobuf")
	bytes, err := protoMarshal(resp)
	if err != nil {
		return err
	}
	_, err = w.Write(bytes)
	return err
}

// newMessage function is removed as it's replaced by the factory approach.
