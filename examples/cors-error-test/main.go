package main

import (
	"context"
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

// Define dummy request/response types
type ErrorRequest struct {
	Data string `json:"data"`
}

type ErrorResponse struct {
	Message string `json:"message"`
}

// Define a handler that always returns an error
func ErrorHandler(r *http.Request, req ErrorRequest) (ErrorResponse, error) {
	// Simulate a processing error
	fmt.Println("ErrorHandler called, returning an error...")
	return ErrorResponse{}, router.NewHTTPError(http.StatusBadRequest, "This is an intentional error from the handler")
}

// Define dummy auth functions (required by NewRouter)
// Note: The UserObjectType U is *string, so authFunction needs to return **string
// and userIdFromUserFunction needs to accept **string.
func dummyAuthFunc(ctx context.Context, token string) (**string, bool) {
	// Return nil for the **string and false, as no auth is needed.
	return nil, false
}

func dummyGetUserID(user **string) string {
	// Check if the outer pointer is nil or the inner pointer is nil
	if user == nil || *user == nil {
		return ""
	}
	// Dereference twice to get the string value
	return **user
}

func main() {
	logger, _ := zap.NewProduction()
	defer logger.Sync()

	// --- CORS Configuration ---
	// Replace "http://localhost:3000" with the actual origin of your frontend app if different
	frontendOrigin := "http://localhost:3000"
	corsOptions := middleware.CORSOptions{
		Origins:          []string{frontendOrigin},
		Methods:          []string{"GET", "POST", "OPTIONS"}, // Ensure OPTIONS is included
		Headers:          []string{"Content-Type", "Authorization"},
		AllowCredentials: true,
		MaxAge:           86400 * time.Second, // 1 day
	}

	// --- Router Configuration ---
	routerConfig := router.RouterConfig{
		ServiceName:       "cors-error-test",
		Logger:            logger,
		GlobalTimeout:     5 * time.Second,
		GlobalMaxBodySize: 1 << 20, // 1 MB
		Middlewares: []common.Middleware{
			middleware.CORS(corsOptions), // Apply CORS middleware globally
		},
		SubRouters: []router.SubRouterConfig{
			{
				PathPrefix: "", // Root level
				Routes: []any{
					// Register the generic route that returns an error
					// Ensure UserIDType (string) and UserObjectType (*string) match NewRouter call
					router.NewGenericRouteDefinition[ErrorRequest, ErrorResponse, string, *string](
						router.RouteConfig[ErrorRequest, ErrorResponse]{
							Path:    "/test-error",
							Methods: []router.HttpMethod{router.MethodPost, router.MethodOptions}, // Include OPTIONS for preflight
							Codec:   codec.NewJSONCodec[ErrorRequest, ErrorResponse](),
							Handler: ErrorHandler,
							// No AuthLevel needed, defaults to NoAuth
						},
					),
				},
			},
		},
		// Enable trace logging for more details if needed
		// TraceIDBufferSize: 1000,
		// EnableTraceLogging: true,
	}

	// --- Create Router ---
	// Using string for UserIDType and *string for UserObjectType (matching dummy funcs)
	r := router.NewRouter[string, *string](routerConfig, dummyAuthFunc, dummyGetUserID)

	// --- Start Server ---
	serverAddr := ":8080"
	fmt.Printf("Server starting on %s\n", serverAddr)
	fmt.Println("-----------------------------------------")
	fmt.Println("To test the CORS error response:")
	fmt.Println("1. Run this server: go run main.go")
	fmt.Printf("2. Open a new terminal and run the following curl command (replace origin if needed):\n")
	fmt.Printf("   curl -v -X POST -H \"Origin: %s\" -H \"Content-Type: application/json\" -d '{\"data\":\"test\"}' http://localhost%s/test-error\n", frontendOrigin, serverAddr)
	fmt.Println("3. Check the response headers in the curl output.")
	fmt.Printf("   Look for 'Access-Control-Allow-Origin: %s'\n", frontendOrigin)
	fmt.Printf("   Also look for 'Access-Control-Allow-Credentials: true'\n")
	fmt.Println("-----------------------------------------")

	log.Fatal(http.ListenAndServe(serverAddr, r))
}
