package router

import (
	"context"
	"net/http/httptest"
	"testing"
)

func TestGetUser(t *testing.T) {
	// Test cases for different user types
	t.Run("string user", func(t *testing.T) {
		// Create a request with a string user in the context
		req := httptest.NewRequest("GET", "/", nil)
		user := "user123"
		ctx := context.WithValue(req.Context(), userObjectContextKey[string]{}, user)
		req = req.WithContext(ctx)

		// Get the user from the request
		gotUser, ok := GetUser[string, string](req)
		if !ok {
			t.Errorf("GetUser() ok = false, want true")
		}
		if gotUser != user {
			t.Errorf("GetUser() = %v, want %v", gotUser, user)
		}
	})

	t.Run("struct user", func(t *testing.T) {
		// Define a custom user type
		type User struct {
			ID   int
			Name string
		}

		// Create a request with a struct user in the context
		req := httptest.NewRequest("GET", "/", nil)
		user := User{ID: 42, Name: "John Doe"}
		ctx := context.WithValue(req.Context(), userObjectContextKey[User]{}, user)
		req = req.WithContext(ctx)

		// Get the user from the request
		gotUser, ok := GetUser[int, User](req)
		if !ok {
			t.Errorf("GetUser() ok = false, want true")
		}
		if gotUser.ID != user.ID || gotUser.Name != user.Name {
			t.Errorf("GetUser() = %v, want %v", gotUser, user)
		}
	})

	t.Run("pointer user", func(t *testing.T) {
		// Define a custom user type
		type User struct {
			ID   int
			Name string
		}

		// Create a request with a pointer user in the context
		req := httptest.NewRequest("GET", "/", nil)
		user := &User{ID: 42, Name: "John Doe"}
		ctx := context.WithValue(req.Context(), userObjectContextKey[*User]{}, user)
		req = req.WithContext(ctx)

		// Get the user from the request
		gotUser, ok := GetUser[int, *User](req)
		if !ok {
			t.Errorf("GetUser() ok = false, want true")
		}
		if gotUser.ID != user.ID || gotUser.Name != user.Name {
			t.Errorf("GetUser() = %v, want %v", gotUser, user)
		}
	})

	t.Run("no user", func(t *testing.T) {
		// Create a request without a user in the context
		req := httptest.NewRequest("GET", "/", nil)

		// Get the user from the request
		_, ok := GetUser[string, string](req)
		if ok {
			t.Errorf("GetUser() ok = true, want false")
		}
	})

	t.Run("wrong type", func(t *testing.T) {
		// Create a request with a string user in the context
		req := httptest.NewRequest("GET", "/", nil)
		user := "user123"
		ctx := context.WithValue(req.Context(), userObjectContextKey[string]{}, user)
		req = req.WithContext(ctx)

		// Try to get the user with a different type
		_, ok := GetUser[string, int](req)
		if ok {
			t.Errorf("GetUser() ok = true, want false")
		}
	})
}
