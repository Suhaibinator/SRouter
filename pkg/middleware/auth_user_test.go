package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Suhaibinator/SRouter/pkg/scontext" // Added import
	"go.uber.org/zap"
)

// User is a test user type
type User struct {
	ID    string
	Name  string
	Email string
	Roles []string
}

func TestBasicUserAuthProvider(t *testing.T) {
	// Create a basic user auth provider
	provider := &BasicUserAuthProvider[User]{
		GetUserFunc: func(username, password string) (*User, error) {
			if username == "user1" && password == "pass1" {
				return &User{
					ID:    "1",
					Name:  "User One",
					Email: "user1@example.com",
					Roles: []string{"user"},
				}, nil
			}
			return nil, errors.New("invalid credentials")
		},
	}

	// Test valid credentials
	req, _ := http.NewRequest("GET", "/", nil)
	req.SetBasicAuth("user1", "pass1")
	user, err := provider.AuthenticateUser(req)
	if err != nil {
		t.Errorf("Expected authentication to succeed with valid credentials, got error: %v", err)
	}
	if user == nil {
		t.Error("Expected user to be returned, got nil")
	} else if user.ID != "1" {
		t.Errorf("Expected user ID to be '1', got '%s'", user.ID)
	}

	// Test invalid password
	req, _ = http.NewRequest("GET", "/", nil)
	req.SetBasicAuth("user1", "wrongpass")
	user, err = provider.AuthenticateUser(req)
	if err == nil {
		t.Error("Expected authentication to fail with invalid password")
	}
	if user != nil {
		t.Error("Expected user to be nil with invalid password")
	}

	// Test missing auth header
	req, _ = http.NewRequest("GET", "/", nil)
	user, err = provider.AuthenticateUser(req)
	if err == nil {
		t.Error("Expected authentication to fail with missing auth header")
	}
	if user != nil {
		t.Error("Expected user to be nil with missing auth header")
	}
}

func TestBearerTokenUserAuthProvider(t *testing.T) {
	// Create a bearer token user auth provider
	provider := &BearerTokenUserAuthProvider[User]{
		GetUserFunc: func(token string) (*User, error) {
			if token == "token1" {
				return &User{
					ID:    "1",
					Name:  "User One",
					Email: "user1@example.com",
					Roles: []string{"user"},
				}, nil
			}
			return nil, errors.New("invalid token")
		},
	}

	// Test valid token
	req, _ := http.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer token1")
	user, err := provider.AuthenticateUser(req)
	if err != nil {
		t.Errorf("Expected authentication to succeed with valid token, got error: %v", err)
	}
	if user == nil {
		t.Error("Expected user to be returned, got nil")
	} else if user.ID != "1" {
		t.Errorf("Expected user ID to be '1', got '%s'", user.ID)
	}

	// Test invalid token
	req, _ = http.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer wrongtoken")
	user, err = provider.AuthenticateUser(req)
	if err == nil {
		t.Error("Expected authentication to fail with invalid token")
	}
	if user != nil {
		t.Error("Expected user to be nil with invalid token")
	}

	// Test missing auth header
	req, _ = http.NewRequest("GET", "/", nil)
	user, err = provider.AuthenticateUser(req)
	if err == nil {
		t.Error("Expected authentication to fail with missing auth header")
	}
	if user != nil {
		t.Error("Expected user to be nil with missing auth header")
	}

	// Test wrong auth header format
	req, _ = http.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "NotBearer token1")
	user, err = provider.AuthenticateUser(req)
	if err == nil {
		t.Error("Expected authentication to fail with wrong auth header format")
	}
	if user != nil {
		t.Error("Expected user to be nil with wrong auth header format")
	}
}

func TestAPIKeyUserAuthProvider(t *testing.T) {
	// Create an API key user auth provider
	provider := &APIKeyUserAuthProvider[User]{
		GetUserFunc: func(key string) (*User, error) {
			if key == "key1" {
				return &User{
					ID:    "1",
					Name:  "User One",
					Email: "user1@example.com",
					Roles: []string{"user"},
				}, nil
			}
			return nil, errors.New("invalid API key")
		},
		Header: "X-API-Key",
		Query:  "api_key",
	}

	// Test valid header
	req, _ := http.NewRequest("GET", "/", nil)
	req.Header.Set("X-API-Key", "key1")
	user, err := provider.AuthenticateUser(req)
	if err != nil {
		t.Errorf("Expected authentication to succeed with valid header, got error: %v", err)
	}
	if user == nil {
		t.Error("Expected user to be returned, got nil")
	} else if user.ID != "1" {
		t.Errorf("Expected user ID to be '1', got '%s'", user.ID)
	}

	// Test valid query parameter
	req, _ = http.NewRequest("GET", "/?api_key=key1", nil)
	user, err = provider.AuthenticateUser(req)
	if err != nil {
		t.Errorf("Expected authentication to succeed with valid query parameter, got error: %v", err)
	}
	if user == nil {
		t.Error("Expected user to be returned, got nil")
	} else if user.ID != "1" {
		t.Errorf("Expected user ID to be '1', got '%s'", user.ID)
	}

	// Test invalid header
	req, _ = http.NewRequest("GET", "/", nil)
	req.Header.Set("X-API-Key", "wrongkey")
	user, err = provider.AuthenticateUser(req)
	if err == nil {
		t.Error("Expected authentication to fail with invalid header")
	}
	if user != nil {
		t.Error("Expected user to be nil with invalid header")
	}

	// Test invalid query parameter
	req, _ = http.NewRequest("GET", "/?api_key=wrongkey", nil)
	user, err = provider.AuthenticateUser(req)
	if err == nil {
		t.Error("Expected authentication to fail with invalid query parameter")
	}
	if user != nil {
		t.Error("Expected user to be nil with invalid query parameter")
	}

	// Test missing auth
	req, _ = http.NewRequest("GET", "/", nil)
	user, err = provider.AuthenticateUser(req)
	if err == nil {
		t.Error("Expected authentication to fail with missing auth")
	}
	if user != nil {
		t.Error("Expected user to be nil with missing auth")
	}

	// Test header only
	provider = &APIKeyUserAuthProvider[User]{
		GetUserFunc: func(key string) (*User, error) {
			if key == "key1" {
				return &User{
					ID:    "1",
					Name:  "User One",
					Email: "user1@example.com",
					Roles: []string{"user"},
				}, nil
			}
			return nil, errors.New("invalid API key")
		},
		Header: "X-API-Key",
	}

	req, _ = http.NewRequest("GET", "/", nil)
	req.Header.Set("X-API-Key", "key1")
	user, err = provider.AuthenticateUser(req)
	if err != nil {
		t.Errorf("Expected authentication to succeed with valid header (header only), got error: %v", err)
	}
	if user == nil {
		t.Error("Expected user to be returned, got nil")
	}

	req, _ = http.NewRequest("GET", "/?api_key=key1", nil)
	user, err = provider.AuthenticateUser(req)
	if err == nil {
		t.Error("Expected authentication to fail with query parameter (header only)")
	}
	if user != nil {
		t.Error("Expected user to be nil with query parameter (header only)")
	}

	// Test query only
	provider = &APIKeyUserAuthProvider[User]{
		GetUserFunc: func(key string) (*User, error) {
			if key == "key1" {
				return &User{
					ID:    "1",
					Name:  "User One",
					Email: "user1@example.com",
					Roles: []string{"user"},
				}, nil
			}
			return nil, errors.New("invalid API key")
		},
		Query: "api_key",
	}

	req, _ = http.NewRequest("GET", "/?api_key=key1", nil)
	user, err = provider.AuthenticateUser(req)
	if err != nil {
		t.Errorf("Expected authentication to succeed with valid query parameter (query only), got error: %v", err)
	}
	if user == nil {
		t.Error("Expected user to be returned, got nil")
	}

	req, _ = http.NewRequest("GET", "/", nil)
	req.Header.Set("X-API-Key", "key1")
	user, err = provider.AuthenticateUser(req)
	if err == nil {
		t.Error("Expected authentication to fail with header (query only)")
	}
	if user != nil {
		t.Error("Expected user to be nil with header (query only)")
	}
}

func TestAuthenticationWithUserProvider(t *testing.T) {
	// Create a logger
	logger, _ := zap.NewDevelopment()

	// Create a provider
	provider := &BasicUserAuthProvider[User]{
		GetUserFunc: func(username, password string) (*User, error) {
			if username == "user1" && password == "pass1" {
				return &User{
					ID:    "1",
					Name:  "User One",
					Email: "user1@example.com",
					Roles: []string{"user"},
				}, nil
			}
			return nil, errors.New("invalid credentials")
		},
	}

	// Create a handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get the user from the context
		user, ok := scontext.GetUserFromRequest[string, User](r) // Use scontext
		if !ok || user == nil {
			t.Error("Expected user to be in context, got nil")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if user.ID != "1" {
			t.Errorf("Expected user ID to be '1', got '%s'", user.ID)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte("OK"))
		if err != nil {
			t.Fatalf("Failed to write response: %v", err)
		}
	})

	// Create a middleware
	middleware := AuthenticationWithUserProvider[string](provider, logger)

	// Wrap the handler
	wrappedHandler := middleware(handler)

	// Test with valid credentials
	req, _ := http.NewRequest("GET", "/", nil)
	req.SetBasicAuth("user1", "pass1")
	rr := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(rr, req)

	// Check status code
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rr.Code)
	}

	// Test with invalid credentials
	req, _ = http.NewRequest("GET", "/", nil)
	req.SetBasicAuth("user1", "wrongpass")
	rr = httptest.NewRecorder()
	wrappedHandler.ServeHTTP(rr, req)

	// Check status code
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Expected status code %d, got %d", http.StatusUnauthorized, rr.Code)
	}
}

func TestAuthenticationWithUser(t *testing.T) {
	// Create a handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get the user from the context
		user, ok := scontext.GetUserFromRequest[string, User](r) // Use scontext
		if !ok || user == nil {
			t.Error("Expected user to be in context, got nil")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if user.ID != "1" {
			t.Errorf("Expected user ID to be '1', got '%s'", user.ID)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte("OK"))
		if err != nil {
			t.Fatalf("Failed to write response: %v", err)
		}
	})

	// Create an auth function
	authFunc := func(r *http.Request) (*User, error) {
		if r.Header.Get("X-Auth") == "valid" {
			return &User{
				ID:    "1",
				Name:  "User One",
				Email: "user1@example.com",
				Roles: []string{"user"},
			}, nil
		}
		return nil, errors.New("invalid auth")
	}

	// Create a middleware
	middleware := AuthenticationWithUser[string](authFunc)

	// Wrap the handler
	wrappedHandler := middleware(handler)

	// Test with valid auth
	req, _ := http.NewRequest("GET", "/", nil)
	req.Header.Set("X-Auth", "valid")
	rr := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(rr, req)

	// Check status code
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rr.Code)
	}

	// Test with invalid auth
	req, _ = http.NewRequest("GET", "/", nil)
	req.Header.Set("X-Auth", "invalid")
	rr = httptest.NewRecorder()
	wrappedHandler.ServeHTTP(rr, req)

	// Check status code
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Expected status code %d, got %d", http.StatusUnauthorized, rr.Code)
	}
}

func TestGetUser(t *testing.T) {
	// Create a user
	user := &User{
		ID:    "1",
		Name:  "User One",
		Email: "user1@example.com",
		Roles: []string{"user"},
	}

	// Create a request with the user in the context using the new context wrapper
	req, _ := http.NewRequest("GET", "/", nil)
	ctx := scontext.WithUser[string](req.Context(), user) // Use scontext
	req = req.WithContext(ctx)

	// Get the user from the context
	retrievedUser, _ := scontext.GetUserFromRequest[string, User](req) // Use scontext
	if retrievedUser == nil {
		t.Error("Expected user to be retrieved, got nil")
	} else if retrievedUser.ID != "1" {
		t.Errorf("Expected user ID to be '1', got '%s'", retrievedUser.ID)
	}

	// Test with no user in context
	req, _ = http.NewRequest("GET", "/", nil)
	retrievedUser, _ = scontext.GetUserFromRequest[string, User](req) // Use scontext
	if retrievedUser != nil {
		t.Error("Expected nil user when no user in context")
	}

	// Test with an empty context (no user stored)
	req, _ = http.NewRequest("GET", "/", nil)
	retrievedUser, _ = scontext.GetUserFromRequest[string, User](req) // Use scontext
	if retrievedUser != nil {
		t.Error("Expected nil user when wrong type in context")
	}
}

func TestNewBearerTokenWithUserMiddleware(t *testing.T) {
	// Create a logger
	logger, _ := zap.NewDevelopment()

	// Create a middleware
	middleware := NewBearerTokenWithUserMiddleware[string](
		func(token string) (*User, error) {
			if token == "token1" {
				return &User{
					ID:    "1",
					Name:  "User One",
					Email: "user1@example.com",
					Roles: []string{"user"},
				}, nil
			}
			return nil, errors.New("invalid token")
		},
		logger,
	)

	// Create a handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get the user from the context
		user, ok := scontext.GetUserFromRequest[string, User](r) // Use scontext
		if !ok || user == nil {
			t.Error("Expected user to be in context, got nil")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if user.ID != "1" {
			t.Errorf("Expected user ID to be '1', got '%s'", user.ID)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte("OK"))
		if err != nil {
			t.Fatalf("Failed to write response: %v", err)
		}
	})

	// Wrap the handler
	wrappedHandler := middleware(handler)

	// Test with valid token
	req, _ := http.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer token1")
	rr := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(rr, req)

	// Check status code
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rr.Code)
	}

	// Test with invalid token
	req, _ = http.NewRequest("GET", "/", nil)
	req.Header.Set("Authorization", "Bearer wrongtoken")
	rr = httptest.NewRecorder()
	wrappedHandler.ServeHTTP(rr, req)

	// Check status code
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Expected status code %d, got %d", http.StatusUnauthorized, rr.Code)
	}
}

func TestNewAPIKeyWithUserMiddleware(t *testing.T) {
	// Create a logger
	logger, _ := zap.NewDevelopment()

	// Create a middleware
	middleware := NewAPIKeyWithUserMiddleware[string](
		func(key string) (*User, error) {
			if key == "key1" {
				return &User{
					ID:    "1",
					Name:  "User One",
					Email: "user1@example.com",
					Roles: []string{"user"},
				}, nil
			}
			return nil, errors.New("invalid API key")
		},
		"X-API-Key",
		"api_key",
		logger,
	)

	// Create a handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Get the user from the context
		user, ok := scontext.GetUserFromRequest[string, User](r) // Use scontext
		if !ok || user == nil {
			t.Error("Expected user to be in context, got nil")
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		if user.ID != "1" {
			t.Errorf("Expected user ID to be '1', got '%s'", user.ID)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, err := w.Write([]byte("OK"))
		if err != nil {
			t.Fatalf("Failed to write response: %v", err)
		}
	})

	// Wrap the handler
	wrappedHandler := middleware(handler)

	// Test with valid header
	req, _ := http.NewRequest("GET", "/", nil)
	req.Header.Set("X-API-Key", "key1")
	rr := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(rr, req)

	// Check status code
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rr.Code)
	}

	// Test with valid query parameter
	req, _ = http.NewRequest("GET", "/?api_key=key1", nil)
	rr = httptest.NewRecorder()
	wrappedHandler.ServeHTTP(rr, req)

	// Check status code
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rr.Code)
	}

	// Test with invalid header
	req, _ = http.NewRequest("GET", "/", nil)
	req.Header.Set("X-API-Key", "wrongkey")
	rr = httptest.NewRecorder()
	wrappedHandler.ServeHTTP(rr, req)

	// Check status code
	if rr.Code != http.StatusUnauthorized {
		t.Errorf("Expected status code %d, got %d", http.StatusUnauthorized, rr.Code)
	}
}
