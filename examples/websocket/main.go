package main

import (
	"context"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/router"
	"github.com/gorilla/websocket"
	"go.uber.org/zap"
)

var upgrader = websocket.Upgrader{
	ReadBufferSize:  1024,
	WriteBufferSize: 1024,
	// Allow all origins for this example
	CheckOrigin: func(r *http.Request) bool { return true },
}

func main() {
	// 1. Setup Server
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	routerConfig := router.RouterConfig{
		ServiceName:   "websocket-example",
		Logger:        logger,
		GlobalTimeout: 5 * time.Second, // Global timeout to test DisableTimeout bypass
	}

	// Simple auth - accept everything
	authFunc := func(ctx context.Context, token string) (*string, bool) {
		user := "generic-user"
		return &user, true
	}
	userIdFunc := func(user *string) string { return *user }

	r := router.NewRouter(routerConfig, authFunc, userIdFunc)

	// REST Endpoint
	r.RegisterRoute(router.RouteConfigBase{
		Path:    "/hello",
		Methods: []router.HttpMethod{router.MethodGet},
		Handler: func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			w.Write([]byte("Hello, World!"))
		},
	})

	// WebSocket Endpoint
	r.RegisterRoute(router.RouteConfigBase{
		Path:        "/ws",
		Methods:     []router.HttpMethod{router.MethodGet},
		DisableTimeout: true, // Crucial: disables global timeout
		Handler: func(w http.ResponseWriter, r *http.Request) {
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				logger.Error("upgrade failed", zap.Error(err))
				return
			}
			defer conn.Close()

			for {
				messageType, p, err := conn.ReadMessage()
				if err != nil {
					return
				}
				// Echo message back
				if err := conn.WriteMessage(messageType, p); err != nil {
					return
				}
			}
		},
	})

	// Start server in goroutine
	port := "8089"
	server := &http.Server{Addr: ":" + port, Handler: r}

	go func() {
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("ListenAndServe(): %v", err)
		}
	}()
	fmt.Printf("Server started on port %s\n", port)

	// Give server a moment to start
	time.Sleep(100 * time.Millisecond)

	// 2. Test Client Logic
	testREST(port)
	testWebSocket(port)

	// Shutdown
	server.Shutdown(context.Background())
	fmt.Println("Done.")
}

func testREST(port string) {
	fmt.Println("--- Testing REST Endpoint ---")
	resp, err := http.Get(fmt.Sprintf("http://localhost:%s/hello", port))
	if err != nil {
		log.Fatalf("REST request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		log.Fatalf("REST expected status 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	fmt.Printf("REST Response: %s\n", string(body))
	fmt.Println("REST Test Passed!")
}

func testWebSocket(port string) {
	fmt.Println("--- Testing WebSocket Endpoint ---")
	u := url.URL{Scheme: "ws", Host: "localhost:" + port, Path: "/ws"}

	c, _, err := websocket.DefaultDialer.Dial(u.String(), nil)
	if err != nil {
		log.Fatalf("WebSocket dial failed: %v", err)
	}
	defer c.Close()

	msg := "hello websocket"
	err = c.WriteMessage(websocket.TextMessage, []byte(msg))
	if err != nil {
		log.Fatalf("WebSocket write failed: %v", err)
	}

	_, message, err := c.ReadMessage()
	if err != nil {
		log.Fatalf("WebSocket read failed: %v", err)
	}

	fmt.Printf("WebSocket Response: %s\n", string(message))
	if string(message) != msg {
		log.Fatalf("WebSocket expected echo '%s', got '%s'", msg, string(message))
	}
	fmt.Println("WebSocket Test Passed!")
}
