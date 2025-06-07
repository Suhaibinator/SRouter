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

// Simple request/response types
type TestRequest struct {
	Value string `json:"value"`
}

type TestResponse struct {
	Result string `json:"result"`
}

// Handler that returns an error
func ErrorHandler(req *http.Request, body TestRequest) (*TestResponse, error) {
	// Simulate a processing error
	return nil, router.NewHTTPError(http.StatusBadRequest, "Invalid input provided")
}

// Handler that succeeds
func SuccessHandler(req *http.Request, body TestRequest) (*TestResponse, error) {
	return &TestResponse{Result: "Success: " + body.Value}, nil
}

func main() {
	// Create a logger
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	// Create router configuration with CORS settings
	routerConfig := router.RouterConfig{
		Logger:            logger,
		GlobalTimeout:     5 * time.Second,
		GlobalMaxBodySize: 1 << 20, // 1 MB
		CORSConfig: &router.CORSConfig{ // Configure CORS directly
			Origins:          []string{"https://frontend.example.com"},
			Methods:          []string{"GET", "POST", "OPTIONS"},
			Headers:          []string{"Content-Type", "Authorization"},
			AllowCredentials: true,
			MaxAge:           86400 * time.Second,
		},
		Middlewares: []common.Middleware{
			// CORS middleware removed, handled by RouterConfig.CORSConfig now
		},
		SubRouters: []router.SubRouterConfig{
			{
				PathPrefix: "/api",
				Routes: []router.RouteDefinition{
					// Route that will return an error
					router.NewGenericRouteDefinition[TestRequest, *TestResponse, string, string](
						router.RouteConfig[TestRequest, *TestResponse]{
							Path:    "/error",
							Methods: []router.HttpMethod{router.MethodPost, router.MethodOptions},
							Handler: ErrorHandler,
							Codec:   codec.NewJSONCodec[TestRequest, *TestResponse](),
						},
					),
					// Route that will succeed
					router.NewGenericRouteDefinition[TestRequest, *TestResponse, string, string](
						router.RouteConfig[TestRequest, *TestResponse]{
							Path:    "/success",
							Methods: []router.HttpMethod{router.MethodPost, router.MethodOptions},
							Handler: SuccessHandler,
							Codec:   codec.NewJSONCodec[TestRequest, *TestResponse](),
						},
					),
				},
			},
		},
	}

	// Auth functions (not used in this example but required by NewRouter)
	authFunction := func(ctx context.Context, token string) (*string, bool) {
		return nil, false
	}
	userIdFromUserFunction := func(user *string) string {
		return ""
	}

	// Create the router
	r := router.NewRouter[string, string](routerConfig, authFunction, userIdFromUserFunction)

	// Start the server
	fmt.Println("Server running on http://localhost:8080")
	fmt.Println("Test with:")
	fmt.Println("curl -i -X OPTIONS -H \"Origin: https://frontend.example.com\" http://localhost:8080/api/error")
	fmt.Println("curl -i -X POST -H \"Origin: https://frontend.example.com\" -H \"Content-Type: application/json\" -d '{\"value\":\"test\"}' http://localhost:8080/api/error")
	fmt.Println("curl -i -X POST -H \"Origin: https://frontend.example.com\" -H \"Content-Type: application/json\" -d '{\"value\":\"test\"}' http://localhost:8080/api/success")
	log.Fatal(http.ListenAndServe(":8080", r))
}
