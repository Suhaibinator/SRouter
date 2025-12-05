package router

import (
	"context"
	"errors"
	"net/http"

	"github.com/Suhaibinator/SRouter/pkg/common"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// WebSocketHandler defines the signature for WebSocket route handlers.
// The request context is passed so callers can respect cancellation or deadlines
// applied by middleware such as timeouts or shutdown handling.
type WebSocketHandler func(ctx context.Context, conn *websocket.Conn)

// WebSocketRouteConfig defines a WebSocket endpoint that can be registered like
// any other RouteDefinition.
//
// Middlewares, authentication, and rate limiting are applied to the handshake
// request using the existing pipeline so behavior matches standard routes.
type WebSocketRouteConfig struct {
	Path        string
	AuthLevel   *AuthLevel
	Overrides   common.RouteOverrides
	Middlewares []common.Middleware
	Upgrader    *websocket.Upgrader
	Handler     WebSocketHandler
}

// isRouteDefinition implements the RouteDefinition interface.
func (WebSocketRouteConfig) isRouteDefinition() {}

// defaultWebSocketUpgrader returns a lenient upgrader suitable for most tests
// and local development scenarios. Users can supply their own Upgrader when they
// need stricter origin checks or advanced configuration.
func defaultWebSocketUpgrader() *websocket.Upgrader {
	return &websocket.Upgrader{
		CheckOrigin: func(*http.Request) bool {
			return true
		},
	}
}

// wrapWebSocketHandler creates an http.Handler that performs the WebSocket upgrade
// before delegating to the provided WebSocketHandler. The handler returned here is
// still wrapped by the router's middleware chain via wrapHandler.
func (r *Router[T, U]) wrapWebSocketHandler(route WebSocketRouteConfig) http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		upgrader := route.Upgrader
		if upgrader == nil {
			upgrader = defaultWebSocketUpgrader()
		}

		conn, err := upgrader.Upgrade(w, req, nil)
		if err != nil {
			// If the error is caused by a failed upgrade the upgrader already
			// wrote the appropriate response. Just log and return.
			var closeError *websocket.CloseError
			if errors.As(err, &closeError) {
				r.logger.Debug("WebSocket upgrade closed", zap.Error(err))
			} else {
				r.logger.Error("WebSocket upgrade failed", zap.Error(err))
			}
			return
		}
		defer func() {
			if closeErr := conn.Close(); closeErr != nil && !errors.Is(closeErr, websocket.ErrCloseSent) {
				r.logger.Warn("WebSocket close failed", zap.Error(closeErr))
			}
		}()

		route.Handler(req.Context(), conn)
	}
}
