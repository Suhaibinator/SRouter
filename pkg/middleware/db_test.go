package middleware

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"gorm.io/gorm" // Import gorm for the DB type
)

// TestGormTransactionWrapper tests the GormTransactionWrapper methods.
func TestGormTransactionWrapper(t *testing.T) {
	assert := assert.New(t)

	// 1. Test with nil DB (basic check for panics)
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

	// 2. Test with a non-nil (but still mock/uninitialized) DB instance
	// We won't actually connect to a DB here, just use a non-nil pointer.
	mockDB := &gorm.DB{}
	wrapper := NewGormTransactionWrapper(mockDB)
	assert.NotNil(wrapper, "NewGormTransactionWrapper should return a non-nil wrapper")
	assert.Same(mockDB, wrapper.GetDB(), "GetDB should return the original DB instance")

	// Note: Testing Commit/Rollback/etc. on a simple mockDB (&gorm.DB{})
	// is unreliable as GORM's methods expect internal state which isn't present,
	// leading to potential panics within GORM itself.
	// We rely on the nil DB checks and the compile-time interface satisfaction check.

	// 3. Verify interface satisfaction (already done via var _ DatabaseTransaction = (*GormTransactionWrapper)(nil) in db.go)
	var dbTx DatabaseTransaction = wrapper
	assert.NotNil(dbTx, "Wrapper should satisfy DatabaseTransaction interface")
}
