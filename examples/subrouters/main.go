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

// API version 1 handlers
func v1GetUsersHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"version":"v1","users":[{"id":1,"name":"John"},{"id":2,"name":"Jane"}]}`))
}

func v1GetUserHandler(w http.ResponseWriter, r *http.Request) {
	id := router.GetParam(r, "id")
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(fmt.Sprintf(`{"version":"v1","user":{"id":%s,"name":"User %s"}}`, id, id)))
}

func v1CreateUserHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"version":"v1","message":"User created","user":{"id":3,"name":"New User"}}`))
}

// API version 2 handlers
func v2GetUsersHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"version":"v2","data":{"users":[{"id":1,"name":"John","email":"john@example.com"},{"id":2,"name":"Jane","email":"jane@example.com"}]}}`))
}

func v2GetUserHandler(w http.ResponseWriter, r *http.Request) {
	id := router.GetParam(r, "id")
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(fmt.Sprintf(`{"version":"v2","data":{"user":{"id":%s,"name":"User %s","email":"user%s@example.com"}}}`, id, id, id)))
}

func v2CreateUserHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"version":"v2","data":{"message":"User created","user":{"id":3,"name":"New User","email":"newuser@example.com"}}}`))
}

// Admin handlers
func adminDashboardHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"message":"Admin Dashboard"}`))
}

func adminUsersHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"message":"Admin Users"}`))
}

func adminSettingsHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"message":"Admin Settings"}`))
}

// Public handlers
func homeHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"message":"Welcome to the home page"}`))
}

func aboutHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"message":"About us"}`))
}

func contactHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"message":"Contact us"}`))
}

// Slow handler for demonstrating timeouts
func slowHandler(w http.ResponseWriter, r *http.Request) {
	// Simulate a slow operation
	time.Sleep(3 * time.Second)
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"message":"This is a slow response"}`))
}

// Large response handler for demonstrating body size limits
func largeResponseHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Generate a large response
	response := `{"message":"This is a large response","data":"`
	for i := 0; i < 1024*1024; i++ { // 1MB of data
		response += "X"
	}
	response += `"}`

	w.Write([]byte(response))
}

// Version middleware adds a version header
func VersionMiddleware(version string) common.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("X-API-Version", version)
			next.ServeHTTP(w, r)
		})
	}
}

// AdminAuthMiddleware checks if the user is an admin
func AdminAuthMiddleware() common.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// In a real app, you would check for admin credentials
			// For this example, we'll just check for an admin header
			if r.Header.Get("X-Admin-Auth") != "admin-secret" {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}
			next.ServeHTTP(w, r)
		})
	}
}

func main() {
	// Create a logger
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	// Create a router configuration with sub-routers
	routerConfig := router.RouterConfig{
		Logger:            logger,
		GlobalTimeout:     5 * time.Second,
		GlobalMaxBodySize: 2 << 20, // 2 MB
		Middlewares: []common.Middleware{
			middleware.Recovery(logger),
			middleware.Logging(logger, false), // Add false for default logging behavior
		},
		SubRouters: []router.SubRouterConfig{
			// API v1 sub-router
			{
				PathPrefix:      "/api/v1",
				TimeoutOverride: 2 * time.Second,
				Middlewares: []common.Middleware{
					VersionMiddleware("1.0"),
				},
				Routes: []any{ // Changed to []any
					router.RouteConfigBase{
						Path:    "/users",
						Methods: []router.HttpMethod{router.MethodGet},
						Handler: v1GetUsersHandler,
					},
					router.RouteConfigBase{ // Add explicit type
						Path:    "/users/:id",
						Methods: []router.HttpMethod{router.MethodGet},
						Handler: v1GetUserHandler,
					},
					router.RouteConfigBase{ // Add explicit type
						Path:    "/users",
						Methods: []router.HttpMethod{router.MethodPost},
						Handler: v1CreateUserHandler,
					},
					router.RouteConfigBase{ // Add explicit type
						Path:    "/slow",
						Methods: []router.HttpMethod{router.MethodGet},
						Handler: slowHandler,
					},
				},
			},
			// API v2 sub-router
			{
				PathPrefix:      "/api/v2",
				TimeoutOverride: 3 * time.Second,
				Middlewares: []common.Middleware{
					VersionMiddleware("2.0"),
				},
				Routes: []any{ // Changed to []any
					router.RouteConfigBase{
						Path:    "/users",
						Methods: []router.HttpMethod{router.MethodGet},
						Handler: v2GetUsersHandler,
					},
					router.RouteConfigBase{ // Add explicit type
						Path:    "/users/:id",
						Methods: []router.HttpMethod{router.MethodGet},
						Handler: v2GetUserHandler,
					},
					router.RouteConfigBase{ // Add explicit type
						Path:    "/users",
						Methods: []router.HttpMethod{router.MethodPost},
						Handler: v2CreateUserHandler,
					},
					router.RouteConfigBase{ // Add explicit type
						Path:    "/slow",
						Methods: []router.HttpMethod{router.MethodGet},
						Handler: slowHandler,
						Timeout: 4 * time.Second, // Override sub-router timeout
					},
				},
			},
			// Admin sub-router
			{
				PathPrefix:          "/admin",
				MaxBodySizeOverride: 5 << 20, // 5 MB
				Middlewares: []common.Middleware{
					AdminAuthMiddleware(),
				},
				Routes: []any{ // Changed to []any
					router.RouteConfigBase{
						Path:    "/dashboard",
						Methods: []router.HttpMethod{router.MethodGet},
						Handler: adminDashboardHandler,
					},
					router.RouteConfigBase{ // Add explicit type
						Path:    "/users",
						Methods: []router.HttpMethod{router.MethodGet},
						Handler: adminUsersHandler,
					},
					router.RouteConfigBase{ // Add explicit type
						Path:    "/settings",
						Methods: []router.HttpMethod{router.MethodGet},
						Handler: adminSettingsHandler,
					},
					router.RouteConfigBase{ // Add explicit type
						Path:    "/large",
						Methods: []router.HttpMethod{router.MethodGet},
						Handler: largeResponseHandler,
					},
				},
			},
			// Public sub-router
			{
				PathPrefix: "/",
				Routes: []any{ // Changed to []any
					router.RouteConfigBase{
						Path:    "/",
						Methods: []router.HttpMethod{router.MethodGet},
						Handler: homeHandler,
					},
					router.RouteConfigBase{ // Add explicit type
						Path:    "/about",
						Methods: []router.HttpMethod{router.MethodGet},
						Handler: aboutHandler,
					},
					router.RouteConfigBase{ // Add explicit type
						Path:    "/contact",
						Methods: []router.HttpMethod{router.MethodGet},
						Handler: contactHandler,
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

	// Start the server
	fmt.Println("Sub-Routers Example Server listening on :8080")
	fmt.Println("Available endpoints:")
	fmt.Println("API v1 (timeout: 2s):")
	fmt.Println("  - GET /api/v1/users")
	fmt.Println("  - GET /api/v1/users/:id")
	fmt.Println("  - POST /api/v1/users")
	fmt.Println("  - GET /api/v1/slow (will timeout)")
	fmt.Println("API v2 (timeout: 3s):")
	fmt.Println("  - GET /api/v2/users")
	fmt.Println("  - GET /api/v2/users/:id")
	fmt.Println("  - POST /api/v2/users")
	fmt.Println("  - GET /api/v2/slow (timeout: 4s)")
	fmt.Println("Admin (requires X-Admin-Auth header):")
	fmt.Println("  - GET /admin/dashboard")
	fmt.Println("  - GET /admin/users")
	fmt.Println("  - GET /admin/settings")
	fmt.Println("  - GET /admin/large")
	fmt.Println("Public:")
	fmt.Println("  - GET /")
	fmt.Println("  - GET /about")
	fmt.Println("  - GET /contact")
	fmt.Println("\nExample curl commands:")
	fmt.Println("  curl http://localhost:8080/api/v1/users")
	fmt.Println("  curl http://localhost:8080/api/v1/users/1")
	fmt.Println("  curl -X POST http://localhost:8080/api/v1/users")
	fmt.Println("  curl http://localhost:8080/api/v1/slow")
	fmt.Println("  curl http://localhost:8080/api/v2/users")
	fmt.Println("  curl http://localhost:8080/api/v2/slow")
	fmt.Println("  curl -H \"X-Admin-Auth: admin-secret\" http://localhost:8080/admin/dashboard")
	fmt.Println("  curl http://localhost:8080/")
	log.Fatal(http.ListenAndServe(":8080", r))
}
