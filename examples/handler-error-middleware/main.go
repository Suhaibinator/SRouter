package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"

	"github.com/Suhaibinator/SRouter/pkg/codec"
	"github.com/Suhaibinator/SRouter/pkg/common"
	"github.com/Suhaibinator/SRouter/pkg/logkeys"
	"github.com/Suhaibinator/SRouter/pkg/router"
	"github.com/Suhaibinator/SRouter/pkg/scontext"
	"go.uber.org/zap"
)

// Example request and response types
type CreateUserRequest struct {
	Name  string `json:"name"`
	Email string `json:"email"`
}

type CreateUserResponse struct {
	ID      string `json:"id"`
	Message string `json:"message"`
}

// Mock transaction type for demonstration
type Transaction struct {
	committed bool
	rolled    bool
}

func (t *Transaction) Commit() error {
	t.committed = true
	log.Println("Transaction committed")
	return nil
}

func (t *Transaction) Rollback() error {
	t.rolled = true
	log.Println("Transaction rolled back")
	return nil
}

// TransactionMiddleware demonstrates how to use handler errors for transaction management
func TransactionMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Begin transaction
		tx := &Transaction{}
		log.Println("Transaction started")

		// Execute the handler
		next.ServeHTTP(w, r)

		// Check if handler returned an error
		if err, ok := scontext.GetHandlerErrorFromRequest[string, any](r); ok && err != nil {
			log.Printf("Handler error detected: %v", err)
			_ = tx.Rollback()
		} else {
			log.Println("No handler error, committing transaction")
			_ = tx.Commit()
		}
	})
}

// ErrorLoggingMiddleware demonstrates custom error logging based on handler errors
func ErrorLoggingMiddleware(logger *zap.Logger) common.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Execute the handler
			next.ServeHTTP(w, r)

			// Check for handler errors and log them with context
			if err, ok := scontext.GetHandlerErrorFromRequest[string, any](r); ok && err != nil {
				// Extract additional context
				path := r.URL.Path
				method := r.Method

				// Log with structured fields
				logger.Error("Handler error occurred",
					zap.NamedError(logkeys.Error, err),
					zap.String(logkeys.Path, path),
					zap.String(logkeys.Method, method),
				)
			}
		})
	}
}

func main() {
	// Create logger
	logger, _ := zap.NewDevelopment()
	defer func() { _ = logger.Sync() }()

	// Create router with basic configuration
	r := router.NewRouter(
		router.RouterConfig{
			Logger:            logger,
			GlobalMaxBodySize: 1 << 20, // 1 MiB; bound JSON decoding before middleware runs
		},
		router.RouterDependencies[string, any]{
			// Simple auth functions for demo
			Authenticate: func(ctx context.Context, userID string) (*any, bool) {
				return nil, false
			},
			UserID: func(user *any) string {
				return ""
			},
		},
	)

	// Register a route that succeeds
	r.Route(router.RouteConfig[CreateUserRequest, CreateUserResponse]{
		Path:      "/users/success",
		Methods:   []router.HttpMethod{router.MethodPost},
		AuthLevel: new(router.NoAuth),
		Middlewares: []common.Middleware{
			TransactionMiddleware,
			ErrorLoggingMiddleware(logger),
		},
		Handler: func(req *http.Request, data CreateUserRequest) (CreateUserResponse, error) {
			// Validation
			if data.Name == "" || data.Email == "" {
				return CreateUserResponse{}, errors.New("name and email are required")
			}

			// Simulate successful user creation
			return CreateUserResponse{
				ID:      "user-123",
				Message: fmt.Sprintf("User %s created successfully", data.Name),
			}, nil
		},
		Codec: &codec.JSONCodec[CreateUserRequest, CreateUserResponse]{},
	})

	// Register a route that fails with validation error
	r.Route(router.RouteConfig[CreateUserRequest, CreateUserResponse]{
		Path:      "/users/validation-error",
		Methods:   []router.HttpMethod{router.MethodPost},
		AuthLevel: new(router.NoAuth),
		Middlewares: []common.Middleware{
			TransactionMiddleware,
			ErrorLoggingMiddleware(logger),
		},
		Handler: func(req *http.Request, data CreateUserRequest) (CreateUserResponse, error) {
			// Always return validation error
			return CreateUserResponse{}, &router.HTTPError{
				StatusCode: http.StatusBadRequest,
				Message:    "Invalid email format",
			}
		},
		Codec: &codec.JSONCodec[CreateUserRequest, CreateUserResponse]{},
	})

	// Register a route that fails with internal error
	r.Route(router.RouteConfig[CreateUserRequest, CreateUserResponse]{
		Path:      "/users/internal-error",
		Methods:   []router.HttpMethod{router.MethodPost},
		AuthLevel: new(router.NoAuth),
		Middlewares: []common.Middleware{
			TransactionMiddleware,
			ErrorLoggingMiddleware(logger),
		},
		Handler: func(req *http.Request, data CreateUserRequest) (CreateUserResponse, error) {
			// Simulate database error
			return CreateUserResponse{}, errors.New("database connection failed")
		},
		Codec: &codec.JSONCodec[CreateUserRequest, CreateUserResponse]{},
	})

	// Example of custom middleware that decides transaction fate based on error type
	customTransactionMiddleware := func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			tx := &Transaction{}
			log.Println("Custom transaction started")

			next.ServeHTTP(w, r)

			if err, ok := scontext.GetHandlerErrorFromRequest[string, any](r); ok && err != nil {
				// Check if it's a client error (4xx) - don't rollback for these
				var httpErr *router.HTTPError
				if errors.As(err, &httpErr) && httpErr.StatusCode >= 400 && httpErr.StatusCode < 500 {
					log.Printf("Client error detected (status %d), committing transaction", httpErr.StatusCode)
					_ = tx.Commit()
				} else {
					log.Printf("Server error detected, rolling back transaction")
					_ = tx.Rollback()
				}
			} else {
				_ = tx.Commit()
			}
		})
	}

	// Register a route with custom transaction logic
	r.Route(router.RouteConfig[CreateUserRequest, CreateUserResponse]{
		Path:      "/users/custom-transaction",
		Methods:   []router.HttpMethod{router.MethodPost},
		AuthLevel: new(router.NoAuth),
		Middlewares: []common.Middleware{
			customTransactionMiddleware,
			ErrorLoggingMiddleware(logger),
		},
		Handler: func(req *http.Request, data CreateUserRequest) (CreateUserResponse, error) {
			// Return different errors based on input
			if data.Name == "client-error" {
				return CreateUserResponse{}, &router.HTTPError{
					StatusCode: http.StatusBadRequest,
					Message:    "Invalid name",
				}
			}
			if data.Name == "server-error" {
				return CreateUserResponse{}, errors.New("internal server error")
			}
			return CreateUserResponse{
				ID:      "user-456",
				Message: "User created with custom transaction handling",
			}, nil
		},
		Codec: &codec.JSONCodec[CreateUserRequest, CreateUserResponse]{},
	})

	// Start server
	addr := ":8080"
	log.Printf("Starting server on %s", addr)
	log.Println("\nExample requests:")
	log.Println("Success: curl -X POST http://localhost:8080/users/success -d '{\"name\":\"John\",\"email\":\"john@example.com\"}'")
	log.Println("Validation error: curl -X POST http://localhost:8080/users/validation-error -d '{\"name\":\"John\",\"email\":\"john@example.com\"}'")
	log.Println("Internal error: curl -X POST http://localhost:8080/users/internal-error -d '{\"name\":\"John\",\"email\":\"john@example.com\"}'")
	log.Println("Custom (client error): curl -X POST http://localhost:8080/users/custom-transaction -d '{\"name\":\"client-error\",\"email\":\"test@example.com\"}'")
	log.Println("Custom (server error): curl -X POST http://localhost:8080/users/custom-transaction -d '{\"name\":\"server-error\",\"email\":\"test@example.com\"}'")

	if err := http.ListenAndServe(addr, r); err != nil {
		log.Fatal(err)
	}
}
