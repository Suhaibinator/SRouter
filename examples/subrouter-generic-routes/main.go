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
	defer func() { _ = logger.Sync() }()

	// Define the auth function that takes a context and token and returns a *string and a boolean
	authFunction := func(ctx context.Context, token string) (*string, bool) {
		// This is a simple example, so we'll just validate that the token is not empty
		if token != "" {
			// Return pointer to token as user object
			return &token, true
		}
		return nil, false // Return nil pointer for user
	}

	// Define the function to get the user ID from a *string
	userIdFromUserFunction := func(user *string) string {
		// In this example, we're using the string itself as the ID
		if user == nil {
			return "" // Handle nil pointer case
		}
		return *user // Dereference pointer
	}

	// Create JSON codecs for our generic routes.
	greetingCodec := codec.NewJSONCodec[GreetingRequest, GreetingResponse]()
	userCodec := codec.NewJSONCodec[UserRequest, UserResponse]()

	// Create a router with string as both the user ID and user type
	r := router.NewRouter(router.RouterConfig{
		ServiceName:       "subrouter-generic-service",
		Logger:            logger,
		GlobalTimeout:     5 * time.Second,
		GlobalMaxBodySize: 1 << 20,
	}, router.RouterDependencies[string, string]{Authenticate: authFunction, UserID: userIdFromUserFunction})

	apiV1 := r.Group("/api/v1")
	apiV1.Route(
		router.RouteConfigBase{
			Path:      "/hello",
			Methods:   []router.HttpMethod{router.MethodGet},
			AuthLevel: new(router.NoAuth),
			Handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"message":"Hello from API v1!"}`))
			},
		},
		router.RouteConfig[GreetingRequest, GreetingResponse]{
			Path:      "/greet",
			Methods:   []router.HttpMethod{router.MethodPost},
			AuthLevel: new(router.NoAuth),
			Codec:     greetingCodec,
			Handler:   greetingHandler,
		},
	)

	r.Group("/api/v2").Route(
		router.RouteConfig[UserRequest, UserResponse]{
			Path:      "/users",
			Methods:   []router.HttpMethod{router.MethodPost},
			AuthLevel: new(router.NoAuth),
			Codec:     userCodec,
			Handler:   userHandler,
		},
		router.RouteConfig[GreetingRequest, GreetingResponse]{
			Path:      "/greet",
			Methods:   []router.HttpMethod{router.MethodPost},
			AuthLevel: new(router.NoAuth),
			Codec:     greetingCodec,
			Handler:   greetingHandler,
		},
	)

	// Start the server
	fmt.Println("Route Group Generic Routes Example Server listening on :8080")
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
