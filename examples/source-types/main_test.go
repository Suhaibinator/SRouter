package main

import (
	"bytes"
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/Suhaibinator/SRouter/pkg/codec"
	"github.com/Suhaibinator/SRouter/pkg/router"
	"go.uber.org/zap"
)

func TestSourceTypeEndpoints(t *testing.T) {
	r := router.NewRouter[string, string](
		router.RouterConfig{Logger: zap.NewNop()}, router.RouterDependencies[string, string]{Authenticate: nil, UserID: nil})

	registerRoutes(r)

	payload := "{\"id\":\"1\"}"
	base64Encoded := base64.StdEncoding.EncodeToString([]byte(payload))
	base62Encoded := codec.EncodeBase62([]byte(payload))

	tests := []struct {
		name   string
		method string
		path   string
		body   []byte
	}{
		{name: "body", method: http.MethodPost, path: "/users/body", body: []byte(payload)},
		{name: "empty", method: http.MethodGet, path: "/users/empty/1"},
		{name: "base64 query", method: http.MethodGet, path: "/users/base64/query?data=" + url.QueryEscape(base64Encoded)},
		{name: "base64 path", method: http.MethodGet, path: "/users/base64/path/" + base64Encoded},
		{name: "base62 query", method: http.MethodGet, path: "/users/base62/query?data=" + base62Encoded},
		{name: "base62 path", method: http.MethodGet, path: "/users/base62/path/" + base62Encoded},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			request := httptest.NewRequest(tt.method, tt.path, bytes.NewReader(tt.body))
			if len(tt.body) > 0 {
				request.Header.Set("Content-Type", "application/json")
			}

			r.ServeHTTP(recorder, request)
			if recorder.Code != http.StatusOK {
				t.Fatalf("status = %d, want %d; body = %s", recorder.Code, http.StatusOK, recorder.Body.String())
			}
			if !bytes.Contains(recorder.Body.Bytes(), []byte(`"id":"1"`)) {
				t.Fatalf("body = %q, want decoded user 1", recorder.Body.String())
			}
		})
	}
}
