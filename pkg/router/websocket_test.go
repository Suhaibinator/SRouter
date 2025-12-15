package router_test

import (
	"bufio"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/common"
	"github.com/Suhaibinator/SRouter/pkg/router"
	"go.uber.org/zap"
)

type hijackableRecorder struct {
	*httptest.ResponseRecorder
	hijacked   bool
	serverConn net.Conn
	clientConn net.Conn
}

func newHijackableRecorder() *hijackableRecorder {
	return &hijackableRecorder{ResponseRecorder: httptest.NewRecorder()}
}

func (rw *hijackableRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if rw.serverConn != nil || rw.clientConn != nil {
		return nil, nil, errors.New("connection already hijacked")
	}

	rw.hijacked = true
	rw.clientConn, rw.serverConn = net.Pipe()
	return rw.serverConn, bufio.NewReadWriter(bufio.NewReader(rw.serverConn), bufio.NewWriter(rw.serverConn)), nil
}

func (rw *hijackableRecorder) Close() {
	if rw.serverConn != nil {
		_ = rw.serverConn.Close()
	}
	if rw.clientConn != nil {
		_ = rw.clientConn.Close()
	}
}

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

func TestWebSocketRoutePreservesHijackerWithTracingEnabled(t *testing.T) {
	logger := zap.NewNop()
	config := router.RouterConfig{
		Logger:            logger,
		GlobalTimeout:     100 * time.Millisecond,
		TraceIDBufferSize: 1,
	}

	r := router.NewRouter[string, string](config, nil, nil)

	var sawHijacker bool
	var hijackErr error

	r.RegisterRoute(router.RouteConfigBase{
		Path:        "/ws",
		Methods:     []router.HttpMethod{router.MethodGet},
		IsWebSocket: true,
		Handler: func(w http.ResponseWriter, _ *http.Request) {
			h, ok := w.(http.Hijacker)
			if !ok {
				hijackErr = errors.New("response writer does not implement http.Hijacker")
				return
			}
			sawHijacker = true

			conn, _, err := h.Hijack()
			if err != nil {
				hijackErr = err
				return
			}
			_ = conn.Close()
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	rr := newHijackableRecorder()
	defer rr.Close()

	r.ServeHTTP(rr, req)

	if !sawHijacker {
		t.Fatalf("expected handler to receive an http.Hijacker when tracing is enabled")
	}
	if hijackErr != nil {
		t.Fatalf("expected Hijack to succeed, got %v", hijackErr)
	}
	if !rr.hijacked {
		t.Fatalf("expected Hijack to be delegated to the underlying response writer")
	}
}

func TestWebSocketRouteHijackNotSupportedIsWrapped(t *testing.T) {
	logger := zap.NewNop()
	config := router.RouterConfig{
		Logger:            logger,
		GlobalTimeout:     100 * time.Millisecond,
		TraceIDBufferSize: 1,
	}

	r := router.NewRouter[string, string](config, nil, nil)

	var hijackErr error

	r.RegisterRoute(router.RouteConfigBase{
		Path:        "/ws",
		Methods:     []router.HttpMethod{router.MethodGet},
		IsWebSocket: true,
		Handler: func(w http.ResponseWriter, _ *http.Request) {
			h, ok := w.(http.Hijacker)
			if !ok {
				hijackErr = errors.New("response writer does not implement http.Hijacker")
				return
			}

			_, _, hijackErr = h.Hijack()
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/ws", nil)
	rr := httptest.NewRecorder()

	r.ServeHTTP(rr, req)

	if hijackErr == nil {
		t.Fatalf("expected Hijack to fail")
	}
	if !errors.Is(hijackErr, http.ErrNotSupported) {
		t.Fatalf("expected errors.Is(hijackErr, http.ErrNotSupported) to be true, got %v", hijackErr)
	}
	if !strings.Contains(hijackErr.Error(), "does not support hijacking") {
		t.Fatalf("expected Hijack error to include context, got %q", hijackErr.Error())
	}
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
