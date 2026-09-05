package main

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/middleware"
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

// Protected resource that requires authentication and uses the user object
func protectedUserHandler(w http.ResponseWriter, r *http.Request) {
	// Get the user from the context
	user, ok := scontext.GetUser[string, User](r.Context())
	if !ok || user == nil {
		http.Error(w, "User not found in context", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprintf(w, `{"message":"Hello, %s! This is a protected resource", "user_id":"%s", "email":"%s", "roles":["%s"]}`,
		user.Name, user.ID, user.Email, strings.Join(user.Roles, `","`))
}

// Protected resource that requires authentication but doesn't use the user object
func protectedHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"message":"This is a protected resource"}`))
}

// Public resource that doesn't require authentication
func publicHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"message":"This is a public resource"}`))
}

func newUserAuthRouter(logger *zap.Logger) *router.Router[string, User] {
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

	customUserAuth := func(r *http.Request) (*User, error) {
		authHeader := r.Header.Get("Authorization")
		token, ok := strings.CutPrefix(authHeader, "Bearer ")
		if !ok || token == "" {
			return nil, errors.New("expected Authorization: Bearer <token>")
		}
		username, exists := tokens[token]
		if !exists {
			return nil, errors.New("invalid token")
		}
		user, exists := users[username]
		if !exists {
			return nil, errors.New("user not found")
		}
		return &user, nil
	}

	bearerTokenUserAuth := func(token string) (*User, error) {
		username, exists := tokens[token]
		if !exists {
			return nil, errors.New("invalid token")
		}
		user, exists := users[username]
		if !exists {
			return nil, errors.New("user not found")
		}
		return &user, nil
	}

	basicUserAuth := func(username, password string) (*User, error) {
		if password != "password" {
			return nil, errors.New("invalid password")
		}
		user, exists := users[username]
		if !exists {
			return nil, errors.New("user not found")
		}
		return &user, nil
	}

	routerConfig := router.RouterConfig{
		ServiceName:       "user-auth-service",
		Logger:            logger,
		GlobalTimeout:     2 * time.Second,
		GlobalMaxBodySize: 1 << 20, // 1 MB
	}

	// These routes demonstrate standalone authentication middleware, so they all
	// use NoAuth at the router's built-in authentication stage.
	r := router.NewRouter[string, User](routerConfig, router.RouterDependencies[string, User]{})

	r.Group("/public").
		Auth(router.NoAuth).
		Route(router.RouteConfigBase{
			Path:    "/resource",
			Methods: []router.HttpMethod{router.MethodGet},
			Handler: publicHandler,
		})
	r.Group("/boolean-auth").
		Auth(router.NoAuth).
		Use(middleware.AuthenticationBool[string, User](func(r *http.Request) bool {
			token, ok := strings.CutPrefix(r.Header.Get("Authorization"), "Bearer ")
			if !ok || token == "" {
				return false
			}
			_, exists := tokens[token]
			return exists
		}, "authenticated")).
		Route(router.RouteConfigBase{
			Path:    "/resource",
			Methods: []router.HttpMethod{router.MethodGet},
			Handler: protectedHandler,
		})

	userAuth := r.Group("/user-auth").Auth(router.NoAuth)
	userAuth.Group("/custom").
		Use(middleware.AuthenticationWithUser[string, User](customUserAuth)).
		Route(router.RouteConfigBase{
			Methods: []router.HttpMethod{router.MethodGet},
			Handler: protectedUserHandler,
		})
	userAuth.Group("/bearer").
		Use(middleware.NewBearerTokenWithUserMiddleware[string, User](bearerTokenUserAuth, logger)).
		Route(router.RouteConfigBase{
			Methods: []router.HttpMethod{router.MethodGet},
			Handler: protectedUserHandler,
		})
	userAuth.Group("/basic").
		Use(middleware.AuthenticationWithUserProvider[string, User](
			&middleware.BasicUserAuthProvider[User]{GetUserFunc: basicUserAuth},
			logger,
		)).
		Route(router.RouteConfigBase{
			Methods: []router.HttpMethod{router.MethodGet},
			Handler: protectedUserHandler,
		})
	return r
}

func main() {
	logger, _ := zap.NewProduction()
	defer func() { _ = logger.Sync() }()

	r := newUserAuthRouter(logger)

	fmt.Println("User Authentication Example Server listening on :8080")
	fmt.Println("Available endpoints:")
	fmt.Println("  - GET /public/resource (no auth required)")
	fmt.Println("  - GET /boolean-auth/resource (boolean auth required)")
	fmt.Println("  - GET /user-auth/custom (custom user auth)")
	fmt.Println("  - GET /user-auth/bearer (bearer token user auth)")
	fmt.Println("  - GET /user-auth/basic (basic user auth)")
	fmt.Println("\nExample curl commands:")
	fmt.Println("  curl http://localhost:8080/public/resource")
	fmt.Println("  curl -H \"Authorization: Bearer token1\" http://localhost:8080/boolean-auth/resource")
	fmt.Println("  curl -H \"Authorization: Bearer token1\" http://localhost:8080/user-auth/custom")
	fmt.Println("  curl -H \"Authorization: Bearer token2\" http://localhost:8080/user-auth/bearer")
	fmt.Println("  curl -u user1:password http://localhost:8080/user-auth/basic")
	log.Fatal(http.ListenAndServe(":8080", r))
}
