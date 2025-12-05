package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/router"
	"go.uber.org/zap"
)

// Message represents a chat message
type Message struct {
	Type    string `json:"type"`
	Content string `json:"content"`
	From    string `json:"from,omitempty"`
	Time    string `json:"time,omitempty"`
}

// echoHandler is a simple WebSocket handler that echoes messages back
func echoHandler(conn *router.WebSocketConnection[string, string]) error {
	log.Printf("Client connected: %s", conn.RemoteAddr())

	// Send welcome message
	welcome := Message{
		Type:    "welcome",
		Content: "Welcome to the echo server!",
		Time:    time.Now().Format(time.RFC3339),
	}
	if err := conn.WriteJSON(welcome); err != nil {
		return err
	}

	// Echo loop
	for {
		// Read message
		var msg Message
		if err := conn.ReadJSON(&msg); err != nil {
			if router.IsCloseError(err, router.CloseNormalClosure, router.CloseGoingAway) {
				log.Printf("Client disconnected normally: %s", conn.RemoteAddr())
				return nil
			}
			return err
		}

		log.Printf("Received from %s: %s", conn.RemoteAddr(), msg.Content)

		// Echo the message back
		response := Message{
			Type:    "echo",
			Content: msg.Content,
			From:    "server",
			Time:    time.Now().Format(time.RFC3339),
		}
		if err := conn.WriteJSON(response); err != nil {
			return err
		}
	}
}

// authChatHandler demonstrates a WebSocket handler with authentication
func authChatHandler(conn *router.WebSocketConnection[string, string]) error {
	// Get the authenticated user ID
	userID, ok := conn.UserID()
	if !ok {
		return conn.WriteText("Error: Not authenticated")
	}

	log.Printf("Authenticated user %s connected from %s", userID, conn.RemoteAddr())

	// Send personalized welcome
	welcome := Message{
		Type:    "welcome",
		Content: fmt.Sprintf("Hello, %s! You are authenticated.", userID),
		Time:    time.Now().Format(time.RFC3339),
	}
	if err := conn.WriteJSON(welcome); err != nil {
		return err
	}

	// Message loop
	for {
		msgType, data, err := conn.ReadMessage()
		if err != nil {
			if router.IsCloseError(err, router.CloseNormalClosure, router.CloseGoingAway) {
				log.Printf("User %s disconnected", userID)
				return nil
			}
			return err
		}

		// Log and echo with user info
		log.Printf("Message from %s: %s", userID, string(data))

		response := Message{
			Type:    "message",
			Content: string(data),
			From:    userID,
			Time:    time.Now().Format(time.RFC3339),
		}

		jsonData, err := json.Marshal(response)
		if err != nil {
			return fmt.Errorf("failed to marshal response: %w", err)
		}
		if err := conn.WriteMessage(msgType, jsonData); err != nil {
			return err
		}
	}
}

// binaryHandler demonstrates handling binary WebSocket messages
func binaryHandler(conn *router.WebSocketConnection[string, string]) error {
	log.Printf("Binary client connected: %s", conn.RemoteAddr())

	for {
		msgType, data, err := conn.ReadMessage()
		if err != nil {
			if router.IsCloseError(err, router.CloseNormalClosure, router.CloseGoingAway) {
				return nil
			}
			return err
		}

		log.Printf("Received %d bytes (type: %d) from %s", len(data), msgType, conn.RemoteAddr())

		// Echo binary data back
		if err := conn.WriteMessage(msgType, data); err != nil {
			return err
		}
	}
}

// pingPongHandler demonstrates the ping/pong keep-alive feature
func pingPongHandler(conn *router.WebSocketConnection[string, string]) error {
	log.Printf("Ping/pong client connected: %s", conn.RemoteAddr())

	// The ping loop is automatically started if PingInterval is configured
	// Here we just demonstrate a simple message loop

	for {
		_, data, err := conn.ReadMessage()
		if err != nil {
			if router.IsCloseError(err, router.CloseNormalClosure, router.CloseGoingAway) {
				log.Printf("Ping/pong client disconnected: %s", conn.RemoteAddr())
				return nil
			}
			return err
		}

		log.Printf("Received message: %s", string(data))
		if err := conn.WriteText("pong: " + string(data)); err != nil {
			return err
		}
	}
}

func main() {
	// Create a logger
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	authRequired := router.AuthRequired

	// Create router configuration with WebSocket routes
	config := router.RouterConfig{
		ServiceName:       "websocket-example",
		Logger:            logger,
		TraceIDBufferSize: 100,
		SubRouters: []router.SubRouterConfig{
			{
				PathPrefix: "/ws",
				Routes: []router.RouteDefinition{
					// Simple echo WebSocket endpoint
					router.WebSocketRouteConfig[string, string]{
						Path:    "/echo",
						Handler: echoHandler,
						Overrides: router.WebSocketOverrides{
							ReadBufferSize:  1024,
							WriteBufferSize: 1024,
							MaxMessageSize:  4096,
						},
					},

					// Binary message handling endpoint
					router.WebSocketRouteConfig[string, string]{
						Path:    "/binary",
						Handler: binaryHandler,
					},

					// WebSocket with ping/pong keep-alive
					router.WebSocketRouteConfig[string, string]{
						Path:    "/keepalive",
						Handler: pingPongHandler,
						Overrides: router.WebSocketOverrides{
							PingInterval: 30 * time.Second,
							PongTimeout:  60 * time.Second,
						},
					},
				},
			},
			{
				PathPrefix: "/api",
				Routes: []router.RouteDefinition{
					// Authenticated WebSocket endpoint using NewWebSocketRouteDefinition
					router.NewWebSocketRouteDefinition(router.WebSocketRouteConfig[string, string]{
						Path:      "/chat",
						AuthLevel: &authRequired,
						Handler:   authChatHandler,
					}),
				},
			},
		},
	}

	// Auth function - validates Bearer tokens
	authFunc := func(ctx context.Context, token string) (*string, bool) {
		// In a real application, validate the token properly
		// For this example, we accept any non-empty token as the username
		if token != "" {
			return &token, true
		}
		return nil, false
	}

	// User ID extraction function
	userIDFunc := func(user *string) string {
		if user == nil {
			return ""
		}
		return *user
	}

	// Create the router
	r := router.NewRouter(config, authFunc, userIDFunc)

	// Add a simple HTTP health check endpoint
	r.RegisterRoute(router.RouteConfigBase{
		Path:    "/health",
		Methods: []router.HttpMethod{router.MethodGet},
		Handler: func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(`{"status":"ok"}`))
		},
	})

	// Start the server
	addr := ":8080"
	fmt.Printf("WebSocket server starting on %s\n", addr)
	fmt.Println("")
	fmt.Println("Available endpoints:")
	fmt.Println("  ws://localhost:8080/ws/echo      - Echo server (no auth)")
	fmt.Println("  ws://localhost:8080/ws/binary    - Binary message echo (no auth)")
	fmt.Println("  ws://localhost:8080/ws/keepalive - Ping/pong keep-alive (no auth)")
	fmt.Println("  ws://localhost:8080/api/chat     - Authenticated chat (requires Bearer token)")
	fmt.Println("  http://localhost:8080/health     - Health check")
	fmt.Println("")
	fmt.Println("Example client usage:")
	fmt.Println("  websocat ws://localhost:8080/ws/echo")
	fmt.Println("  websocat -H 'Authorization: Bearer myuser' ws://localhost:8080/api/chat")
	fmt.Println("")

	log.Fatal(http.ListenAndServe(addr, r))
}
