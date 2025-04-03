package middleware

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gorm.io/gorm" // Import gorm for the DB type
)

// TestGormTransactionWrapper tests the GormTransactionWrapper methods.
func TestGormTransactionWrapper(t *testing.T) {
	assert := assert.New(t)

	// --- Test Case 1: Wrapper with nil DB ---
	t.Run("NilDB", func(t *testing.T) {
		nilWrapper := NewGormTransactionWrapper(nil)
		assert.NotNil(nilWrapper, "NewGormTransactionWrapper should return a non-nil wrapper even with nil DB")
		assert.Nil(nilWrapper.GetDB(), "GetDB should return nil when wrapper created with nil DB")

		// Check interface methods return errors with nil DB
		err := nilWrapper.Commit()
		assert.Error(err, "Commit should return an error with nil DB")
		assert.Contains(err.Error(), "nil transaction", "Commit error message should indicate nil transaction")

		err = nilWrapper.Rollback()
		assert.Error(err, "Rollback should return an error with nil DB")
		assert.Contains(err.Error(), "nil transaction", "Rollback error message should indicate nil transaction")

		err = nilWrapper.SavePoint("sp1")
		assert.Error(err, "SavePoint should return an error with nil DB")
		assert.Contains(err.Error(), "nil transaction", "SavePoint error message should indicate nil transaction")

		err = nilWrapper.RollbackTo("sp1")
		assert.Error(err, "RollbackTo should return an error with nil DB")
		assert.Contains(err.Error(), "nil transaction", "RollbackTo error message should indicate nil transaction")
	})

	// --- Test Case 2: Wrapper with Non-Nil DB ---
	t.Run("NonNilDB", func(t *testing.T) {
		// Use a simple non-nil pointer. Testing actual GORM method calls
		// requires integration testing or more complex mocking.
		mockDB := &gorm.DB{}
		wrapper := NewGormTransactionWrapper(mockDB)
		assert.NotNil(wrapper, "NewGormTransactionWrapper should return a non-nil wrapper")
		assert.Same(mockDB, wrapper.GetDB(), "GetDB should return the original DB instance")
	})

	// --- Test Case 3: Interface Satisfaction ---
	// This is implicitly tested by the compile-time check in db.go and usage above.
	t.Run("InterfaceSatisfaction", func(t *testing.T) {
		var dbTx DatabaseTransaction = NewGormTransactionWrapper(&gorm.DB{})
		assert.NotNil(dbTx, "Wrapper should satisfy DatabaseTransaction interface")
	})
}
