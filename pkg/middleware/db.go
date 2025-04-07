package middleware

import (
	"errors" // Import errors

	"github.com/Suhaibinator/SRouter/pkg/scontext" // Import scontext
	"gorm.io/gorm"
)

// GormTransactionWrapper wraps a *gorm.DB instance to implement the scontext.DatabaseTransaction interface.
// This is necessary because GORM's methods like Commit return *gorm.DB for chaining,
// which doesn't match the interface signature aiming for simple error returns.
type GormTransactionWrapper struct {
	DB *gorm.DB
}

// NewGormTransactionWrapper creates a new wrapper around a GORM transaction.
func NewGormTransactionWrapper(tx *gorm.DB) *GormTransactionWrapper {
	return &GormTransactionWrapper{DB: tx}
}

// Commit implements the DatabaseTransaction interface.
func (w *GormTransactionWrapper) Commit() error {
	if w.DB == nil {
		return errors.New("cannot commit on nil transaction")
	}
	// Note: GORM's Commit() returns *gorm.DB, we access its Error field.
	return w.DB.Commit().Error
}

// Rollback implements the DatabaseTransaction interface.
func (w *GormTransactionWrapper) Rollback() error {
	if w.DB == nil {
		return errors.New("cannot rollback on nil transaction")
	}
	// GORM's Rollback might not return an error in the same way Commit does,
	// but we check the Error field for consistency.
	return w.DB.Rollback().Error
}

// SavePoint implements the DatabaseTransaction interface.
func (w *GormTransactionWrapper) SavePoint(name string) error {
	if w.DB == nil {
		return errors.New("cannot savepoint on nil transaction")
	}
	return w.DB.SavePoint(name).Error
}

// RollbackTo implements the DatabaseTransaction interface.
func (w *GormTransactionWrapper) RollbackTo(name string) error {
	if w.DB == nil {
		return errors.New("cannot rollback to savepoint on nil transaction")
	}
	return w.DB.RollbackTo(name).Error
}

// GetDB returns the underlying *gorm.DB instance.
func (w *GormTransactionWrapper) GetDB() *gorm.DB {
	return w.DB
}

// Ensure GormTransactionWrapper implements scontext.DatabaseTransaction (compile-time check).
var _ scontext.DatabaseTransaction = (*GormTransactionWrapper)(nil) // Use scontext.DatabaseTransaction
