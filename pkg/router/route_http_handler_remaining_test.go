package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/require"
)

type remainingRouteRuntime struct {
	status  int
	message string
}

func (runtime *remainingRouteRuntime) handleError(w http.ResponseWriter, _ *http.Request, _ error, status int, message string) {
	runtime.status = status
	runtime.message = message
	http.Error(w, message, status)
}

func (*remainingRouteRuntime) recordHandlerError(*http.Request, error) {}

func (*remainingRouteRuntime) warnMissingSanitizer(string, []HttpMethod) {}

func TestTypedRouteHTTPHandlerRemainingSourceErrors(t *testing.T) {
	tests := []struct {
		name        string
		sourceType  SourceType
		target      string
		wantStatus  int
		wantMessage string
	}{
		{
			name:        "missing Base64 query parameter",
			sourceType:  Base64QueryParameter,
			target:      "/test",
			wantStatus:  http.StatusBadRequest,
			wantMessage: "Missing required query parameter: data",
		},
		{
			name:        "malformed Base64 query parameter",
			sourceType:  Base64QueryParameter,
			target:      "/test?data=not-base64!",
			wantStatus:  http.StatusBadRequest,
			wantMessage: "Failed to decode base64 query parameter: data",
		},
		{
			name:        "unsupported source type",
			sourceType:  SourceType(999),
			target:      "/test",
			wantStatus:  http.StatusInternalServerError,
			wantMessage: "Unsupported source type",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			runtime := &remainingRouteRuntime{}
			route := RouteConfig[struct{}, struct{}]{
				SourceType: tt.sourceType,
				SourceKey:  "data",
				Handler: func(*http.Request, struct{}) (struct{}, error) {
					t.Fatal("handler should not be called for a source decoding error")
					return struct{}{}, nil
				},
			}

			recorder := httptest.NewRecorder()
			route.httpHandler(runtime)(recorder, httptest.NewRequest(http.MethodGet, tt.target, nil))

			require.Equal(t, tt.wantStatus, runtime.status)
			require.Equal(t, tt.wantStatus, recorder.Code)
			require.Equal(t, tt.wantMessage, runtime.message)
		})
	}
}
