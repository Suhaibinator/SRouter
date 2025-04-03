package middleware

import "gorm.io/gorm"

// DatabaseTransaction defines an interface for essential transaction control methods.
// This allows mocking transaction behavior for testing purposes.
type DatabaseTransaction interface {
	Commit() error
	Rollback() error
	SavePoint(name string) error
	RollbackTo(name string) error
	// GetDB returns the underlying GORM DB instance for direct use when needed.
	GetDB() *gorm.DB
}

// GormTransactionWrapper wraps a *gorm.DB instance to implement the DatabaseTransaction interface.
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
	return w.DB.Commit().Error
}

// Rollback implements the DatabaseTransaction interface.
func (w *GormTransactionWrapper) Rollback() error {
	// GORM's Rollback might not return an error in the same way Commit does,
	// but we check the Error field for consistency.
	return w.DB.Rollback().Error
}

// SavePoint implements the DatabaseTransaction interface.
func (w *GormTransactionWrapper) SavePoint(name string) error {
	return w.DB.SavePoint(name).Error
}

// RollbackTo implements the DatabaseTransaction interface.
func (w *GormTransactionWrapper) RollbackTo(name string) error {
	return w.DB.RollbackTo(name).Error
}

// GetDB returns the underlying *gorm.DB instance.
func (w *GormTransactionWrapper) GetDB() *gorm.DB {
	return w.DB
}

// Ensure GormTransactionWrapper implements DatabaseTransaction (compile-time check).
var _ DatabaseTransaction = (*GormTransactionWrapper)(nil)
