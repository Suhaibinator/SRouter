package router

import (
	"context"
	"reflect"
	"testing"
)

// TestRouterContext tests the router context wrapper functionality
func TestRouterContext(t *testing.T) {
	// Create a mock router for testing
	// Using string as T and any as U
	router := &Router[string, any]{}

	// Test case 1: Creating a new router context
	t.Run("NewRouterContext", func(t *testing.T) {
		rc := router.NewRouterContext()
		if rc == nil {
			t.Fatal("Expected non-nil router context")
		}
		if rc.Flags == nil {
			t.Error("Expected non-nil flags map")
		}
		if len(rc.Flags) != 0 {
			t.Errorf("Expected empty flags map, got %v", rc.Flags)
		}
	})

	// Test case 2: Get router context from empty context
	t.Run("GetRouterContext_EmptyContext", func(t *testing.T) {
		ctx := context.Background()
		rc, ok := router.GetRouterContext(ctx)
		if ok {
			t.Error("Expected false for empty context, got true")
		}
		if rc != nil {
			t.Errorf("Expected nil router context, got %v", rc)
		}
	})

	// Test case 3: Ensure router context creates one if not present
	t.Run("EnsureRouterContext_EmptyContext", func(t *testing.T) {
		ctx := context.Background()
		rc, newCtx := router.EnsureRouterContext(ctx)
		if rc == nil {
			t.Fatal("Expected non-nil router context")
		}
		if newCtx == ctx {
			t.Error("Expected different context after ensuring router context")
		}

		// Verify we can retrieve the context
		retrievedRC, ok := router.GetRouterContext(newCtx)
		if !ok {
			t.Error("Expected to find router context, got false")
		}
		if retrievedRC != rc {
			t.Error("Expected same router context instance")
		}
	})

	// Test case 4: Ensure router context reuses existing one
	t.Run("EnsureRouterContext_ExistingContext", func(t *testing.T) {
		// Create a context with router context
		rc := router.NewRouterContext()
		ctx := router.WithRouterContext(context.Background(), rc)

		// Ensure router context
		rc2, newCtx := router.EnsureRouterContext(ctx)
		if rc2 != rc {
			t.Error("Expected same router context instance, got different one")
		}
		if newCtx != ctx {
			t.Error("Expected same context, got different one")
		}
	})

	// Test case 5: With/Get UserID
	t.Run("WithUserID_GetUserIDFromContext", func(t *testing.T) {
		// Add user ID to context
		ctx := context.Background()
		userID := "user123"
		ctx = router.WithUserID(ctx, userID)

		// Get user ID from context
		retrievedID, ok := router.GetUserIDFromContext(ctx)
		if !ok {
			t.Error("Expected to find user ID, got false")
		}
		if retrievedID != userID {
			t.Errorf("Expected user ID %s, got %s", userID, retrievedID)
		}
	})

	// Test case 6: Get nonexistent UserID
	t.Run("GetUserIDFromContext_Nonexistent", func(t *testing.T) {
		// Get user ID from empty context
		ctx := context.Background()
		retrievedID, ok := router.GetUserIDFromContext(ctx)
		if ok {
			t.Error("Expected not to find user ID, got true")
		}
		if retrievedID != "" {
			t.Errorf("Expected empty user ID, got %s", retrievedID)
		}

		// Get user ID from context with router context but no user ID
		rc := router.NewRouterContext()
		ctx = router.WithRouterContext(ctx, rc)
		retrievedID, ok = router.GetUserIDFromContext(ctx)
		if ok {
			t.Error("Expected not to find user ID, got true")
		}
		if retrievedID != "" {
			t.Errorf("Expected empty user ID, got %s", retrievedID)
		}
	})

	// Test case 7: With/Get User
	t.Run("WithUser_GetUserFromContext", func(t *testing.T) {
		// Add user to context
		ctx := context.Background()
		// Use any (interface{}) for the user
		type userType struct {
			Name string
			Age  int
		}
		// Create the user as an any type
		var user any = &userType{
			Name: "John",
			Age:  30,
		}
		ctx = router.WithUser(ctx, &user)

		// Get user from context
		retrievedUser, ok := router.GetUserFromContext(ctx)
		if !ok {
			t.Error("Expected to find user, got false")
		}
		if retrievedUser == nil {
			t.Error("Expected non-nil user")
		} else {
			// Need to dereference and type assert to compare
			actualUser := *retrievedUser
			if actualUser != user {
				t.Errorf("Expected user %v, got %v", user, actualUser)
			}
		}
	})

	// Test case 8: Get nonexistent User
	t.Run("GetUserFromContext_Nonexistent", func(t *testing.T) {
		// Get user from empty context
		ctx := context.Background()
		retrievedUser, ok := router.GetUserFromContext(ctx)
		if ok {
			t.Error("Expected not to find user, got true")
		}
		if retrievedUser != nil {
			t.Errorf("Expected nil user, got %v", retrievedUser)
		}

		// Get user from context with router context but no user
		rc := router.NewRouterContext()
		ctx = router.WithRouterContext(ctx, rc)
		retrievedUser, ok = router.GetUserFromContext(ctx)
		if ok {
			t.Error("Expected not to find user, got true")
		}
		if retrievedUser != nil {
			t.Errorf("Expected nil user, got %v", retrievedUser)
		}
	})

	// Test case 9: With/Get Flag
	t.Run("WithFlag_GetFlagFromContext", func(t *testing.T) {
		// Add flag to context
		ctx := context.Background()
		flagName := "testFlag"
		flagValue := true
		ctx = router.WithFlag(ctx, flagName, flagValue)

		// Get flag from context
		retrievedValue, ok := router.GetFlagFromContext(ctx, flagName)
		if !ok {
			t.Error("Expected to find flag, got false")
		}
		if retrievedValue != flagValue {
			t.Errorf("Expected flag value %v, got %v", flagValue, retrievedValue)
		}

		// Add another flag
		flagName2 := "anotherFlag"
		flagValue2 := false
		ctx = router.WithFlag(ctx, flagName2, flagValue2)

		// Get both flags
		retrievedValue, ok = router.GetFlagFromContext(ctx, flagName)
		if !ok {
			t.Error("Expected to find first flag, got false")
		}
		if retrievedValue != flagValue {
			t.Errorf("Expected first flag value %v, got %v", flagValue, retrievedValue)
		}

		retrievedValue2, ok := router.GetFlagFromContext(ctx, flagName2)
		if !ok {
			t.Error("Expected to find second flag, got false")
		}
		if retrievedValue2 != flagValue2 {
			t.Errorf("Expected second flag value %v, got %v", flagValue2, retrievedValue2)
		}

		// Check underlying structure
		rc, ok := router.GetRouterContext(ctx)
		if !ok {
			t.Fatal("Expected to find router context")
		}
		expectedFlags := map[string]bool{
			flagName:  flagValue,
			flagName2: flagValue2,
		}
		if !reflect.DeepEqual(rc.Flags, expectedFlags) {
			t.Errorf("Expected flags %v, got %v", expectedFlags, rc.Flags)
		}
	})

	// Test case 10: Get nonexistent Flag
	t.Run("GetFlagFromContext_Nonexistent", func(t *testing.T) {
		// Get flag from empty context
		ctx := context.Background()
		flagName := "nonexistentFlag"
		retrievedValue, ok := router.GetFlagFromContext(ctx, flagName)
		if ok {
			t.Error("Expected not to find flag, got true")
		}
		if retrievedValue != false {
			t.Errorf("Expected false flag value, got %v", retrievedValue)
		}

		// Get flag from context with router context but no matching flag
		rc := router.NewRouterContext()
		rc.Flags["otherFlag"] = true
		ctx = router.WithRouterContext(ctx, rc)
		retrievedValue, ok = router.GetFlagFromContext(ctx, flagName)
		if ok {
			t.Error("Expected not to find flag, got true")
		}
		if retrievedValue != false {
			t.Errorf("Expected false flag value, got %v", retrievedValue)
		}
	})

	// Test case 11: Context with wrong type
	t.Run("GetRouterContext_WrongType", func(t *testing.T) {
		// Create a context with a value using the same key type but different value type
		wrongType := "not a router context"
		ctx := context.WithValue(context.Background(), routerContextKey{}, wrongType)

		rc, ok := router.GetRouterContext(ctx)
		if ok {
			t.Error("Expected false for context with wrong type, got true")
		}
		if rc != nil {
			t.Errorf("Expected nil router context, got %v", rc)
		}
	})

	// Test case 12: WithRouterContext
	t.Run("WithRouterContext", func(t *testing.T) {
		ctx := context.Background()
		rc := router.NewRouterContext()

		// Add some data to the router context
		rc.UserID = "test-user"
		rc.userIDSet = true
		rc.Flags["testFlag"] = true

		// Add router context to context
		ctx = router.WithRouterContext(ctx, rc)

		// Retrieve router context
		retrievedRC, ok := router.GetRouterContext(ctx)
		if !ok {
			t.Fatal("Expected to find router context")
		}

		// Verify it's the same instance
		if retrievedRC != rc {
			t.Error("Expected same router context instance")
		}

		// Verify data is preserved
		if retrievedRC.UserID != "test-user" {
			t.Errorf("Expected user ID test-user, got %s", retrievedRC.UserID)
		}
		if !retrievedRC.userIDSet {
			t.Error("Expected userIDSet to be true")
		}
		if !retrievedRC.Flags["testFlag"] {
			t.Error("Expected testFlag to be true")
		}
	})
}
