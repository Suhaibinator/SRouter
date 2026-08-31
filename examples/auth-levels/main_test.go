package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"
)

func TestBuiltInAuthenticationLevels(t *testing.T) {
	tests := []struct {
		name       string
		path       string
		token      string
		wantStatus int
		wantBody   string
	}{
		{name: "no auth", path: "/auth-levels/no-auth", wantStatus: http.StatusOK},
		{name: "optional without token", path: "/auth-levels/optional-auth", wantStatus: http.StatusOK, wantBody: `"authenticated":false`},
		{name: "optional invalid token", path: "/auth-levels/optional-auth", token: "invalid", wantStatus: http.StatusOK, wantBody: `"authenticated":false`},
		{name: "optional valid token", path: "/auth-levels/optional-auth", token: "token1", wantStatus: http.StatusOK, wantBody: `"authenticated":true`},
		{name: "required without token", path: "/auth-levels/required-auth", wantStatus: http.StatusUnauthorized},
		{name: "required valid token", path: "/auth-levels/required-auth", token: "token1", wantStatus: http.StatusOK, wantBody: "User One"},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := newAuthLevelsRouter(zap.NewNop())
			request := httptest.NewRequest(http.MethodGet, tt.path, nil)
			if tt.token != "" {
				request.Header.Set("Authorization", "Bearer "+tt.token)
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
