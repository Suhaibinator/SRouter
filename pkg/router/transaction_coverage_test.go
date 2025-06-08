package router

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Suhaibinator/SRouter/pkg/common"
	"github.com/Suhaibinator/SRouter/pkg/router/internal/mocks"
	"github.com/Suhaibinator/SRouter/pkg/scontext"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zaptest"
)

// TestTransactionCommitFailure tests the scenario where transaction commit fails
func TestTransactionCommitFailure(t *testing.T) {
	// Create mock transaction that fails on commit
	mockTx := &mocks.MockTransaction{
		CommitFunc: func() error {
			return errors.New("commit failed")
		},
	}
	
	// Create mock factory
	mockFactory := &mocks.MockTransactionFactory{
		BeginFunc: func(ctx context.Context, options map[string]any) (scontext.DatabaseTransaction, error) {
			return mockTx, nil
		},
	}

	// Create router with transaction factory
	r := NewRouter[string, TestUser](RouterConfig{
		Logger:             zaptest.NewLogger(t),
		TransactionFactory: mockFactory,
	}, nil, nil)

	// Handler that succeeds (should trigger commit)
	handler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("success"))
	})

	// Register route with transaction
	r.RegisterRoute(RouteConfigBase{
		Path:    "/test",
		Methods: []HttpMethod{MethodGet},
		Handler: handler,
		Overrides: common.RouteOverrides{
			Transaction: &common.TransactionConfig{
				Enabled: true,
			},
		},
	})

	// Make request
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Check response - should still be successful despite commit failure
	assert.Equal(t, http.StatusOK, w.Code)
	assert.Equal(t, "success", w.Body.String())
	
	// Verify commit was attempted
	assert.True(t, mockTx.IsCommitCalled())
	assert.False(t, mockTx.IsRollbackCalled())
}

// TestTransactionRollbackFailure tests the scenario where transaction rollback fails
func TestTransactionRollbackFailure(t *testing.T) {
	// Create mock transaction that fails on rollback
	mockTx := &mocks.MockTransaction{
		RollbackFunc: func() error {
			return errors.New("rollback failed")
		},
	}
	
	// Create mock factory
	mockFactory := &mocks.MockTransactionFactory{
		BeginFunc: func(ctx context.Context, options map[string]any) (scontext.DatabaseTransaction, error) {
			return mockTx, nil
		},
	}

	// Create router with transaction factory
	r := NewRouter[string, TestUser](RouterConfig{
		Logger:             zaptest.NewLogger(t),
		TransactionFactory: mockFactory,
	}, nil, nil)

	// Handler that fails (should trigger rollback)
	handler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		w.Write([]byte("error"))
	})

	// Register route with transaction
	r.RegisterRoute(RouteConfigBase{
		Path:    "/test",
		Methods: []HttpMethod{MethodGet},
		Handler: handler,
		Overrides: common.RouteOverrides{
			Transaction: &common.TransactionConfig{
				Enabled: true,
			},
		},
	})

	// Make request
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Check response - should still show the error despite rollback failure
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Equal(t, "error", w.Body.String())
	
	// Verify rollback was attempted
	assert.False(t, mockTx.IsCommitCalled())
	assert.True(t, mockTx.IsRollbackCalled())
}

// TestTransactionBeginFailure tests the scenario where BeginTransaction fails
func TestTransactionBeginFailure(t *testing.T) {
	// Create mock factory that fails to begin transaction
	mockFactory := &mocks.MockTransactionFactory{
		BeginFunc: func(ctx context.Context, options map[string]any) (scontext.DatabaseTransaction, error) {
			return nil, errors.New("database connection failed")
		},
	}

	// Create router with transaction factory
	r := NewRouter[string, TestUser](RouterConfig{
		Logger:             zaptest.NewLogger(t),
		TransactionFactory: mockFactory,
	}, nil, nil)

	// Handler that should not be called
	handlerCalled := false
	handler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		handlerCalled = true
		w.WriteHeader(http.StatusOK)
	})

	// Register route with transaction
	r.RegisterRoute(RouteConfigBase{
		Path:    "/test",
		Methods: []HttpMethod{MethodGet},
		Handler: handler,
		Overrides: common.RouteOverrides{
			Transaction: &common.TransactionConfig{
				Enabled: true,
			},
		},
	})

	// Make request
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Check response - should be 500 Internal Server Error
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "Failed to begin transaction")
	
	// Handler should not have been called
	assert.False(t, handlerCalled)
}

// TestTransactionPanicWithRollbackFailure tests the scenario where a panic occurs and rollback also fails
func TestTransactionPanicWithRollbackFailure(t *testing.T) {
	// Create mock transaction that fails on rollback
	mockTx := &mocks.MockTransaction{
		RollbackFunc: func() error {
			return errors.New("rollback failed after panic")
		},
	}
	
	// Create mock factory
	mockFactory := &mocks.MockTransactionFactory{
		BeginFunc: func(ctx context.Context, options map[string]any) (scontext.DatabaseTransaction, error) {
			return mockTx, nil
		},
	}

	// Create router with transaction factory
	r := NewRouter[string, TestUser](RouterConfig{
		Logger:             zaptest.NewLogger(t),
		TransactionFactory: mockFactory,
	}, nil, nil)

	// Handler that panics
	handler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// Verify transaction is in context
		tx, ok := scontext.GetTransaction[string, TestUser](req.Context())
		assert.True(t, ok)
		assert.NotNil(t, tx)
		
		// Panic!
		panic("something went wrong")
	})

	// Register route with transaction
	r.RegisterRoute(RouteConfigBase{
		Path:    "/panic",
		Methods: []HttpMethod{MethodGet},
		Handler: handler,
		Overrides: common.RouteOverrides{
			Transaction: &common.TransactionConfig{
				Enabled: true,
			},
		},
	})

	// Make request
	req := httptest.NewRequest("GET", "/panic", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Check response - should be 500 despite rollback failure
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	
	// Verify rollback was attempted
	assert.False(t, mockTx.IsCommitCalled())
	assert.True(t, mockTx.IsRollbackCalled())
}

// TestTransactionWithTraceID tests transaction error logging with trace ID
func TestTransactionWithTraceID(t *testing.T) {
	// Create mock transaction that fails on commit
	mockTx := &mocks.MockTransaction{
		CommitFunc: func() error {
			return errors.New("commit failed")
		},
	}
	
	// Create mock factory
	mockFactory := &mocks.MockTransactionFactory{
		BeginFunc: func(ctx context.Context, options map[string]any) (scontext.DatabaseTransaction, error) {
			return mockTx, nil
		},
	}

	// Create router with transaction factory and trace ID generation
	r := NewRouter[string, TestUser](RouterConfig{
		Logger:             zaptest.NewLogger(t),
		TransactionFactory: mockFactory,
		TraceIDBufferSize:  10, // Enable trace ID generation
	}, nil, nil)

	// Handler that succeeds
	handler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		// Verify trace ID is in context
		traceID := scontext.GetTraceIDFromRequest[string, TestUser](req)
		assert.NotEmpty(t, traceID)
		
		w.WriteHeader(http.StatusOK)
	})

	// Register route with transaction
	r.RegisterRoute(RouteConfigBase{
		Path:    "/test",
		Methods: []HttpMethod{MethodGet},
		Handler: handler,
		Overrides: common.RouteOverrides{
			Transaction: &common.TransactionConfig{
				Enabled: true,
			},
		},
	})

	// Make request
	req := httptest.NewRequest("GET", "/test", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Check response
	assert.Equal(t, http.StatusOK, w.Code)
	
	// Verify commit was attempted
	assert.True(t, mockTx.IsCommitCalled())
}