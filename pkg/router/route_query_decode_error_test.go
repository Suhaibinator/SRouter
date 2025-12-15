package router

import (
	"encoding/base64"
	"encoding/json"
	"math/big"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Suhaibinator/SRouter/pkg/codec"
	"github.com/Suhaibinator/SRouter/pkg/router/internal/mocks"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func encodeBase62(b []byte) string {
	const alphabet = "0123456789ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz"

	n := new(big.Int).SetBytes(b)
	if n.Sign() == 0 {
		return "0"
	}

	base := big.NewInt(62)
	mod := new(big.Int)

	var out []byte
	for n.Sign() > 0 {
		n.DivMod(n, base, mod)
		out = append(out, alphabet[mod.Int64()])
	}

	for i, j := 0, len(out)-1; i < j; i, j = i+1, j-1 {
		out[i], out[j] = out[j], out[i]
	}
	return string(out)
}

func TestRegisterGenericRoute_Base64QueryParameter_DecodeBytesError(t *testing.T) {
	logger := zap.NewNop()
	r := NewRouter(RouterConfig{Logger: logger}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	RegisterGenericRoute(r, RouteConfig[RequestType, ResponseType]{
		Path:       "/test",
		Methods:    []HttpMethod{MethodGet},
		Codec:      codec.NewJSONCodec[RequestType, ResponseType](),
		SourceType: Base64QueryParameter,
		SourceKey:  "qdata",
		Handler: func(r *http.Request, req RequestType) (ResponseType, error) {
			t.Fatalf("handler should not be called on decode error")
			return ResponseType{}, nil
		},
	}, 0, 0, nil)

	invalidJSONBase64 := base64.StdEncoding.EncodeToString([]byte("{invalid json"))
	req := httptest.NewRequest(http.MethodGet, "/test?qdata="+invalidJSONBase64, nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)

	var body map[string]map[string]string
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
	require.Equal(t, "Failed to decode query parameter data", body["error"]["message"])
}

func TestRegisterGenericRoute_Base62QueryParameter_DecodeBytesError(t *testing.T) {
	logger := zap.NewNop()
	r := NewRouter(RouterConfig{Logger: logger}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	RegisterGenericRoute(r, RouteConfig[RequestType, ResponseType]{
		Path:       "/test",
		Methods:    []HttpMethod{MethodGet},
		Codec:      codec.NewJSONCodec[RequestType, ResponseType](),
		SourceType: Base62QueryParameter,
		SourceKey:  "qdata",
		Handler: func(r *http.Request, req RequestType) (ResponseType, error) {
			t.Fatalf("handler should not be called on decode error")
			return ResponseType{}, nil
		},
	}, 0, 0, nil)

	invalidJSONBase62 := encodeBase62([]byte("{invalid json"))
	req := httptest.NewRequest(http.MethodGet, "/test?qdata="+invalidJSONBase62, nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	require.Equal(t, http.StatusBadRequest, rr.Code)

	var body map[string]map[string]string
	require.NoError(t, json.Unmarshal(rr.Body.Bytes(), &body))
	require.Equal(t, "Failed to decode query parameter data", body["error"]["message"])
}
