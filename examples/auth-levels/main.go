package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/router"
	"github.com/Suhaibinator/SRouter/pkg/scontext"
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
	_, _ = w.Write([]byte(`{"message":"This route does not require authentication"}`))
}

// Handler for routes with optional authentication
func optionalAuthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Try to get the user from the context
	user, ok := scontext.GetUser[string, User](r.Context())
	if ok && user != nil {
		// User is authenticated
		_, _ = fmt.Fprintf(w, `{"message":"Hello, %s! This route has optional authentication", "authenticated":true}`, user.Name)
	} else {
		// User is not authenticated
		_, _ = w.Write([]byte(`{"message":"This route has optional authentication", "authenticated":false}`))
	}
}

// Handler for routes with required authentication
func requiredAuthHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Get the user from the context
	user, ok := scontext.GetUser[string, User](r.Context())
	if !ok || user == nil {
		// This should not happen since the middleware should have rejected the request
		http.Error(w, "User not found in context", http.StatusInternalServerError)
		return
	}

	// User is authenticated
	_, _ = fmt.Fprintf(w, `{"message":"Hello, %s! This route requires authentication", "user_id":"%s", "email":"%s"}`,
		user.Name, user.ID, user.Email)
}

func newAuthLevelsRouter(logger *zap.Logger) *router.Router[string, User] {
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

	tokens := map[string]string{
		"token1": "user1",
		"token2": "user2",
	}

	routerConfig := router.RouterConfig{
		ServiceName:        "auth-levels-service",
		Logger:             logger,
		GlobalTimeout:      2 * time.Second,
		GlobalMaxBodySize:  1 << 20, // 1 MB
		AddUserObjectToCtx: true,
	}

	authFunction := func(_ context.Context, token string) (*User, bool) {
		username, exists := tokens[token]
		if !exists {
			return nil, false
		}
		user, exists := users[username]
		if !exists {
			return nil, false
		}
		return &user, true
	}
	userIDFromUser := func(user *User) string {
		if user == nil {
			return ""
		}
		return user.ID
	}

	r := router.NewRouter(routerConfig, router.RouterDependencies[string, User]{
		Authenticate: authFunction,
		UserID:       userIDFromUser,
	})

	authLevels := r.Group("/auth-levels")
	authLevels.Group("/no-auth").
		Auth(router.NoAuth).
		Route(router.RouteConfigBase{
			Methods: []router.HttpMethod{router.MethodGet},
			Handler: noAuthHandler,
		})
	authLevels.Group("/optional-auth").
		Auth(router.AuthOptional).
		Route(router.RouteConfigBase{
			Methods: []router.HttpMethod{router.MethodGet},
			Handler: optionalAuthHandler,
		})
	authLevels.Group("/required-auth").
		Auth(router.AuthRequired).
		Route(router.RouteConfigBase{
			Methods: []router.HttpMethod{router.MethodGet},
			Handler: requiredAuthHandler,
		})
	return r
}

func main() {
	logger, _ := zap.NewProduction()
	defer func() { _ = logger.Sync() }()

	r := newAuthLevelsRouter(logger)

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
