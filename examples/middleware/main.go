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

// SimpleHandler is a simple handler that returns a message
func SimpleHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"message":"Hello, World!"}`))
}

// RequestIDMiddleware adds a request ID to the response headers
func RequestIDMiddleware() common.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Generate a request ID (in a real app, use a UUID)
			requestID := fmt.Sprintf("req-%d", time.Now().UnixNano())

			// Add it to the response headers
			w.Header().Set("X-Request-ID", requestID)

			// Call the next handler
			next.ServeHTTP(w, r)

			// Log after the request is complete
			fmt.Printf("Request ID: %s completed\n", requestID)
		})
	}
}

// TimingMiddleware measures the time taken to process a request
func TimingMiddleware() common.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Record the start time
			start := time.Now()

			// Call the next handler
			next.ServeHTTP(w, r)

			// Calculate the duration
			duration := time.Since(start)

			// Log the duration
			fmt.Printf("Request to %s took %s\n", r.URL.Path, duration)
		})
	}
}

// HeadersMiddleware adds custom headers to the response
func HeadersMiddleware(headers map[string]string) common.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Add headers to the response
			for key, value := range headers {
				w.Header().Set(key, value)
			}

			// Call the next handler
			next.ServeHTTP(w, r)
		})
	}
}

// RateLimitMiddleware implements a simple rate limiter
func RateLimitMiddleware(requestsPerSecond int) common.Middleware {
	// Create a channel to control the rate
	ticker := time.NewTicker(time.Second / time.Duration(requestsPerSecond))
	defer ticker.Stop()

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Wait for a token from the ticker
			<-ticker.C

			// Call the next handler
			next.ServeHTTP(w, r)
		})
	}
}

// LoggingResponseWriter is a wrapper around http.ResponseWriter that captures the status code
type LoggingResponseWriter struct {
	http.ResponseWriter
	statusCode int
}

// WriteHeader captures the status code and calls the underlying ResponseWriter.WriteHeader
func (lrw *LoggingResponseWriter) WriteHeader(code int) {
	lrw.statusCode = code
	lrw.ResponseWriter.WriteHeader(code)
}

// DetailedLoggingMiddleware logs detailed information about the request and response
func DetailedLoggingMiddleware(logger *zap.Logger) common.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Create a response writer that captures the status code
			lrw := &LoggingResponseWriter{
				ResponseWriter: w,
				statusCode:     http.StatusOK, // Default status code
			}

			// Log the request
			logger.Info("Request received",
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.String("remote_addr", r.RemoteAddr),
				zap.String("user_agent", r.UserAgent()),
			)

			// Call the next handler
			next.ServeHTTP(lrw, r)

			// Log the response
			logger.Info("Response sent",
				zap.String("method", r.Method),
				zap.String("path", r.URL.Path),
				zap.Int("status", lrw.statusCode),
			)
		})
	}
}

func main() {
	// Create a logger
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	// Create custom headers
	customHeaders := map[string]string{
		"X-Powered-By": "SRouter",
		"X-Version":    "1.0.0",
	}

	// Create a router configuration with global middlewares
	routerConfig := router.RouterConfig{
		Logger:            logger,
		GlobalTimeout:     2 * time.Second,
		GlobalMaxBodySize: 1 << 20, // 1 MB
		Middlewares: []common.Middleware{
			middleware.Recovery(logger),       // Use variable
			DetailedLoggingMiddleware(logger), // Log detailed request/response info
			middleware.CORS(middleware.CORSOptions{ // Use variable
				Origins: []string{"*"},
			}), // Add CORS headers
			HeadersMiddleware(customHeaders),    // Add custom headers
			RequestIDMiddleware(),               // Add request ID
			middleware.Timeout(1 * time.Second), // Use variable
			middleware.MaxBodySize(1 << 20),     // Use variable
		},
		SubRouters: []router.SubRouterConfig{
			{
				PathPrefix: "/api",
				Middlewares: []common.Middleware{
					TimingMiddleware(), // Measure request time
				},
				Routes: []any{ // Changed to []any
					router.RouteConfigBase{
						Path:    "/hello",
						Methods: []router.HttpMethod{router.MethodGet},
						Handler: SimpleHandler,
					},
					router.RouteConfigBase{ // Add explicit type
						Path:    "/slow",
						Methods: []router.HttpMethod{router.MethodGet},
						Handler: func(w http.ResponseWriter, r *http.Request) {
							// Simulate a slow operation
							time.Sleep(500 * time.Millisecond)
							w.Header().Set("Content-Type", "application/json")
							w.Write([]byte(`{"message":"Slow operation completed"}`))
						},
					},
				},
			},
			{
				PathPrefix: "/rate-limited",
				Middlewares: []common.Middleware{
					RateLimitMiddleware(2), // Limit to 2 requests per second
				},
				Routes: []any{ // Changed to []any
					router.RouteConfigBase{
						Path:    "/resource",
						Methods: []router.HttpMethod{router.MethodGet},
						Handler: SimpleHandler,
					},
				},
			},
		},
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

	// Register a route with route-specific middleware
	r.RegisterRoute(router.RouteConfigBase{
		Path:    "/custom",
		Methods: []router.HttpMethod{router.MethodGet},
		Middlewares: []common.Middleware{
			func(next http.Handler) http.Handler {
				return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
					fmt.Println("Route-specific middleware executed")
					next.ServeHTTP(w, r)
				})
			},
		},
		Handler: SimpleHandler,
	})

	// Start the server
	fmt.Println("Middleware Example Server listening on :8080")
	fmt.Println("Available endpoints:")
	fmt.Println("  - GET /api/hello (with timing middleware)")
	fmt.Println("  - GET /api/slow (with timing middleware)")
	fmt.Println("  - GET /rate-limited/resource (with rate limiting middleware)")
	fmt.Println("  - GET /custom (with route-specific middleware)")
	fmt.Println("\nExample curl commands:")
	fmt.Println("  curl http://localhost:8080/api/hello")
	fmt.Println("  curl http://localhost:8080/api/slow")
	fmt.Println("  curl http://localhost:8080/rate-limited/resource")
	fmt.Println("  curl http://localhost:8080/custom")
	log.Fatal(http.ListenAndServe(":8080", r))
}
