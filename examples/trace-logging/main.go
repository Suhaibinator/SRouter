package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/common"
	"github.com/Suhaibinator/SRouter/pkg/middleware"
	"github.com/Suhaibinator/SRouter/pkg/router"
	"go.uber.org/zap"
)

func main() {
	// Create a logger
	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	defer logger.Sync()

	// Create a router configuration with trace middleware
	routerConfig := router.RouterConfig{
		Logger:            logger,
		GlobalTimeout:     2 * time.Second,
		GlobalMaxBodySize: 1 << 20, // 1 MB
		EnableTraceID:     true,    // Enable trace ID logging
		Middlewares: []common.Middleware{
			middleware.Trace(), // Use Trace variable
		},
	}

	// Define the auth function
	authFunction := func(ctx context.Context, token string) (string, bool) {
		if token != "" {
			return token, true
		}
		return "", false
	}

	// Define the function to get the user ID from a string
	userIdFromUserFunction := func(user string) string {
		return user
	}

	// Create a router
	r := router.NewRouter(routerConfig, authFunction, userIdFromUserFunction)

	// Register a route that logs with trace ID
	r.RegisterRoute(router.RouteConfigBase{
		Path:    "/hello",
		Methods: []router.HttpMethod{router.MethodGet},
		Handler: func(w http.ResponseWriter, r *http.Request) {
			// Get the trace ID
			traceID := middleware.GetTraceID(r)

			// Log with trace ID
			logger.Info("Processing request",
				zap.String("trace_id", traceID),
				zap.String("handler", "hello"),
			)

			// Simulate some processing
			time.Sleep(100 * time.Millisecond)

			// Log again with the same trace ID
			logger.Info("Request processed successfully",
				zap.String("trace_id", traceID),
				zap.String("handler", "hello"),
			)

			// Return a response
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(fmt.Sprintf(`{"message":"Hello, World!", "trace_id":"%s"}`, traceID)))
		},
	})

	// Register a route that demonstrates propagating trace ID to a downstream service
	r.RegisterRoute(router.RouteConfigBase{
		Path:    "/downstream",
		Methods: []router.HttpMethod{router.MethodGet},
		Handler: func(w http.ResponseWriter, r *http.Request) {
			// Get the trace ID
			traceID := middleware.GetTraceID(r)

			// Log with trace ID
			logger.Info("Received request, calling downstream service",
				zap.String("trace_id", traceID),
				zap.String("handler", "downstream"),
			)

			// Create a new request to a downstream service
			// In a real application, this would be a different service
			req, err := http.NewRequest("GET", "http://localhost:8080/hello", nil)
			if err != nil {
				logger.Error("Failed to create request",
					zap.String("trace_id", traceID),
					zap.Error(err),
				)
				http.Error(w, "Failed to create request", http.StatusInternalServerError)
				return
			}

			// Propagate the trace ID to the downstream service
			req.Header.Set("X-Trace-ID", traceID)

			// Make the request
			client := &http.Client{}
			resp, err := client.Do(req)
			if err != nil {
				logger.Error("Failed to call downstream service",
					zap.String("trace_id", traceID),
					zap.Error(err),
				)
				http.Error(w, "Failed to call downstream service", http.StatusInternalServerError)
				return
			}
			defer resp.Body.Close()

			// Log success
			logger.Info("Downstream service call successful",
				zap.String("trace_id", traceID),
				zap.Int("status", resp.StatusCode),
			)

			// Return a response
			w.Header().Set("Content-Type", "application/json")
			w.Write([]byte(fmt.Sprintf(`{"message":"Downstream service call successful", "trace_id":"%s"}`, traceID)))
		},
	})

	// Start the server
	fmt.Println("Server running on :8080")
	fmt.Println("Try accessing:")
	fmt.Println("  - http://localhost:8080/hello")
	fmt.Println("  - http://localhost:8080/downstream")
	log.Fatal(http.ListenAndServe(":8080", r))
}
