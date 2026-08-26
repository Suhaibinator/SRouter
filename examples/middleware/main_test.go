package main

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"go.uber.org/zap"
)

func TestRateLimitMiddlewareReturnsWithoutBlocking(t *testing.T) {
	handler := RateLimitMiddleware(1, zap.NewNop())(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	serve := func() *httptest.ResponseRecorder {
		t.Helper()
		done := make(chan *httptest.ResponseRecorder, 1)
		go func() {
			response := httptest.NewRecorder()
			request := httptest.NewRequest(http.MethodGet, "/rate-limited/resource", nil)
			handler.ServeHTTP(response, request)
			done <- response
		}()

		select {
		case response := <-done:
			return response
		case <-time.After(250 * time.Millisecond):
			t.Fatal("rate-limit middleware blocked the request")
			return nil
		}
	}

	if status := serve().Code; status != http.StatusOK {
		t.Fatalf("first request status = %d, want %d", status, http.StatusOK)
	}
	if status := serve().Code; status != http.StatusTooManyRequests {
		t.Fatalf("over-limit request status = %d, want %d", status, http.StatusTooManyRequests)
	}
}
