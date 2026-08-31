package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"
)

func TestAdvertisedAuthenticationEndpoints(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		header     string
		value      string
		wantStatus int
	}{
		{name: "public", path: "/public/resource", wantStatus: http.StatusOK},
		{name: "bearer missing", path: "/bearer-auth/resource", wantStatus: http.StatusUnauthorized},
		{name: "bearer accepted", path: "/bearer-auth/resource", header: "Authorization", value: "Bearer token1", wantStatus: http.StatusOK},
		{name: "API key header", path: "/api-key-auth/resource", header: "X-API-Key", value: "key1", wantStatus: http.StatusOK},
		{name: "API key query", path: "/api-key-auth/resource?api_key=key1", wantStatus: http.StatusOK},
		{name: "built-in auth missing", path: "/require-auth/resource", wantStatus: http.StatusUnauthorized},
		{name: "built-in auth accepted", path: "/require-auth/resource", header: "Authorization", value: "Bearer token1", wantStatus: http.StatusOK},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newAuthRouter(zap.NewNop())
			request := httptest.NewRequest(http.MethodGet, tt.path, nil)
			if tt.header != "" {
				request.Header.Set(tt.header, tt.value)
			}
			response := httptest.NewRecorder()

			r.ServeHTTP(response, request)
			if response.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, tt.wantStatus, response.Body.String())
			}
		})
	}
}
