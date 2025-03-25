package router

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/codec"
	"github.com/Suhaibinator/SRouter/pkg/middleware"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Test request and response types
type TestProfileRequest struct {
	// Empty request, we'll get the user from the context
}

type TestProfileResponse struct {
	UserID   string `json:"user_id"`
	IsAdmin  bool   `json:"is_admin"`
	LoggedIn bool   `json:"logged_in"`
}

// Test request and response types for query parameters
type TestQueryRequest struct {
	ID   int    `query:"id"`
	Name string `query:"name"`
}

type TestQueryResponse struct {
	ID   int    `json:"id"`
	Name string `json:"name"`
}

// Test request and response types for error handling
type TestErrorRequest struct {
	ShouldError bool `json:"should_error"`
}

type TestErrorResponse struct {
	Message string `json:"message"`
}

// Test handler that accesses user information from the request context
func testProfileHandler(req *http.Request, data TestProfileRequest) (TestProfileResponse, error) {
	// Get the user ID from the request context
	userID, loggedIn := middleware.GetUserIDFromRequest[string, string](req)

	// Create a response with the user information
	response := TestProfileResponse{
		LoggedIn: loggedIn,
	}

	if loggedIn {
		response.UserID = userID
		// Check if the user is an admin
		response.IsAdmin = userID == "admin"
	}

	return response, nil
}

// Test handler for query parameters
func testQueryHandler(req *http.Request, data TestQueryRequest) (TestQueryResponse, error) {
	// Simply echo back the query parameters
	return TestQueryResponse(data), nil
}

// Test handler for error handling
func testErrorHandler(req *http.Request, data TestErrorRequest) (TestErrorResponse, error) {
	if data.ShouldError {
		return TestErrorResponse{}, NewHTTPError(http.StatusBadRequest, "Error requested by client")
	}
	return TestErrorResponse{
		Message: "Success",
	}, nil
}

// Test that generic routes registered with SubRouters can access user information from the request context
func TestRegisterGenericRouteWithSubRouterUserContext(t *testing.T) {
	// Create a logger
	logger, _ := zap.NewDevelopment()
	defer func() {
		_ = logger.Sync() // ignoring sync error
	}()

	// Define the auth function
	authFunction := func(ctx context.Context, token string) (string, bool) {
		if token == "admin" {
			return "admin", true
		} else if token == "user" {
			return "user", true
		}
		return "", false
	}

	// Define the function to get the user ID from a string
	userIdFromUserFunction := func(user string) string {
		return user
	}

	// Create a router
	r := NewRouter[string, string](RouterConfig{
		Logger:        logger,
		GlobalTimeout: 5 * time.Second,
	}, authFunction, userIdFromUserFunction)

	// Create a JSON codec for our generic routes
	profileCodec := codec.NewJSONCodec[TestProfileRequest, TestProfileResponse]()

	// Create a main API sub-router
	apiSubRouter := SubRouterConfig{
		PathPrefix: "/api",
	}

	// Create a v1 sub-router
	apiV1SubRouter := SubRouterConfig{
		PathPrefix: "/v1",
	}

	// Create an auth sub-router under v1 for authenticated routes
	authV1SubRouter := SubRouterConfig{
		PathPrefix: "/auth",
	}

	// Register a generic route with the auth v1 sub-router that requires authentication
	RegisterGenericRouteWithSubRouter[TestProfileRequest, TestProfileResponse, string, string](
		&authV1SubRouter,
		RouteConfig[TestProfileRequest, TestProfileResponse]{
			Path:      "/profile",
			Methods:   []string{"POST"},
			AuthLevel: AuthRequired, // This route requires authentication
			Codec:     profileCodec,
			Handler:   testProfileHandler,
		},
	)

	// Add the auth sub-router to the v1 sub-router
	RegisterSubRouterWithSubRouter(&apiV1SubRouter, authV1SubRouter)

	// Add the v1 sub-router to the main API sub-router
	RegisterSubRouterWithSubRouter(&apiSubRouter, apiV1SubRouter)

	// Register the main API sub-router with the router
	r.RegisterSubRouter(apiSubRouter)

	// Test with admin authentication
	t.Run("Admin Authentication", func(t *testing.T) {
		// Create a request with admin authentication
		req := httptest.NewRequest("POST", "/api/v1/auth/profile", strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer admin")

		// Create a response recorder
		rr := httptest.NewRecorder()

		// Serve the request
		r.ServeHTTP(rr, req)

		// Check the status code
		if status := rr.Code; status != http.StatusOK {
			t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
		}

		// Check the response body
		var response TestProfileResponse
		err := json.Unmarshal(rr.Body.Bytes(), &response)
		if err != nil {
			t.Errorf("error unmarshaling response: %v", err)
		}

		// Check that the user ID is "admin"
		if response.UserID != "admin" {
			t.Errorf("handler returned wrong user ID: got %v want %v", response.UserID, "admin")
		}

		// Check that the user is an admin
		if !response.IsAdmin {
			t.Errorf("handler returned wrong admin status: got %v want %v", response.IsAdmin, true)
		}

		// Check that the user is logged in
		if !response.LoggedIn {
			t.Errorf("handler returned wrong logged in status: got %v want %v", response.LoggedIn, true)
		}
	})

	// Test with user authentication
	t.Run("User Authentication", func(t *testing.T) {
		// Create a request with user authentication
		req := httptest.NewRequest("POST", "/api/v1/auth/profile", strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer user")

		// Create a response recorder
		rr := httptest.NewRecorder()

		// Serve the request
		r.ServeHTTP(rr, req)

		// Check the status code
		if status := rr.Code; status != http.StatusOK {
			t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
		}

		// Check the response body
		var response TestProfileResponse
		err := json.Unmarshal(rr.Body.Bytes(), &response)
		if err != nil {
			t.Errorf("error unmarshaling response: %v", err)
		}

		// Check that the user ID is "user"
		if response.UserID != "user" {
			t.Errorf("handler returned wrong user ID: got %v want %v", response.UserID, "user")
		}

		// Check that the user is not an admin
		if response.IsAdmin {
			t.Errorf("handler returned wrong admin status: got %v want %v", response.IsAdmin, false)
		}

		// Check that the user is logged in
		if !response.LoggedIn {
			t.Errorf("handler returned wrong logged in status: got %v want %v", response.LoggedIn, true)
		}
	})

	// Test with no authentication
	t.Run("No Authentication", func(t *testing.T) {
		// Create a request with no authentication
		req := httptest.NewRequest("POST", "/api/v1/auth/profile", strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")

		// Create a response recorder
		rr := httptest.NewRecorder()

		// Serve the request
		r.ServeHTTP(rr, req)

		// Check the status code (should be 401 Unauthorized)
		if status := rr.Code; status != http.StatusUnauthorized {
			t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusUnauthorized)
		}
	})
}

// Test that generic routes registered with SubRouters can handle query parameters
func TestRegisterGenericRouteWithSubRouterQuery(t *testing.T) {
	// Create a logger
	logger, _ := zap.NewDevelopment()
	defer func() {
		_ = logger.Sync() // ignoring sync error
	}()

	// Define the auth function
	authFunction := func(ctx context.Context, token string) (string, bool) {
		return "", false
	}

	// Define the function to get the user ID from a string
	userIdFromUserFunction := func(user string) string {
		return user
	}

	// Create a router
	r := NewRouter[string, string](RouterConfig{
		Logger:        logger,
		GlobalTimeout: 5 * time.Second,
	}, authFunction, userIdFromUserFunction)

	// Create a JSON codec for our generic routes
	queryCodec := codec.NewJSONCodec[TestQueryRequest, TestQueryResponse]()

	// Create a main API sub-router
	apiSubRouter := SubRouterConfig{
		PathPrefix: "/api",
	}

	// Create a v1 sub-router
	apiV1SubRouter := SubRouterConfig{
		PathPrefix: "/v1",
	}

	// Register a generic route with the v1 sub-router that handles query parameters
	RegisterGenericRouteWithSubRouter[TestQueryRequest, TestQueryResponse, string, string](
		&apiV1SubRouter,
		RouteConfig[TestQueryRequest, TestQueryResponse]{
			Path:       "/query",
			Methods:    []string{"GET"},
			AuthLevel:  NoAuth,
			Codec:      queryCodec,
			Handler:    testQueryHandler,
			SourceType: Base64QueryParameter,
			SourceKey:  "data",
		},
	)

	// Add the v1 sub-router to the main API sub-router
	RegisterSubRouterWithSubRouter(&apiSubRouter, apiV1SubRouter)

	// Register the main API sub-router with the router
	r.RegisterSubRouter(apiSubRouter)

	// Test with query parameters
	t.Run("Query Parameters", func(t *testing.T) {
		// Create a request with base64-encoded query parameter
		reqData := TestQueryRequest{
			ID:   123,
			Name: "test",
		}
		reqBytes, _ := json.Marshal(reqData)
		base64Data := base64.StdEncoding.EncodeToString(reqBytes)
		req := httptest.NewRequest("GET", "/api/v1/query?data="+base64Data, nil)

		// Create a response recorder
		rr := httptest.NewRecorder()

		// Serve the request
		r.ServeHTTP(rr, req)

		// Check the status code
		if status := rr.Code; status != http.StatusOK {
			t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
		}

		// Check the response body
		var response TestQueryResponse
		err := json.Unmarshal(rr.Body.Bytes(), &response)
		if err != nil {
			t.Errorf("error unmarshaling response: %v", err)
		}

		// Check that the ID is 123
		if response.ID != 123 {
			t.Errorf("handler returned wrong ID: got %v want %v", response.ID, 123)
		}

		// Check that the name is "test"
		if response.Name != "test" {
			t.Errorf("handler returned wrong name: got %v want %v", response.Name, "test")
		}
	})

	// Test with missing query parameters
	t.Run("Missing Query Parameters", func(t *testing.T) {
		// Create a request with no query parameters
		req := httptest.NewRequest("GET", "/api/v1/query", nil)

		// Create a response recorder
		rr := httptest.NewRecorder()

		// Serve the request
		r.ServeHTTP(rr, req)

		// Check the status code (should be 400 Bad Request)
		if status := rr.Code; status != http.StatusBadRequest {
			t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusBadRequest)
		}
	})
}

// Test that generic routes registered with SubRouters can handle errors
func TestRegisterGenericRouteWithSubRouterError(t *testing.T) {
	// Create a logger
	logger, _ := zap.NewDevelopment()
	defer func() {
		_ = logger.Sync() // ignoring sync error
	}()

	// Define the auth function
	authFunction := func(ctx context.Context, token string) (string, bool) {
		return "", false
	}

	// Define the function to get the user ID from a string
	userIdFromUserFunction := func(user string) string {
		return user
	}

	// Create a router
	r := NewRouter[string, string](RouterConfig{
		Logger:        logger,
		GlobalTimeout: 5 * time.Second,
	}, authFunction, userIdFromUserFunction)

	// Create a JSON codec for our generic routes
	errorCodec := codec.NewJSONCodec[TestErrorRequest, TestErrorResponse]()

	// Create a main API sub-router
	apiSubRouter := SubRouterConfig{
		PathPrefix: "/api",
	}

	// Create a v1 sub-router
	apiV1SubRouter := SubRouterConfig{
		PathPrefix: "/v1",
	}

	// Register a generic route with the v1 sub-router that handles errors
	RegisterGenericRouteWithSubRouter[TestErrorRequest, TestErrorResponse, string, string](
		&apiV1SubRouter,
		RouteConfig[TestErrorRequest, TestErrorResponse]{
			Path:      "/error",
			Methods:   []string{"POST"},
			AuthLevel: NoAuth,
			Codec:     errorCodec,
			Handler:   testErrorHandler,
		},
	)

	// Add the v1 sub-router to the main API sub-router
	RegisterSubRouterWithSubRouter(&apiSubRouter, apiV1SubRouter)

	// Register the main API sub-router with the router
	r.RegisterSubRouter(apiSubRouter)

	// Test with no error
	t.Run("No Error", func(t *testing.T) {
		// Create a request with no error
		req := httptest.NewRequest("POST", "/api/v1/error", strings.NewReader(`{"should_error":false}`))
		req.Header.Set("Content-Type", "application/json")

		// Create a response recorder
		rr := httptest.NewRecorder()

		// Serve the request
		r.ServeHTTP(rr, req)

		// Check the status code
		if status := rr.Code; status != http.StatusOK {
			t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
		}

		// Check the response body
		var response TestErrorResponse
		err := json.Unmarshal(rr.Body.Bytes(), &response)
		if err != nil {
			t.Errorf("error unmarshaling response: %v", err)
		}

		// Check that the message is "Success"
		if response.Message != "Success" {
			t.Errorf("handler returned wrong message: got %v want %v", response.Message, "Success")
		}
	})

	// Test with error
	t.Run("With Error", func(t *testing.T) {
		// Create a request with error
		req := httptest.NewRequest("POST", "/api/v1/error", strings.NewReader(`{"should_error":true}`))
		req.Header.Set("Content-Type", "application/json")

		// Create a response recorder
		rr := httptest.NewRecorder()

		// Serve the request
		r.ServeHTTP(rr, req)

		// Check the status code (should be 400 Bad Request)
		if status := rr.Code; status != http.StatusBadRequest {
			t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusBadRequest)
		}

		// Check the response body contains the error message
		if !strings.Contains(rr.Body.String(), "Error requested by client") {
			t.Errorf("handler returned wrong error message: got %v", rr.Body.String())
		}
	})
}

// Test that generic routes created with CreateGenericRouteForSubRouter work correctly
func TestCreateGenericRouteForSubRouter(t *testing.T) {
	// Create a logger
	logger, _ := zap.NewDevelopment()
	defer func() {
		_ = logger.Sync() // ignoring sync error
	}()

	// Define the auth function
	authFunction := func(ctx context.Context, token string) (string, bool) {
		if token == "admin" {
			return "admin", true
		} else if token == "user" {
			return "user", true
		}
		return "", false
	}

	// Define the function to get the user ID from a string
	userIdFromUserFunction := func(user string) string {
		return user
	}

	// Create a router
	r := NewRouter[string, string](RouterConfig{
		Logger:        logger,
		GlobalTimeout: 5 * time.Second,
	}, authFunction, userIdFromUserFunction)

	// Create a JSON codec for our generic routes
	profileCodec := codec.NewJSONCodec[TestProfileRequest, TestProfileResponse]()

	// Create a main API sub-router
	apiSubRouter := SubRouterConfig{
		PathPrefix: "/api",
	}

	// Create a v1 sub-router
	apiV1SubRouter := SubRouterConfig{
		PathPrefix: "/v1",
	}

	// Create an auth sub-router under v1 for authenticated routes
	authV1SubRouter := SubRouterConfig{
		PathPrefix: "/auth",
	}

	// Create a GenericRouteConfigs to hold multiple generic routes
	var authRoutes GenericRouteConfigs

	// Create a generic route for getting profile info
	profileRoute := CreateGenericRouteForSubRouter[TestProfileRequest, TestProfileResponse, string, string](
		RouteConfig[TestProfileRequest, TestProfileResponse]{
			Path:      "/profile",
			Methods:   []string{"POST"},
			AuthLevel: AuthRequired, // This route requires authentication
			Codec:     profileCodec,
			Handler:   testProfileHandler,
		},
	)

	// Add the generic route to the GenericRouteConfigs
	authRoutes = append(authRoutes, profileRoute)

	// Set the GenericRoutes field of the auth v1 sub-router
	authV1SubRouter.GenericRoutes = authRoutes

	// Add the auth sub-router to the v1 sub-router
	RegisterSubRouterWithSubRouter(&apiV1SubRouter, authV1SubRouter)

	// Add the v1 sub-router to the main API sub-router
	RegisterSubRouterWithSubRouter(&apiSubRouter, apiV1SubRouter)

	// Register the main API sub-router with the router
	r.RegisterSubRouter(apiSubRouter)

	// Test with admin authentication
	t.Run("Admin Authentication", func(t *testing.T) {
		// Create a request with admin authentication
		req := httptest.NewRequest("POST", "/api/v1/auth/profile", strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer admin")

		// Create a response recorder
		rr := httptest.NewRecorder()

		// Serve the request
		r.ServeHTTP(rr, req)

		// Check the status code
		if status := rr.Code; status != http.StatusOK {
			t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
		}

		// Check the response body
		var response TestProfileResponse
		err := json.Unmarshal(rr.Body.Bytes(), &response)
		if err != nil {
			t.Errorf("error unmarshaling response: %v", err)
		}

		// Check that the user ID is "admin"
		if response.UserID != "admin" {
			t.Errorf("handler returned wrong user ID: got %v want %v", response.UserID, "admin")
		}

		// Check that the user is an admin
		if !response.IsAdmin {
			t.Errorf("handler returned wrong admin status: got %v want %v", response.IsAdmin, true)
		}

		// Check that the user is logged in
		if !response.LoggedIn {
			t.Errorf("handler returned wrong logged in status: got %v want %v", response.LoggedIn, true)
		}
	})

	// Test with no authentication
	t.Run("No Authentication", func(t *testing.T) {
		// Create a request with no authentication
		req := httptest.NewRequest("POST", "/api/v1/auth/profile", strings.NewReader("{}"))
		req.Header.Set("Content-Type", "application/json")

		// Create a response recorder
		rr := httptest.NewRecorder()

		// Serve the request
		r.ServeHTTP(rr, req)

		// Check the status code (should be 401 Unauthorized)
		if status := rr.Code; status != http.StatusUnauthorized {
			t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusUnauthorized)
		}
	})
}

// Mock GenericRouteRegistrar that returns an error
type MockErrorGenericRouteRegistrar struct{}

func (m MockErrorGenericRouteRegistrar) RegisterWith(router interface{}, pathPrefix string) error {
	return fmt.Errorf("mock error")
}

// Mock GenericRouteConfig that implements GenericRouteRegistrar
type MockGenericRouteConfig struct{}

func (m MockGenericRouteConfig) RegisterWith(router interface{}, pathPrefix string) error {
	// Cast will fail because we're passing a string, not a *Router
	_, ok := router.(string)
	if !ok {
		return fmt.Errorf("router is not of type string")
	}
	return nil
}

// Test error handling in registerSubRouter
func TestRegisterSubRouterErrorHandling(t *testing.T) {
	// Create a logger with a buffer to capture logs
	var buf bytes.Buffer
	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(zap.NewDevelopmentEncoderConfig()),
		zapcore.AddSync(&buf),
		zapcore.DebugLevel,
	)
	logger := zap.New(core)

	// Define the auth function
	authFunction := func(ctx context.Context, token string) (string, bool) {
		return "", false
	}

	// Define the function to get the user ID from a string
	userIdFromUserFunction := func(user string) string {
		return user
	}

	// Create a router
	r := NewRouter[string, string](RouterConfig{
		Logger:        logger,
		GlobalTimeout: 5 * time.Second,
	}, authFunction, userIdFromUserFunction)

	// Test case 1: RegisterWith returns an error
	t.Run("RegisterWith Error", func(t *testing.T) {
		// Clear the buffer
		buf.Reset()

		// Create a sub-router with a mock GenericRouteRegistrar that returns an error
		subRouter := SubRouterConfig{
			PathPrefix:    "/api",
			GenericRoutes: MockErrorGenericRouteRegistrar{},
		}

		// Register the sub-router
		r.RegisterSubRouter(subRouter)

		// Check that the error was logged
		logs := buf.String()
		if !strings.Contains(logs, "Failed to register generic route") {
			t.Errorf("Expected error log not found: %s", logs)
		}
		if !strings.Contains(logs, "mock error") {
			t.Errorf("Expected error message not found: %s", logs)
		}
	})

	// Test case 2: Invalid GenericRoutes type
	t.Run("Invalid GenericRoutes Type", func(t *testing.T) {
		// Clear the buffer
		buf.Reset()

		// Create a sub-router with an invalid GenericRoutes type (string)
		subRouter := SubRouterConfig{
			PathPrefix:    "/api",
			GenericRoutes: "invalid",
		}

		// Register the sub-router
		r.RegisterSubRouter(subRouter)

		// Check that the error was logged
		logs := buf.String()
		if !strings.Contains(logs, "Invalid GenericRoutes type") {
			t.Errorf("Expected error log not found: %s", logs)
		}
	})

	// Test case 3: Type casting error in RegisterWith
	t.Run("Type Casting Error", func(t *testing.T) {
		// Clear the buffer
		buf.Reset()

		// Create a GenericRouteConfigs with a mock GenericRouteConfig
		var routes GenericRouteConfigs
		routes = append(routes, MockGenericRouteConfig{})

		// Create a sub-router with the GenericRouteConfigs
		subRouter := SubRouterConfig{
			PathPrefix:    "/api",
			GenericRoutes: routes,
		}

		// Register the sub-router
		r.RegisterSubRouter(subRouter)

		// Check that the error was logged
		logs := buf.String()
		if !strings.Contains(logs, "Failed to register generic route") {
			t.Errorf("Expected error log not found: %s", logs)
		}
		if !strings.Contains(logs, "router is not of type string") {
			t.Errorf("Expected error message not found: %s", logs)
		}
	})
}

// Test RegisterGenericRouteWithSubRouter function
func TestRegisterGenericRouteWithSubRouterFunction(t *testing.T) {
	// Create a logger
	logger, _ := zap.NewDevelopment()
	defer func() {
		_ = logger.Sync() // ignoring sync error
	}()

	// Define the auth function
	authFunction := func(ctx context.Context, token string) (string, bool) {
		return "", false
	}

	// Define the function to get the user ID from a string
	userIdFromUserFunction := func(user string) string {
		return user
	}

	// Create a router
	r := NewRouter[string, string](RouterConfig{
		Logger:        logger,
		GlobalTimeout: 5 * time.Second,
	}, authFunction, userIdFromUserFunction)

	// Create a JSON codec for our generic routes
	queryCodec := codec.NewJSONCodec[TestQueryRequest, TestQueryResponse]()

	// Test case 1: GenericRoutes is nil
	t.Run("GenericRoutes is nil", func(t *testing.T) {
		// Create a sub-router with nil GenericRoutes
		subRouter := SubRouterConfig{
			PathPrefix:    "/api",
			GenericRoutes: nil,
		}

		// Register a generic route with the sub-router
		RegisterGenericRouteWithSubRouter[TestQueryRequest, TestQueryResponse, string, string](
			&subRouter,
			RouteConfig[TestQueryRequest, TestQueryResponse]{
				Path:      "/query",
				Methods:   []string{"GET"},
				AuthLevel: NoAuth,
				Codec:     queryCodec,
				Handler:   testQueryHandler,
			},
		)

		// Check that GenericRoutes is now a GenericRouteConfigs with one route
		routes, ok := subRouter.GenericRoutes.(GenericRouteConfigs)
		if !ok {
			t.Errorf("Expected GenericRoutes to be a GenericRouteConfigs, got %T", subRouter.GenericRoutes)
		}
		if len(routes) != 1 {
			t.Errorf("Expected GenericRoutes to have 1 route, got %d", len(routes))
		}

		// Register the sub-router with the router
		r.RegisterSubRouter(subRouter)
	})

	// Test case 2: GenericRoutes is already a GenericRouteConfigs
	t.Run("GenericRoutes is already a GenericRouteConfigs", func(t *testing.T) {
		// Create a GenericRouteConfigs with one route
		var routes GenericRouteConfigs
		routes = append(routes, MockGenericRouteConfig{})

		// Create a sub-router with the GenericRouteConfigs
		subRouter := SubRouterConfig{
			PathPrefix:    "/api",
			GenericRoutes: routes,
		}

		// Register a generic route with the sub-router
		RegisterGenericRouteWithSubRouter[TestQueryRequest, TestQueryResponse, string, string](
			&subRouter,
			RouteConfig[TestQueryRequest, TestQueryResponse]{
				Path:      "/query",
				Methods:   []string{"GET"},
				AuthLevel: NoAuth,
				Codec:     queryCodec,
				Handler:   testQueryHandler,
			},
		)

		// Check that GenericRoutes is still a GenericRouteConfigs but now with two routes
		routes, ok := subRouter.GenericRoutes.(GenericRouteConfigs)
		if !ok {
			t.Errorf("Expected GenericRoutes to be a GenericRouteConfigs, got %T", subRouter.GenericRoutes)
		}
		if len(routes) != 2 {
			t.Errorf("Expected GenericRoutes to have 2 routes, got %d", len(routes))
		}
	})

	// Test case 3: GenericRoutes is a single GenericRouteRegistrar
	t.Run("GenericRoutes is a single GenericRouteRegistrar", func(t *testing.T) {
		// Create a sub-router with a single GenericRouteRegistrar
		subRouter := SubRouterConfig{
			PathPrefix:    "/api",
			GenericRoutes: MockGenericRouteConfig{},
		}

		// Register a generic route with the sub-router
		RegisterGenericRouteWithSubRouter[TestQueryRequest, TestQueryResponse, string, string](
			&subRouter,
			RouteConfig[TestQueryRequest, TestQueryResponse]{
				Path:      "/query",
				Methods:   []string{"GET"},
				AuthLevel: NoAuth,
				Codec:     queryCodec,
				Handler:   testQueryHandler,
			},
		)

		// Check that GenericRoutes is now a GenericRouteConfigs with two routes
		routes, ok := subRouter.GenericRoutes.(GenericRouteConfigs)
		if !ok {
			t.Errorf("Expected GenericRoutes to be a GenericRouteConfigs, got %T", subRouter.GenericRoutes)
		}
		if len(routes) != 2 {
			t.Errorf("Expected GenericRoutes to have 2 routes, got %d", len(routes))
		}
	})

	// Test case 4: GenericRoutes is something else
	t.Run("GenericRoutes is something else", func(t *testing.T) {
		// Create a sub-router with an invalid GenericRoutes type (string)
		subRouter := SubRouterConfig{
			PathPrefix:    "/api",
			GenericRoutes: "invalid",
		}

		// Register a generic route with the sub-router
		RegisterGenericRouteWithSubRouter[TestQueryRequest, TestQueryResponse, string, string](
			&subRouter,
			RouteConfig[TestQueryRequest, TestQueryResponse]{
				Path:      "/query",
				Methods:   []string{"GET"},
				AuthLevel: NoAuth,
				Codec:     queryCodec,
				Handler:   testQueryHandler,
			},
		)

		// Check that GenericRoutes is now a GenericRouteConfigs with one route
		routes, ok := subRouter.GenericRoutes.(GenericRouteConfigs)
		if !ok {
			t.Errorf("Expected GenericRoutes to be a GenericRouteConfigs, got %T", subRouter.GenericRoutes)
		}
		if len(routes) != 1 {
			t.Errorf("Expected GenericRoutes to have 1 route, got %d", len(routes))
		}
	})
}

// Test that nested SubRouters work correctly
func TestNestedSubRouters(t *testing.T) {
	// Create a logger
	logger, _ := zap.NewDevelopment()
	defer func() {
		_ = logger.Sync() // ignoring sync error
	}()

	// Define the auth function
	authFunction := func(ctx context.Context, token string) (string, bool) {
		if token != "" {
			return token, true
		}
		return "", false
	}

	// Define the function to get the user ID from a string
	userIdFromUserFunction := func(user string) string {
		return user
	}

	// Create a router
	r := NewRouter(RouterConfig{
		Logger:        logger,
		GlobalTimeout: 5 * time.Second,
	}, authFunction, userIdFromUserFunction)

	// Create a main API sub-router
	apiSubRouter := SubRouterConfig{
		PathPrefix: "/api",
		Routes: []RouteConfigBase{
			{
				Path:      "/status",
				Methods:   []string{"GET"},
				AuthLevel: NoAuth,
				Handler: func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{"status":"ok"}`))
				},
			},
		},
	}

	// Create a v1 sub-router
	apiV1SubRouter := SubRouterConfig{
		PathPrefix: "/v1",
		Routes: []RouteConfigBase{
			{
				Path:      "/hello",
				Methods:   []string{"GET"},
				AuthLevel: NoAuth,
				Handler: func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{"message":"Hello from API v1!"}`))
				},
			},
		},
	}

	// Create a users sub-router under v1
	usersV1SubRouter := SubRouterConfig{
		PathPrefix: "/users",
		Routes: []RouteConfigBase{
			{
				Path:      "",
				Methods:   []string{"GET"},
				AuthLevel: NoAuth,
				Handler: func(w http.ResponseWriter, r *http.Request) {
					w.Header().Set("Content-Type", "application/json")
					_, _ = w.Write([]byte(`{"users":[{"id":1,"name":"Alice"},{"id":2,"name":"Bob"}]}`))
				},
			},
		},
	}

	// Add the users sub-router to the v1 sub-router
	RegisterSubRouterWithSubRouter(&apiV1SubRouter, usersV1SubRouter)

	// Add the v1 sub-router to the main API sub-router
	RegisterSubRouterWithSubRouter(&apiSubRouter, apiV1SubRouter)

	// Register the main API sub-router with the router
	r.RegisterSubRouter(apiSubRouter)

	// Test the API status endpoint
	t.Run("API Status", func(t *testing.T) {
		// Create a request
		req := httptest.NewRequest("GET", "/api/status", nil)

		// Create a response recorder
		rr := httptest.NewRecorder()

		// Serve the request
		r.ServeHTTP(rr, req)

		// Check the status code
		if status := rr.Code; status != http.StatusOK {
			t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
		}

		// Check the response body
		expected := `{"status":"ok"}`
		if rr.Body.String() != expected {
			t.Errorf("handler returned unexpected body: got %v want %v", rr.Body.String(), expected)
		}
	})

	// Test the API v1 hello endpoint
	t.Run("API v1 Hello", func(t *testing.T) {
		// Create a request
		req := httptest.NewRequest("GET", "/api/v1/hello", nil)

		// Create a response recorder
		rr := httptest.NewRecorder()

		// Serve the request
		r.ServeHTTP(rr, req)

		// Check the status code
		if status := rr.Code; status != http.StatusOK {
			t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
		}

		// Check the response body
		expected := `{"message":"Hello from API v1!"}`
		if rr.Body.String() != expected {
			t.Errorf("handler returned unexpected body: got %v want %v", rr.Body.String(), expected)
		}
	})

	// Test the API v1 users endpoint
	t.Run("API v1 Users", func(t *testing.T) {
		// Create a request
		req := httptest.NewRequest("GET", "/api/v1/users", nil)

		// Create a response recorder
		rr := httptest.NewRecorder()

		// Serve the request
		r.ServeHTTP(rr, req)

		// Check the status code
		if status := rr.Code; status != http.StatusOK {
			t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
		}

		// Check the response body
		expected := `{"users":[{"id":1,"name":"Alice"},{"id":2,"name":"Bob"}]}`
		if rr.Body.String() != expected {
			t.Errorf("handler returned unexpected body: got %v want %v", rr.Body.String(), expected)
		}
	})
}
