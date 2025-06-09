package mocks

import (
	"context"
	"errors"
	"sync"

	"github.com/Suhaibinator/SRouter/pkg/scontext"
	"gorm.io/gorm"
)

// MockTransactionFactory is a mock implementation of common.TransactionFactory
type MockTransactionFactory struct {
	BeginFunc  func(ctx context.Context, options map[string]any) (scontext.DatabaseTransaction, error)
	BeginCount int
	mu         sync.Mutex
}

// BeginTransaction implements common.TransactionFactory
func (m *MockTransactionFactory) BeginTransaction(ctx context.Context, options map[string]any) (scontext.DatabaseTransaction, error) {
	m.mu.Lock()
	m.BeginCount++
	m.mu.Unlock()
	
	if m.BeginFunc != nil {
		return m.BeginFunc(ctx, options)
	}
	return &MockTransaction{}, nil
}

// GetBeginCount returns the number of times BeginTransaction was called
func (m *MockTransactionFactory) GetBeginCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.BeginCount
}

// MockTransaction is a mock implementation of scontext.DatabaseTransaction
type MockTransaction struct {
	CommitFunc     func() error
	RollbackFunc   func() error
	SavePointFunc  func(name string) error
	RollbackToFunc func(name string) error
	GetDBFunc      func() *gorm.DB
	
	CommitCalled     bool
	RollbackCalled   bool
	SavePointCalled  bool
	RollbackToCalled bool
	
	mu sync.Mutex
}

// Commit implements scontext.DatabaseTransaction
func (m *MockTransaction) Commit() error {
	m.mu.Lock()
	m.CommitCalled = true
	m.mu.Unlock()
	
	if m.CommitFunc != nil {
		return m.CommitFunc()
	}
	return nil
}

// Rollback implements scontext.DatabaseTransaction
func (m *MockTransaction) Rollback() error {
	m.mu.Lock()
	m.RollbackCalled = true
	m.mu.Unlock()
	
	if m.RollbackFunc != nil {
		return m.RollbackFunc()
	}
	return nil
}

// SavePoint implements scontext.DatabaseTransaction
func (m *MockTransaction) SavePoint(name string) error {
	m.mu.Lock()
	m.SavePointCalled = true
	m.mu.Unlock()
	
	if m.SavePointFunc != nil {
		return m.SavePointFunc(name)
	}
	return nil
}

// RollbackTo implements scontext.DatabaseTransaction
func (m *MockTransaction) RollbackTo(name string) error {
	m.mu.Lock()
	m.RollbackToCalled = true
	m.mu.Unlock()
	
	if m.RollbackToFunc != nil {
		return m.RollbackToFunc(name)
	}
	return nil
}

// GetDB implements scontext.DatabaseTransaction
func (m *MockTransaction) GetDB() *gorm.DB {
	if m.GetDBFunc != nil {
		return m.GetDBFunc()
	}
	return nil
}

// IsCommitCalled returns whether Commit was called (thread-safe)
func (m *MockTransaction) IsCommitCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.CommitCalled
}

// IsRollbackCalled returns whether Rollback was called (thread-safe)
func (m *MockTransaction) IsRollbackCalled() bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.RollbackCalled
}

// ErrorTransactionFactory always returns an error when BeginTransaction is called
type ErrorTransactionFactory struct{}

// BeginTransaction always returns an error
func (e *ErrorTransactionFactory) BeginTransaction(ctx context.Context, options map[string]any) (scontext.DatabaseTransaction, error) {
	return nil, errors.New("failed to begin transaction")
}