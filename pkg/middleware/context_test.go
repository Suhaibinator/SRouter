package middleware

import (
	"context"
	"errors"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"gorm.io/gorm" // Import gorm for GetDB return type in mock
)

// mockTx implements the DatabaseTransaction interface for testing
type mockTx struct {
	id          int // To differentiate instances
	commitErr   error
	rollbackErr error
	dbInstance  *gorm.DB // Can be nil for tests not needing it
}

func (m *mockTx) Commit() error                { return m.commitErr }
func (m *mockTx) Rollback() error              { return m.rollbackErr }
func (m *mockTx) SavePoint(name string) error  { return nil } // No-op for basic tests
func (m *mockTx) RollbackTo(name string) error { return nil } // No-op for basic tests
func (m *mockTx) GetDB() *gorm.DB              { return m.dbInstance }

// TestTransactionContext tests adding and retrieving DatabaseTransaction from context
func TestTransactionContext(t *testing.T) {
	assert := assert.New(t)
	ctx := context.Background()

	// Mock transactions
	mockTx1 := &mockTx{id: 1}
	mockTx2 := &mockTx{id: 2, commitErr: errors.New("commit failed")}

	// 1. Get from empty context
	retrievedTx, ok := GetTransaction[string, any](ctx)
	assert.False(ok, "Should return false when getting tx from empty context")
	assert.Nil(retrievedTx, "Should return nil tx from empty context")

	// 2. Add tx1 to context
	ctxWithTx1 := WithTransaction[string, any](ctx, mockTx1)
	// assert.NotSame(ctx, ctxWithTx1, "Context should be different after adding tx") // NotSame might not work reliably on context interface values

	// 3. Get tx1 from context
	retrievedTx, ok = GetTransaction[string, any](ctxWithTx1)
	assert.True(ok, "Should return true when getting tx from context with tx")
	assert.NotNil(retrievedTx, "Retrieved tx should not be nil")
	assert.Same(mockTx1, retrievedTx, "Retrieved tx should be the same instance as added")
	// Verify type assertion works
	if mt, typeOK := retrievedTx.(*mockTx); typeOK {
		assert.Equal(1, mt.id, "Retrieved mock tx should have correct ID")
	} else {
		t.Errorf("Retrieved transaction was not of expected type *mockTx")
	}

	// 4. Ensure original context is unchanged
	retrievedTx, ok = GetTransaction[string, any](ctx)
	assert.False(ok, "Original context should remain unchanged (no tx)")
	assert.Nil(retrievedTx, "Original context should still have nil tx")

	// 5. Overwrite tx1 with tx2
	ctxWithTx2 := WithTransaction[string, any](ctxWithTx1, mockTx2)
	// assert.NotSame(ctxWithTx1, ctxWithTx2, "Context should be different after overwriting tx") // NotSame might not work reliably on context interface values

	// 6. Get tx2 from the new context
	retrievedTx, ok = GetTransaction[string, any](ctxWithTx2)
	assert.True(ok, "Should return true after overwriting tx")
	assert.NotNil(retrievedTx, "Retrieved overwritten tx should not be nil")
	assert.Same(mockTx2, retrievedTx, "Retrieved tx should be the second instance")
	// Verify type assertion and error field
	if mt, typeOK := retrievedTx.(*mockTx); typeOK {
		assert.Equal(2, mt.id, "Retrieved overwritten mock tx should have correct ID")
		assert.Error(mt.Commit(), "Commit error should be present on mockTx2")
	} else {
		t.Errorf("Retrieved overwritten transaction was not of expected type *mockTx")
	}

	// 7. Get tx from the intermediate context (should now be tx2 due to mutation)
	retrievedTx, ok = GetTransaction[string, any](ctxWithTx1)
	assert.True(ok, "Intermediate context should reflect the mutation")
	assert.Same(mockTx2, retrievedTx, "Intermediate context should now hold tx2 instance due to mutation")

	// 8. Test with different generic types (int, struct{})
	type CustomUser struct{ Name string }
	ctxWithIntUser := WithTransaction[int, CustomUser](context.Background(), mockTx1)
	retrievedTx, ok = GetTransaction[int, CustomUser](ctxWithIntUser)
	assert.True(ok, "Should work with different generic types [int, CustomUser]")
	assert.Same(mockTx1, retrievedTx, "Should retrieve correct tx with different generic types")

	// 9. Test GetTransactionFromRequest
	req := httptest.NewRequest("GET", "/", nil)
	// Get from request with no context tx
	retrievedTx, ok = GetTransactionFromRequest[string, any](req)
	assert.False(ok, "GetTransactionFromRequest should return false for request with no tx")
	assert.Nil(retrievedTx, "GetTransactionFromRequest should return nil for request with no tx")

	// Add tx to request context (using ctxWithTx1, which now effectively points to the state of ctxWithTx2)
	reqWithTx := req.WithContext(ctxWithTx1)
	retrievedTx, ok = GetTransactionFromRequest[string, any](reqWithTx)
	assert.True(ok, "GetTransactionFromRequest should return true for request with tx")
	assert.Same(mockTx2, retrievedTx, "GetTransactionFromRequest should retrieve the mutated tx instance (tx2)")
}

// TestFlagContext tests adding and retrieving flags from context
func TestFlagContext(t *testing.T) {
	assert := assert.New(t)
	ctx := context.Background()

	flagName1 := "feature-enabled"
	flagName2 := "debug-mode"
	nonExistentFlag := "does-not-exist"

	// 1. Get flag from empty context
	val, ok := GetFlag[string, any](ctx, flagName1)
	assert.False(ok, "Should return false when getting flag from empty context")
	assert.False(val, "Value should be false for non-existent flag in empty context")

	// 2. Add flag1 (true)
	ctxWithFlag1 := WithFlag[string, any](ctx, flagName1, true)

	// 3. Get flag1
	val, ok = GetFlag[string, any](ctxWithFlag1, flagName1)
	assert.True(ok, "Should return true when getting existing flag")
	assert.True(val, "Value should be true for flag1")

	// 4. Get non-existent flag from context with flag1
	val, ok = GetFlag[string, any](ctxWithFlag1, nonExistentFlag)
	assert.False(ok, "Should return false when getting non-existent flag")
	assert.False(val, "Value should be false for non-existent flag")

	// 5. Add flag2 (false) to the context that already has flag1
	ctxWithBothFlags := WithFlag[string, any](ctxWithFlag1, flagName2, false)

	// 6. Get flag1 from context with both flags
	val, ok = GetFlag[string, any](ctxWithBothFlags, flagName1)
	assert.True(ok, "Should still get flag1 after adding flag2")
	assert.True(val, "Value of flag1 should still be true")

	// 7. Get flag2 from context with both flags
	val, ok = GetFlag[string, any](ctxWithBothFlags, flagName2)
	assert.True(ok, "Should get flag2")
	assert.False(val, "Value of flag2 should be false")

	// 8. Overwrite flag1 to false
	ctxOverwritten := WithFlag[string, any](ctxWithBothFlags, flagName1, false)

	// 9. Get overwritten flag1
	val, ok = GetFlag[string, any](ctxOverwritten, flagName1)
	assert.True(ok, "Should get overwritten flag1")
	assert.False(val, "Value of overwritten flag1 should be false")

	// 10. Get flag2 from overwritten context (should still be there)
	val, ok = GetFlag[string, any](ctxOverwritten, flagName2)
	assert.True(ok, "Should still get flag2 after overwriting flag1")
	assert.False(val, "Value of flag2 should still be false")

	// 11. Test with different generic types
	type CustomUser struct{ Name string }
	ctxWithIntUser := WithFlag[int, CustomUser](context.Background(), flagName1, true)
	val, ok = GetFlag[int, CustomUser](ctxWithIntUser, flagName1)
	assert.True(ok, "Should work with different generic types [int, CustomUser]")
	assert.True(val, "Should retrieve correct flag value with different generic types")
}

// TestGetFlagFromRequest tests the GetFlagFromRequest convenience function
func TestGetFlagFromRequest(t *testing.T) {
	assert := assert.New(t)
	req := httptest.NewRequest("GET", "/", nil)
	flagName := "test-flag"

	// 1. Get from request with no context flag
	val, ok := GetFlagFromRequest[string, any](req, flagName)
	assert.False(ok, "GetFlagFromRequest should return false for request with no flag")
	assert.False(val, "GetFlagFromRequest value should be false for request with no flag")

	// 2. Add flag to request context
	ctxWithFlag := WithFlag[string, any](req.Context(), flagName, true)
	reqWithFlag := req.WithContext(ctxWithFlag)

	// 3. Get flag from request
	val, ok = GetFlagFromRequest[string, any](reqWithFlag, flagName)
	assert.True(ok, "GetFlagFromRequest should return true for request with flag")
	assert.True(val, "GetFlagFromRequest value should be true for request with flag")

	// 4. Get non-existent flag from request with flag
	val, ok = GetFlagFromRequest[string, any](reqWithFlag, "other-flag")
	assert.False(ok, "GetFlagFromRequest should return false for non-existent flag")
	assert.False(val, "GetFlagFromRequest value should be false for non-existent flag")
}
