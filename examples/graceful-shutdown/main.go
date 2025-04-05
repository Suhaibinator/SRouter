package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/router"
	"go.uber.org/zap"
)

// Counter for active requests
var activeRequests int32

// SlowHandler is a handler that simulates a slow operation
func SlowHandler(w http.ResponseWriter, r *http.Request) {
	// Increment active requests counter
	atomic.AddInt32(&activeRequests, 1)
	defer atomic.AddInt32(&activeRequests, -1)

	// Get the duration from the query parameter
	durationStr := r.URL.Query().Get("duration")
	duration := 5 * time.Second // Default duration
	if durationStr != "" {
		var err error
		parsedDuration, err := time.ParseDuration(durationStr)
		if err == nil {
			duration = parsedDuration
		}
	}

	// Log the start of the request
	fmt.Printf("Starting slow request with duration %s\n", duration)

	// Simulate a slow operation
	select {
	case <-time.After(duration):
		// Operation completed successfully
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{"message":"Slow operation completed successfully"}`))
		fmt.Println("Slow request completed")
	case <-r.Context().Done():
		// Request was canceled (e.g., due to timeout or shutdown)
		fmt.Println("Slow request canceled")
		return
	}
}

// QuickHandler is a handler that returns immediately
func QuickHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"message":"Quick operation completed"}`))
}

// StatusHandler returns the current server status
func StatusHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(fmt.Sprintf(`{"active_requests":%d}`, atomic.LoadInt32(&activeRequests))))
}

func main() {
	// Create a logger
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	// Create a router configuration
	routerConfig := router.RouterConfig{
		Logger:            logger,
		GlobalTimeout:     30 * time.Second,
		GlobalMaxBodySize: 1 << 20, // 1 MB
	}

	// Define the auth function that takes a context and token and returns a string and a boolean
	authFunction := func(ctx context.Context, token string) (string, bool) {
		// This is a simple example, so we'll just validate that the token is not empty
		if token != "" {
			return token, true
		}
		return "", false
	}

	// Define the function to get the user ID from a string
	userIdFromUserFunction := func(user string) string {
		// In this example, we're using the string itself as the ID
		return user
	}

	// Create a router with string as both the user ID and user type
	r := router.NewRouter[string, string](routerConfig, authFunction, userIdFromUserFunction)

	// Register routes
	r.RegisterRoute(router.RouteConfigBase{
		Path:    "/slow",
		Methods: []router.HttpMethod{router.MethodGet},
		Handler: SlowHandler,
	})

	r.RegisterRoute(router.RouteConfigBase{
		Path:    "/quick",
		Methods: []router.HttpMethod{router.MethodGet},
		Handler: QuickHandler,
	})

	r.RegisterRoute(router.RouteConfigBase{
		Path:    "/status",
		Methods: []router.HttpMethod{router.MethodGet},
		Handler: StatusHandler,
	})

	// Create a server
	srv := &http.Server{
		Addr:    ":8080",
		Handler: r,
	}

	// Channel to listen for interrupt signals
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)

	// Start the server in a goroutine
	go func() {
		fmt.Println("Graceful Shutdown Example Server listening on :8080")
		fmt.Println("Available endpoints:")
		fmt.Println("  - GET /slow?duration=5s (slow operation that takes the specified duration)")
		fmt.Println("  - GET /quick (quick operation that returns immediately)")
		fmt.Println("  - GET /status (returns the number of active requests)")
		fmt.Println("\nExample curl commands:")
		fmt.Println("  curl http://localhost:8080/slow?duration=10s")
		fmt.Println("  curl http://localhost:8080/quick")
		fmt.Println("  curl http://localhost:8080/status")
		fmt.Println("\nPress Ctrl+C to initiate graceful shutdown")

		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("listen: %s\n", err)
		}
	}()

	// Wait for interrupt signal
	<-stop
	fmt.Println("\nShutdown signal received, initiating graceful shutdown...")

	// Create a deadline for the shutdown
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Shutdown the router first (this will stop accepting new requests)
	fmt.Println("Shutting down router...")
	if err := r.Shutdown(ctx); err != nil {
		log.Fatalf("Router shutdown failed: %v", err)
	}

	// Shutdown the server
	fmt.Println("Shutting down server...")
	if err := srv.Shutdown(ctx); err != nil {
		log.Fatalf("Server shutdown failed: %v", err)
	}

	// Wait for active requests to complete
	for {
		active := atomic.LoadInt32(&activeRequests)
		if active == 0 {
			break
		}
		fmt.Printf("Waiting for %d active requests to complete...\n", active)
		time.Sleep(1 * time.Second)
	}

	fmt.Println("Server gracefully stopped")
}
