package main

import (
	"context"
	"log"
	"net/http"

	pb "github.com/Suhaibinator/SRouter/examples/codec/proto_models"
	"github.com/Suhaibinator/SRouter/pkg/codec"
	"github.com/Suhaibinator/SRouter/pkg/router"
	"go.uber.org/zap" // Added import for logger
)

// handleCreateUser is a GenericHandler that processes a decoded User request
// and returns a User response to be encoded.
func handleCreateUser(r *http.Request, reqUser *pb.User) (*pb.User, error) {
	// The 'reqUser' argument is already decoded by the router using the ProtoCodec.

	log.Printf("Received User via Generic Handler: ID=%s, Name=%s, Email=%s", reqUser.GetId(), reqUser.GetName(), reqUser.GetEmail())

	// --- Business Logic ---
	// For this example, we'll just log the received user and send it back.
	// In a real application, you might save the user to a database, etc.
	// You could modify the user or create a different response object here.
	respUser := reqUser // Echoing the request back for simplicity

	// The router will automatically encode 'respUser' using the ProtoCodec.
	log.Printf("Returning response for User ID: %s", respUser.GetId())

	// Return the response object and nil error for success.
	// If an error occurs (e.g., database error), return nil and the error.
	// The router handles encoding the response or converting the error.
	return respUser, nil
}

// Placeholder auth function matching [string, string] for T and U
func placeholderAuth(ctx context.Context, token string) (*string, bool) {
	// No actual auth, just return placeholder user ID and true
	// In a real app, you'd validate the token and return user context/ID
	log.Printf("Placeholder Auth called with token: %s", token) // Added log for visibility
	user := "placeholder-user-id"
	return &user, true
}

// Placeholder user ID extractor matching [string, string] for T and U
func placeholderGetUserID(user *string) string {
	// User object is just the ID string itself in this placeholder
	log.Printf("Placeholder GetUserID called with user: %v", user) // Added log for visibility
	if user == nil {
		return "" // Handle nil pointer case
	}
	return *user // Dereference the pointer
}

func main() {
	// Create a logger
	logger, _ := zap.NewProduction() // Use zap logger
	defer logger.Sync()

	// Create a new router instance with default config and placeholder context functions
	// Using [string, string] for UserID (T) and User (U) types.
	r := router.NewRouter(router.RouterConfig{
		ServiceName: "codec-service", // Added ServiceName
		Logger:      logger,          // Added Logger
	}, placeholderAuth, placeholderGetUserID)

	// Instantiate the ProtoCodec (can be done once outside the handler if reused)
	// Create a factory function for User messages
	userFactory := func() *pb.User { return &pb.User{} }
	// Create a ProtoCodec for User messages, providing the factory
	protoCodec := codec.NewProtoCodec[*pb.User, *pb.User](userFactory)

	// Define the generic route configuration
	routeCfg := router.RouteConfig[*pb.User, *pb.User]{
		Path:    "/users",
		Methods: []router.HttpMethod{router.MethodPost},
		Codec:   protoCodec,
		Handler: handleCreateUser,
		// AuthLevel defaults to NoAuth if nil
	}

	// Register the generic route directly on the router instance 'r'
	// Provide zero/nil for effective settings (timeout, body size, rate limit)
	// as these are not overridden at the route level here.
	router.RegisterGenericRoute(r, routeCfg, 0, 0, nil)

	// Start the HTTP server
	port := ":8080"
	log.Printf("Server starting on port %s", port)
	if err := http.ListenAndServe(port, r); err != nil {
		log.Fatalf("Failed to start server: %v", err)
	}
}
