package main

import (
	"context"
	json "encoding/json/v2"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/common"
	"github.com/Suhaibinator/SRouter/pkg/router"
	"github.com/Suhaibinator/SRouter/pkg/scontext"
	"go.uber.org/zap"
)

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
	if err := json.UnmarshalRead(r.Body, &req); err != nil {
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
	_ = json.MarshalWrite(w, LoginResponse{
		Token: token,
		User:  user,
	})
}

func userProfileHandler(w http.ResponseWriter, r *http.Request) {
	user, ok := scontext.GetUserFromRequest[string, User](r)
	if !ok || user == nil {
		http.Error(w, "User not found in context", http.StatusInternalServerError)
		return
	}

	// Return the user profile
	w.Header().Set("Content-Type", "application/json")
	_ = json.MarshalWrite(w, APIResponse{
		Success: true,
		Message: "User profile retrieved successfully",
		Data:    user,
	})
}

func publicEndpointHandler(w http.ResponseWriter, r *http.Request) {
	// Return a public response
	w.Header().Set("Content-Type", "application/json")
	_ = json.MarshalWrite(w, APIResponse{
		Success: true,
		Message: "Public endpoint accessed successfully",
		Data:    map[string]string{"info": "This is a public endpoint"},
	})
}

// Custom rate limit exceeded handler
func rateLimitExceededHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusTooManyRequests)
	_ = json.MarshalWrite(w, APIResponse{
		Success: false,
		Message: "Rate limit exceeded. Please try again later.",
	})
}

func newRateLimitingRouter(logger *zap.Logger) *router.Router[string, User] {
	routerConfig := router.RouterConfig{
		ServiceName:        "rate-limit-service",
		Logger:             logger,
		GlobalMaxBodySize:  1 << 20, // 1 MiB; bound the login JSON body before decoding
		AddUserObjectToCtx: true,
		GlobalRateLimit: &common.RateLimitConfig[any, any]{
			BucketName: "global",
			Limit:      100,
			Window:     time.Minute,
			Strategy:   common.StrategyIP,
		},
		IPConfig: &router.IPConfig{
			Source:     router.IPSourceXForwardedFor,
			TrustProxy: true,
		},
	}

	authFunction := func(_ context.Context, token string) (*User, bool) {
		userID, ok := tokens[token]
		if !ok {
			return nil, false
		}
		user, ok := users[userID]
		if !ok {
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

	r := router.NewRouter(routerConfig, authFunction, userIDFromUser)

	// Login requests have no authenticated identity yet, so this bucket is keyed
	// by client IP.
	r.Group("/auth").
		RateLimit(&common.RateLimitConfig[string, User]{
			BucketName:      "auth-endpoints",
			Limit:           5,
			Window:          time.Minute,
			Strategy:        common.StrategyIP,
			ExceededHandler: http.HandlerFunc(rateLimitExceededHandler),
		}).
		Route(router.RouteConfigBase{
			Path:    "/login",
			Methods: []router.HttpMethod{router.MethodPost},
			Handler: loginHandler,
		})

	// Built-in authentication executes before configured rate limiting, so the
	// profile bucket is keyed by the authenticated user ID.
	r.Group("/api/profile").
		Auth(router.AuthRequired).
		RateLimit(&common.RateLimitConfig[string, User]{
			BucketName: "user-profile",
			Limit:      10,
			Window:     time.Minute,
			Strategy:   common.StrategyUser,
		}).
		Route(router.RouteConfigBase{
			Methods: []router.HttpMethod{router.MethodGet},
			Handler: userProfileHandler,
		})

	r.Group("/api/public").
		RateLimit(&common.RateLimitConfig[string, User]{
			BucketName: "public-endpoints",
			Limit:      20,
			Window:     time.Minute,
			Strategy:   common.StrategyIP,
		}).
		Route(router.RouteConfigBase{
			Methods: []router.HttpMethod{router.MethodGet},
			Handler: publicEndpointHandler,
		})
	return r
}

func main() {
	logger, _ := zap.NewProduction()
	defer func() { _ = logger.Sync() }()

	r := newRateLimitingRouter(logger)

	fmt.Println("Rate Limiting Example Server listening on :8080")
	fmt.Println("Available endpoints:")
	fmt.Println("  - POST /auth/login (IP-based rate limit)")
	fmt.Println("  - GET /api/profile (built-in auth, user-based rate limit)")
	fmt.Println("  - GET /api/public (IP-based rate limit)")
	fmt.Println("\nExample curl commands:")
	fmt.Println(`  curl -X POST -H "Content-Type: application/json" -d '{"username":"user1","password":"password1"}' http://localhost:8080/auth/login`)
	fmt.Println(`  curl -H "Authorization: Bearer token1" http://localhost:8080/api/profile`)
	fmt.Println("  curl http://localhost:8080/api/public")
	log.Fatal(http.ListenAndServe(":8080", r))
}
