package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"
)

func TestUserRateLimitRunsAfterBuiltInAuthentication(t *testing.T) {
	t.Run("authentication is required", func(t *testing.T) {
		r := newRateLimitingRouter(zap.NewNop())
		response := httptest.NewRecorder()
		r.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/api/profile", nil))
		if response.Code != http.StatusUnauthorized {
			t.Fatalf("status = %d, want %d; body = %s", response.Code, http.StatusUnauthorized, response.Body.String())
		}
	})

	t.Run("limits are isolated by user ID", func(t *testing.T) {
		r := newRateLimitingRouter(zap.NewNop())
		requestProfile := func(token string) *httptest.ResponseRecorder {
			request := httptest.NewRequest(http.MethodGet, "/api/profile", nil)
			request.Header.Set("Authorization", "Bearer "+token)
			response := httptest.NewRecorder()
			r.ServeHTTP(response, request)
			return response
		}

		for requestNumber := 1; requestNumber <= 10; requestNumber++ {
			response := requestProfile("token1")
			if response.Code != http.StatusOK {
				t.Fatalf("user1 request %d status = %d, want %d; body = %s", requestNumber, response.Code, http.StatusOK, response.Body.String())
			}
			if !strings.Contains(response.Body.String(), "User One") {
				t.Fatalf("user1 request %d body = %q, want authenticated profile", requestNumber, response.Body.String())
			}
		}

		response := requestProfile("token1")
		if response.Code != http.StatusTooManyRequests {
			t.Fatalf("user1 request 11 status = %d, want %d; body = %s", response.Code, http.StatusTooManyRequests, response.Body.String())
		}

		response = requestProfile("token2")
		if response.Code != http.StatusOK {
			t.Fatalf("user2 first request status = %d, want %d; body = %s", response.Code, http.StatusOK, response.Body.String())
		}
	})
}
