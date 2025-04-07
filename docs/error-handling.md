# Custom Error Handling

SRouter provides mechanisms for handling errors within your application, particularly within generic handlers, and translating them into appropriate HTTP responses.

## Returning Errors from Generic Handlers

Generic handlers (`GenericHandler[T, U]`) have the signature `func(*http.Request, T) (U, error)`. When a generic handler returns a non-nil `error`, SRouter's internal wrapper intercepts it and handles sending an appropriate HTTP error response back to the client.

-   **Standard Errors**: If the handler returns a standard Go error (e.g., `errors.New("something went wrong")`, `fmt.Errorf("details: %w", err)`), SRouter typically defaults to sending a `500 Internal Server Error` response. The specific error message returned by the handler is logged by the server (at Error level) but is usually *not* sent to the client for security reasons.

-   **`router.HTTPError`**: To send a specific HTTP status code and a custom message to the client, return an error of type `*router.HTTPError`.

## `router.HTTPError`

This struct (defined in `pkg/router/router.go`) allows you to specify both the HTTP status code and the error message that should be sent to the client.

```go
package router // Defined in pkg/router/router.go

type HTTPError struct {
    StatusCode int
    Message    string // This message IS sent to the client
}

// Error makes HTTPError satisfy the error interface.
func (e *HTTPError) Error() string {
    return fmt.Sprintf("%d: %s", e.StatusCode, e.Message) // Corrected implementation
}

// NewHTTPError is a constructor for creating HTTPError instances.
func NewHTTPError(statusCode int, message string) *HTTPError { // Defined in pkg/router/router.go
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

1.  SRouter logs the error message at the appropriate level (Warn for 4xx, Error for 5xx).
2.  It sets the HTTP response status code to `HTTPError.StatusCode`.
3.  It writes the `HTTPError.Message` as the response body (often plain text, unless your codec or error handling middleware modifies it).

## Error Handling Reference

-   **`router.HTTPError`**: Struct defined in `pkg/router/router.go` containing `StatusCode` (int) and `Message` (string). Implements the `error` interface.
-   **`router.NewHTTPError(statusCode int, message string) *router.HTTPError`**: Constructor function defined in `pkg/router/router.go` to create `*HTTPError` instances.
