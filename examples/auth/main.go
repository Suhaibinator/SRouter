package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/middleware"
	"github.com/Suhaibinator/SRouter/pkg/router"
	"go.uber.org/zap"
)

// Protected resource that requires authentication
func protectedHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"message":"This is a protected resource"}`))
}

// Public resource that doesn't require authentication
func publicHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"message":"This is a public resource"}`))
}

func newAuthRouter(logger *zap.Logger) *router.Router[int64, int64] {
	bearerTokens := map[string]int64{
		"token1": 34,
		"token2": 35,
	}
	apiKeys := map[string]int64{
		"key1": 24,
		"key2": 25,
	}

	bearerTokenMiddleware := middleware.NewBearerTokenMiddleware[int64, int64](bearerTokens, logger)
	apiKeyMiddleware := middleware.NewAPIKeyMiddleware[int64, int64](apiKeys, "X-API-Key", "api_key", logger)

	config := router.RouterConfig{
		ServiceName:       "auth-example-service",
		Logger:            logger,
		GlobalTimeout:     2 * time.Second,
		GlobalMaxBodySize: 1 << 20, // 1 MB
	}

	authFunction := func(_ context.Context, token string) (*int64, bool) {
		userID, ok := bearerTokens[token]
		if !ok {
			return nil, false
		}
		return &userID, true
	}
	userIDFromUser := func(user *int64) int64 {
		if user == nil {
			return 0
		}
		return *user
	}

	r := router.NewRouter(config, authFunction, userIDFromUser)

	r.Group("/public").Route(router.RouteConfigBase{
		Path:    "/resource",
		Methods: []router.HttpMethod{router.MethodGet},
		Handler: publicHandler,
	})
	r.Group("/bearer-auth").
		Use(bearerTokenMiddleware).
		Route(router.RouteConfigBase{
			Path:    "/resource",
			Methods: []router.HttpMethod{router.MethodGet},
			Handler: protectedHandler,
		})
	r.Group("/api-key-auth").
		Use(apiKeyMiddleware).
		Route(router.RouteConfigBase{
			Path:    "/resource",
			Methods: []router.HttpMethod{router.MethodGet},
			Handler: protectedHandler,
		})
	r.Group("/require-auth").
		Auth(router.AuthRequired).
		Route(router.RouteConfigBase{
			Path:    "/resource",
			Methods: []router.HttpMethod{router.MethodGet},
			Handler: protectedHandler,
		})
	return r
}

func main() {
	logger, _ := zap.NewProduction()
	defer func() { _ = logger.Sync() }()

	r := newAuthRouter(logger)

	fmt.Println("Authentication Example Server listening on :8080")
	fmt.Println("Available endpoints:")
	fmt.Println("  - GET /public/resource (no auth required)")
	fmt.Println("  - GET /bearer-auth/resource (bearer token required)")
	fmt.Println("  - GET /api-key-auth/resource (API key required)")
	fmt.Println("  - GET /require-auth/resource (built-in required authentication)")
	fmt.Println("\nExample curl commands:")
	fmt.Println("  curl http://localhost:8080/public/resource")
	fmt.Println("  curl -H \"Authorization: Bearer token1\" http://localhost:8080/bearer-auth/resource")
	fmt.Println("  curl -H \"X-API-Key: key1\" http://localhost:8080/api-key-auth/resource")
	fmt.Println("  curl \"http://localhost:8080/api-key-auth/resource?api_key=key1\"")
	fmt.Println("  curl -H \"Authorization: Bearer token1\" http://localhost:8080/require-auth/resource")
	log.Fatal(http.ListenAndServe(":8080", r))
}
