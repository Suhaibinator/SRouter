package codec

import "net/http"

// Codec defines an interface for marshaling and unmarshaling request and response data.
// It provides methods for creating new request objects, decoding request data, and encoding
// response data. This allows for different data formats (e.g., JSON, Protocol Buffers).
// The framework includes implementations for JSON and Protocol Buffers in the codec package.
type Codec[T any, U any] interface {
	// NewRequest creates a new zero-value instance of the request type T.
	// This is used by the framework to get an instance for decoding, avoiding reflection.
	NewRequest() T

	// Decode extracts and deserializes data from an HTTP request body into a value of type T.
	// The type T represents the request data type.
	Decode(r *http.Request) (T, error)

	// DecodeBytes extracts and deserializes data from a byte slice into a value of type T.
	// This is used for source types where the data is already extracted (e.g., query/path parameters).
	// The type T represents the request data type.
	DecodeBytes(data []byte) (T, error)

	// Encode serializes a value of type U and writes it to the HTTP response.
	// It converts the Go value to the wire format (e.g., JSON, Protocol Buffers) and
	// sets appropriate headers (e.g., Content-Type). If the serialization fails, it returns an error.
	// The type U represents the response data type.
	Encode(w http.ResponseWriter, resp U) error
}
