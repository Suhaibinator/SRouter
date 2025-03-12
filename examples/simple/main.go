package main

import (
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/codec"
	"github.com/Suhaibinator/SRouter/pkg/common"
	"github.com/Suhaibinator/SRouter/pkg/middleware"
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
		Logger:            logger,
		GlobalTimeout:     2 * time.Second,
		GlobalMaxBodySize: 1 << 20, // 1 MB
		EnableMetrics:     true,
		Middlewares: []common.Middleware{
			middleware.Logging(logger),
		},
		SubRouters: []router.SubRouterConfig{
			{
				PathPrefix:          "/api",
				TimeoutOverride:     3 * time.Second,
				MaxBodySizeOverride: 2 << 20, // 2 MB
				Routes: []router.RouteConfigBase{
					{
						Path:      "/health",
						Methods:   []string{"GET"},
						AuthLevel: router.NoAuth,
						Handler:   HealthCheckHandler,
					},
				},
			},
		},
	}

	// Define the auth function that takes a token and returns a string and a boolean
	authFunction := func(token string) (string, bool) {
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
	r := router.NewRouter[string, string](routerConfig, authFunction, userIdFromUserFunction)

	// Register a generic JSON route
	router.RegisterGenericRoute[CreateUserReq, CreateUserResp, string](r, router.RouteConfig[CreateUserReq, CreateUserResp]{
		Path:      "/api/users",
		Methods:   []string{"POST"},
		AuthLevel: router.AuthRequired,
		Timeout:   3 * time.Second, // override
		Codec:     codec.NewJSONCodec[CreateUserReq, CreateUserResp](),
		Handler:   CreateUserHandler,
	})

	// Start the server
	fmt.Println("Server listening on :8080")
	log.Fatal(http.ListenAndServe(":8080", r))
}
