package router_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/common"
	"github.com/Suhaibinator/SRouter/pkg/router"
	"go.uber.org/zap"
)

func TestWebSocketRoute(t *testing.T) {
	logger := zap.NewNop()
	config := router.RouterConfig{
		Logger:        logger,
		GlobalTimeout: 100 * time.Millisecond,
	}

	r := router.NewRouter[string, string](config, nil, nil)

	// Register a "WebSocket" route that sleeps longer than the global timeout
	// Since IsWebSocket is true, it should NOT timeout.
	r.RegisterRoute(router.RouteConfigBase{
		Path:        "/ws",
		Methods:     []router.HttpMethod{router.MethodGet},
		IsWebSocket: true,
		Handler: func(w http.ResponseWriter, r *http.Request) {
			// Simulate long-lived connection
			time.Sleep(200 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		},
	})

	// Register a normal route that SHOULD timeout
	r.RegisterRoute(router.RouteConfigBase{
		Path:    "/normal",
		Methods: []router.HttpMethod{router.MethodGet},
		Handler: func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(200 * time.Millisecond)
			w.WriteHeader(http.StatusOK)
		},
	})

	server := httptest.NewServer(r)
	defer server.Close()

	client := server.Client()

	// Test WebSocket Route
	t.Run("WebSocket Route should not timeout", func(t *testing.T) {
		start := time.Now()
		resp, err := client.Get(server.URL + "/ws")
		duration := time.Since(start)

		if err != nil {
			t.Fatalf("/ws request failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("/ws: expected 200 OK, got %d", resp.StatusCode)
		}
		if duration < 200*time.Millisecond {
			t.Errorf("/ws: completed too fast (%v), sleep didn't happen?", duration)
		}
	})

	// Test Normal Route (Control Case)
	t.Run("Normal Route should timeout", func(t *testing.T) {
		resp, err := client.Get(server.URL + "/normal")
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusRequestTimeout {
			t.Errorf("/normal: expected 408 Timeout, got %d", resp.StatusCode)
		}
	})
}

func TestSubRouterWebSocketRoute(t *testing.T) {
	logger := zap.NewNop()
	config := router.RouterConfig{
		Logger:        logger,
		GlobalTimeout: 100 * time.Millisecond,
		SubRouters: []router.SubRouterConfig{
			{
				PathPrefix: "/sub",
				Overrides: common.RouteOverrides{
					Timeout: 50 * time.Millisecond,
				},
				Routes: []router.RouteDefinition{
					router.RouteConfigBase{
						Path:        "/ws",
						Methods:     []router.HttpMethod{router.MethodGet},
						IsWebSocket: true,
						Handler: func(w http.ResponseWriter, r *http.Request) {
							// Simulate long-lived connection
							time.Sleep(200 * time.Millisecond)
							w.WriteHeader(http.StatusOK)
						},
					},
					router.RouteConfigBase{
						Path:    "/normal",
						Methods: []router.HttpMethod{router.MethodGet},
						Handler: func(w http.ResponseWriter, r *http.Request) {
							time.Sleep(200 * time.Millisecond)
							w.WriteHeader(http.StatusOK)
						},
					},
				},
			},
		},
	}

	r := router.NewRouter[string, string](config, nil, nil)

	server := httptest.NewServer(r)
	defer server.Close()

	client := server.Client()

	// Test SubRouter WebSocket Route
	t.Run("SubRouter WebSocket Route should not timeout", func(t *testing.T) {
		start := time.Now()
		resp, err := client.Get(server.URL + "/sub/ws")
		duration := time.Since(start)

		if err != nil {
			t.Fatalf("/sub/ws request failed: %v", err)
		}
		if resp.StatusCode != http.StatusOK {
			t.Errorf("/sub/ws: expected 200 OK, got %d", resp.StatusCode)
		}
		if duration < 200*time.Millisecond {
			t.Errorf("/sub/ws: completed too fast (%v), sleep didn't happen?", duration)
		}
	})

	// Test SubRouter Normal Route (Control Case)
	t.Run("SubRouter Normal Route should timeout", func(t *testing.T) {
		resp, err := client.Get(server.URL + "/sub/normal")
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != http.StatusRequestTimeout {
			t.Errorf("/sub/normal: expected 408 Timeout, got %d", resp.StatusCode)
		}
	})
}
