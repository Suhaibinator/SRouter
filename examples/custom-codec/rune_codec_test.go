package main

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/stretchr/testify/require"
)

const sampleScroll = `SROUTER-RUNE-SCROLL/1
seeker :: Ada Lovelace
question :: Will custom codecs make APIs more fun?
vibe :: electric`

func TestRuneScrollCodecDecodeBytes(t *testing.T) {
	request, err := NewRuneScrollCodec().DecodeBytes([]byte(sampleScroll))

	require.NoError(t, err)
	require.Equal(t, OracleRequest{
		Seeker:   "Ada Lovelace",
		Question: "Will custom codecs make APIs more fun?",
		Vibe:     "electric",
	}, request)
}

func TestRuneScrollCodecRejectsUnknownFields(t *testing.T) {
	_, err := NewRuneScrollCodec().DecodeBytes([]byte(sampleScroll + "\nwand :: oak"))

	require.EqualError(t, err, `line 5 contains unknown field "wand"`)
}

func TestRuneScrollRoutesUseBodyAndDecodeBytes(t *testing.T) {
	tests := []struct {
		name   string
		method string
		target string
		body   string
	}{
		{
			name:   "request body calls Decode",
			method: http.MethodPost,
			target: "/oracle",
			body:   sampleScroll,
		},
		{
			name:   "base64 query calls DecodeBytes",
			method: http.MethodGet,
			target: "/oracle?scroll=" + url.QueryEscape(base64.StdEncoding.EncodeToString([]byte(sampleScroll))),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(tt.method, tt.target, strings.NewReader(tt.body))

			newOracleRouter().ServeHTTP(recorder, request)

			require.Equal(t, http.StatusOK, recorder.Code)
			require.Equal(t, runeScrollType, recorder.Header().Get("Content-Type"))
			require.Contains(t, recorder.Body.String(), runeScrollVersion)
			require.Contains(t, recorder.Body.String(), "✦ seeker :: Ada Lovelace")
			require.Contains(t, recorder.Body.String(), "✦ lucky-rune :: ")
		})
	}
}
