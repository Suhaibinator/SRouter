package middleware

import (
	"errors" // Import errors

	"github.com/Suhaibinator/SRouter/pkg/scontext" // Import scontext
	"gorm.io/gorm"
)

// GormTransactionWrapper wraps a *gorm.DB instance to implement the scontext.DatabaseTransaction interface.
// This adapter allows GORM transactions to be used with SRouter's context-based transaction management.
// It's necessary because GORM's methods return *gorm.DB for method chaining, while the DatabaseTransaction
// interface expects simple error returns. This wrapper bridges that gap, making GORM transactions
// compatible with SRouter's transaction middleware patterns.
type GormTransactionWrapper struct {
	DB *gorm.DB
}

// NewGormTransactionWrapper creates a new GormTransactionWrapper instance.
// It takes a GORM database instance (typically a transaction) and returns
// a wrapper that implements the scontext.DatabaseTransaction interface.
// This is typically used by database middleware when starting a new transaction.
func NewGormTransactionWrapper(tx *gorm.DB) *GormTransactionWrapper {
	return &GormTransactionWrapper{DB: tx}
}

// Commit commits the wrapped GORM transaction.
// It implements the DatabaseTransaction interface.
// Returns an error if the transaction is nil or if the commit fails.
func (w *GormTransactionWrapper) Commit() error {
	if w.DB == nil {
		return errors.New("cannot commit on nil transaction")
	}
	// Note: GORM's Commit() returns *gorm.DB, we access its Error field.
	return w.DB.Commit().Error
}

// Rollback rolls back the wrapped GORM transaction.
// It implements the DatabaseTransaction interface.
// Returns an error if the transaction is nil or if the rollback fails.
func (w *GormTransactionWrapper) Rollback() error {
	if w.DB == nil {
		return errors.New("cannot rollback on nil transaction")
	}
	// GORM's Rollback might not return an error in the same way Commit does,
	// but we check the Error field for consistency.
	return w.DB.Rollback().Error
}

// SavePoint creates a savepoint within the transaction with the given name.
// It implements the DatabaseTransaction interface.
// Savepoints allow partial rollbacks within a transaction.
// Returns an error if the transaction is nil or if creating the savepoint fails.
func (w *GormTransactionWrapper) SavePoint(name string) error {
	if w.DB == nil {
		return errors.New("cannot savepoint on nil transaction")
	}
	return w.DB.SavePoint(name).Error
}

// RollbackTo rolls back the transaction to a previously created savepoint.
// It implements the DatabaseTransaction interface.
// Only the changes made after the savepoint will be undone.
// Returns an error if the transaction is nil or if the rollback fails.
func (w *GormTransactionWrapper) RollbackTo(name string) error {
	if w.DB == nil {
		return errors.New("cannot rollback to savepoint on nil transaction")
	}
	return w.DB.RollbackTo(name).Error
}

// GetDB returns the underlying *gorm.DB instance.
// This allows handlers to access the full GORM API when needed,
// while still benefiting from SRouter's transaction management.
// The returned instance is typically a transaction, not the main DB connection.
func (w *GormTransactionWrapper) GetDB() *gorm.DB {
	return w.DB
}

// Ensure GormTransactionWrapper implements scontext.DatabaseTransaction (compile-time check).
var _ scontext.DatabaseTransaction = (*GormTransactionWrapper)(nil) // Use scontext.DatabaseTransaction
