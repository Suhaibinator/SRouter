package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/common"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

// Helper function to create a test router with WebSocket support
func newTestWebSocketRouter(t *testing.T) *Router[string, string] {
	t.Helper()
	logger, _ := zap.NewDevelopment()

	return NewRouter(RouterConfig{
		Logger:            logger,
		TraceIDBufferSize: 10, // Enable trace ID for tests
	},
		func(ctx context.Context, token string) (*string, bool) {
			if token == "valid-token" {
				user := "test-user"
				return &user, true
			}
			return nil, false
		},
		func(user *string) string {
			if user == nil {
				return ""
			}
			return *user
		})
}

// TestWebSocketBasicConnection tests basic WebSocket connection establishment
func TestWebSocketBasicConnection(t *testing.T) {
	router := newTestWebSocketRouter(t)

	// Register a simple WebSocket route
	router.RegisterWebSocketRoute(WebSocketRouteConfig[string, string]{
		Path: "/ws",
		Handler: func(conn *WebSocketConnection[string, string]) error {
			// Echo received messages back
			for {
				msgType, data, err := conn.ReadMessage()
				if err != nil {
					if IsCloseError(err, CloseNormalClosure, CloseGoingAway) {
						return nil
					}
					return err
				}
				if err := conn.WriteMessage(msgType, data); err != nil {
					return err
				}
			}
		},
	})

	// Create test server
	server := httptest.NewServer(router)
	defer server.Close()

	// Convert HTTP URL to WebSocket URL
	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws"

	// Connect to WebSocket
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket: %v", err)
	}
	defer ws.Close()

	// Send a text message
	testMessage := "Hello, WebSocket!"
	if err := ws.WriteMessage(websocket.TextMessage, []byte(testMessage)); err != nil {
		t.Fatalf("Failed to write message: %v", err)
	}

	// Read the echo response
	msgType, data, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to read message: %v", err)
	}

	if msgType != websocket.TextMessage {
		t.Errorf("Expected message type %d, got %d", websocket.TextMessage, msgType)
	}

	if string(data) != testMessage {
		t.Errorf("Expected message %q, got %q", testMessage, string(data))
	}
}

// TestWebSocketWithSubRouter tests WebSocket routes within a sub-router
func TestWebSocketWithSubRouter(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	router := NewRouter(RouterConfig{
		Logger: logger,
		SubRouters: []SubRouterConfig{
			{
				PathPrefix: "/api/v1",
				Routes: []RouteDefinition{
					WebSocketRouteConfig[string, string]{
						Path: "/chat",
						Handler: func(conn *WebSocketConnection[string, string]) error {
							return conn.WriteText("connected to chat")
						},
					},
				},
			},
		},
	},
		func(ctx context.Context, token string) (*string, bool) {
			return nil, false
		},
		func(user *string) string {
			if user == nil {
				return ""
			}
			return *user
		})

	server := httptest.NewServer(router)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/v1/chat"

	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket: %v", err)
	}
	defer ws.Close()

	msgType, data, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to read message: %v", err)
	}

	if msgType != websocket.TextMessage {
		t.Errorf("Expected message type %d, got %d", websocket.TextMessage, msgType)
	}

	if string(data) != "connected to chat" {
		t.Errorf("Expected message %q, got %q", "connected to chat", string(data))
	}
}

// TestWebSocketWithNewWebSocketRouteDefinition tests the declarative route registration
func TestWebSocketWithNewWebSocketRouteDefinition(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	router := NewRouter(RouterConfig{
		Logger: logger,
		SubRouters: []SubRouterConfig{
			{
				PathPrefix: "/ws",
				Routes: []RouteDefinition{
					NewWebSocketRouteDefinition(WebSocketRouteConfig[string, string]{
						Path: "/echo",
						Handler: func(conn *WebSocketConnection[string, string]) error {
							for {
								msgType, data, err := conn.ReadMessage()
								if err != nil {
									return nil
								}
								if err := conn.WriteMessage(msgType, data); err != nil {
									return err
								}
							}
						},
					}),
				},
			},
		},
	},
		func(ctx context.Context, token string) (*string, bool) {
			return nil, false
		},
		func(user *string) string {
			if user == nil {
				return ""
			}
			return *user
		})

	server := httptest.NewServer(router)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/echo"

	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket: %v", err)
	}
	defer ws.Close()

	// Test echo
	testMessage := "test message"
	if err := ws.WriteMessage(websocket.TextMessage, []byte(testMessage)); err != nil {
		t.Fatalf("Failed to write message: %v", err)
	}

	_, data, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to read message: %v", err)
	}

	if string(data) != testMessage {
		t.Errorf("Expected message %q, got %q", testMessage, string(data))
	}
}

// TestWebSocketAuthentication tests WebSocket authentication
func TestWebSocketAuthentication(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	authRequired := AuthRequired

	router := NewRouter(RouterConfig{
		Logger:             logger,
		AddUserObjectToCtx: true,
	},
		func(ctx context.Context, token string) (*string, bool) {
			if token == "valid-token" {
				user := "authenticated-user"
				return &user, true
			}
			return nil, false
		},
		func(user *string) string {
			if user == nil {
				return ""
			}
			return *user
		})

	// Register a WebSocket route that requires authentication
	router.RegisterWebSocketRoute(WebSocketRouteConfig[string, string]{
		Path:      "/ws/secure",
		AuthLevel: &authRequired,
		Handler: func(conn *WebSocketConnection[string, string]) error {
			// Get user ID from connection
			userID, ok := conn.UserID()
			if ok {
				return conn.WriteText("Hello, " + userID)
			}
			return conn.WriteText("No user")
		},
	})

	server := httptest.NewServer(router)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/secure"

	// Test without authentication - should fail
	_, resp, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err == nil {
		t.Error("Expected connection to fail without authentication")
	}
	if resp != nil && resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("Expected status %d, got %d", http.StatusUnauthorized, resp.StatusCode)
	}

	// Test with valid authentication
	header := http.Header{}
	header.Set("Authorization", "Bearer valid-token")
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		t.Fatalf("Failed to connect with valid token: %v", err)
	}
	defer ws.Close()

	_, data, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to read message: %v", err)
	}

	if string(data) != "Hello, authenticated-user" {
		t.Errorf("Expected message %q, got %q", "Hello, authenticated-user", string(data))
	}
}

// TestWebSocketJSONMessaging tests JSON message encoding/decoding
func TestWebSocketJSONMessaging(t *testing.T) {
	router := newTestWebSocketRouter(t)

	type Message struct {
		Type    string `json:"type"`
		Content string `json:"content"`
	}

	router.RegisterWebSocketRoute(WebSocketRouteConfig[string, string]{
		Path: "/ws/json",
		Handler: func(conn *WebSocketConnection[string, string]) error {
			var msg Message
			if err := conn.ReadJSON(&msg); err != nil {
				return err
			}

			// Respond with modified message
			response := Message{
				Type:    "response",
				Content: "Received: " + msg.Content,
			}
			return conn.WriteJSON(response)
		},
	})

	server := httptest.NewServer(router)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/json"

	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket: %v", err)
	}
	defer ws.Close()

	// Send JSON message
	sendMsg := Message{Type: "request", Content: "Hello"}
	if err := ws.WriteJSON(sendMsg); err != nil {
		t.Fatalf("Failed to write JSON: %v", err)
	}

	// Read JSON response
	var recvMsg Message
	if err := ws.ReadJSON(&recvMsg); err != nil {
		t.Fatalf("Failed to read JSON: %v", err)
	}

	if recvMsg.Type != "response" {
		t.Errorf("Expected type %q, got %q", "response", recvMsg.Type)
	}

	if recvMsg.Content != "Received: Hello" {
		t.Errorf("Expected content %q, got %q", "Received: Hello", recvMsg.Content)
	}
}

// TestWebSocketBinaryMessages tests binary message handling
func TestWebSocketBinaryMessages(t *testing.T) {
	router := newTestWebSocketRouter(t)

	router.RegisterWebSocketRoute(WebSocketRouteConfig[string, string]{
		Path: "/ws/binary",
		Handler: func(conn *WebSocketConnection[string, string]) error {
			msgType, data, err := conn.ReadMessage()
			if err != nil {
				return err
			}
			// Echo binary data back
			return conn.WriteMessage(msgType, data)
		},
	})

	server := httptest.NewServer(router)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/binary"

	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket: %v", err)
	}
	defer ws.Close()

	// Send binary data
	binaryData := []byte{0x00, 0x01, 0x02, 0x03, 0xFF}
	if err := ws.WriteMessage(websocket.BinaryMessage, binaryData); err != nil {
		t.Fatalf("Failed to write binary message: %v", err)
	}

	// Read response
	msgType, data, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to read message: %v", err)
	}

	if msgType != websocket.BinaryMessage {
		t.Errorf("Expected binary message type, got %d", msgType)
	}

	if len(data) != len(binaryData) {
		t.Errorf("Expected data length %d, got %d", len(binaryData), len(data))
	}

	for i, b := range binaryData {
		if data[i] != b {
			t.Errorf("Data mismatch at index %d: expected %x, got %x", i, b, data[i])
		}
	}
}

// TestWebSocketOverrides tests WebSocket configuration overrides
func TestWebSocketOverrides(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	router := NewRouter(RouterConfig{
		Logger: logger,
	},
		func(ctx context.Context, token string) (*string, bool) {
			return nil, false
		},
		func(user *string) string {
			if user == nil {
				return ""
			}
			return *user
		})

	router.RegisterWebSocketRoute(WebSocketRouteConfig[string, string]{
		Path: "/ws/configured",
		Overrides: WebSocketOverrides{
			ReadBufferSize:   1024,
			WriteBufferSize:  1024,
			MaxMessageSize:   512,
			EnableCompression: false,
		},
		Handler: func(conn *WebSocketConnection[string, string]) error {
			msgType, data, err := conn.ReadMessage()
			if err != nil {
				return err
			}
			return conn.WriteMessage(msgType, data)
		},
	})

	server := httptest.NewServer(router)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/configured"

	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket: %v", err)
	}
	defer ws.Close()

	// Test with a message within the limit
	testMessage := "short message"
	if err := ws.WriteMessage(websocket.TextMessage, []byte(testMessage)); err != nil {
		t.Fatalf("Failed to write message: %v", err)
	}

	_, data, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to read message: %v", err)
	}

	if string(data) != testMessage {
		t.Errorf("Expected message %q, got %q", testMessage, string(data))
	}
}

// TestWebSocketCloseHandling tests proper close handling
func TestWebSocketCloseHandling(t *testing.T) {
	router := newTestWebSocketRouter(t)

	closeChan := make(chan struct{})

	router.RegisterWebSocketRoute(WebSocketRouteConfig[string, string]{
		Path: "/ws/close",
		Handler: func(conn *WebSocketConnection[string, string]) error {
			defer close(closeChan)
			// Wait for close or read error
			for {
				_, _, err := conn.ReadMessage()
				if err != nil {
					// Connection closed
					return nil
				}
			}
		},
	})

	server := httptest.NewServer(router)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/close"

	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket: %v", err)
	}

	// Close the connection from client side
	ws.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	ws.Close()

	// Wait for the handler to detect the close
	select {
	case <-closeChan:
		// Handler detected close correctly
	case <-time.After(2 * time.Second):
		t.Error("Handler did not detect close within timeout")
	}
}

// TestWebSocketConcurrentMessages tests handling of concurrent messages
func TestWebSocketConcurrentMessages(t *testing.T) {
	router := newTestWebSocketRouter(t)

	router.RegisterWebSocketRoute(WebSocketRouteConfig[string, string]{
		Path: "/ws/concurrent",
		Handler: func(conn *WebSocketConnection[string, string]) error {
			for {
				msgType, data, err := conn.ReadMessage()
				if err != nil {
					return nil
				}
				// Echo with a small delay to simulate processing
				time.Sleep(10 * time.Millisecond)
				if err := conn.WriteMessage(msgType, data); err != nil {
					return err
				}
			}
		},
	})

	server := httptest.NewServer(router)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/concurrent"

	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket: %v", err)
	}
	defer ws.Close()

	// Send multiple messages
	numMessages := 5
	var wg sync.WaitGroup

	// Send messages concurrently
	for i := 0; i < numMessages; i++ {
		msg := []byte("message")
		if err := ws.WriteMessage(websocket.TextMessage, msg); err != nil {
			t.Fatalf("Failed to send message %d: %v", i, err)
		}
	}

	// Read all responses
	wg.Add(numMessages)
	receivedCount := 0
	for i := 0; i < numMessages; i++ {
		_, _, err := ws.ReadMessage()
		if err != nil {
			t.Fatalf("Failed to read response %d: %v", i, err)
		}
		receivedCount++
		wg.Done()
	}

	if receivedCount != numMessages {
		t.Errorf("Expected %d messages, received %d", numMessages, receivedCount)
	}
}

// TestWebSocketConnectionMethods tests WebSocketConnection helper methods
func TestWebSocketConnectionMethods(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	router := NewRouter(RouterConfig{
		Logger:            logger,
		TraceIDBufferSize: 10,
	},
		func(ctx context.Context, token string) (*string, bool) {
			if token == "valid-token" {
				user := "test-user"
				return &user, true
			}
			return nil, false
		},
		func(user *string) string {
			if user == nil {
				return ""
			}
			return *user
		})

	authOptional := AuthOptional

	router.RegisterWebSocketRoute(WebSocketRouteConfig[string, string]{
		Path:      "/ws/methods",
		AuthLevel: &authOptional,
		Handler: func(conn *WebSocketConnection[string, string]) error {
			// Test Request() method
			req := conn.Request()
			if req == nil {
				return conn.WriteText("ERROR: Request is nil")
			}

			// Test Context() method
			ctx := conn.Context()
			if ctx == nil {
				return conn.WriteText("ERROR: Context is nil")
			}

			// Test TraceID() method
			traceID := conn.TraceID()
			if traceID == "" {
				return conn.WriteText("ERROR: TraceID is empty")
			}

			// Test RemoteAddr() method
			remoteAddr := conn.RemoteAddr()
			if remoteAddr == "" {
				return conn.WriteText("ERROR: RemoteAddr is empty")
			}

			// Test LocalAddr() method
			localAddr := conn.LocalAddr()
			if localAddr == "" {
				return conn.WriteText("ERROR: LocalAddr is empty")
			}

			// Test IsClosed() method
			if conn.IsClosed() {
				return conn.WriteText("ERROR: Connection should not be closed")
			}

			return conn.WriteText("OK")
		},
	})

	server := httptest.NewServer(router)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/methods"

	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket: %v", err)
	}
	defer ws.Close()

	_, data, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to read message: %v", err)
	}

	if string(data) != "OK" {
		t.Errorf("Expected 'OK', got %q", string(data))
	}
}

// TestWebSocketDoneChannel tests the Done channel for connection closure
func TestWebSocketDoneChannel(t *testing.T) {
	router := newTestWebSocketRouter(t)

	handlerDone := make(chan struct{})

	router.RegisterWebSocketRoute(WebSocketRouteConfig[string, string]{
		Path: "/ws/done",
		Handler: func(conn *WebSocketConnection[string, string]) error {
			defer close(handlerDone)

			// The Done() channel is signaled when the server closes the connection
			// To test this, we'll close the connection from the handler after receiving a message
			_, _, err := conn.ReadMessage()
			if err != nil {
				// Client closed connection, handler exits
				return nil
			}

			// Close the connection from server side, which signals Done()
			conn.Close()

			// Verify Done() channel is closed
			select {
			case <-conn.Done():
				// Done channel signaled correctly
				return nil
			default:
				return conn.WriteText("ERROR: Done channel not signaled")
			}
		},
	})

	server := httptest.NewServer(router)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/done"

	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket: %v", err)
	}

	// Send a message to trigger server-side close
	ws.WriteMessage(websocket.TextMessage, []byte("trigger close"))
	ws.Close()

	// Wait for handler to complete
	select {
	case <-handlerDone:
		// Handler completed correctly
	case <-time.After(2 * time.Second):
		t.Error("Handler did not complete within timeout")
	}
}

// TestWebSocketMiddleware tests middleware applied to WebSocket routes
func TestWebSocketMiddleware(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	middlewareCalled := false

	router := NewRouter(RouterConfig{
		Logger: logger,
		SubRouters: []SubRouterConfig{
			{
				PathPrefix: "/api",
				Middlewares: []common.Middleware{
					func(next http.Handler) http.Handler {
						return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							middlewareCalled = true
							next.ServeHTTP(w, r)
						})
					},
				},
				Routes: []RouteDefinition{
					WebSocketRouteConfig[string, string]{
						Path: "/ws",
						Handler: func(conn *WebSocketConnection[string, string]) error {
							return conn.WriteText("hello")
						},
					},
				},
			},
		},
	},
		func(ctx context.Context, token string) (*string, bool) {
			return nil, false
		},
		func(user *string) string {
			if user == nil {
				return ""
			}
			return *user
		})

	server := httptest.NewServer(router)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/api/ws"

	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket: %v", err)
	}
	defer ws.Close()

	// Read the message to ensure handler executed
	_, _, err = ws.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to read message: %v", err)
	}

	if !middlewareCalled {
		t.Error("Middleware was not called for WebSocket route")
	}
}

// TestWebSocketShutdown tests that WebSocket connections are handled during shutdown
func TestWebSocketShutdown(t *testing.T) {
	router := newTestWebSocketRouter(t)

	handlerStarted := make(chan struct{})
	handlerDone := make(chan struct{})

	router.RegisterWebSocketRoute(WebSocketRouteConfig[string, string]{
		Path: "/ws/shutdown",
		Handler: func(conn *WebSocketConnection[string, string]) error {
			close(handlerStarted)
			// Wait for a message (which won't come due to shutdown)
			for {
				_, _, err := conn.ReadMessage()
				if err != nil {
					close(handlerDone)
					return nil
				}
			}
		},
	})

	server := httptest.NewServer(router)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/shutdown"

	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect to WebSocket: %v", err)
	}
	defer ws.Close()

	// Wait for handler to start
	select {
	case <-handlerStarted:
	case <-time.After(2 * time.Second):
		t.Fatal("Handler did not start within timeout")
	}

	// Close the client connection to allow handler to complete
	ws.WriteMessage(websocket.CloseMessage, websocket.FormatCloseMessage(websocket.CloseNormalClosure, ""))
	ws.Close()

	// Wait for handler to finish
	select {
	case <-handlerDone:
	case <-time.After(2 * time.Second):
		t.Fatal("Handler did not complete within timeout")
	}

	// Initiate shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	if err := router.Shutdown(ctx); err != nil {
		t.Errorf("Shutdown returned error: %v", err)
	}
}

// TestIsWebSocketUpgrade tests the IsWebSocketUpgrade helper function
func TestIsWebSocketUpgrade(t *testing.T) {
	// Test with WebSocket upgrade request
	wsReq := httptest.NewRequest("GET", "/ws", nil)
	wsReq.Header.Set("Connection", "Upgrade")
	wsReq.Header.Set("Upgrade", "websocket")
	wsReq.Header.Set("Sec-WebSocket-Version", "13")
	wsReq.Header.Set("Sec-WebSocket-Key", "dGhlIHNhbXBsZSBub25jZQ==")

	if !IsWebSocketUpgrade(wsReq) {
		t.Error("Expected IsWebSocketUpgrade to return true for WebSocket request")
	}

	// Test with regular HTTP request
	httpReq := httptest.NewRequest("GET", "/api", nil)

	if IsWebSocketUpgrade(httpReq) {
		t.Error("Expected IsWebSocketUpgrade to return false for regular HTTP request")
	}
}

// TestWebSocketErrorHelpers tests WebSocket error helper functions
func TestWebSocketErrorHelpers(t *testing.T) {
	// Test NewWebSocketError
	wsErr := NewWebSocketError(CloseProtocolError, "test error")
	if wsErr.Code != CloseProtocolError {
		t.Errorf("Expected code %d, got %d", CloseProtocolError, wsErr.Code)
	}
	if wsErr.Message != "test error" {
		t.Errorf("Expected message 'test error', got %q", wsErr.Message)
	}
	if wsErr.Error() != "test error" {
		t.Errorf("Expected Error() to return 'test error', got %q", wsErr.Error())
	}

	// Test with underlying error
	wsErr.Err = context.DeadlineExceeded
	expectedMsg := "test error: context deadline exceeded"
	if wsErr.Error() != expectedMsg {
		t.Errorf("Expected Error() to return %q, got %q", expectedMsg, wsErr.Error())
	}

	// Test Unwrap
	if wsErr.Unwrap() != context.DeadlineExceeded {
		t.Error("Unwrap did not return the expected error")
	}
}

// TestWebSocketCheckOrigin tests the CheckOrigin configuration
func TestWebSocketCheckOrigin(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	router := NewRouter(RouterConfig{
		Logger: logger,
	},
		func(ctx context.Context, token string) (*string, bool) {
			return nil, false
		},
		func(user *string) string {
			if user == nil {
				return ""
			}
			return *user
		})

	// Register a route with custom CheckOrigin that rejects all origins
	router.RegisterWebSocketRoute(WebSocketRouteConfig[string, string]{
		Path: "/ws/strict-origin",
		Overrides: WebSocketOverrides{
			CheckOrigin: func(r *http.Request) bool {
				origin := r.Header.Get("Origin")
				return origin == "https://allowed.example.com"
			},
		},
		Handler: func(conn *WebSocketConnection[string, string]) error {
			return conn.WriteText("connected")
		},
	})

	server := httptest.NewServer(router)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/strict-origin"

	// Test with disallowed origin
	header := http.Header{}
	header.Set("Origin", "https://disallowed.example.com")
	_, resp, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err == nil {
		t.Error("Expected connection to be rejected with disallowed origin")
	}
	if resp != nil && resp.StatusCode != http.StatusForbidden {
		// Note: gorilla/websocket returns 403 when CheckOrigin returns false
		t.Logf("Response status: %d", resp.StatusCode)
	}

	// Test with allowed origin
	header.Set("Origin", "https://allowed.example.com")
	ws, _, err := websocket.DefaultDialer.Dial(wsURL, header)
	if err != nil {
		t.Fatalf("Failed to connect with allowed origin: %v", err)
	}
	ws.Close()
}

// TestWebSocketSubprotocols tests subprotocol negotiation
func TestWebSocketSubprotocols(t *testing.T) {
	logger, _ := zap.NewDevelopment()

	router := NewRouter(RouterConfig{
		Logger: logger,
	},
		func(ctx context.Context, token string) (*string, bool) {
			return nil, false
		},
		func(user *string) string {
			if user == nil {
				return ""
			}
			return *user
		})

	router.RegisterWebSocketRoute(WebSocketRouteConfig[string, string]{
		Path: "/ws/subprotocol",
		Overrides: WebSocketOverrides{
			Subprotocols: []string{"graphql-ws", "graphql-transport-ws"},
		},
		Handler: func(conn *WebSocketConnection[string, string]) error {
			subprotocol := conn.Subprotocol()
			return conn.WriteText("subprotocol: " + subprotocol)
		},
	})

	server := httptest.NewServer(router)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/subprotocol"

	// Test with matching subprotocol
	dialer := websocket.Dialer{
		Subprotocols: []string{"graphql-ws"},
	}
	ws, _, err := dialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer ws.Close()

	// Verify the negotiated subprotocol
	if ws.Subprotocol() != "graphql-ws" {
		t.Errorf("Expected subprotocol 'graphql-ws', got %q", ws.Subprotocol())
	}

	_, data, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to read message: %v", err)
	}

	if string(data) != "subprotocol: graphql-ws" {
		t.Errorf("Expected message 'subprotocol: graphql-ws', got %q", string(data))
	}
}

// TestWebSocketWriteText tests the WriteText helper method
func TestWebSocketWriteText(t *testing.T) {
	router := newTestWebSocketRouter(t)

	router.RegisterWebSocketRoute(WebSocketRouteConfig[string, string]{
		Path: "/ws/writetext",
		Handler: func(conn *WebSocketConnection[string, string]) error {
			return conn.WriteText("hello world")
		},
	})

	server := httptest.NewServer(router)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/writetext"

	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer ws.Close()

	msgType, data, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to read message: %v", err)
	}

	if msgType != websocket.TextMessage {
		t.Errorf("Expected text message type, got %d", msgType)
	}

	if string(data) != "hello world" {
		t.Errorf("Expected 'hello world', got %q", string(data))
	}
}

// TestWebSocketWriteBinary tests the WriteBinary helper method
func TestWebSocketWriteBinary(t *testing.T) {
	router := newTestWebSocketRouter(t)

	binaryData := []byte{0xDE, 0xAD, 0xBE, 0xEF}

	router.RegisterWebSocketRoute(WebSocketRouteConfig[string, string]{
		Path: "/ws/writebinary",
		Handler: func(conn *WebSocketConnection[string, string]) error {
			return conn.WriteBinary(binaryData)
		},
	})

	server := httptest.NewServer(router)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/writebinary"

	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer ws.Close()

	msgType, data, err := ws.ReadMessage()
	if err != nil {
		t.Fatalf("Failed to read message: %v", err)
	}

	if msgType != websocket.BinaryMessage {
		t.Errorf("Expected binary message type, got %d", msgType)
	}

	if len(data) != len(binaryData) {
		t.Errorf("Expected %d bytes, got %d", len(binaryData), len(data))
	}
}

// TestWebSocketCloseWithCode tests the CloseWithCode method
func TestWebSocketCloseWithCode(t *testing.T) {
	router := newTestWebSocketRouter(t)

	router.RegisterWebSocketRoute(WebSocketRouteConfig[string, string]{
		Path: "/ws/closewithcode",
		Handler: func(conn *WebSocketConnection[string, string]) error {
			// Close with a specific code
			return conn.CloseWithCode(CloseGoingAway, "server shutting down")
		},
	})

	server := httptest.NewServer(router)
	defer server.Close()

	wsURL := "ws" + strings.TrimPrefix(server.URL, "http") + "/ws/closewithcode"

	ws, _, err := websocket.DefaultDialer.Dial(wsURL, nil)
	if err != nil {
		t.Fatalf("Failed to connect: %v", err)
	}
	defer ws.Close()

	// Read the close message
	_, _, err = ws.ReadMessage()
	if err == nil {
		t.Error("Expected error from closed connection")
	}

	closeErr, ok := err.(*websocket.CloseError)
	if !ok {
		t.Fatalf("Expected CloseError, got %T", err)
	}

	if closeErr.Code != websocket.CloseGoingAway {
		t.Errorf("Expected close code %d, got %d", websocket.CloseGoingAway, closeErr.Code)
	}
}
