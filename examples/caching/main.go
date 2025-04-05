package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/codec"
	"github.com/Suhaibinator/SRouter/pkg/router"
	"go.uber.org/zap"
)

// Simple in-memory cache implementation
type InMemoryCache struct {
	cache map[string][]byte
	mu    sync.RWMutex
}

func NewInMemoryCache() *InMemoryCache {
	return &InMemoryCache{
		cache: make(map[string][]byte),
	}
}

func (c *InMemoryCache) Get(key string) ([]byte, bool) {
	c.mu.RLock()
	defer c.mu.RUnlock()
	value, found := c.cache[key]
	return value, found
}

func (c *InMemoryCache) Set(key string, value []byte) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.cache[key] = value
	return nil
}

// Request and response types
type UserRequest struct {
	ID int `json:"id"`
}

type UserResponse struct {
	ID        int    `json:"id"`
	Name      string `json:"name"`
	Email     string `json:"email"`
	CreatedAt string `json:"created_at"`
}

// Mock user database
var users = map[int]UserResponse{
	1: {ID: 1, Name: "Alice", Email: "alice@example.com", CreatedAt: "2023-01-01"},
	2: {ID: 2, Name: "Bob", Email: "bob@example.com", CreatedAt: "2023-02-15"},
	3: {ID: 3, Name: "Charlie", Email: "charlie@example.com", CreatedAt: "2023-03-20"},
}

// Handler function
func getUserHandler(r *http.Request, req UserRequest) (UserResponse, error) {
	// Simulate a slow database query
	time.Sleep(500 * time.Millisecond)

	user, ok := users[req.ID]
	if !ok {
		return UserResponse{}, router.NewHTTPError(http.StatusNotFound, "User not found")
	}
	return user, nil
}

func main() {
	// Create a logger
	logger, _ := zap.NewDevelopment()
	defer logger.Sync()

	// Create an in-memory cache (Note: Caching functionality seems removed from RouterConfig)
	// cache := NewInMemoryCache()

	// Create a JSON codec
	jsonCodec := codec.NewJSONCodec[UserRequest, UserResponse]()

	// Create a router (Note: Removed caching fields from config)
	r := router.NewRouter[int, any](
		router.RouterConfig{
			Logger:        logger,
			EnableMetrics: true,
			MetricsConfig: &router.MetricsConfig{
				Namespace:        "caching_example",
				Subsystem:        "api",
				EnableLatency:    true,
				EnableThroughput: true,
				EnableQPS:        true,
				EnableErrors:     true,
			},
			// CacheGet:       cache.Get, // Removed
			// CacheSet:       cache.Set, // Removed
			// CacheKeyPrefix: "global", // Removed
			// Add a sub-router
			SubRouters: []router.SubRouterConfig{
				{
					PathPrefix: "/api/v1",
					// CacheResponse:  true, // Removed
					// CacheKeyPrefix: "api-v1", // Removed
					Routes: []any{ // Changed to []any
						router.RouteConfigBase{
							Path:    "/users/:id",
							Methods: []router.HttpMethod{router.MethodGet},
							Handler: func(w http.ResponseWriter, req *http.Request) {
								// Get the user ID from the path parameter
								idStr := router.GetParam(req, "id")
								var id int
								fmt.Sscanf(idStr, "%d", &id)

								// Get the user
								user, ok := users[id]
								if !ok {
									http.Error(w, "User not found", http.StatusNotFound)
									return
								}

								// Return the user as JSON
								w.Header().Set("Content-Type", "application/json")
								json.NewEncoder(w).Encode(user)
							},
						},
					},
				},
			},
		},
		func(ctx context.Context, token string) (any, bool) {
			// No authentication for this example
			return nil, true
		},
		func(user any) int {
			// No user ID for this example
			return 0
		},
	)

	// Register routes

	// 1. Standard route without caching (using request body)
	router.RegisterGenericRoute(r, router.RouteConfig[UserRequest, UserResponse]{
		Path:    "/users/body",
		Methods: []router.HttpMethod{router.MethodPost},
		Codec:   jsonCodec,
		Handler: getUserHandler,
		// CacheResponse is false by default (and functionality removed)
	}, time.Duration(0), int64(0), nil) // Added effective settings

	// 2. Route with caching using query parameter (Note: Caching functionality removed)
	router.RegisterGenericRoute(r, router.RouteConfig[UserRequest, UserResponse]{
		Path:       "/users/query",
		Methods:    []router.HttpMethod{router.MethodGet},
		Codec:      jsonCodec,
		Handler:    getUserHandler,
		SourceType: router.Base64QueryParameter,
		SourceKey:  "data",
		// CacheResponse:  true, // Removed
		// CacheKeyPrefix: "query", // Removed
	}, time.Duration(0), int64(0), nil) // Added effective settings

	// 3. Route with caching using path parameter (Note: Caching functionality removed)
	router.RegisterGenericRoute(r, router.RouteConfig[UserRequest, UserResponse]{
		Path:       "/users/path/:data",
		Methods:    []router.HttpMethod{router.MethodGet},
		Codec:      jsonCodec,
		Handler:    getUserHandler,
		SourceType: router.Base64PathParameter,
		SourceKey:  "data",
		// CacheResponse:  true, // Removed
		// CacheKeyPrefix: "path", // Removed
	}, time.Duration(0), int64(0), nil) // Added effective settings

	// The sub-router is already registered in the RouterConfig

	// Start the server
	fmt.Println("Server running on http://localhost:8080")
	fmt.Println("\nExample usage:")
	fmt.Println("1. Using request body (no caching):")
	fmt.Println("   curl -X POST -H \"Content-Type: application/json\" -d '{\"id\":1}' http://localhost:8080/users/body")

	// Create a base64 encoded request for user with ID 1
	userReq := UserRequest{ID: 1}
	jsonData, _ := json.Marshal(userReq)
	base64Data := base64.StdEncoding.EncodeToString(jsonData)

	fmt.Println("\n2. Using query parameter (caching removed):")
	fmt.Printf("   curl \"http://localhost:8080/users/query?data=%s\"\n", base64Data)

	fmt.Println("\n3. Using path parameter (caching removed):")
	fmt.Printf("   curl \"http://localhost:8080/users/path/%s\"\n", base64Data)

	fmt.Println("\n4. Using sub-router:")
	fmt.Println("   curl \"http://localhost:8080/api/v1/users/1\"")

	// fmt.Println("\nCache key prefixes used in this example:")
	// fmt.Println("- Global prefix: \"global\"")
	// fmt.Println("- Route-specific prefix for query parameter route: \"query\"")
	// fmt.Println("- Route-specific prefix for path parameter route: \"path\"")
	// fmt.Println("- Sub-router prefix: \"api-v1\"")

	// fmt.Println("\nTry making the same request multiple times to see the caching in action!")

	log.Fatal(http.ListenAndServe(":8080", r))
}
