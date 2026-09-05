# Custom Error Handling

Typed route handlers return `(response, error)`. SRouter converts a non-nil error into a JSON response and records the original error in the request context so outer middleware can inspect it after the handler returns.

```go
func handleCreate(r *http.Request, req CreateRequest) (CreateResponse, error) {
	// ...
}
```

Use `scontext.GetHandlerError[T, U]` in middleware that needs to decide whether to commit, roll back, or record an outcome. See [Context Management](./context-management.md) for the context API.

## Default errors

Returning an ordinary, otherwise-unrecognized handler error produces status
500. SRouter logs the error, but the client receives the generic boundary
message rather than `err.Error()`:

```json
{
  "error": {
    "message": "Handler error"
  }
}
```

When automatic trace IDs are enabled with `TraceIDBufferSize > 0`, the error object also contains `trace_id`.

## HTTPError

Return `*router.HTTPError` when the client should receive a specific 4xx or 5xx status and safe message:

```go
if req.UserID == "" {
	return User{}, router.NewHTTPError(
		http.StatusBadRequest,
		"user ID is required",
	)
}
```

The public API includes:

```go
type HTTPError struct {
	StatusCode int
	Message    string
}

func NewHTTPError(statusCode int, message string) *HTTPError
func NewHTTPErrorWithCause(statusCode int, message string, cause error) *HTTPError

func (e *HTTPError) Error() string
func (e *HTTPError) WithFields(fields ...zap.Field) *HTTPError
func (e *HTTPError) WithLogLevel(level zapcore.Level) *HTTPError
func (e *HTTPError) Cause() error
func (e *HTTPError) Unwrap() error
func (e *HTTPError) Fields() []zap.Field
func (e *HTTPError) LogLevel() (zapcore.Level, bool)
```

`WithFields` and `WithLogLevel` return copies, so a base error can be safely enriched at successive layers without mutating it. `Fields` also returns a copy of its slice.

### Preserve an internal cause

`NewHTTPErrorWithCause` keeps diagnostic details in logs and the Go error chain while exposing only the safe message to the client:

```go
user, err := store.FindUser(r.Context(), req.UserID)
if errors.Is(err, sql.ErrNoRows) {
	return User{}, router.NewHTTPErrorWithCause(
		http.StatusNotFound,
		"user not found",
		err,
	).WithFields(
		zap.String("user_id", req.UserID),
	)
}
if err != nil {
	return User{}, router.NewHTTPErrorWithCause(
		http.StatusInternalServerError,
		"unable to load user",
		err,
	)
}
```

The cause is used for `errors.Is` and `errors.As`, and the router logs it as the `error` field. It is never added to the response body.

### Structured fields

Attach request-domain fields that help diagnose the failure:

```go
return Order{}, router.NewHTTPErrorWithCause(
	http.StatusConflict,
	"order cannot be updated",
	err,
).WithFields(
	zap.String("order_id", orderID),
	zap.String("state", state),
)
```

When a key is attached more than once, the most recently attached value wins. SRouter discards these boundary-owned keys from custom fields so application data cannot replace authoritative values:

- `error`
- `method`
- `path`
- `status_code`
- `trace_id`

### Log level selection

Boundary errors use these defaults:

| Condition | Level |
| --- | --- |
| Explicit `WithLogLevel` | The supplied level |
| Error chain contains `context.Canceled` | `Debug` |
| Error chain contains `context.DeadlineExceeded` | `Warn` |
| Status from 400 through 499 | `Info` |
| Status from 500 through 599 or another unexpected error | `Error` |

Use an override only when operational severity genuinely differs from the HTTP classification:

```go
return Result{}, router.NewHTTPErrorWithCause(
	http.StatusConflict,
	"account invariant violated",
	err,
).WithLogLevel(zapcore.ErrorLevel)
```

Route timeouts are logged separately by the timeout middleware at `Warn`. A body rejected by `http.MaxBytesReader` becomes a 413 response and follows the normal 4xx `Info` classification.

## Response contract

For a valid `HTTPError`, SRouter:

1. Uses its status and message.
2. Logs the cause, attached fields, status, method, path, and trace ID.
3. Sets `Content-Type: application/json; charset=utf-8`.
4. Writes the safe JSON response.

```json
{
  "error": {
    "message": "user not found",
    "trace_id": "0196..."
  }
}
```

`trace_id` is present in the JSON only when automatic trace generation is enabled. Error log records still receive a correlation ID when it is disabled; that log-only ID is not exposed to the client.

`HTTPError.StatusCode` must be between 400 and 599. Values outside that range are replaced with `500 Internal Server Error`, and the rejected value is logged as `invalid_status_code`.

Panics are recovered and logged at `Error`. If no response has started, SRouter returns the same generic 500 JSON contract. If a handler already wrote headers or body bytes, SRouter logs the panic without trying to append a second response.
