package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"
)

func TestUserAuthenticationMiddlewareEndpoints(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		auth       string
		basicUser  string
		basicPass  string
		wantStatus int
		wantBody   string
	}{
		{name: "public", path: "/public/resource", wantStatus: http.StatusOK},
		{name: "boolean missing", path: "/boolean-auth/resource", wantStatus: http.StatusUnauthorized},
		{name: "boolean bearer", path: "/boolean-auth/resource", auth: "Bearer token1", wantStatus: http.StatusOK},
		{name: "custom malformed", path: "/user-auth/custom", auth: "x", wantStatus: http.StatusUnauthorized},
		{name: "custom bearer", path: "/user-auth/custom", auth: "Bearer token1", wantStatus: http.StatusOK, wantBody: "User One"},
		{name: "bearer provider", path: "/user-auth/bearer", auth: "Bearer token2", wantStatus: http.StatusOK, wantBody: "User Two"},
		{name: "basic provider", path: "/user-auth/basic", basicUser: "user1", basicPass: "password", wantStatus: http.StatusOK, wantBody: "User One"},
		{name: "basic rejected", path: "/user-auth/basic", basicUser: "user1", basicPass: "wrong", wantStatus: http.StatusUnauthorized},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newUserAuthRouter(zap.NewNop())
			request := httptest.NewRequest(http.MethodGet, tt.path, nil)
			if tt.auth != "" {
				request.Header.Set("Authorization", tt.auth)
			}
			if tt.basicUser != "" {
				request.SetBasicAuth(tt.basicUser, tt.basicPass)
			}
			response := httptest.NewRecorder()

			r.ServeHTTP(response, request)
			if response.Code != tt.wantStatus {
				t.Fatalf("status = %d, want %d; body = %s", response.Code, tt.wantStatus, response.Body.String())
			}
			if tt.wantBody != "" && !strings.Contains(response.Body.String(), tt.wantBody) {
				t.Fatalf("body = %q, want substring %q", response.Body.String(), tt.wantBody)
			}
		})
	}
}
