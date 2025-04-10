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
	defer logger.Sync()

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

	// --- Define Sub-Router Configurations (Declarative Part) ---

	// Create a users sub-router under v1
	usersV1SubRouter := router.SubRouterConfig{
		PathPrefix: "/users", // Relative to parent (/api/v1)
		Routes: []any{
			router.RouteConfigBase{ // This type was already added, just confirming context
				Path:      "", // Becomes /api/v1/users
				Methods:   []router.HttpMethod{router.MethodGet},
				AuthLevel: router.Ptr(router.NoAuth), // Changed
				Handler: func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.Write([]byte(`{"users":[{"id":1,"name":"Alice"},{"id":2,"name":"Bob"},{"id":3,"name":"Charlie"}]}`))
				},
			},
		},
		// Generic routes will be added imperatively later
	}

	// Create a v1 sub-router
	apiV1SubRouter := router.SubRouterConfig{
		PathPrefix: "/v1", // Relative to parent (/api)
		Routes: []any{
			router.RouteConfigBase{ // Add explicit type
				Path:      "/hello", // Becomes /api/v1/hello
				Methods:   []router.HttpMethod{router.MethodGet},
				AuthLevel: router.Ptr(router.NoAuth), // Changed
				Handler: func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.Write([]byte(`{"message":"Hello from API v1!"}`))
				},
			},
		},
		SubRouters: []router.SubRouterConfig{usersV1SubRouter}, // Nest usersV1
		// Generic routes will be added imperatively later
	}

	// Create a users sub-router under v2
	usersV2SubRouter := router.SubRouterConfig{
		PathPrefix: "/users", // Relative to parent (/api/v2)
		// Generic routes will be added imperatively later
	}

	// Create an auth sub-router under v2 for authenticated routes
	authV2SubRouter := router.SubRouterConfig{
		PathPrefix: "/auth", // Relative to parent (/api/v2)
		// Generic routes will be added imperatively later
	}

	// Create a v2 sub-router
	apiV2SubRouter := router.SubRouterConfig{
		PathPrefix: "/v2", // Relative to parent (/api)
		Routes: []any{ // Changed to []any
			router.RouteConfigBase{ // Add explicit type
				Path:      "/hello", // Becomes /api/v2/hello
				Methods:   []router.HttpMethod{router.MethodGet},
				AuthLevel: router.Ptr(router.NoAuth), // Changed
				Handler: func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.Write([]byte(`{"message":"Hello from API v2!"}`))
				},
			},
		},
		SubRouters: []router.SubRouterConfig{usersV2SubRouter, authV2SubRouter}, // Nest usersV2 and authV2
	}

	// Create a main API sub-router
	apiSubRouter := router.SubRouterConfig{
		PathPrefix: "/api", // Root prefix
		Routes: []any{ // Changed to []any
			router.RouteConfigBase{ // Add explicit type
				Path:      "/status", // Becomes /api/status
				Methods:   []router.HttpMethod{router.MethodGet},
				AuthLevel: router.Ptr(router.NoAuth), // Changed
				Handler: func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.Write([]byte(`{"status":"ok"}`))
				},
			},
		},
		SubRouters: []router.SubRouterConfig{apiV1SubRouter, apiV2SubRouter}, // Nest v1 and v2
	}

	// --- Create Router and Register Sub-Routers ---

	// Create a router with string as both the user ID and user type
	r := router.NewRouter(router.RouterConfig{
		Logger:        logger,
		GlobalTimeout: 5 * time.Second,
		SubRouters:    []router.SubRouterConfig{apiSubRouter}, // Register only the top-level sub-router
	}, authFunction, userIdFromUserFunction)

	// --- Imperatively Register Generic Routes ---

	// Create JSON codecs for our generic routes
	greetingCodec := codec.NewJSONCodec[GreetingRequest, GreetingResponse]()
	userCodec := codec.NewJSONCodec[UserRequest, UserResponse]()
	profileCodec := codec.NewJSONCodec[ProfileRequest, ProfileResponse]()

	// Register generic route for /api/v1/greet
	errV1Greet := router.RegisterGenericRouteOnSubRouter(
		r,
		"/api/v1", // Target prefix
		router.RouteConfig[GreetingRequest, GreetingResponse]{
			Path:      "/greet", // Relative path
			Methods:   []router.HttpMethod{router.MethodPost},
			AuthLevel: router.Ptr(router.NoAuth), // Changed
			Codec:     greetingCodec,
			Handler:   greetingHandler,
		},
	)
	if errV1Greet != nil {
		log.Fatalf("Failed to register generic route on /api/v1: %v", errV1Greet)
	}

	// Register generic route for /api/v1/users/info
	errV1UserInfo := router.RegisterGenericRouteOnSubRouter(
		r,
		"/api/v1/users", // Target prefix (nested)
		router.RouteConfig[UserRequest, UserResponse]{
			Path:      "/info", // Relative path
			Methods:   []router.HttpMethod{router.MethodPost},
			AuthLevel: router.Ptr(router.NoAuth), // Changed
			Codec:     userCodec,
			Handler:   userHandler,
		},
	)
	if errV1UserInfo != nil {
		log.Fatalf("Failed to register generic route on /api/v1/users: %v", errV1UserInfo)
	}

	// Register generic route for /api/v2/users/info
	errV2UserInfo := router.RegisterGenericRouteOnSubRouter(
		r,
		"/api/v2/users", // Target prefix (nested)
		router.RouteConfig[UserRequest, UserResponse]{
			Path:      "/info", // Relative path
			Methods:   []router.HttpMethod{router.MethodPost},
			AuthLevel: router.Ptr(router.NoAuth), // Changed
			Codec:     userCodec,
			Handler:   userHandler,
		},
	)
	if errV2UserInfo != nil {
		log.Fatalf("Failed to register generic route on /api/v2/users: %v", errV2UserInfo)
	}

	// Register generic route for /api/v2/auth/profile
	errV2AuthProfile := router.RegisterGenericRouteOnSubRouter(
		r,
		"/api/v2/auth", // Target prefix (nested)
		router.RouteConfig[ProfileRequest, ProfileResponse]{
			Path:      "/profile", // Relative path
			Methods:   []router.HttpMethod{router.MethodPost},
			AuthLevel: router.Ptr(router.AuthRequired), // Changed - This route requires authentication
			Codec:     profileCodec,
			Handler:   profileHandler,
		},
	)
	if errV2AuthProfile != nil {
		log.Fatalf("Failed to register generic route on /api/v2/auth: %v", errV2AuthProfile)
	}

	// Start the server
	fmt.Println("Nested SubRouters Example Server listening on :8080")
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
