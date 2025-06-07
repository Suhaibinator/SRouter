# Source Types

SRouter's generic routes offer flexibility in how the request data (`T` in `RouteConfig[T, U]`) is retrieved and decoded. By default, it reads from the request body, but you can configure it to read from query or path parameters, especially useful for GET requests or when request bodies are restricted.

This is controlled by the `SourceType` and `SourceKey` fields in the `RouteConfig[T, U]` struct.

## Available Source Types

SRouter defines constants for the available source types in the `router` package:

1.  **`router.Body`** (Default):
    *   Retrieves data directly from the `http.Request.Body`.
    *   `SourceKey` is ignored.
    *   The configured `Codec`'s `Decode` method is used.
    *   Example: Standard POST/PUT requests with JSON/Proto payloads.

    ```go
    router.RouteConfig[MyRequest, MyResponse]{
        // ... Path, Methods, Handler ...
        Codec: codec.NewJSONCodec[MyRequest, MyResponse](),
        // SourceType defaults to Body if omitted
    }
    ```

2.  **`router.Base64QueryParameter`**:
    *   Retrieves data from a **Base64-encoded** string in a query parameter.
    *   `SourceKey` specifies the name of the query parameter (e.g., `data` for `?data=...`).
    *   The value is Base64-decoded, and the resulting bytes are passed to the `Codec`'s `DecodeBytes` method.
    *   Example: Sending complex data via GET requests where the data is encoded to fit in the URL.

    ```go
    router.RouteConfig[MyRequest, MyResponse]{
        Path:       "/data/from/query",
        Methods:    []router.HttpMethod{router.MethodGet},
        Handler:    MyHandler,
        Codec:      codec.NewJSONCodec[MyRequest, MyResponse](), // Codec still needed for DecodeBytes
        SourceType: router.Base64QueryParameter,
        SourceKey:  "payload", // Expects URL like /data/from/query?payload=BASE64STRING
    }
    ```

3.  **`router.Base62QueryParameter`**:
    *   Similar to `Base64QueryParameter`, but uses **Base62 encoding**. Base62 is URL-safe without padding characters, potentially producing shorter strings than Base64.
    *   Retrieves data from a Base62-encoded string in a query parameter specified by `SourceKey`.
    *   The value is Base62-decoded, and the bytes are passed to the `Codec`'s `DecodeBytes` method.

    ```go
    router.RouteConfig[MyRequest, MyResponse]{
        // ... Path, Methods, Handler, Codec ...
        SourceType: router.Base62QueryParameter,
        SourceKey:  "q", // Expects URL like /path?q=BASE62STRING
    }
    ```

4.  **`router.Base64PathParameter`**:
    *   Retrieves data from a **Base64-encoded** string in a named path parameter.
    *   The route's `Path` must include a corresponding named parameter (e.g., `/:data`).
    *   `SourceKey` specifies the name of the path parameter (e.g., `data`).
    *   If `SourceKey` is empty, the first path parameter in the request URL is used.
    *   The parameter value is Base64-decoded, and the bytes are passed to the `Codec`'s `DecodeBytes` method.

    ```go
    router.RouteConfig[MyRequest, MyResponse]{
        Path:       "/data/from/path/:payload", // Define path parameter
        Methods:    []router.HttpMethod{router.MethodGet},
        Handler:    MyHandler,
        Codec:      codec.NewJSONCodec[MyRequest, MyResponse](),
        SourceType: router.Base64PathParameter,
        SourceKey:  "payload", // Matches the :payload name in the Path
    }
    ```

5.  **`router.Base62PathParameter`**:
    *   Similar to `Base64PathParameter`, but uses **Base62 encoding**.
    *   Retrieves data from a Base62-encoded string in a named path parameter specified by `SourceKey`.
    *   If `SourceKey` is empty, the first path parameter in the request URL is used.
    *   The parameter value is Base62-decoded, and the bytes are passed to the `Codec`'s `DecodeBytes` method.

    ```go
    router.RouteConfig[MyRequest, MyResponse]{
        Path:       "/data/b62/:p", // Define path parameter
        Methods:    []router.HttpMethod{router.MethodGet},
        Handler:    MyHandler,
        Codec:      codec.NewJSONCodec[MyRequest, MyResponse](),
        SourceType: router.Base62PathParameter,
        SourceKey:  "p", // Matches the :p name in the Path
    }
    ```


6.  **`router.Empty`**:
    *   No request decoding is performed. The handler receives the zero value of the request type.
    *   Useful for endpoints that do not accept input but still use generic handlers.

## Codec Requirement

Even when using query or path parameter source types, a `Codec` is still required in the `RouteConfig`. This is because the router needs the codec's `DecodeBytes` method to unmarshal the decoded byte slice (`[]byte`) into the target request type `T`.

```go
// Codec interface likely includes:
type Codec[T any, U any] interface {
    // ... Decode(r *http.Request) (T, error) ...
    DecodeBytes(data []byte) (T, error) // Used by non-Body source types
    // ... Encode(w http.ResponseWriter, resp U) error ...
    // ... NewRequest() T ...
}
```

Ensure your chosen codec implements `DecodeBytes` correctly for the data format you expect (e.g., JSON, Proto).

See the `examples/source-types` directory for a runnable example demonstrating different source types.
