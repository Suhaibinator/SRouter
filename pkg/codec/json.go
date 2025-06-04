// Package codec provides encoding and decoding functionality for different data formats.
package codec

import (
	"encoding/json"
	"io"
	"net/http"
)

// JSONCodec is a codec that uses JSON for marshaling and unmarshaling.
// It implements the Codec interface for encoding responses and decoding requests.
type JSONCodec[T any, U any] struct {
	// Optional configuration for JSON encoding/decoding
	// For example, custom field naming strategies, etc.
}

// NewRequest creates a new zero-value instance of the request type T.
// This method is required by the Codec interface and is used internally
// by the framework to get an instance for decoding without using reflection.
func (c *JSONCodec[T, U]) NewRequest() T {
	var data T
	return data
}

// Decode reads and unmarshals JSON data from the HTTP request body into type T.
// It implements the Codec interface. The entire request body is read and the
// body is closed after reading. If the JSON is malformed or doesn't match
// the structure of T, an error is returned along with the zero value of T.
func (c *JSONCodec[T, U]) Decode(r *http.Request) (T, error) {
	data := c.NewRequest() // Use NewRequest to get an instance

	// Read the request body
	body, err := io.ReadAll(r.Body)
	if err != nil {
		return data, err
	}
	defer r.Body.Close()

	// Unmarshal the JSON
	// We need to unmarshal into a pointer for JSON, even if T is not a pointer type itself.
	// If T is already a pointer, &data will be **Type, which json.Unmarshal handles.
	// If T is a struct, &data will be *Type.
	err = json.Unmarshal(body, &data)
	if err != nil {
		// Return the zero value of T in case of error
		var zero T
		return zero, err
	}

	return data, nil
}

// DecodeBytes unmarshals JSON data from a byte slice into type T.
// It implements the Codec interface. This method is used when the request
// data comes from sources other than the request body (e.g., base64-encoded
// query parameters). Returns an error if the JSON is invalid.
func (c *JSONCodec[T, U]) DecodeBytes(body []byte) (T, error) {
	data := c.NewRequest() // Use NewRequest to get an instance

	// Unmarshal the JSON
	err := json.Unmarshal(body, &data)
	if err != nil {
		// Return the zero value of T in case of error
		var zero T
		return zero, err
	}

	return data, nil
}

// Encode marshals the response value of type U to JSON and writes it to the HTTP response.
// It implements the Codec interface. Sets the Content-Type header to "application/json"
// before writing the response body. Returns an error if marshaling fails or if
// writing to the response writer fails.
func (c *JSONCodec[T, U]) Encode(w http.ResponseWriter, resp U) error {
	// Set the content type
	w.Header().Set("Content-Type", "application/json")

	// Marshal the response
	body, err := json.Marshal(resp)
	if err != nil {
		return err
	}

	// Write the response
	_, err = w.Write(body)
	return err
}

// NewJSONCodec creates a new JSONCodec instance for the specified types.
// T represents the request type and U represents the response type.
// The returned codec can be used with generic routes to automatically
// handle JSON marshaling and unmarshaling of request and response data.
//
// Example:
//   codec := NewJSONCodec[CreateUserReq, CreateUserResp]()
func NewJSONCodec[T any, U any]() *JSONCodec[T, U] {
	return &JSONCodec[T, U]{}
}
