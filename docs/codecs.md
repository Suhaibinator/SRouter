# Custom Codecs

Codecs are responsible for encoding and decoding data in SRouter's generic routes. They handle the serialization and deserialization between Go types (`T` for request, `U` for response) and the format used for transmission (like JSON, Protocol Buffers, XML, etc.).

## The Codec Interface

Any codec used with SRouter's generic routes must implement the `codec.Codec[T, U]` interface defined in `pkg/codec/codec.go`:

```go
package codec

import "net/http"

// Codec defines the interface for encoding and decoding request/response data
// for generic routes. T is the request type, U is the response type.
type Codec[T any, U any] interface {
	// NewRequest creates a new zero-value instance of the request type T.
	// This is crucial for decoding into a concrete type, especially for formats
	// like Protocol Buffers that might require it.
	NewRequest() T

	// Decode extracts and deserializes data from an HTTP request into a value of type T.
	// Typically reads from r.Body. Used when SourceType is Body.
	Decode(r *http.Request) (T, error)

	// DecodeBytes extracts and deserializes data from a byte slice into a value of type T.
	// Used when SourceType is one of the query or path parameter types.
	DecodeBytes(data []byte) (T, error)

	// Encode serializes a value of type U (the response object from the handler)
	// and writes it to the HTTP response writer. It should also set appropriate
	// headers, like Content-Type.
	Encode(w http.ResponseWriter, resp U) error
}

```

## Built-in Codecs

SRouter typically provides codecs for common formats in the `pkg/codec` package:

### `codec.JSONCodec`

Handles standard JSON encoding and decoding using Go's `encoding/json` package.

```go
import "github.com/Suhaibinator/SRouter/pkg/codec"

// Create a new JSON codec for specific request/response types
jsonCodec := codec.NewJSONCodec[MyRequest, MyResponse]()

// Use it in RouteConfig
route := router.RouteConfig[MyRequest, MyResponse]{
    // ... Path, Methods, Handler ...
    Codec: jsonCodec,
}
```

### `codec.ProtoCodec`

Handles Protocol Buffers encoding and decoding using Google's `protobuf` libraries (e.g., `google.golang.org/protobuf/proto`).

**Important:** Due to the nature of Protocol Buffers, the `ProtoCodec` requires a factory function (`codec.ProtoRequestFactory`) when being constructed. This function must return a new, zero-value instance of the specific *request* proto message type (`T`). This is needed internally to provide a concrete type for unmarshaling without relying on reflection.

```go
import (
	"github.com/Suhaibinator/SRouter/pkg/codec"
	pb "path/to/your/generated/proto/package" // Import your generated proto package
)

// Define the factory function for your request proto message type
// It must return the specific pointer type (e.g., *pb.MyRequestProto)
var myRequestFactory = func() *pb.MyRequestProto {
	return &pb.MyRequestProto{}
}

// Create a new Proto codec, providing the factory function
// T is *pb.MyRequestProto, U is *pb.MyResponseProto (or appropriate response type)
protoCodec := codec.NewProtoCodec[*pb.MyRequestProto, *pb.MyResponseProto](myRequestFactory)


// Use it in RouteConfig
route := router.RouteConfig[*pb.MyRequestProto, *pb.MyResponseProto]{
    // ... Path, Methods, Handler ...
    Codec: protoCodec,
}
```

## Creating Custom Codecs

You can implement support for other formats (e.g., XML, MessagePack, YAML) by creating your own struct that implements the `codec.Codec[T, U]` interface.

```go
package customcodec

import (
        "encoding/xml"
        "io"
        "net/http"
        "github.com/Suhaibinator/SRouter/pkg/codec"  // For Codec interface
        "github.com/Suhaibinator/SRouter/pkg/router" // For NewHTTPError
)

// Define your request and response types if not already defined
type MyXMLRequest struct {
	XMLName xml.Name `xml:"request"`
	Value   string   `xml:"value"`
}

type MyXMLResponse struct {
	XMLName xml.Name `xml:"response"`
	Result  string   `xml:"result"`
}

// XMLCodec implements the codec.Codec interface for XML
type XMLCodec[T any, U any] struct{}

// NewXMLCodec creates a new XMLCodec instance.
// Note: Unlike ProtoCodec, we don't need a factory here if T is a struct type,
// as 'var data T' works. If T were an interface, a factory might be needed.
func NewXMLCodec[T any, U any]() *XMLCodec[T, U] {
	return &XMLCodec[T, U]{}
}

// NewRequest creates a zero-value instance of the request type T.
func (c *XMLCodec[T, U]) NewRequest() T {
	var data T
	return data
}

// Decode reads from the request body and unmarshals XML.
func (c *XMLCodec[T, U]) Decode(r *http.Request) (T, error) {
	var data T // Create a zero value of type T

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		return data, router.NewHTTPError(http.StatusInternalServerError, "Failed to read request body")
	}
	defer r.Body.Close()

	if err := xml.Unmarshal(bodyBytes, &data); err != nil {
		return data, router.NewHTTPError(http.StatusBadRequest, "Failed to unmarshal XML request: "+err.Error())
	}
	return data, nil
}

// DecodeBytes unmarshals XML from a byte slice.
func (c *XMLCodec[T, U]) DecodeBytes(dataBytes []byte) (T, error) {
	var data T // Create a zero value of type T
	if err := xml.Unmarshal(dataBytes, &data); err != nil {
		// Consider what error type is appropriate here - depends if the source was client input
		return data, router.NewHTTPError(http.StatusBadRequest, "Failed to unmarshal XML from bytes: "+err.Error())
	}
	return data, nil
}

// Encode marshals the response to XML and writes it to the response writer.
func (c *XMLCodec[T, U]) Encode(w http.ResponseWriter, resp U) error {
	xmlBytes, err := xml.MarshalIndent(resp, "", "  ") // Use MarshalIndent for readability
	if err != nil {
		// Log the internal error
		// logger.Error("Failed to marshal XML response", zap.Error(err))
		// Return an internal server error to the client
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		// Return the original error so SRouter knows encoding failed
		return err
	}

	// Set Content-Type header
	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	w.WriteHeader(http.StatusOK) // Or appropriate status code if needed
	_, err = w.Write(xmlBytes)
	if err != nil {
		// Log error during write
		// logger.Error("Failed to write XML response", zap.Error(err))
		return err // Return error
	}
	return nil // Success
}

// --- Usage ---
// xmlCodec := customcodec.NewXMLCodec[customcodec.MyXMLRequest, customcodec.MyXMLResponse]()
// route := router.RouteConfig[customcodec.MyXMLRequest, customcodec.MyXMLResponse]{
//     // ...
//     Codec: xmlCodec,
//     // ...
// }
```

Remember to handle errors appropriately within your codec methods, potentially returning `router.HTTPError` for client-side issues (like bad input formatting) or standard errors for server-side issues (which SRouter will likely turn into a 500 response).

## Codec Reference

-   **`codec.Codec[T, U]`**: Interface defining methods `NewRequest() T`, `Decode(*http.Request) (T, error)`, `DecodeBytes([]byte) (T, error)`, and `Encode(http.ResponseWriter, U) error`.
-   **`codec.NewJSONCodec[T, U]() *codec.JSONCodec[T, U]`**: Constructor for the built-in JSON codec.
-   **`codec.NewProtoCodec[T, U](factory codec.ProtoRequestFactory[T]) *codec.ProtoCodec[T, U]`**: Constructor for the built-in Protocol Buffers codec, requiring a factory function for the request type `T`.

See the `examples/codec` directory for runnable examples using different codecs.
