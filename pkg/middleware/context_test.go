package middleware

import (
	"context"
	"reflect"
	"testing"
)

// TestGetAllFlagsFromContext tests the getAllFlagsFromContext function
// including all edge cases and non-happy paths
func TestGetAllFlagsFromContext(t *testing.T) {
	// Test case 1: Empty context (should return nil)
	t.Run("EmptyContext", func(t *testing.T) {
		ctx := context.Background()
		result := getAllFlagsFromContext(ctx)
		if result != nil {
			t.Errorf("Expected nil for empty context, got %v", result)
		}
	})

	// Test case 2: Context with non-SRouterContext value (should return nil)
	t.Run("NonSRouterContextValue", func(t *testing.T) {
		// Use a different key type to ensure no collision with sRouterContextKey
		type differentKeyType struct{}
		differentKey := differentKeyType{}

		ctx := context.WithValue(context.Background(), differentKey, "some value")
		result := getAllFlagsFromContext(ctx)
		if result != nil {
			t.Errorf("Expected nil for context with non-SRouterContext value, got %v", result)
		}
	})

	// Test case 3: Context with value of wrong type for sRouterContextKey (should return nil)
	t.Run("WrongTypeValue", func(t *testing.T) {
		ctx := context.WithValue(context.Background(), sRouterContextKey{}, "wrong type")
		result := getAllFlagsFromContext(ctx)
		if result != nil {
			t.Errorf("Expected nil for context with wrong type value, got %v", result)
		}
	})

	// Test case 4: Context with string SRouterContext but nil Flags (should return nil)
	t.Run("StringSRouterContextNilFlags", func(t *testing.T) {
		rc := &SRouterContext[string, any]{
			UserID: "user123",
			Flags:  nil, // nil flags
		}
		ctx := context.WithValue(context.Background(), sRouterContextKey{}, rc)
		result := getAllFlagsFromContext(ctx)
		if result != nil {
			t.Errorf("Expected nil for context with nil Flags, got %v", result)
		}
	})

	// Test case 5: Context with int SRouterContext but nil Flags (should return nil)
	t.Run("IntSRouterContextNilFlags", func(t *testing.T) {
		rc := &SRouterContext[int, any]{
			UserID: 123,
			Flags:  nil, // nil flags
		}
		ctx := context.WithValue(context.Background(), sRouterContextKey{}, rc)
		result := getAllFlagsFromContext(ctx)
		if result != nil {
			t.Errorf("Expected nil for context with nil Flags, got %v", result)
		}
	})

	// Test case 6: Context with string SRouterContext and empty Flags (should return empty map)
	t.Run("StringSRouterContextEmptyFlags", func(t *testing.T) {
		rc := &SRouterContext[string, any]{
			UserID: "user123",
			Flags:  make(map[string]bool), // empty flags map
		}
		ctx := context.WithValue(context.Background(), sRouterContextKey{}, rc)
		result := getAllFlagsFromContext(ctx)
		if result == nil {
			t.Errorf("Expected empty map for context with empty Flags, got nil")
		} else if len(result) != 0 {
			t.Errorf("Expected empty map for context with empty Flags, got %v", result)
		}
	})

	// Test case 7: Context with int SRouterContext and empty Flags (should return empty map)
	t.Run("IntSRouterContextEmptyFlags", func(t *testing.T) {
		rc := &SRouterContext[int, any]{
			UserID: 123,
			Flags:  make(map[string]bool), // empty flags map
		}
		ctx := context.WithValue(context.Background(), sRouterContextKey{}, rc)
		result := getAllFlagsFromContext(ctx)
		if result == nil {
			t.Errorf("Expected empty map for context with empty Flags, got nil")
		} else if len(result) != 0 {
			t.Errorf("Expected empty map for context with empty Flags, got %v", result)
		}
	})

	// Test case 8: Happy path - string SRouterContext with Flags (should return flags)
	t.Run("StringSRouterContextWithFlags", func(t *testing.T) {
		expected := map[string]bool{
			"flag1": true,
			"flag2": false,
		}
		rc := &SRouterContext[string, any]{
			UserID: "user123",
			Flags:  expected,
		}
		ctx := context.WithValue(context.Background(), sRouterContextKey{}, rc)
		result := getAllFlagsFromContext(ctx)
		if !reflect.DeepEqual(result, expected) {
			t.Errorf("Expected %v, got %v", expected, result)
		}
	})

	// Test case 9: Happy path - int SRouterContext with Flags (should return flags)
	t.Run("IntSRouterContextWithFlags", func(t *testing.T) {
		expected := map[string]bool{
			"flag1": true,
			"flag2": false,
		}
		rc := &SRouterContext[int, any]{
			UserID: 123,
			Flags:  expected,
		}
		ctx := context.WithValue(context.Background(), sRouterContextKey{}, rc)
		result := getAllFlagsFromContext(ctx)
		if !reflect.DeepEqual(result, expected) {
			t.Errorf("Expected %v, got %v", expected, result)
		}
	})

	// Test case 10: Context with unusual SRouterContext type that doesn't match known ones (should return nil)
	t.Run("UnusualSRouterContextType", func(t *testing.T) {
		// We can't directly create a *SRouterContext[float64, any] due to type checking,
		// but we can mimic this situation by using an interface value with a different type
		// This simulates a case where the type parameters don't match the common ones we check for
		type unusualStruct struct {
			Flags map[string]bool
		}
		unusual := &unusualStruct{
			Flags: map[string]bool{"flag": true},
		}
		ctx := context.WithValue(context.Background(), sRouterContextKey{}, unusual)
		result := getAllFlagsFromContext(ctx)
		if result != nil {
			t.Errorf("Expected nil for unusual SRouterContext type, got %v", result)
		}
	})
}
