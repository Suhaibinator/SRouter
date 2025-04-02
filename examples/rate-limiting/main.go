package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/middleware"
	"github.com/Suhaibinator/SRouter/pkg/router"
	"go.uber.org/zap"
)

// Define a custom context key type for user information
type userContextKey struct{}

// Define a user type
type User struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// Define request and response types
type LoginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
}

type LoginResponse struct {
	Token string `json:"token"`
	User  User   `json:"user"`
}

type APIResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
	Data    any    `json:"data,omitempty"`
}

// Mock user database
var users = map[string]User{
	"user1": {ID: "user1", Name: "User One"},
	"user2": {ID: "user2", Name: "User Two"},
}

// Mock token database
var tokens = map[string]string{
	"token1": "user1",
	"token2": "user2",
}

// Handler functions
func loginHandler(w http.ResponseWriter, r *http.Request) {
	var req LoginRequest
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "Invalid request body", http.StatusBadRequest)
		return
	}

	// Simple mock authentication
	var user User
	var token string
	if req.Username == "user1" && req.Password == "password1" {
		user = users["user1"]
		token = "token1"
	} else if req.Username == "user2" && req.Password == "password2" {
		user = users["user2"]
		token = "token2"
	} else {
		http.Error(w, "Invalid credentials", http.StatusUnauthorized)
		return
	}

	// Return the token and user
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(LoginResponse{
		Token: token,
		User:  user,
	})
}

func userProfileHandler(w http.ResponseWriter, r *http.Request) {
	// Get the user from the context
	user, ok := r.Context().Value(userContextKey{}).(User)
	if !ok {
		http.Error(w, "User not found in context", http.StatusInternalServerError)
		return
	}

	// Return the user profile
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Message: "User profile retrieved successfully",
		Data:    user,
	})
}

func publicEndpointHandler(w http.ResponseWriter, r *http.Request) {
	// Return a public response
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(APIResponse{
		Success: true,
		Message: "Public endpoint accessed successfully",
		Data:    map[string]string{"info": "This is a public endpoint"},
	})
}

// Custom authentication middleware
func authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get the token from the Authorization header
		token := r.Header.Get("Authorization")
		if token == "" {
			http.Error(w, "Authorization header required", http.StatusUnauthorized)
			return
		}

		// Remove the "Bearer " prefix if present
		if len(token) > 7 && token[:7] == "Bearer " {
			token = token[7:]
		}

		// Validate the token
		userID, ok := tokens[token]
		if !ok {
			http.Error(w, "Invalid token", http.StatusUnauthorized)
			return
		}

		// Get the user from the database
		user, ok := users[userID]
		if !ok {
			http.Error(w, "User not found", http.StatusInternalServerError)
			return
		}

		// Add the user to the context
		ctx := r.Context()
		ctx = context.WithValue(ctx, userContextKey{}, user)

		// Call the next handler with the updated context
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// Custom rate limit exceeded handler
func rateLimitExceededHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusTooManyRequests)
	json.NewEncoder(w).Encode(APIResponse{
		Success: false,
		Message: "Rate limit exceeded. Please try again later.",
	})
}

func main() {
	// Create a logger
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	// Note: The router creates its own rate limiter internally

	// Create auth subrouter
	authSubrouter := router.SubRouterConfig{
		PathPrefix: "/auth",
		Routes: []any{ // Changed to []any
			router.RouteConfigBase{
				Path:    "/login",
				Methods: []string{"POST"},
				// Strict rate limit for auth endpoints (shared bucket)
				RateLimit: &middleware.RateLimitConfig[any, any]{
					BucketName:      "auth-endpoints",
					Limit:           5,
					Window:          time.Minute,
					Strategy:        middleware.StrategyUser,
					ExceededHandler: http.HandlerFunc(rateLimitExceededHandler),
				},
				Handler: loginHandler,
			},
		},
	}

	// Create API subrouter
	apiSubrouter := router.SubRouterConfig{
		PathPrefix: "/api",
		Routes: []any{ // Changed to []any
			router.RouteConfigBase{
				Path:    "/profile",
				Methods: []string{"GET"},
				// User-based rate limiting
				RateLimit: &middleware.RateLimitConfig[any, any]{
					BucketName: "user-profile",
					Limit:      10,
					Window:     time.Minute,
					Strategy:   middleware.StrategyUser,
				},
				Middlewares: []router.Middleware{
					authMiddleware,
				},
				Handler: userProfileHandler,
			},
			router.RouteConfigBase{ // Add explicit type
				Path:    "/public",
				Methods: []string{"GET"},
				// IP-based rate limiting
				RateLimit: &middleware.RateLimitConfig[any, any]{
					BucketName: "public-endpoints",
					Limit:      20,
					Window:     time.Minute,
					Strategy:   middleware.StrategyIP,
				},
				Handler: publicEndpointHandler,
			},
		},
	}

	// Create a router configuration with global rate limiting
	routerConfig := router.RouterConfig{
		Logger: logger,
		// Global rate limit (applies to all routes)
		GlobalRateLimit: &middleware.RateLimitConfig[any, any]{
			BucketName: "global",
			Limit:      100,
			Window:     time.Minute,
			Strategy:   middleware.StrategyIP,
		},
		// Configure IP extraction to use X-Forwarded-For header
		IPConfig: &middleware.IPConfig{
			Source:     middleware.IPSourceXForwardedFor,
			TrustProxy: true,
		},
		// Add subrouters to the configuration
		SubRouters: []router.SubRouterConfig{
			authSubrouter,
			apiSubrouter,
		},
	}

	// Define the auth function that takes a context and token and returns a User and a boolean
	authFunction := func(ctx context.Context, token string) (User, bool) {
		// Validate the token
		userID, ok := tokens[token]
		if !ok {
			return User{}, false
		}

		// Get the user from the database
		user, ok := users[userID]
		if !ok {
			return User{}, false
		}

		return user, true
	}

	// Define the function to get the user ID from a User
	userIdFromUserFunction := func(user User) User {
		// In this example, we're using the User itself as the ID
		return user
	}

	// Create a router with User as both the user ID and user type
	r := router.NewRouter[User, User](routerConfig, authFunction, userIdFromUserFunction)

	// Start the server
	fmt.Println("Server listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", r))
}
