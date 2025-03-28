package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/codec"
	"github.com/Suhaibinator/SRouter/pkg/router"
	"go.uber.org/zap"
)

// Define request and response types for our generic routes
type GreetingRequest struct {
	Name string `json:"name"`
}

type GreetingResponse struct {
	Message string    `json:"message"`
	Time    time.Time `json:"time"`
}

type UserRequest struct {
	ID int `json:"id"`
}

type UserResponse struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
	Role string `json:"role"`
}

// Generic handler for greeting
func greetingHandler(req *http.Request, data GreetingRequest) (GreetingResponse, error) {
	return GreetingResponse{
		Message: fmt.Sprintf("Hello, %s!", data.Name),
		Time:    time.Now(),
	}, nil
}

// Generic handler for user info
func userHandler(req *http.Request, data UserRequest) (UserResponse, error) {
	// In a real app, you would fetch this from a database
	users := map[int]UserResponse{
		1: {ID: 1, Name: "Alice", Role: "Admin"},
		2: {ID: 2, Name: "Bob", Role: "User"},
		3: {ID: 3, Name: "Charlie", Role: "User"},
	}

	user, found := users[data.ID]
	if !found {
		return UserResponse{}, router.NewHTTPError(http.StatusNotFound, "User not found")
	}

	return user, nil
}

func main() {
	// Create a logger
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

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

	// Create a sub-router for API v1
	apiV1SubRouter := router.SubRouterConfig{
		PathPrefix: "/api/v1",
		Routes: []router.RouteConfigBase{
			{
				Path:      "/hello",
				Methods:   []string{"GET"},
				AuthLevel: router.Ptr(router.NoAuth), // Changed
				Handler: func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.Write([]byte(`{"message":"Hello from API v1!"}`))
				},
			},
		},
	}

	// Create a sub-router for API v2 (no generic routes defined here anymore)
	apiV2SubRouter := router.SubRouterConfig{
		PathPrefix: "/api/v2",
		// Routes can still be defined here if needed
	}

	// Create a router with string as both the user ID and user type
	// Register the sub-routers declaratively
	r := router.NewRouter[string, string](router.RouterConfig{
		Logger:        logger,
		GlobalTimeout: 5 * time.Second,
		SubRouters:    []router.SubRouterConfig{apiV1SubRouter, apiV2SubRouter}, // Register sub-routers
	}, authFunction, userIdFromUserFunction)

	// Create JSON codecs for our generic routes
	greetingCodec := codec.NewJSONCodec[GreetingRequest, GreetingResponse]()
	userCodec := codec.NewJSONCodec[UserRequest, UserResponse]()

	// --- Imperatively register generic routes AFTER router creation ---

	// Register generic route for API v1
	errV1 := router.RegisterGenericRouteOnSubRouter[GreetingRequest, GreetingResponse, string, string](
		r,         // The router instance
		"/api/v1", // The prefix of the target sub-router
		router.RouteConfig[GreetingRequest, GreetingResponse]{
			Path:      "/greet", // Path relative to the sub-router prefix
			Methods:   []string{"POST"},
			AuthLevel: router.Ptr(router.NoAuth), // Changed
			Codec:     greetingCodec,
			Handler:   greetingHandler,
		},
	)
	if errV1 != nil {
		log.Fatalf("Failed to register generic route on /api/v1: %v", errV1)
	}

	// Register generic routes for API v2
	errV2User := router.RegisterGenericRouteOnSubRouter[UserRequest, UserResponse, string, string](
		r,
		"/api/v2",
		router.RouteConfig[UserRequest, UserResponse]{
			Path:      "/users",
			Methods:   []string{"POST"},
			AuthLevel: router.Ptr(router.NoAuth), // Changed
			Codec:     userCodec,
			Handler:   userHandler,
		},
	)
	if errV2User != nil {
		log.Fatalf("Failed to register generic user route on /api/v2: %v", errV2User)
	}

	errV2Greet := router.RegisterGenericRouteOnSubRouter[GreetingRequest, GreetingResponse, string, string](
		r,
		"/api/v2",
		router.RouteConfig[GreetingRequest, GreetingResponse]{
			Path:      "/greet",
			Methods:   []string{"POST"},
			AuthLevel: router.Ptr(router.NoAuth), // Changed
			Codec:     greetingCodec,
			Handler:   greetingHandler,
		},
	)
	if errV2Greet != nil {
		log.Fatalf("Failed to register generic greet route on /api/v2: %v", errV2Greet)
	}

	// Start the server
	fmt.Println("SubRouter Generic Routes Example Server listening on :8080")
	fmt.Println("Available endpoints:")
	fmt.Println("API v1:")
	fmt.Println("  - GET /api/v1/hello")
	fmt.Println("  - POST /api/v1/greet")
	fmt.Println("API v2:")
	fmt.Println("  - POST /api/v2/users")
	fmt.Println("  - POST /api/v2/greet")
	fmt.Println("\nExample curl commands:")
	fmt.Println("  curl http://localhost:8080/api/v1/hello")
	fmt.Println("  curl -X POST -H \"Content-Type: application/json\" -d '{\"name\":\"John\"}' http://localhost:8080/api/v1/greet")
	fmt.Println("  curl -X POST -H \"Content-Type: application/json\" -d '{\"id\":1}' http://localhost:8080/api/v2/users")
	fmt.Println("  curl -X POST -H \"Content-Type: application/json\" -d '{\"name\":\"Jane\"}' http://localhost:8080/api/v2/greet")
	log.Fatal(http.ListenAndServe(":8080", r))
}
