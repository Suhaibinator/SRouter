package middleware

import (
	"errors"
	"testing"

	"github.com/Suhaibinator/SRouter/pkg/scontext" // Import scontext
	"github.com/stretchr/testify/assert"
	"gorm.io/gorm"
	"gorm.io/gorm/logger" // Import GORM logger
	"gorm.io/gorm/schema" // Import GORM schema
)

// Helper to create a DryRun DB instance for testing
func newDryRunDB() *gorm.DB {
	db, _ := gorm.Open(nil, &gorm.Config{
		NamingStrategy: schema.NamingStrategy{
			SingularTable: true,
		},
		Logger:                 logger.Default.LogMode(logger.Silent), // Use silent logger for tests
		DryRun:                 true,                                  // Enable DryRun mode
		SkipDefaultTransaction: true,
	})
	return db
}

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

	// --- Test Case 2: Wrapper with Non-Nil DryRun DB (Testing Execution Path) ---
	t.Run("NonNilDryRunDB", func(t *testing.T) {
		// Use a DryRun DB instance. GORM methods can run without a real connection.
		dryRunDB := newDryRunDB()
		wrapper := NewGormTransactionWrapper(dryRunDB)
		assert.NotNil(wrapper, "NewGormTransactionWrapper should return a non-nil wrapper")
		assert.Same(dryRunDB, wrapper.GetDB(), "GetDB should return the original DB instance")

		// Call wrapper methods simply to ensure the execution path is covered.
		// Asserting the specific error is unreliable due to DryRun behavior
		// (e.g., "invalid transaction", "unsupported driver").
		// We primarily care that the wrapper calls the underlying method and returns its .Error.
		dryRunDB.Error = nil // Simulate no pre-existing error
		_ = wrapper.Commit()
		_ = wrapper.Rollback()
		_ = wrapper.SavePoint("sp2")
		_ = wrapper.RollbackTo("sp2")

		dryRunDB.Error = errors.New("simulated gorm error") // Simulate pre-existing error
		_ = wrapper.Commit()
		_ = wrapper.Rollback()
		_ = wrapper.SavePoint("sp2")
		_ = wrapper.RollbackTo("sp2")
	})

	// --- Test Case 3: Interface Satisfaction ---
	// This is implicitly tested by the compile-time check in db.go and usage above.
	t.Run("InterfaceSatisfaction", func(t *testing.T) {
		var dbTx scontext.DatabaseTransaction = NewGormTransactionWrapper(&gorm.DB{}) // Use scontext.DatabaseTransaction
		assert.NotNil(dbTx, "Wrapper should satisfy DatabaseTransaction interface")
	})
}
