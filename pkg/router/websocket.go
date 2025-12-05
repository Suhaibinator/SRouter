// Package router provides WebSocket support for the SRouter framework.
package router

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/common"
	"github.com/Suhaibinator/SRouter/pkg/middleware"
	"github.com/Suhaibinator/SRouter/pkg/scontext"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// WebSocketOverrides contains WebSocket-specific settings that can be overridden at different levels.
type WebSocketOverrides struct {
	// ReadBufferSize specifies the size of the read buffer in bytes.
	// If zero, a default size is used.
	ReadBufferSize int

	// WriteBufferSize specifies the size of the write buffer in bytes.
	// If zero, a default size is used.
	WriteBufferSize int

	// HandshakeTimeout specifies the duration for the handshake to complete.
	// If zero, no timeout is applied.
	HandshakeTimeout time.Duration

	// ReadTimeout specifies the maximum duration for reading a message.
	// If zero, no timeout is applied.
	ReadTimeout time.Duration

	// WriteTimeout specifies the maximum duration for writing a message.
	// If zero, no timeout is applied.
	WriteTimeout time.Duration

	// PingInterval specifies the interval between ping messages sent to the client.
	// If zero, ping messages are not sent automatically.
	PingInterval time.Duration

	// PongTimeout specifies the maximum duration to wait for a pong response.
	// Should be greater than PingInterval. If zero, defaults to PingInterval + 10 seconds.
	PongTimeout time.Duration

	// MaxMessageSize specifies the maximum size of a message in bytes.
	// If zero, no limit is applied.
	MaxMessageSize int64

	// EnableCompression enables per-message compression.
	EnableCompression bool

	// CheckOrigin is a function that returns true if the request origin is acceptable.
	// If nil, a safe default is used that checks that the Origin header matches the Host.
	CheckOrigin func(r *http.Request) bool

	// Subprotocols specifies the server's preferred subprotocols in order of preference.
	Subprotocols []string
}

// MessageType represents the type of a WebSocket message.
type MessageType int

// WebSocket message types as defined in RFC 6455.
const (
	// TextMessage denotes a text data message.
	TextMessage MessageType = websocket.TextMessage

	// BinaryMessage denotes a binary data message.
	BinaryMessage MessageType = websocket.BinaryMessage

	// CloseMessage denotes a close control message.
	CloseMessage MessageType = websocket.CloseMessage

	// PingMessage denotes a ping control message.
	PingMessage MessageType = websocket.PingMessage

	// PongMessage denotes a pong control message.
	PongMessage MessageType = websocket.PongMessage
)

// WebSocketCloseCode represents a WebSocket close status code.
type WebSocketCloseCode int

// Standard WebSocket close codes as defined in RFC 6455.
const (
	CloseNormalClosure           WebSocketCloseCode = websocket.CloseNormalClosure
	CloseGoingAway               WebSocketCloseCode = websocket.CloseGoingAway
	CloseProtocolError           WebSocketCloseCode = websocket.CloseProtocolError
	CloseUnsupportedData         WebSocketCloseCode = websocket.CloseUnsupportedData
	CloseNoStatusReceived        WebSocketCloseCode = websocket.CloseNoStatusReceived
	CloseAbnormalClosure         WebSocketCloseCode = websocket.CloseAbnormalClosure
	CloseInvalidFramePayloadData WebSocketCloseCode = websocket.CloseInvalidFramePayloadData
	ClosePolicyViolation         WebSocketCloseCode = websocket.ClosePolicyViolation
	CloseMessageTooBig           WebSocketCloseCode = websocket.CloseMessageTooBig
	CloseMandatoryExtension      WebSocketCloseCode = websocket.CloseMandatoryExtension
	CloseInternalServerErr       WebSocketCloseCode = websocket.CloseInternalServerErr
	CloseServiceRestart          WebSocketCloseCode = websocket.CloseServiceRestart
	CloseTryAgainLater           WebSocketCloseCode = websocket.CloseTryAgainLater
	CloseTLSHandshake            WebSocketCloseCode = websocket.CloseTLSHandshake
)

// WebSocketConnection wraps a gorilla/websocket connection with additional functionality.
// It provides a type-safe interface for WebSocket communication with access to SRouter's
// context management features.
//
// Thread Safety: All write operations (WriteMessage, WriteText, WriteBinary, WriteJSON, Ping,
// Close, CloseWithCode) are protected by an internal mutex and are safe for concurrent use.
// Read operations are NOT protected - only one goroutine should read at a time.
type WebSocketConnection[T comparable, U any] struct {
	conn       *websocket.Conn
	request    *http.Request
	overrides  WebSocketOverrides
	logger     *zap.Logger
	writeMu    sync.Mutex // Protects all write operations
	closeMu    sync.Mutex
	closed     bool
	closeChan  chan struct{}
	pingTicker *time.Ticker
}

// Request returns the original HTTP request that initiated the WebSocket connection.
// This can be used to access headers, context values, and other request information.
func (c *WebSocketConnection[T, U]) Request() *http.Request {
	return c.request
}

// Context returns the context from the original HTTP request.
func (c *WebSocketConnection[T, U]) Context() context.Context {
	return c.request.Context()
}

// UserID returns the authenticated user ID from the request context.
// Returns the zero value of T and false if the user ID is not set.
func (c *WebSocketConnection[T, U]) UserID() (T, bool) {
	return scontext.GetUserIDFromRequest[T, U](c.request)
}

// User returns the authenticated user from the request context.
// Returns nil and false if the user is not set.
func (c *WebSocketConnection[T, U]) User() (*U, bool) {
	return scontext.GetUserFromRequest[T, U](c.request)
}

// TraceID returns the trace ID from the request context.
func (c *WebSocketConnection[T, U]) TraceID() string {
	return scontext.GetTraceIDFromRequest[T, U](c.request)
}

// ClientIP returns the client IP address from the request context.
func (c *WebSocketConnection[T, U]) ClientIP() (string, bool) {
	return scontext.GetClientIPFromRequest[T, U](c.request)
}

// Subprotocol returns the negotiated subprotocol for the connection.
func (c *WebSocketConnection[T, U]) Subprotocol() string {
	return c.conn.Subprotocol()
}

// LocalAddr returns the local network address.
func (c *WebSocketConnection[T, U]) LocalAddr() string {
	return c.conn.LocalAddr().String()
}

// RemoteAddr returns the remote network address.
func (c *WebSocketConnection[T, U]) RemoteAddr() string {
	return c.conn.RemoteAddr().String()
}

// ReadMessage reads a message from the connection.
// It returns the message type and the message payload.
// This method blocks until a message is received or an error occurs.
func (c *WebSocketConnection[T, U]) ReadMessage() (MessageType, []byte, error) {
	if c.overrides.ReadTimeout > 0 {
		_ = c.conn.SetReadDeadline(time.Now().Add(c.overrides.ReadTimeout))
	}
	msgType, data, err := c.conn.ReadMessage()
	return MessageType(msgType), data, err
}

// WriteMessage writes a message to the connection.
// The message type must be TextMessage or BinaryMessage.
// This method is safe for concurrent use.
func (c *WebSocketConnection[T, U]) WriteMessage(messageType MessageType, data []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.overrides.WriteTimeout > 0 {
		_ = c.conn.SetWriteDeadline(time.Now().Add(c.overrides.WriteTimeout))
	}
	return c.conn.WriteMessage(int(messageType), data)
}

// WriteText writes a text message to the connection.
// This is a convenience method for WriteMessage(TextMessage, data).
func (c *WebSocketConnection[T, U]) WriteText(text string) error {
	return c.WriteMessage(TextMessage, []byte(text))
}

// WriteBinary writes a binary message to the connection.
// This is a convenience method for WriteMessage(BinaryMessage, data).
func (c *WebSocketConnection[T, U]) WriteBinary(data []byte) error {
	return c.WriteMessage(BinaryMessage, data)
}

// WriteJSON writes a JSON-encoded value to the connection.
// This method is safe for concurrent use.
func (c *WebSocketConnection[T, U]) WriteJSON(v any) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.overrides.WriteTimeout > 0 {
		_ = c.conn.SetWriteDeadline(time.Now().Add(c.overrides.WriteTimeout))
	}
	return c.conn.WriteJSON(v)
}

// ReadJSON reads a JSON-encoded message from the connection.
func (c *WebSocketConnection[T, U]) ReadJSON(v any) error {
	if c.overrides.ReadTimeout > 0 {
		_ = c.conn.SetReadDeadline(time.Now().Add(c.overrides.ReadTimeout))
	}
	return c.conn.ReadJSON(v)
}

// Close closes the WebSocket connection with a normal closure.
func (c *WebSocketConnection[T, U]) Close() error {
	return c.CloseWithCode(CloseNormalClosure, "")
}

// CloseWithCode closes the WebSocket connection with the specified close code and message.
// This method is safe for concurrent use.
func (c *WebSocketConnection[T, U]) CloseWithCode(code WebSocketCloseCode, message string) error {
	c.closeMu.Lock()
	defer c.closeMu.Unlock()

	if c.closed {
		return nil
	}
	c.closed = true

	// Stop ping ticker if running
	if c.pingTicker != nil {
		c.pingTicker.Stop()
	}

	// Signal close to any goroutines waiting on closeChan
	close(c.closeChan)

	// Send close message with write mutex protection
	c.writeMu.Lock()
	closeMessage := websocket.FormatCloseMessage(int(code), message)
	// Use min(1 second, WriteTimeout) for close message deadline
	closeDeadline := time.Second
	if c.overrides.WriteTimeout > 0 && c.overrides.WriteTimeout < closeDeadline {
		closeDeadline = c.overrides.WriteTimeout
	}
	_ = c.conn.SetWriteDeadline(time.Now().Add(closeDeadline))
	_ = c.conn.WriteMessage(websocket.CloseMessage, closeMessage)
	c.writeMu.Unlock()

	return c.conn.Close()
}

// IsClosed returns true if the connection has been closed.
func (c *WebSocketConnection[T, U]) IsClosed() bool {
	c.closeMu.Lock()
	defer c.closeMu.Unlock()
	return c.closed
}

// Done returns a channel that is closed when the connection is closed.
// This can be used to detect when the connection has been closed.
func (c *WebSocketConnection[T, U]) Done() <-chan struct{} {
	return c.closeChan
}

// SetReadLimit sets the maximum size in bytes for a message read from the peer.
// If a message exceeds the limit, the connection sends a close message and returns
// an error to the application.
func (c *WebSocketConnection[T, U]) SetReadLimit(limit int64) {
	c.conn.SetReadLimit(limit)
}

// Ping sends a ping message to the peer and waits for a pong response.
// This method is safe for concurrent use.
func (c *WebSocketConnection[T, U]) Ping() error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()
	if c.overrides.WriteTimeout > 0 {
		_ = c.conn.SetWriteDeadline(time.Now().Add(c.overrides.WriteTimeout))
	}
	return c.conn.WriteMessage(websocket.PingMessage, nil)
}

// startPingLoop starts a goroutine that sends periodic ping messages.
// This should be called after the connection is established if PingInterval is set.
func (c *WebSocketConnection[T, U]) startPingLoop() {
	if c.overrides.PingInterval <= 0 {
		return
	}

	pongTimeout := c.overrides.PongTimeout
	if pongTimeout <= 0 {
		pongTimeout = c.overrides.PingInterval + 10*time.Second
	}

	_ = c.conn.SetReadDeadline(time.Now().Add(pongTimeout))
	c.conn.SetPongHandler(func(string) error {
		return c.conn.SetReadDeadline(time.Now().Add(pongTimeout))
	})

	c.pingTicker = time.NewTicker(c.overrides.PingInterval)

	go func() {
		defer c.pingTicker.Stop()
		for {
			select {
			case <-c.pingTicker.C:
				if err := c.Ping(); err != nil {
					c.logger.Debug("Ping failed, closing connection",
						zap.Error(err),
						zap.String("remote_addr", c.RemoteAddr()),
					)
					c.Close()
					return
				}
			case <-c.closeChan:
				return
			}
		}
	}()
}

// WebSocketHandler is the function signature for WebSocket connection handlers.
// It receives the original HTTP request (for accessing headers, context, etc.)
// and the WebSocket connection wrapper.
//
// The handler should manage the WebSocket connection lifecycle:
// - Read and write messages as needed
// - Handle connection errors
// - Close the connection when done (or let it be closed automatically on return)
//
// If the handler returns an error, it will be logged. The connection will be
// closed automatically when the handler returns if it hasn't been closed already.
type WebSocketHandler[T comparable, U any] func(conn *WebSocketConnection[T, U]) error

// WebSocketRouteConfig defines the configuration for a WebSocket route.
// It follows the same patterns as RouteConfigBase but is specialized for WebSocket connections.
type WebSocketRouteConfig[T comparable, U any] struct {
	// Path is the route path (will be prefixed with sub-router path prefix if applicable).
	Path string

	// AuthLevel specifies the authentication level for this route.
	// If nil, inherits from sub-router or defaults to NoAuth.
	// Authentication is performed during the HTTP upgrade request, before
	// the WebSocket connection is established.
	AuthLevel *AuthLevel

	// Overrides contains WebSocket-specific configuration overrides for this route.
	Overrides WebSocketOverrides

	// Handler is the function that handles WebSocket connections.
	Handler WebSocketHandler[T, U]

	// Middlewares are applied during the HTTP upgrade request phase.
	// These middlewares run before the WebSocket connection is established.
	Middlewares []common.Middleware
}

// Implement RouteDefinition for WebSocketRouteConfig
func (WebSocketRouteConfig[T, U]) isRouteDefinition() {}

// upgraderFromOverrides creates a websocket.Upgrader from WebSocketOverrides.
func upgraderFromOverrides(overrides WebSocketOverrides) *websocket.Upgrader {
	upgrader := &websocket.Upgrader{
		ReadBufferSize:    overrides.ReadBufferSize,
		WriteBufferSize:   overrides.WriteBufferSize,
		HandshakeTimeout:  overrides.HandshakeTimeout,
		Subprotocols:      overrides.Subprotocols,
		EnableCompression: overrides.EnableCompression,
	}

	if overrides.CheckOrigin != nil {
		upgrader.CheckOrigin = overrides.CheckOrigin
	} else {
		// Default: allow all origins (common for development)
		// In production, you should set a proper CheckOrigin function
		upgrader.CheckOrigin = func(r *http.Request) bool {
			return true
		}
	}

	return upgrader
}

// wrapWebSocketHandler wraps a WebSocket handler with middleware and upgrade logic.
func (r *Router[T, U]) wrapWebSocketHandler(
	handler WebSocketHandler[T, U],
	authLevel *AuthLevel,
	overrides WebSocketOverrides,
	middlewares []common.Middleware,
) http.Handler {
	// Create the upgrader
	upgrader := upgraderFromOverrides(overrides)

	// Create the base handler that performs the WebSocket upgrade
	baseHandler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// Check shutdown
		r.wg.Add(1)
		defer r.wg.Done()
		r.shutdownMu.RLock()
		isShutdown := r.shutdown
		r.shutdownMu.RUnlock()
		if isShutdown {
			http.Error(w, "Service Unavailable", http.StatusServiceUnavailable)
			return
		}

		// Upgrade the connection
		conn, err := upgrader.Upgrade(w, req, nil)
		if err != nil {
			// Upgrader already sent the error response
			r.logger.Error("WebSocket upgrade failed",
				zap.Error(err),
				zap.String("path", req.URL.Path),
				zap.String("remote_addr", req.RemoteAddr),
			)
			return
		}

		// Create the WebSocket connection wrapper
		wsConn := &WebSocketConnection[T, U]{
			conn:      conn,
			request:   req,
			overrides: overrides,
			logger:    r.logger,
			closeChan: make(chan struct{}),
		}

		// Set read limit if specified
		if overrides.MaxMessageSize > 0 {
			wsConn.SetReadLimit(overrides.MaxMessageSize)
		}

		// Start ping loop if configured
		wsConn.startPingLoop()

		// Ensure connection is closed when handler returns
		defer func() {
			if !wsConn.IsClosed() {
				wsConn.Close()
			}
		}()

		// Call the handler
		if err := handler(wsConn); err != nil {
			// Log the error
			fields := []zap.Field{
				zap.Error(err),
				zap.String("path", req.URL.Path),
				zap.String("remote_addr", wsConn.RemoteAddr()),
			}
			if traceID := wsConn.TraceID(); traceID != "" {
				fields = append(fields, zap.String("trace_id", traceID))
			}
			r.logger.Error("WebSocket handler error", fields...)
		}
	})

	// Build the middleware chain (same pattern as wrapHandler)
	chain := common.NewMiddlewareChain()

	// 1. Recovery (catches panics during upgrade)
	chain = chain.Append(r.recoveryMiddleware)

	// 2. Trace middleware (if enabled)
	if r.traceIDGenerator != nil {
		traceMW := middleware.CreateTraceMiddleware[T, U](r.traceIDGenerator)
		chain = chain.Append(traceMW)
	}

	// 3. Authentication (performed during upgrade)
	if authLevel != nil {
		switch *authLevel {
		case AuthRequired:
			chain = chain.Append(r.authRequiredMiddleware)
		case AuthOptional:
			chain = chain.Append(r.authOptionalMiddleware)
		}
	}

	// 4. Route-specific middlewares
	chain = chain.Append(middlewares...)

	// 5. Global middlewares
	chain = chain.Append(r.middlewares...)

	// Note: We don't apply timeout middleware for WebSocket connections
	// as they are long-lived by nature

	return chain.Then(baseHandler)
}

// registerWebSocketRoute registers a WebSocket route with the router.
func (r *Router[T, U]) registerWebSocketRoute(
	path string,
	config WebSocketRouteConfig[T, U],
	subRouterMiddlewares []common.Middleware,
	subRouterAuthLevel *AuthLevel,
) {
	// Determine auth level
	authLevel := config.AuthLevel
	if authLevel == nil {
		authLevel = subRouterAuthLevel
	}

	// Combine middlewares: sub-router + route-specific
	allMiddlewares := make([]common.Middleware, 0, len(subRouterMiddlewares)+len(config.Middlewares))
	allMiddlewares = append(allMiddlewares, subRouterMiddlewares...)
	allMiddlewares = append(allMiddlewares, config.Middlewares...)

	// Create the wrapped handler
	handler := r.wrapWebSocketHandler(config.Handler, authLevel, config.Overrides, allMiddlewares)

	// Register with httprouter (WebSocket uses GET method for upgrade)
	r.router.Handle(http.MethodGet, path, r.convertToHTTPRouterHandle(handler, path))
}

// NewWebSocketRouteDefinition creates a WebSocket route definition that can be added to
// SubRouterConfig.Routes for declarative WebSocket route registration.
//
// Example:
//
//	subRouter := router.SubRouterConfig{
//	    PathPrefix: "/ws",
//	    Routes: []router.RouteDefinition{
//	        router.NewWebSocketRouteDefinition(router.WebSocketRouteConfig[string, User]{
//	            Path:    "/chat",
//	            Handler: chatHandler,
//	        }),
//	    },
//	}
func NewWebSocketRouteDefinition[T comparable, U any](config WebSocketRouteConfig[T, U]) GenericRouteRegistrationFunc[T, U] {
	return func(r *Router[T, U], sr SubRouterConfig) {
		fullPath := sr.PathPrefix + config.Path
		r.registerWebSocketRoute(fullPath, config, sr.Middlewares, sr.AuthLevel)
	}
}

// RegisterWebSocketRoute registers a WebSocket route directly on the router.
// This is useful for routes that don't belong to a sub-router.
//
// For routes within a sub-router, prefer using NewWebSocketRouteDefinition in
// SubRouterConfig.Routes for declarative configuration.
func (r *Router[T, U]) RegisterWebSocketRoute(config WebSocketRouteConfig[T, U]) {
	r.registerWebSocketRoute(config.Path, config, nil, nil)
}

// IsWebSocketUpgrade returns true if the request is a WebSocket upgrade request.
// This can be used in middleware or handlers to detect WebSocket requests.
func IsWebSocketUpgrade(r *http.Request) bool {
	return websocket.IsWebSocketUpgrade(r)
}

// WebSocketError represents an error that occurred during WebSocket communication.
type WebSocketError struct {
	Code    WebSocketCloseCode
	Message string
	Err     error
}

// Error implements the error interface.
func (e *WebSocketError) Error() string {
	if e.Err != nil {
		return e.Message + ": " + e.Err.Error()
	}
	return e.Message
}

// Unwrap returns the underlying error.
func (e *WebSocketError) Unwrap() error {
	return e.Err
}

// NewWebSocketError creates a new WebSocket error with the specified code and message.
func NewWebSocketError(code WebSocketCloseCode, message string) *WebSocketError {
	return &WebSocketError{
		Code:    code,
		Message: message,
	}
}

// IsCloseError returns true if the error is a WebSocket close error with one of the specified codes.
func IsCloseError(err error, codes ...WebSocketCloseCode) bool {
	intCodes := make([]int, len(codes))
	for i, code := range codes {
		intCodes[i] = int(code)
	}
	return websocket.IsCloseError(err, intCodes...)
}

// IsUnexpectedCloseError returns true if the error is a WebSocket close error
// that is NOT one of the specified codes.
func IsUnexpectedCloseError(err error, expectedCodes ...WebSocketCloseCode) bool {
	intCodes := make([]int, len(expectedCodes))
	for i, code := range expectedCodes {
		intCodes[i] = int(code)
	}
	return websocket.IsUnexpectedCloseError(err, intCodes...)
}
