package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/codec"
	"github.com/Suhaibinator/SRouter/pkg/common"
	"github.com/Suhaibinator/SRouter/pkg/router"
	"go.uber.org/zap"
)

// Define request and response types for our generic handler
type CreateUserReq struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type CreateUserResp struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// HealthCheckHandler is a simple handler that returns a 200 OK
func HealthCheckHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.Write([]byte(`{"status":"ok"}`))
}

// CreateUserHandler is a generic handler that creates a user
func CreateUserHandler(r *http.Request, req CreateUserReq) (CreateUserResp, error) {
	// In a real application, you would create a user in a database
	// For this example, we'll just return a mock response
	return CreateUserResp{
		ID:    "123",
		Name:  req.Name,
		Email: req.Email,
	}, nil
}

func main() {
	// Create a logger
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	// Create a router configuration
	routerConfig := router.RouterConfig{
		ServiceName:       "simple-service", // Added ServiceName
		Logger:            logger,
		GlobalTimeout:     2 * time.Second,
		GlobalMaxBodySize: 1 << 20, // 1 MB
		Middlewares:       []common.Middleware{},
		SubRouters: []router.SubRouterConfig{
			{
				PathPrefix: "/api",
				Overrides: common.RouteOverrides{
					Timeout:     3 * time.Second,
					MaxBodySize: 2 << 20, // 2 MB
				},
				Routes: []router.RouteDefinition{
					router.RouteConfigBase{
						Path:      "/health",
						Methods:   []router.HttpMethod{router.MethodGet},
						AuthLevel: router.Ptr(router.NoAuth), // Changed
						Handler:   HealthCheckHandler,
					},
				},
			},
		},
	}

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
	r := router.NewRouter[string, string](routerConfig, authFunction, userIdFromUserFunction)

	// Register a generic JSON route
	// Note: Since this route is under "/api", we use RegisterGenericRouteOnSubRouter
	userRouteConfig := router.RouteConfig[CreateUserReq, CreateUserResp]{
		Path:      "/users", // Relative path
		Methods:   []router.HttpMethod{router.MethodPost},
		AuthLevel: router.Ptr(router.AuthRequired), // Changed
		Overrides: common.RouteOverrides{
			Timeout: 3 * time.Second, // Route-specific override (will be used by getEffectiveTimeout)
		},
		Codec:   codec.NewJSONCodec[CreateUserReq, CreateUserResp](),
		Handler: CreateUserHandler,
	}
	err := router.RegisterGenericRouteOnSubRouter[CreateUserReq, CreateUserResp, string, string](
		r,
		"/api", // Target sub-router prefix
		userRouteConfig,
	)
	if err != nil {
		log.Fatalf("Failed to register generic route on /api: %v", err)
	}

	// Start the server
	fmt.Println("Server listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", r))
}
