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
	userID, loggedIn := router.GetUserID[string, string](req)

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
	r := router.NewRouter[string, string](router.RouterConfig{
		Logger:        logger,
		GlobalTimeout: 5 * time.Second,
	}, authFunction, userIdFromUserFunction)

	// Create a JSON codec for our generic routes
	greetingCodec := codec.NewJSONCodec[GreetingRequest, GreetingResponse]()
	userCodec := codec.NewJSONCodec[UserRequest, UserResponse]()
	profileCodec := codec.NewJSONCodec[ProfileRequest, ProfileResponse]()

	// Create a main API sub-router
	apiSubRouter := router.SubRouterConfig{
		PathPrefix: "/api",
		Routes: []router.RouteConfigBase{
			{
				Path:      "/status",
				Methods:   []string{"GET"},
				AuthLevel: router.NoAuth,
				Handler: func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.Write([]byte(`{"status":"ok"}`))
				},
			},
		},
	}

	// Create a v1 sub-router
	apiV1SubRouter := router.SubRouterConfig{
		PathPrefix: "/v1",
		Routes: []router.RouteConfigBase{
			{
				Path:      "/hello",
				Methods:   []string{"GET"},
				AuthLevel: router.NoAuth,
				Handler: func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.Write([]byte(`{"message":"Hello from API v1!"}`))
				},
			},
		},
	}

	// Register a generic route with the v1 sub-router
	router.RegisterGenericRouteWithSubRouter[GreetingRequest, GreetingResponse, string, string](
		&apiV1SubRouter,
		router.RouteConfig[GreetingRequest, GreetingResponse]{
			Path:      "/greet",
			Methods:   []string{"POST"},
			AuthLevel: router.NoAuth,
			Codec:     greetingCodec,
			Handler:   greetingHandler,
		},
	)

	// Create a users sub-router under v1
	usersV1SubRouter := router.SubRouterConfig{
		PathPrefix: "/users",
		Routes: []router.RouteConfigBase{
			{
				Path:      "",
				Methods:   []string{"GET"},
				AuthLevel: router.NoAuth,
				Handler: func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.Write([]byte(`{"users":[{"id":1,"name":"Alice"},{"id":2,"name":"Bob"},{"id":3,"name":"Charlie"}]}`))
				},
			},
		},
	}

	// Register a generic route with the users v1 sub-router
	router.RegisterGenericRouteWithSubRouter[UserRequest, UserResponse, string, string](
		&usersV1SubRouter,
		router.RouteConfig[UserRequest, UserResponse]{
			Path:      "/info",
			Methods:   []string{"POST"},
			AuthLevel: router.NoAuth,
			Codec:     userCodec,
			Handler:   userHandler,
		},
	)

	// Add the users sub-router to the v1 sub-router
	router.RegisterSubRouterWithSubRouter(&apiV1SubRouter, usersV1SubRouter)

	// Create a v2 sub-router
	apiV2SubRouter := router.SubRouterConfig{
		PathPrefix: "/v2",
		Routes: []router.RouteConfigBase{
			{
				Path:      "/hello",
				Methods:   []string{"GET"},
				AuthLevel: router.NoAuth,
				Handler: func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					w.Write([]byte(`{"message":"Hello from API v2!"}`))
				},
			},
		},
	}

	// Create a users sub-router under v2
	usersV2SubRouter := router.SubRouterConfig{
		PathPrefix: "/users",
	}

	// Create a GenericRouteConfigs to hold multiple generic routes
	var userRoutes router.GenericRouteConfigs

	// Create a generic route for getting user info
	userRoute := router.CreateGenericRouteForSubRouter[UserRequest, UserResponse, string, string](
		router.RouteConfig[UserRequest, UserResponse]{
			Path:      "/info",
			Methods:   []string{"POST"},
			AuthLevel: router.NoAuth,
			Codec:     userCodec,
			Handler:   userHandler,
		},
	)

	// Add the generic route to the GenericRouteConfigs
	userRoutes = append(userRoutes, userRoute)

	// Set the GenericRoutes field of the users v2 sub-router
	usersV2SubRouter.GenericRoutes = userRoutes

	// Add the users sub-router to the v2 sub-router
	router.RegisterSubRouterWithSubRouter(&apiV2SubRouter, usersV2SubRouter)

	// Create an auth sub-router under v2 for authenticated routes
	authV2SubRouter := router.SubRouterConfig{
		PathPrefix: "/auth",
	}

	// Register a generic route with the auth v2 sub-router that requires authentication
	router.RegisterGenericRouteWithSubRouter[ProfileRequest, ProfileResponse, string, string](
		&authV2SubRouter,
		router.RouteConfig[ProfileRequest, ProfileResponse]{
			Path:      "/profile",
			Methods:   []string{"POST"},
			AuthLevel: router.AuthRequired, // This route requires authentication
			Codec:     profileCodec,
			Handler:   profileHandler,
		},
	)

	// Add the auth sub-router to the v2 sub-router
	router.RegisterSubRouterWithSubRouter(&apiV2SubRouter, authV2SubRouter)

	// Add the v1 and v2 sub-routers to the main API sub-router
	router.RegisterSubRouterWithSubRouter(&apiSubRouter, apiV1SubRouter)
	router.RegisterSubRouterWithSubRouter(&apiSubRouter, apiV2SubRouter)

	// Register the main API sub-router with the router
	r.RegisterSubRouter(apiSubRouter)

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
