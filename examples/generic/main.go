package main

import (
	"errors"
	"fmt"
	"log"
	"net/http"
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

// CreateUserRequest is the request body for creating a user
type CreateUserRequest struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

// CreateUserResponse is the response body for creating a user
type CreateUserResponse struct {
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

// UpdateUserRequest is the request body for updating a user
type UpdateUserRequest struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// UpdateUserResponse is the response body for updating a user
type UpdateUserResponse struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

// DeleteUserRequest is the request body for deleting a user
type DeleteUserRequest struct {
	ID string `json:"id"`
}

// DeleteUserResponse is the response body for deleting a user
type DeleteUserResponse struct {
	Success bool   `json:"success"`
	Message string `json:"message"`
}

// ListUsersRequest is the request body for listing users
type ListUsersRequest struct {
	Limit  int `json:"limit"`
	Offset int `json:"offset"`
}

// ListUsersResponse is the response body for listing users
type ListUsersResponse struct {
	Users []User `json:"users"`
	Total int    `json:"total"`
}

// In-memory user store
var users = map[string]User{
	"1": {ID: "1", Name: "John Doe", Email: "john@example.com"},
	"2": {ID: "2", Name: "Jane Smith", Email: "jane@example.com"},
	"3": {ID: "3", Name: "Bob Johnson", Email: "bob@example.com"},
}

// CreateUserHandler handles creating a user
func CreateUserHandler(r *http.Request, req CreateUserRequest) (CreateUserResponse, error) {
	// Validate request
	if req.Name == "" {
		return CreateUserResponse{}, router.NewHTTPError(http.StatusBadRequest, "Name is required")
	}
	if req.Email == "" {
		return CreateUserResponse{}, router.NewHTTPError(http.StatusBadRequest, "Email is required")
	}

	// Generate a new ID (in a real app, this would be done by the database)
	id := fmt.Sprintf("%d", len(users)+1)

	// Create the user
	user := User{
		ID:    id,
		Name:  req.Name,
		Email: req.Email,
	}

	// Store the user
	users[id] = user

	// Return the response
	return CreateUserResponse(user), nil
}

// GetUserHandler handles getting a user
func GetUserHandler(r *http.Request, req GetUserRequest) (GetUserResponse, error) {
	// Get the user ID from the path parameter
	id := router.GetParam(r, "id")
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

// UpdateUserHandler handles updating a user
func UpdateUserHandler(r *http.Request, req UpdateUserRequest) (UpdateUserResponse, error) {
	// Get the user ID from the path parameter
	id := router.GetParam(r, "id")
	if id == "" {
		return UpdateUserResponse{}, router.NewHTTPError(http.StatusBadRequest, "User ID is required")
	}

	// Validate request
	if req.Name == "" {
		return UpdateUserResponse{}, router.NewHTTPError(http.StatusBadRequest, "Name is required")
	}
	if req.Email == "" {
		return UpdateUserResponse{}, router.NewHTTPError(http.StatusBadRequest, "Email is required")
	}

	// Get the user
	user, ok := users[id]
	if !ok {
		return UpdateUserResponse{}, router.NewHTTPError(http.StatusNotFound, "User not found")
	}

	// Update the user
	user.Name = req.Name
	user.Email = req.Email
	users[id] = user

	// Return the response
	return UpdateUserResponse(user), nil
}

// DeleteUserHandler handles deleting a user
func DeleteUserHandler(r *http.Request, req DeleteUserRequest) (DeleteUserResponse, error) {
	// Get the user ID from the path parameter
	id := router.GetParam(r, "id")
	if id == "" {
		return DeleteUserResponse{}, router.NewHTTPError(http.StatusBadRequest, "User ID is required")
	}

	// Get the user
	_, ok := users[id]
	if !ok {
		return DeleteUserResponse{}, router.NewHTTPError(http.StatusNotFound, "User not found")
	}

	// Delete the user
	delete(users, id)

	// Return the response
	return DeleteUserResponse{
		Success: true,
		Message: "User deleted successfully",
	}, nil
}

// ListUsersHandler handles listing users
func ListUsersHandler(r *http.Request, req ListUsersRequest) (ListUsersResponse, error) {
	// Default limit and offset
	limit := req.Limit
	if limit <= 0 {
		limit = 10
	}
	offset := req.Offset
	if offset < 0 {
		offset = 0
	}

	// Get all users
	var userList []User
	for _, user := range users {
		userList = append(userList, user)
	}

	// Apply pagination
	total := len(userList)
	if offset >= total {
		return ListUsersResponse{
			Users: []User{},
			Total: total,
		}, nil
	}

	end := offset + limit
	if end > total {
		end = total
	}

	// Return the response
	return ListUsersResponse{
		Users: userList[offset:end],
		Total: total,
	}, nil
}

// EmptyRequest is an empty request body
type EmptyRequest struct{}

// ErrorResponse is a response body for errors
type ErrorResponse struct {
	Error string `json:"error"`
}

// ErrorHandler demonstrates returning an error from a handler
func ErrorHandler(r *http.Request, req EmptyRequest) (ErrorResponse, error) {
	return ErrorResponse{}, errors.New("this is a deliberate error")
}

func main() {
	// Create a logger
	logger, err := zap.NewProduction()
	if err != nil {
		log.Fatalf("Failed to initialize logger: %v", err)
	}
	defer func() {
		if syncErr := logger.Sync(); syncErr != nil {
			// We can't use log.Fatalf here as it would exit the program
			// Just log the error since we're already in a defer
			log.Printf("Failed to sync logger: %v", syncErr)
		}
	}()

	// Create a router configuration
	routerConfig := router.RouterConfig{
		Logger:            logger,
		GlobalTimeout:     2 * time.Second,
		GlobalMaxBodySize: 1 << 20, // 1 MB
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

	// Register generic routes
	router.RegisterGenericRoute[CreateUserRequest, CreateUserResponse, string](r, router.RouteConfig[CreateUserRequest, CreateUserResponse]{
		Path:    "/users",
		Methods: []string{"POST"},
		Codec:   codec.NewJSONCodec[CreateUserRequest, CreateUserResponse](),
		Handler: CreateUserHandler,
	})

	router.RegisterGenericRoute[GetUserRequest, GetUserResponse, string](r, router.RouteConfig[GetUserRequest, GetUserResponse]{
		Path:    "/users/:id",
		Methods: []string{"GET"},
		Codec:   codec.NewJSONCodec[GetUserRequest, GetUserResponse](),
		Handler: GetUserHandler,
	})

	router.RegisterGenericRoute[UpdateUserRequest, UpdateUserResponse, string](r, router.RouteConfig[UpdateUserRequest, UpdateUserResponse]{
		Path:    "/users/:id",
		Methods: []string{"PUT"},
		Codec:   codec.NewJSONCodec[UpdateUserRequest, UpdateUserResponse](),
		Handler: UpdateUserHandler,
	})

	router.RegisterGenericRoute[DeleteUserRequest, DeleteUserResponse, string](r, router.RouteConfig[DeleteUserRequest, DeleteUserResponse]{
		Path:    "/users/:id",
		Methods: []string{"DELETE"},
		Codec:   codec.NewJSONCodec[DeleteUserRequest, DeleteUserResponse](),
		Handler: DeleteUserHandler,
	})

	router.RegisterGenericRoute[ListUsersRequest, ListUsersResponse, string](r, router.RouteConfig[ListUsersRequest, ListUsersResponse]{
		Path:    "/users",
		Methods: []string{"GET"},
		Codec:   codec.NewJSONCodec[ListUsersRequest, ListUsersResponse](),
		Handler: ListUsersHandler,
	})

	router.RegisterGenericRoute[EmptyRequest, ErrorResponse, string](r, router.RouteConfig[EmptyRequest, ErrorResponse]{
		Path:    "/error",
		Methods: []string{"GET"},
		Codec:   codec.NewJSONCodec[EmptyRequest, ErrorResponse](),
		Handler: ErrorHandler,
	})

	// Start the server
	fmt.Println("Generic Routes Example Server listening on :8080")
	fmt.Println("Available endpoints:")
	fmt.Println("  - POST /users (create a user)")
	fmt.Println("  - GET /users/:id (get a user)")
	fmt.Println("  - PUT /users/:id (update a user)")
	fmt.Println("  - DELETE /users/:id (delete a user)")
	fmt.Println("  - GET /users (list users)")
	fmt.Println("  - GET /error (trigger an error)")
	fmt.Println("\nExample curl commands:")
	fmt.Println("  curl -X POST -H \"Content-Type: application/json\" -d '{\"name\":\"Alice\", \"email\":\"alice@example.com\"}' http://localhost:8080/users")
	fmt.Println("  curl http://localhost:8080/users/1")
	fmt.Println("  curl -X PUT -H \"Content-Type: application/json\" -d '{\"name\":\"Alice Updated\", \"email\":\"alice@example.com\"}' http://localhost:8080/users/1")
	fmt.Println("  curl -X DELETE http://localhost:8080/users/1")
	fmt.Println("  curl http://localhost:8080/users")
	fmt.Println("  curl http://localhost:8080/error")
	log.Fatal(http.ListenAndServe(":8080", r))
}
