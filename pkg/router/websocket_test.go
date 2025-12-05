package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Suhaibinator/SRouter/pkg/common"
	"github.com/Suhaibinator/SRouter/pkg/router/internal/mocks"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/require"
	"go.uber.org/zap"
)

func TestWebSocketRoute(t *testing.T) {
	logger := zap.NewNop()

	middlewareCalled := atomic.Bool{}

	wsRoute := WebSocketRouteConfig{
		Path: "/echo",
		Middlewares: []common.Middleware{
			func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					middlewareCalled.Store(true)
					next.ServeHTTP(w, r)
				})
			},
		},
		Handler: func(ctx context.Context, conn *websocket.Conn) {
			msgType, data, err := conn.ReadMessage()
			require.NoError(t, err)
			require.NoError(t, conn.WriteMessage(msgType, append([]byte("echo:"), data...)))
		},
	}

	r := NewRouter(RouterConfig{Logger: logger, SubRouters: []SubRouterConfig{{
		PathPrefix: "/ws",
		Routes:     []RouteDefinition{wsRoute},
	}}}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	server := httptest.NewServer(r)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/echo"

	conn, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	require.NoError(t, err)
	t.Cleanup(func() {
		_ = conn.Close()
	})

	require.NoError(t, conn.WriteMessage(websocket.TextMessage, []byte("hello")))
	_, resp, err := conn.ReadMessage()
	require.NoError(t, err)
	require.Equal(t, "echo:hello", string(resp))

	require.True(t, middlewareCalled.Load(), "middleware should run before WebSocket handler")
}
