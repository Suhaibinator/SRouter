package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/codec"
	"github.com/Suhaibinator/SRouter/pkg/router"
	"github.com/Suhaibinator/SRouter/pkg/scontext" // Added import
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

type ProfileRequest struct {
	// Empty request, we'll get the user from the context
}

type ProfileResponse struct {
	UserID   string `json:"user_id"`
	Message  string `json:"message"`
	IsAdmin  bool   `json:"is_admin"`
	LoggedIn bool   `json:"logged_in"`
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

// Generic handler for profile that accesses user information from the request context
func profileHandler(req *http.Request, data ProfileRequest) (ProfileResponse, error) {
	// Get the user ID from the request context
	userID, loggedIn := scontext.GetUserIDFromRequest[string, string](req) // Use scontext

	// Create a response with the user information
	response := ProfileResponse{
		LoggedIn: loggedIn,
		Message:  "Profile information",
	}

	if loggedIn {
		response.UserID = userID
		// Check if the user is an admin (in a real app, you would check this in a database)
		response.IsAdmin = userID == "admin"
	}

	return response, nil
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

	// Create a router with string as both the user ID and user type
	r := router.NewRouter(router.RouterConfig{
		ServiceName:   "nested-subrouters-service", // Added ServiceName
		Logger:        logger,
		GlobalTimeout: 5 * time.Second,
	}, authFunction, userIdFromUserFunction)

	// Create JSON codecs for our generic routes
	greetingCodec := codec.NewJSONCodec[GreetingRequest, GreetingResponse]()
	userCodec := codec.NewJSONCodec[UserRequest, UserResponse]()
	profileCodec := codec.NewJSONCodec[ProfileRequest, ProfileResponse]()

	// Groups can contain routes and recursively nested groups.
	api := r.Group("/api")
	api.Route(router.RouteConfigBase{
		Path:      "/status",
		Methods:   []router.HttpMethod{router.MethodGet},
		AuthLevel: new(router.NoAuth),
		Handler: func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"status":"ok"}`))
		},
	})

	apiV1 := api.Group("/v1")
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

	usersV1 := apiV1.Group("/users")
	usersV1.Route(
		router.RouteConfigBase{
			Path:      "",
			Methods:   []router.HttpMethod{router.MethodGet},
			AuthLevel: new(router.NoAuth),
			Handler: func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				_, _ = w.Write([]byte(`{"users":[{"id":1,"name":"Alice"},{"id":2,"name":"Bob"},{"id":3,"name":"Charlie"}]}`))
			},
		},
		router.RouteConfig[UserRequest, UserResponse]{
			Path:      "/info",
			Methods:   []router.HttpMethod{router.MethodPost},
			AuthLevel: new(router.NoAuth),
			Codec:     userCodec,
			Handler:   userHandler,
		},
	)

	apiV2 := api.Group("/v2")
	apiV2.Route(router.RouteConfigBase{
		Path:      "/hello",
		Methods:   []router.HttpMethod{router.MethodGet},
		AuthLevel: new(router.NoAuth),
		Handler: func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"message":"Hello from API v2!"}`))
		},
	})

	apiV2.Group("/users").Route(router.RouteConfig[UserRequest, UserResponse]{
		Path:      "/info",
		Methods:   []router.HttpMethod{router.MethodPost},
		AuthLevel: new(router.NoAuth),
		Codec:     userCodec,
		Handler:   userHandler,
	})

	apiV2.Group("/auth").Auth(router.AuthRequired).Route(router.RouteConfig[ProfileRequest, ProfileResponse]{
		Path:    "/profile",
		Methods: []router.HttpMethod{router.MethodPost},
		Codec:   profileCodec,
		Handler: profileHandler,
	})

	// Start the server
	fmt.Println("Nested Route Groups Example Server listening on :8080")
	fmt.Println("Available endpoints:")
	fmt.Println("API:")
	fmt.Println("  - GET /api/status")
	fmt.Println("API v1:")
	fmt.Println("  - GET /api/v1/hello")
	fmt.Println("  - POST /api/v1/greet")
	fmt.Println("  - GET /api/v1/users")
	fmt.Println("  - POST /api/v1/users/info")
	fmt.Println("API v2:")
	fmt.Println("  - GET /api/v2/hello")
	fmt.Println("  - POST /api/v2/users/info")
	fmt.Println("  - POST /api/v2/auth/profile (requires authentication)")
	fmt.Println("\nExample curl commands:")
	fmt.Println("  curl http://localhost:8080/api/status")
	fmt.Println("  curl http://localhost:8080/api/v1/hello")
	fmt.Println("  curl -X POST -H \"Content-Type: application/json\" -d '{\"name\":\"John\"}' http://localhost:8080/api/v1/greet")
	fmt.Println("  curl http://localhost:8080/api/v1/users")
	fmt.Println("  curl -X POST -H \"Content-Type: application/json\" -d '{\"id\":1}' http://localhost:8080/api/v1/users/info")
	fmt.Println("  curl http://localhost:8080/api/v2/hello")
	fmt.Println("  curl -X POST -H \"Content-Type: application/json\" -d '{\"id\":2}' http://localhost:8080/api/v2/users/info")
	fmt.Println("  curl -X POST -H \"Content-Type: application/json\" -H \"Authorization: Bearer admin\" -d '{}' http://localhost:8080/api/v2/auth/profile")
	log.Fatal(http.ListenAndServe(":8080", r))
}
