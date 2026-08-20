package router

import (
	json "encoding/json/v2"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Suhaibinator/SRouter/pkg/codec"
	"github.com/Suhaibinator/SRouter/pkg/router/internal/mocks"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestRegisterTypedRoute_Base62PathParameter_DecodeBytesError(t *testing.T) {
	logger := zap.NewNop()
	r := NewRouter(RouterConfig{Logger: logger}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	r.Route(RouteConfig[RequestType, ResponseType]{
		Path:       "/test/:data",
		Methods:    []HttpMethod{MethodGet},
		Codec:      codec.NewJSONCodec[RequestType, ResponseType](),
		SourceType: Base62PathParameter,
		SourceKey:  "data",
		Handler: func(r *http.Request, req RequestType) (ResponseType, error) {
			t.Fatal("handler should not be called on decode error")
			return ResponseType{}, nil
		},
	})

	invalidJSONBase62 := encodeBase62([]byte("{invalid json"))
	req := httptest.NewRequest(http.MethodGet, "/test/"+invalidJSONBase62, nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)

	var body map[string]map[string]string
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
	require.Equal(t, "Failed to decode path parameter data", body["error"]["message"])
}
