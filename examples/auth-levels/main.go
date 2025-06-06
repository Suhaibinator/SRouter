package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/common"
	"github.com/Suhaibinator/SRouter/pkg/middleware" // Keep for AuthenticationWithUser
	"github.com/Suhaibinator/SRouter/pkg/router"
	"github.com/Suhaibinator/SRouter/pkg/scontext" // Added import
	"go.uber.org/zap"
)

// User represents a user in the system
type User struct {
	ID    string
	Name  string
	Email string
	Roles []string
}

// Handler for routes with no authentication
func noAuthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"message":"This route does not require authentication"}`))
}

// Handler for routes with optional authentication
func optionalAuthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Try to get the user from the context
	user, ok := scontext.GetUserFromRequest[*User, User](r) // Use scontext
	if ok && user != nil {
		// User is authenticated
		fmt.Fprintf(w, `{"message":"Hello, %s! This route has optional authentication", "authenticated":true}`, user.Name)
	} else {
		// User is not authenticated
		w.Write([]byte(`{"message":"This route has optional authentication", "authenticated":false}`))
	}
}

// Handler for routes with required authentication
func requiredAuthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Get the user from the context
	user, ok := scontext.GetUserFromRequest[*User, User](r) // Use scontext
	if !ok || user == nil {
		// This should not happen since the middleware should have rejected the request
		http.Error(w, "User not found in context", http.StatusInternalServerError)
		return
	}

	// User is authenticated
	fmt.Fprintf(w, `{"message":"Hello, %s! This route requires authentication", "user_id":"%s", "email":"%s"}`,
		user.Name, user.ID, user.Email)
}

func main() {
	// Create a logger
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	// Mock user database
	users := map[string]User{
		"user1": {
			ID:    "1",
			Name:  "User One",
			Email: "user1@example.com",
			Roles: []string{"user"},
		},
		"user2": {
			ID:    "2",
			Name:  "User Two",
			Email: "user2@example.com",
			Roles: []string{"admin", "user"},
		},
	}

	// Mock token to user mapping
	tokens := map[string]string{
		"token1": "user1",
		"token2": "user2",
	}

	// Create a custom authentication function that returns a user
	customUserAuth := func(r *http.Request) (*User, error) {
		authHeader := r.Header.Get("Authorization")
		if authHeader == "" {
			return nil, fmt.Errorf("no authorization header")
		}

		// Extract the token
		token := authHeader[len("Bearer "):]

		// Look up the username for this token
		username, exists := tokens[token]
		if !exists {
			return nil, fmt.Errorf("invalid token")
		}

		// Look up the user
		user, exists := users[username]
		if !exists {
			return nil, fmt.Errorf("user not found")
		}

		// Return a pointer to the user
		return &user, nil
	}

	// Create a router configuration
	routerConfig := router.RouterConfig{
		ServiceName:       "auth-levels-service", // Added ServiceName
		Logger:            logger,
		GlobalTimeout:     2 * time.Second,
		GlobalMaxBodySize: 1 << 20, // 1 MB
		SubRouters: []router.SubRouterConfig{
			{
				PathPrefix: "/auth-levels",
				Routes: []router.RouteDefinition{
					router.RouteConfigBase{
						Path:      "/no-auth",
						Methods:   []router.HttpMethod{router.MethodGet},
						AuthLevel: router.Ptr(router.NoAuth), // Changed
						Handler:   noAuthHandler,
					},
					router.RouteConfigBase{ // Add explicit type
						Path:      "/optional-auth",
						Methods:   []router.HttpMethod{router.MethodGet},
						AuthLevel: router.Ptr(router.AuthOptional), // Authentication is optional. OPTIONS requests are automatically allowed.
						Middlewares: []common.Middleware{
							middleware.AuthenticationWithUser[*User](customUserAuth), // Middleware to add user to context if authenticated
						},
						Handler: optionalAuthHandler,
					},
					router.RouteConfigBase{ // Add explicit type
						Path:      "/required-auth",
						Methods:   []router.HttpMethod{router.MethodGet},
						AuthLevel: router.Ptr(router.AuthRequired), // Authentication is required. OPTIONS requests are automatically allowed.
						Middlewares: []common.Middleware{
							middleware.AuthenticationWithUser[*User](customUserAuth), // Middleware to add user to context if authenticated
						},
						Handler: requiredAuthHandler,
					},
				},
			},
		},
	}

	// Define the auth function that takes a context and token and returns a *User and a boolean
	authFunction := func(ctx context.Context, token string) (*User, bool) {
		// Look up the username for this token
		username, exists := tokens[token]
		if !exists {
			return nil, false // Return nil pointer for user
		}

		// Look up the user
		user, exists := users[username]
		if !exists {
			return nil, false // Return nil pointer for user
		}

		// Return a pointer to the user struct
		return &user, true
	}

	// Define the function to get the user ID (*User) from a *User
	userIdFromUserFunction := func(user *User) *User {
		// In this example, the user object pointer itself is the ID (T = *User)
		// If user is nil, we return nil, otherwise return the pointer itself.
		return user
	}

	// Create a router with *User as the user ID type (T) and User as the user type (U)
	r := router.NewRouter(routerConfig, authFunction, userIdFromUserFunction)

	// Start the server
	fmt.Println("Authentication Levels Example Server listening on :8080")
	fmt.Println("Available endpoints:")
	fmt.Println("  - GET /auth-levels/no-auth (no authentication required)")
	fmt.Println("  - GET /auth-levels/optional-auth (authentication optional)")
	fmt.Println("  - GET /auth-levels/required-auth (authentication required)")
	fmt.Println("\nExample curl commands:")
	fmt.Println("  curl http://localhost:8080/auth-levels/no-auth")
	fmt.Println("  curl http://localhost:8080/auth-levels/optional-auth")
	fmt.Println("  curl -H \"Authorization: Bearer token1\" http://localhost:8080/auth-levels/optional-auth")
	fmt.Println("  curl -H \"Authorization: Bearer token1\" http://localhost:8080/auth-levels/required-auth")
	log.Fatal(http.ListenAndServe(":8080", r))
}
