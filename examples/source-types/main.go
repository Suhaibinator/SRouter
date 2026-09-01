package main

import (
	"encoding/base64"
	json "encoding/json/v2"
	"fmt"
	"log"
	"net/http"
	"net/url"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/codec"
	"github.com/Suhaibinator/SRouter/pkg/router"
	"go.uber.org/zap"
)

// User represents a user in our system
type User struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// GetUserRequest is the request body for getting a user
type GetUserRequest struct {
	ID string `json:"id"`
}

// GetUserResponse is the response body for getting a user
type GetUserResponse struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// In-memory user store
var users = map[string]User{
	"1": {ID: "1", Name: "John Doe", Email: "john@example.com"},
	"2": {ID: "2", Name: "Jane Smith", Email: "jane@example.com"},
	"3": {ID: "3", Name: "Bob Johnson", Email: "bob@example.com"},
}

// GetUserHandler handles getting a user
func GetUserHandler(r *http.Request, req GetUserRequest) (GetUserResponse, error) {
	// Encoded sources and request bodies populate req. Empty sources leave it at
	// its zero value, so that route reads the ordinary :id path parameter.
	id := req.ID
	if id == "" {
		id = router.GetParam(r, "id")
	}

	if id == "" {
		return GetUserResponse{}, router.NewHTTPError(http.StatusBadRequest, "User ID is required")
	}

	// Get the user
	user, ok := users[id]
	if !ok {
		return GetUserResponse{}, router.NewHTTPError(http.StatusNotFound, "User not found")
	}

	// Return the response
	return GetUserResponse(user), nil
}

func registerRoutes(r *router.Router[string, string]) {
	// Body is the default source type. This route states it explicitly because
	// the example compares all request sources side by side.
	r.Route(router.RouteConfig[GetUserRequest, GetUserResponse]{
		Path:       "/users/body",
		Methods:    []router.HttpMethod{router.MethodPost},
		Codec:      codec.NewJSONCodec[GetUserRequest, GetUserResponse](),
		Handler:    GetUserHandler,
		SourceType: router.Body,
	})

	// Empty skips request decoding. The handler reads the ordinary :id path
	// parameter directly from the matched request.
	r.Route(router.RouteConfig[GetUserRequest, GetUserResponse]{
		Path:       "/users/empty/:id",
		Methods:    []router.HttpMethod{router.MethodGet},
		Codec:      codec.NewJSONCodec[GetUserRequest, GetUserResponse](),
		Handler:    GetUserHandler,
		SourceType: router.Empty,
	})

	r.Route(router.RouteConfig[GetUserRequest, GetUserResponse]{
		Path:       "/users/base64/query",
		Methods:    []router.HttpMethod{router.MethodGet},
		Codec:      codec.NewJSONCodec[GetUserRequest, GetUserResponse](),
		Handler:    GetUserHandler,
		SourceType: router.Base64QueryParameter,
		SourceKey:  "data",
	})

	r.Route(router.RouteConfig[GetUserRequest, GetUserResponse]{
		Path:       "/users/base64/path/:data",
		Methods:    []router.HttpMethod{router.MethodGet},
		Codec:      codec.NewJSONCodec[GetUserRequest, GetUserResponse](),
		Handler:    GetUserHandler,
		SourceType: router.Base64PathParameter,
		SourceKey:  "data",
	})

	r.Route(router.RouteConfig[GetUserRequest, GetUserResponse]{
		Path:       "/users/base62/query",
		Methods:    []router.HttpMethod{router.MethodGet},
		Codec:      codec.NewJSONCodec[GetUserRequest, GetUserResponse](),
		Handler:    GetUserHandler,
		SourceType: router.Base62QueryParameter,
		SourceKey:  "data",
	})

	r.Route(router.RouteConfig[GetUserRequest, GetUserResponse]{
		Path:       "/users/base62/path/:data",
		Methods:    []router.HttpMethod{router.MethodGet},
		Codec:      codec.NewJSONCodec[GetUserRequest, GetUserResponse](),
		Handler:    GetUserHandler,
		SourceType: router.Base62PathParameter,
		SourceKey:  "data",
	})
}

func main() {
	// Create a logger
	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	defer func() {
		if syncErr := logger.Sync(); syncErr != nil {
			log.Printf("Failed to sync logger: %v", syncErr)
		}
	}()

	// Create a router configuration
	routerConfig := router.RouterConfig{
		ServiceName:       "source-types-service",
		Logger:            logger,
		GlobalTimeout:     2 * time.Second,
		GlobalMaxBodySize: 1 << 20, // 1 MB
	}

	// Authentication is unrelated to request-source decoding, so this example
	// leaves the router's authentication dependencies unset.
	r := router.NewRouter[string, string](routerConfig, router.RouterDependencies[string, string]{})

	registerRoutes(r)

	// Start the server
	fmt.Println("Source Types Example Server listening on :8080")
	fmt.Println("Available endpoints:")
	fmt.Println("  - POST /users/body (JSON request body)")
	fmt.Println("  - GET /users/empty/:id (no request decoding)")
	fmt.Println("  - GET /users/base64/query?data=... (base64 query source)")
	fmt.Println("  - GET /users/base64/path/:data (base64 path source)")
	fmt.Println("  - GET /users/base62/query?data=... (base62 query source)")
	fmt.Println("  - GET /users/base62/path/:data (base62 path source)")
	fmt.Println("\nExample curl commands:")

	// Create a sample request payload { "id": "1" }
	sampleReq := GetUserRequest{ID: "1"}
	jsonBytes, err := json.Marshal(sampleReq)
	if err != nil {
		log.Fatalf("Failed to encode sample request: %v", err)
	}
	base64Str := base64.StdEncoding.EncodeToString(jsonBytes)
	base62Str := codec.EncodeBase62(jsonBytes)

	fmt.Println("  curl -X POST -H \"Content-Type: application/json\" -d '{\"id\":\"1\"}' http://localhost:8080/users/body")
	fmt.Println("  curl http://localhost:8080/users/empty/1")
	fmt.Printf("  curl \"http://localhost:8080/users/base64/query?data=%s\"\n", url.QueryEscape(base64Str))
	fmt.Printf("  curl http://localhost:8080/users/base64/path/%s\n", base64Str)
	fmt.Printf("  curl \"http://localhost:8080/users/base62/query?data=%s\"\n", base62Str)
	fmt.Printf("  curl http://localhost:8080/users/base62/path/%s\n", base62Str)

	log.Fatal(http.ListenAndServe(":8080", r))
}
