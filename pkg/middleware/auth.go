// Package middleware provides a collection of HTTP middleware components for the SRouter framework.
package middleware

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Suhaibinator/SRouter/pkg/common"
	"go.uber.org/zap"
)

// AuthProvider defines an interface for authentication providers.
// Different authentication mechanisms can implement this interface
// to be used with the AuthenticationWithProvider middleware.
// The framework includes several implementations: BasicAuthProvider,
// BearerTokenProvider, and APIKeyProvider.
// The type parameter T represents the user ID type, which can be any comparable type.
type AuthProvider[T comparable] interface {
	// Authenticate authenticates a request and returns the user ID if authentication is successful.
	// It examines the request for authentication credentials (such as headers, cookies, or query parameters)
	// and validates them according to the provider's implementation.
	// Returns the user ID if the request is authenticated, the zero value of T otherwise.
	Authenticate(r *http.Request) (T, bool)
}

// BearerTokenProvider provides Bearer Token Authentication.
// It can validate tokens against a predefined map or using a custom validator function.
// The type parameter T represents the user ID type, which can be any comparable type.
type BearerTokenProvider[T comparable] struct {
	ValidTokens map[string]T                 // token -> user ID
	Validator   func(token string) (T, bool) // optional token validator
}

// Authenticate authenticates a request using Bearer Token Authentication.
// It extracts the token from the Authorization header and validates it
// using either the validator function (if provided) or the ValidTokens map.
// Returns the user ID if authentication is successful, the zero value of T and false otherwise.
func (p *BearerTokenProvider[T]) Authenticate(r *http.Request) (T, bool) {
	var zeroValue T

	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return zeroValue, false
	}

	// Check if the header starts with "Bearer "
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return zeroValue, false
	}

	// Extract the token
	token := strings.TrimPrefix(authHeader, "Bearer ")

	// If a validator is provided, use it
	if p.Validator != nil {
		return p.Validator(token)
	}

	// Otherwise, check if the token is in the valid tokens map
	if userID, ok := p.ValidTokens[token]; ok {
		return userID, true
	}

	return zeroValue, false
}

// APIKeyProvider provides API Key Authentication.
// It can validate API keys provided in a header or query parameter.
// The type parameter T represents the user ID type, which can be any comparable type.
type APIKeyProvider[T comparable] struct {
	ValidKeys map[string]T // key -> user ID
	Header    string       // header name (e.g., "X-API-Key")
	Query     string       // query parameter name (e.g., "api_key")
}

// Authenticate authenticates a request using API Key Authentication.
// It checks for the API key in either the specified header or query parameter
// and validates it against the stored valid keys.
// Returns the user ID if authentication is successful, the zero value of T and false otherwise.
func (p *APIKeyProvider[T]) Authenticate(r *http.Request) (T, bool) {
	var zeroValue T

	// Check header
	if p.Header != "" {
		key := r.Header.Get(p.Header)
		if key != "" {
			if userID, ok := p.ValidKeys[key]; ok {
				return userID, true
			}
		}
	}

	// Check query parameter
	if p.Query != "" {
		key := r.URL.Query().Get(p.Query)
		if key != "" {
			if userID, ok := p.ValidKeys[key]; ok {
				return userID, true
			}
		}
	}

	return zeroValue, false
}

// AuthenticationWithProvider is a middleware that checks if a request is authenticated
// using the provided auth provider. If authentication fails, it returns a 401 Unauthorized response.
// This middleware allows for flexible authentication mechanisms by accepting any AuthProvider implementation.
// T is the User ID type (comparable), U is the User object type (any).
//
// This implementation uses the SRouterContext approach to store the authenticated user ID,
// which avoids deep nesting of context values by using a single wrapper structure. The type
// parameters allow for type-safe access to the user ID without type assertions.
func AuthenticationWithProvider[T comparable, U any](
	provider AuthProvider[T],
	logger *zap.Logger,
) common.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Check if the request is authenticated
			if r.Method == http.MethodOptions {
				// Allow preflight requests without authentication
				next.ServeHTTP(w, r)
				return
			}
			userID, ok := provider.Authenticate(r)
			if !ok {
				logger.Warn("Authentication failed",
					zap.String("method", r.Method),
					zap.String("path", r.URL.Path),
					zap.String("remote_addr", r.RemoteAddr),
				)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			// Store the user ID in the SRouterContext
			ctx := WithUserID[T, U](r.Context(), userID)

			// Call next handler with the updated context
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// Authentication is a middleware that checks if a request is authenticated using a simple auth function.
// T is the User ID type (comparable), U is the User object type (any).
// It allows for custom authentication logic to be provided as a simple function.
func Authentication[T comparable, U any](
	authFunc func(*http.Request) (T, bool),
) common.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodOptions {
				// Allow preflight requests without authentication
				next.ServeHTTP(w, r)
				return
			}
			// Check if the request is authenticated
			userID, ok := authFunc(r)
			if !ok {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			// Store the user ID in the SRouterContext
			ctx := WithUserID[T, U](r.Context(), userID)

			// Call next handler with the updated context
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// AuthenticationBool is a middleware that checks if a request is authenticated using a simple auth function.
// It allows for custom authentication logic to be provided as a simple function that returns a boolean.
// It adds a boolean flag to the SRouterContext if authentication is successful.
// T is the User ID type (comparable), U is the User object type (any).
func AuthenticationBool[T comparable, U any](
	authFunc func(*http.Request) bool,
	flagName string, // Flag name parameter
) common.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.Method == http.MethodOptions {
				// Allow preflight requests without authentication
				next.ServeHTTP(w, r)
				return
			}
			// Check if the request is authenticated
			if !authFunc(r) {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			// Store the flag in the SRouterContext
			ctx := WithFlag[T, U](r.Context(), flagName, true)

			// Call next handler with the updated context
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// NewBearerTokenMiddleware creates a middleware that uses Bearer Token Authentication.
// T is the User ID type (comparable), U is the User object type (any).
func NewBearerTokenMiddleware[T comparable, U any](
	validTokens map[string]T,
	logger *zap.Logger,
) common.Middleware {
	provider := &BearerTokenProvider[T]{
		ValidTokens: validTokens,
	}
	return AuthenticationWithProvider[T, U](provider, logger)
}

// NewBearerTokenValidatorMiddleware creates a middleware that uses Bearer Token Authentication
// NewBearerTokenValidatorMiddleware creates a middleware that uses Bearer Token Authentication
// with a custom validator function.
// T is the User ID type (comparable), U is the User object type (any).
func NewBearerTokenValidatorMiddleware[T comparable, U any](
	validator func(string) (T, bool),
	logger *zap.Logger,
) common.Middleware {
	provider := &BearerTokenProvider[T]{
		Validator: validator,
	}
	return AuthenticationWithProvider[T, U](provider, logger)
}

// NewAPIKeyMiddleware creates a middleware that uses API Key Authentication.
// T is the User ID type (comparable), U is the User object type (any).
func NewAPIKeyMiddleware[T comparable, U any](
	validKeys map[string]T,
	header, query string,
	logger *zap.Logger,
) common.Middleware {
	provider := &APIKeyProvider[T]{
		ValidKeys: validKeys,
		Header:    header,
		Query:     query,
	}
	return AuthenticationWithProvider[T, U](provider, logger)
}

// UserAuthProvider defines an interface for authentication providers that return a user object.
// Different authentication mechanisms can implement this interface
// to be used with the AuthenticationWithUserProvider middleware.
type UserAuthProvider[T any] interface {
	// AuthenticateUser authenticates a request and returns the user object if authentication is successful.
	// It examines the request for authentication credentials (such as headers, cookies, or query parameters)
	// and validates them according to the provider's implementation.
	// Returns the user object if the request is authenticated, nil and an error otherwise.
	AuthenticateUser(r *http.Request) (*T, error)
}

// BasicUserAuthProvider provides HTTP Basic Authentication with user object return.
type BasicUserAuthProvider[T any] struct {
	GetUserFunc func(username, password string) (*T, error)
}

// AuthenticateUser authenticates a request using HTTP Basic Authentication.
// It extracts the username and password from the Authorization header
// and validates them using the GetUserFunc.
// Returns the user object if authentication is successful, nil and an error otherwise.
func (p *BasicUserAuthProvider[T]) AuthenticateUser(r *http.Request) (*T, error) {
	username, password, ok := r.BasicAuth()
	if !ok {
		return nil, errors.New("no basic auth credentials")
	}

	return p.GetUserFunc(username, password)
}

// BearerTokenUserAuthProvider provides Bearer Token Authentication with user object return.
type BearerTokenUserAuthProvider[T any] struct {
	GetUserFunc func(token string) (*T, error)
}

// AuthenticateUser authenticates a request using Bearer Token Authentication.
// It extracts the token from the Authorization header and validates it
// using the GetUserFunc.
// Returns the user object if authentication is successful, nil and an error otherwise.
func (p *BearerTokenUserAuthProvider[T]) AuthenticateUser(r *http.Request) (*T, error) {
	authHeader := r.Header.Get("Authorization")
	if authHeader == "" {
		return nil, errors.New("no authorization header")
	}

	// Check if the header starts with "Bearer "
	if !strings.HasPrefix(authHeader, "Bearer ") {
		return nil, errors.New("invalid authorization header format")
	}

	// Extract the token
	token := strings.TrimPrefix(authHeader, "Bearer ")

	return p.GetUserFunc(token)
}

// APIKeyUserAuthProvider provides API Key Authentication with user object return.
type APIKeyUserAuthProvider[T any] struct {
	GetUserFunc func(key string) (*T, error)
	Header      string // header name (e.g., "X-API-Key")
	Query       string // query parameter name (e.g., "api_key")
}

// AuthenticateUser authenticates a request using API Key Authentication.
// It checks for the API key in either the specified header or query parameter
// and validates it using the GetUserFunc.
// Returns the user object if authentication is successful, nil and an error otherwise.
func (p *APIKeyUserAuthProvider[T]) AuthenticateUser(r *http.Request) (*T, error) {
	// Check header
	if p.Header != "" {
		key := r.Header.Get(p.Header)
		if key != "" {
			return p.GetUserFunc(key)
		}
	}

	// Check query parameter
	if p.Query != "" {
		key := r.URL.Query().Get(p.Query)
		if key != "" {
			return p.GetUserFunc(key)
		}
	}

	return nil, errors.New("no API key found")
}

// AuthenticationWithUserProvider is a middleware that uses an auth provider that returns a user object
// and adds it to the SRouterContext if authentication is successful.
// T is the User ID type (comparable), U is the User object type (any).
func AuthenticationWithUserProvider[T comparable, U any](
	provider UserAuthProvider[U], // Provider uses U
	logger *zap.Logger,
) common.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Authenticate the request
			user, err := provider.AuthenticateUser(r)
			if err != nil || user == nil {
				logger.Warn("Authentication failed",
					zap.Error(err),
					zap.String("method", r.Method),
					zap.String("path", r.URL.Path),
					zap.String("remote_addr", r.RemoteAddr),
				)
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			// Store the user object in the SRouterContext
			ctx := WithUser[T](r.Context(), user)

			// Call next handler with the updated context
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// AuthenticationWithUser is a middleware that uses a custom auth function that returns a user object
// and adds it to the SRouterContext if authentication is successful.
// T is the User ID type (comparable), U is the User object type (any).
func AuthenticationWithUser[T comparable, U any](
	authFunc func(*http.Request) (*U, error), // Auth func uses U
) common.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Authenticate the request
			user, err := authFunc(r)
			if err != nil || user == nil {
				http.Error(w, "Unauthorized", http.StatusUnauthorized)
				return
			}

			// Store the user object in the SRouterContext
			ctx := WithUser[T](r.Context(), user)

			// Call next handler with the updated context
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// NewBearerTokenWithUserMiddleware creates a middleware that uses Bearer Token Authentication
// NewBearerTokenWithUserMiddleware creates a middleware that uses Bearer Token Authentication
// and returns a user object, adding it to the SRouterContext.
// T is the User ID type (comparable), U is the User object type (any).
func NewBearerTokenWithUserMiddleware[T comparable, U any](
	getUserFunc func(token string) (*U, error),
	logger *zap.Logger,
) common.Middleware {
	provider := &BearerTokenUserAuthProvider[U]{
		GetUserFunc: getUserFunc,
	}
	return AuthenticationWithUserProvider[T](provider, logger)
}

// NewAPIKeyWithUserMiddleware creates a middleware that uses API Key Authentication
// NewAPIKeyWithUserMiddleware creates a middleware that uses API Key Authentication
// and returns a user object, adding it to the SRouterContext.
// T is the User ID type (comparable), U is the User object type (any).
func NewAPIKeyWithUserMiddleware[T comparable, U any](
	getUserFunc func(key string) (*U, error),
	header, query string,
	logger *zap.Logger,
) common.Middleware {
	provider := &APIKeyUserAuthProvider[U]{
		GetUserFunc: getUserFunc,
		Header:      header,
		Query:       query,
	}
	return AuthenticationWithUserProvider[T](provider, logger)
}
