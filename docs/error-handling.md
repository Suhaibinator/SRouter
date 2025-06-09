# Custom Error Handling

SRouter provides mechanisms for handling errors within your application, particularly within generic handlers, and translating them into appropriate HTTP responses.

## Returning Errors from Generic Handlers

Generic handlers (`GenericHandler[T, U]`) have the signature `func(*http.Request, T) (U, error)`. When a generic handler returns a non-nil `error`, SRouter's internal wrapper intercepts it and handles sending an appropriate HTTP error response back to the client.

-   **Standard Errors**: If the handler returns a standard Go error (e.g., `errors.New("something went wrong")`, `fmt.Errorf("details: %w", err)`), SRouter typically defaults to sending a `500 Internal Server Error` response. The specific error message returned by the handler is logged by the server (at Error level) but is usually *not* sent to the client for security reasons.

-   **`router.HTTPError`**: To send a specific HTTP status code and a custom message to the client, return an error of type `*router.HTTPError`.

-   **Error Context Storage**: When a generic handler returns an error, SRouter stores this error in the request context using `scontext.WithHandlerError`. This allows middleware to access the error after the handler has executed, which is useful for scenarios like transaction rollback. See [Context Management](./context-management.md) for details on accessing handler errors from middleware.

## `router.HTTPError`

This struct allows you to specify both the HTTP status code and the error message that should be sent to the client.

```go
// Defined in the router package (router.go)
type HTTPError struct {
    StatusCode int    // HTTP status code to send to the client
    Message    string // Error message that will appear in the response body
}

// Error makes HTTPError satisfy the error interface.
// It returns "<status>: <message>".
func (e *HTTPError) Error() string {
    return fmt.Sprintf("%d: %s", e.StatusCode, e.Message)
}

// NewHTTPError constructs an HTTPError instance.
func NewHTTPError(statusCode int, message string) *HTTPError {
    return &HTTPError{
        StatusCode: statusCode,
        Message:    message,
    }
}
```

### Using `HTTPError` in Handlers

```go
import (
    "fmt"
    "net/http"
    "github.com/Suhaibinator/SRouter/pkg/router"
)

// Assume GetUserReq and GetUserResp types exist

// Example generic handler
func GetUserHandler(r *http.Request, req GetUserReq) (GetUserResp, error) {
    userID := req.UserID // Assuming GetUserReq has UserID field

    if userID == "" {
        // Return a 400 Bad Request error
        return GetUserResp{}, router.NewHTTPError(http.StatusBadRequest, "User ID cannot be empty")
    }

    user, err := findUserByID(r.Context(), userID) // Assume this function exists
    if err != nil {
        // Log the internal error
        // logger.Error("Failed to find user", zap.String("user_id", userID), zap.Error(err))

        // Check if it's a specific "not found" error
        if errors.Is(err, ErrUserNotFound) { // Assume ErrUserNotFound exists
             // Return a 404 Not Found error to the client
            return GetUserResp{}, router.NewHTTPError(http.StatusNotFound, fmt.Sprintf("User with ID %s not found", userID))
        }

        // For other unexpected database errors, return a generic 500
        // The original error 'err' will be logged by SRouter's recovery/error handling mechanism.
        return GetUserResp{}, router.NewHTTPError(http.StatusInternalServerError, "An internal error occurred while fetching the user")
        // Or just return the original error for a default 500: return GetUserResp{}, err
    }

    // Success case
    return GetUserResp{
        ID:    user.ID,
        Name:  user.Name,
        Email: user.Email,
    }, nil
}
```

When `NewHTTPError` is returned:

1.  SRouter logs the error via the `handleError` method. `HTTPError` values are logged at `Error` level and certain client errors (e.g., request body too large) are logged at `Warn`.
2.  It sets the HTTP response status code to `HTTPError.StatusCode`.
3.  It sets the `Content-Type` header to `application/json; charset=utf-8`.
4.  It writes a JSON response body containing the error message and trace ID (if enabled):
    ```json
    {
      "error": {
        "message": "Your HTTPError.Message here",
        "trace_id": "generated-trace-id-if-enabled"
      }
    }
    ```

## Error Handling Reference

-   **`router.HTTPError`**: Struct containing `StatusCode` (int) and `Message` (string). Implements the `error` interface.
-   **`router.NewHTTPError(statusCode int, message string) *router.HTTPError`**: Constructor function to create `*HTTPError` instances.
