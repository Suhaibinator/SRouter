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

> **Body-size safety:** Every route whose `SourceType` is `Body` (the default)
> must inherit or set a positive `MaxBodySize`. The built-in JSON and protobuf
> codecs rely on this router boundary while consuming the request stream. A
> timeout does not cap the number of bytes read.

## Built-in Codecs

SRouter typically provides codecs for common formats in the `pkg/codec` package:

### `codec.JSONCodec`

Handles JSON encoding and decoding using Go's `encoding/json/v2` package. The
v2 defaults reject duplicate object names and invalid UTF-8, and match struct
field names case-sensitively.

```go
import (
    json "encoding/json/v2"

    "github.com/Suhaibinator/SRouter/pkg/codec"
)

// Create a new JSON codec for specific request/response types
jsonCodec := codec.NewJSONCodec[MyRequest, MyResponse]()

// Optional encoding/json/v2 options apply to both encoding and decoding.
strictJSONCodec := codec.NewJSONCodec[MyRequest, MyResponse](
    json.RejectUnknownMembers(true),
)

// Use it in RouteConfig
route := router.RouteConfig[MyRequest, MyResponse]{
    // ... Path, Methods, Handler ...
    Codec: jsonCodec,
}
r.MaxBodySize(1 << 20).Route(route) // 1 MiB
```

### `codec.ProtoCodec`

Handles Protocol Buffers encoding and decoding using Google's `protobuf` libraries (e.g., `google.golang.org/protobuf/proto`).

**Important:** `ProtoCodec` infers the concrete request message type from `T`, so it can allocate fresh zero-value messages for unmarshaling without reflection.

```go
import (
	"github.com/Suhaibinator/SRouter/pkg/codec"
	pb "path/to/your/generated/proto/package" // Import your generated proto package
)

// Create a new Proto codec.
// T is *pb.MyRequestProto, U is *pb.MyResponseProto (or appropriate response type)
protoCodec := codec.NewProtoCodec[*pb.MyRequestProto, *pb.MyResponseProto]()


// Use it in RouteConfig
route := router.RouteConfig[*pb.MyRequestProto, *pb.MyResponseProto]{
    // ... Path, Methods, Handler ...
    Codec: protoCodec,
}
r.MaxBodySize(1 << 20).Route(route) // 1 MiB
```

## Creating Custom Codecs

You can implement support for other formats (e.g., XML, MessagePack, YAML) by creating your own struct that implements the `codec.Codec[T, U]` interface.

```go
package customcodec

import (
	"encoding/xml"
	"fmt"
	"io"
	"net/http"

	"github.com/Suhaibinator/SRouter/pkg/codec"
	"github.com/Suhaibinator/SRouter/pkg/router"
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

var _ codec.Codec[MyXMLRequest, MyXMLResponse] =
	(*XMLCodec[MyXMLRequest, MyXMLResponse])(nil)

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
	var data T
	defer func() { _ = r.Body.Close() }()

	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		// Preserve *http.MaxBytesError so SRouter can return HTTP 413.
		return data, fmt.Errorf("read XML request body: %w", err)
	}

	if err := xml.Unmarshal(bodyBytes, &data); err != nil {
		return data, router.NewHTTPErrorWithCause(
			http.StatusBadRequest,
			"invalid XML request",
			err,
		)
	}
	return data, nil
}

// DecodeBytes unmarshals XML from a byte slice.
func (c *XMLCodec[T, U]) DecodeBytes(dataBytes []byte) (T, error) {
	var data T
	if err := xml.Unmarshal(dataBytes, &data); err != nil {
		return data, router.NewHTTPErrorWithCause(
			http.StatusBadRequest,
			"invalid XML request",
			err,
		)
	}
	return data, nil
}

// Encode marshals the response to XML and writes it to the response writer.
func (c *XMLCodec[T, U]) Encode(w http.ResponseWriter, resp U) error {
	xmlBytes, err := xml.MarshalIndent(resp, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal XML response: %w", err)
	}

	w.Header().Set("Content-Type", "application/xml; charset=utf-8")
	_, err = w.Write(xmlBytes)
	if err != nil {
		return fmt.Errorf("write XML response: %w", err)
	}
	return nil
}

// --- Usage ---
// xmlCodec := customcodec.NewXMLCodec[customcodec.MyXMLRequest, customcodec.MyXMLResponse]()
// route := router.RouteConfig[customcodec.MyXMLRequest, customcodec.MyXMLResponse]{
//     Path:      "/xml",
//     Methods:   []router.HttpMethod{router.MethodPost},
//     Codec:     xmlCodec,
//     // Handler: ...
// }
// r.MaxBodySize(1 << 20).Route(route) // 1 MiB
```

Whole-body codecs such as this one must run behind a positive effective
`MaxBodySize` so `http.MaxBytesReader` rejects oversized input before `io.ReadAll`
can allocate it. Set the limit globally, on the containing group, or in the route
override as shown above. A request timeout does not replace this byte limit. If a
codec can be called outside SRouter, add an equivalent limit inside that codec or
use a streaming decoder.

Return `router.HTTPError` for safe client-facing decode failures and wrap the
underlying cause with `NewHTTPErrorWithCause` when it is useful for logs. Return
ordinary errors for encoding or other server failures. Do not write an error
response and then return an error: the router handles a returned error, and doing
both can produce duplicate writes.

## Codec Reference

-   **`codec.Codec[T, U]`**: Interface defining methods `NewRequest() T`, `Decode(*http.Request) (T, error)`, `DecodeBytes([]byte) (T, error)`, and `Encode(http.ResponseWriter, U) error`.
-   **`codec.NewJSONCodec[T, U](...json.Options) *codec.JSONCodec[T, U]`**: Constructor for the built-in JSON v2 codec.
-   **`codec.NewProtoCodec[T, U]() *codec.ProtoCodec[T, U]`**: Constructor for the built-in Protocol Buffers codec.

See `examples/codec` for the built-in codecs and `examples/custom-codec` for a
runnable, from-scratch “Rune Scroll” codec that demonstrates all four interface
methods with both body and query-parameter request sources.
