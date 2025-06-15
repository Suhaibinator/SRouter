package router

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Suhaibinator/SRouter/pkg/codec"
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
		_, _ = w.Write([]byte("success"))
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
		_, _ = w.Write([]byte("error"))
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

// TestRegisterRouteTransactionCommitFailure tests commit failure for routes registered via RegisterRoute
func TestRegisterRouteTransactionCommitFailure(t *testing.T) {
	// Create mock transaction that fails on commit
	mockTx := &mocks.MockTransaction{
		CommitFunc: func() error {
			return errors.New("commit failed in RegisterRoute")
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

	// Handler that succeeds
	handler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	// Register route directly with RegisterRoute
	r.RegisterRoute(RouteConfigBase{
		Path:    "/test-register-route",
		Methods: []HttpMethod{MethodGet},
		Handler: handler,
		Overrides: common.RouteOverrides{
			Transaction: &common.TransactionConfig{
				Enabled: true,
			},
		},
	})

	// Make request
	req := httptest.NewRequest("GET", "/test-register-route", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Check response
	assert.Equal(t, http.StatusOK, w.Code)
	
	// Verify commit was attempted
	assert.True(t, mockTx.IsCommitCalled())
}

// TestRegisterGenericRouteTransactionCommitFailure tests commit failure for generic routes
func TestRegisterGenericRouteTransactionCommitFailure(t *testing.T) {
	type Request struct {
		Name string `json:"name"`
	}
	type Response struct {
		Message string `json:"message"`
	}

	// Create mock transaction that fails on commit
	mockTx := &mocks.MockTransaction{
		CommitFunc: func() error {
			return errors.New("commit failed in RegisterGenericRoute")
		},
	}
	
	// Create mock factory
	mockFactory := &mocks.MockTransactionFactory{
		BeginFunc: func(ctx context.Context, options map[string]any) (scontext.DatabaseTransaction, error) {
			return mockTx, nil
		},
	}

	// Handler that succeeds
	handler := func(req *http.Request, data Request) (Response, error) {
		return Response{Message: "success"}, nil
	}

	// Create router with transaction factory and generic route in subrouter
	r := NewRouter[string, TestUser](RouterConfig{
		Logger:             zaptest.NewLogger(t),
		TransactionFactory: mockFactory,
		SubRouters: []SubRouterConfig{
			{
				PathPrefix: "",
				Routes: []RouteDefinition{
					NewGenericRouteDefinition[Request, Response, string, TestUser](
						RouteConfig[Request, Response]{
							Path:    "/test-generic",
							Methods: []HttpMethod{MethodPost},
							Handler: handler,
							Codec:   codec.NewJSONCodec[Request, Response](),
							Overrides: common.RouteOverrides{
								Transaction: &common.TransactionConfig{
									Enabled: true,
								},
							},
						},
					),
				},
			},
		},
	}, nil, nil)

	// Make request
	reqBody := `{"name":"test"}`
	req := httptest.NewRequest("POST", "/test-generic", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Check response
	assert.Equal(t, http.StatusOK, w.Code)
	
	// Verify commit was attempted
	assert.True(t, mockTx.IsCommitCalled())
}

// TestRegisterRouteTransactionRollbackFailure tests rollback failure for routes registered via RegisterRoute
func TestRegisterRouteTransactionRollbackFailure(t *testing.T) {
	// Create mock transaction that fails on rollback
	mockTx := &mocks.MockTransaction{
		RollbackFunc: func() error {
			return errors.New("rollback failed in RegisterRoute")
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

	// Handler that fails
	handler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	})

	// Register route directly with RegisterRoute
	r.RegisterRoute(RouteConfigBase{
		Path:    "/test-register-route-fail",
		Methods: []HttpMethod{MethodGet},
		Handler: handler,
		Overrides: common.RouteOverrides{
			Transaction: &common.TransactionConfig{
				Enabled: true,
			},
		},
	})

	// Make request
	req := httptest.NewRequest("GET", "/test-register-route-fail", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Check response
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	
	// Verify rollback was attempted
	assert.True(t, mockTx.IsRollbackCalled())
}

// TestRegisterGenericRouteTransactionRollbackFailure tests rollback failure for generic routes
func TestRegisterGenericRouteTransactionRollbackFailure(t *testing.T) {
	type Request struct {
		Name string `json:"name"`
	}
	type Response struct {
		Message string `json:"message"`
	}

	// Create mock transaction that fails on rollback
	mockTx := &mocks.MockTransaction{
		RollbackFunc: func() error {
			return errors.New("rollback failed in RegisterGenericRoute")
		},
	}
	
	// Create mock factory
	mockFactory := &mocks.MockTransactionFactory{
		BeginFunc: func(ctx context.Context, options map[string]any) (scontext.DatabaseTransaction, error) {
			return mockTx, nil
		},
	}

	// Handler that fails
	handler := func(req *http.Request, data Request) (Response, error) {
		return Response{}, errors.New("handler error")
	}

	// Create router with transaction factory and generic route in subrouter
	r := NewRouter[string, TestUser](RouterConfig{
		Logger:             zaptest.NewLogger(t),
		TransactionFactory: mockFactory,
		SubRouters: []SubRouterConfig{
			{
				PathPrefix: "",
				Routes: []RouteDefinition{
					NewGenericRouteDefinition[Request, Response, string, TestUser](
						RouteConfig[Request, Response]{
							Path:    "/test-generic-fail",
							Methods: []HttpMethod{MethodPost},
							Handler: handler,
							Codec:   codec.NewJSONCodec[Request, Response](),
							Overrides: common.RouteOverrides{
								Transaction: &common.TransactionConfig{
									Enabled: true,
								},
							},
						},
					),
				},
			},
		},
	}, nil, nil)

	// Make request
	reqBody := `{"name":"test"}`
	req := httptest.NewRequest("POST", "/test-generic-fail", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Check response
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	
	// Verify rollback was attempted
	assert.True(t, mockTx.IsRollbackCalled())
}

// TestRegisterRouteTransactionBeginFailure tests begin failure for routes registered via RegisterRoute
func TestRegisterRouteTransactionBeginFailure(t *testing.T) {
	// Create mock factory that fails to begin transaction
	mockFactory := &mocks.MockTransactionFactory{
		BeginFunc: func(ctx context.Context, options map[string]any) (scontext.DatabaseTransaction, error) {
			return nil, errors.New("begin failed in RegisterRoute")
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

	// Register route directly with RegisterRoute
	r.RegisterRoute(RouteConfigBase{
		Path:    "/test-register-route-begin-fail",
		Methods: []HttpMethod{MethodGet},
		Handler: handler,
		Overrides: common.RouteOverrides{
			Transaction: &common.TransactionConfig{
				Enabled: true,
			},
		},
	})

	// Make request
	req := httptest.NewRequest("GET", "/test-register-route-begin-fail", nil)
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Check response
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "Failed to begin transaction")
	
	// Handler should not have been called
	assert.False(t, handlerCalled)
}

// TestRegisterGenericRouteTransactionBeginFailure tests begin failure for generic routes
func TestRegisterGenericRouteTransactionBeginFailure(t *testing.T) {
	type Request struct {
		Name string `json:"name"`
	}
	type Response struct {
		Message string `json:"message"`
	}

	// Create mock factory that fails to begin transaction
	mockFactory := &mocks.MockTransactionFactory{
		BeginFunc: func(ctx context.Context, options map[string]any) (scontext.DatabaseTransaction, error) {
			return nil, errors.New("begin failed in RegisterGenericRoute")
		},
	}

	// Handler that should not be called
	handlerCalled := false
	handler := func(req *http.Request, data Request) (Response, error) {
		handlerCalled = true
		return Response{Message: "success"}, nil
	}

	// Create router with transaction factory and generic route in subrouter
	r := NewRouter[string, TestUser](RouterConfig{
		Logger:             zaptest.NewLogger(t),
		TransactionFactory: mockFactory,
		SubRouters: []SubRouterConfig{
			{
				PathPrefix: "",
				Routes: []RouteDefinition{
					NewGenericRouteDefinition[Request, Response, string, TestUser](
						RouteConfig[Request, Response]{
							Path:    "/test-generic-begin-fail",
							Methods: []HttpMethod{MethodPost},
							Handler: handler,
							Codec:   codec.NewJSONCodec[Request, Response](),
							Overrides: common.RouteOverrides{
								Transaction: &common.TransactionConfig{
									Enabled: true,
								},
							},
						},
					),
				},
			},
		},
	}, nil, nil)

	// Make request
	reqBody := `{"name":"test"}`
	req := httptest.NewRequest("POST", "/test-generic-begin-fail", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)

	// Check response
	assert.Equal(t, http.StatusInternalServerError, w.Code)
	assert.Contains(t, w.Body.String(), "Failed to begin transaction")
	
	// Handler should not have been called
	assert.False(t, handlerCalled)
}
// TestStatusCapturingResponseWriterFlush tests the Flush method of statusCapturingResponseWriter
func TestStatusCapturingResponseWriterFlush(t *testing.T) {
	// Test with a response writer that implements http.Flusher
	t.Run("with flusher", func(t *testing.T) {
		// Create a mock flusher recorder
		mockFlusher := mocks.NewFlusherRecorder()
		
		// Create statusCapturingResponseWriter wrapping the flusher
		captureWriter := &statusCapturingResponseWriter{
			ResponseWriter: mockFlusher,
		}
		
		// Call Flush
		captureWriter.Flush()
		
		// Verify that the underlying Flush was called
		assert.True(t, mockFlusher.Flushed, "Expected Flush to be called on the underlying response writer")
	})
	
	// Test with a response writer that does NOT implement http.Flusher
	t.Run("without flusher", func(t *testing.T) {
		// Create a regular httptest.ResponseRecorder (doesn't implement Flusher)
		recorder := httptest.NewRecorder()
		
		// Create statusCapturingResponseWriter wrapping the recorder
		captureWriter := &statusCapturingResponseWriter{
			ResponseWriter: recorder,
		}
		
		// Call Flush - should not panic
		assert.NotPanics(t, func() {
			captureWriter.Flush()
		}, "Flush should not panic when underlying writer doesn't implement Flusher")
	})
	
	// Test Flush after Write operations
	t.Run("flush after write", func(t *testing.T) {
		mockFlusher := mocks.NewFlusherRecorder()
		captureWriter := &statusCapturingResponseWriter{
			ResponseWriter: mockFlusher,
		}
		
		// Write some data
		data := []byte("test data")
		n, err := captureWriter.Write(data)
		assert.NoError(t, err)
		assert.Equal(t, len(data), n)
		
		// Status should be set to 200 after write
		assert.Equal(t, http.StatusOK, captureWriter.status)
		assert.True(t, captureWriter.written)
		
		// Now flush
		captureWriter.Flush()
		assert.True(t, mockFlusher.Flushed, "Flush should be called after write")
	})
	
	// Test Flush after WriteHeader
	t.Run("flush after write header", func(t *testing.T) {
		mockFlusher := mocks.NewFlusherRecorder()
		captureWriter := &statusCapturingResponseWriter{
			ResponseWriter: mockFlusher,
		}
		
		// Write header
		captureWriter.WriteHeader(http.StatusCreated)
		assert.Equal(t, http.StatusCreated, captureWriter.status)
		assert.True(t, captureWriter.written)
		
		// Now flush
		captureWriter.Flush()
		assert.True(t, mockFlusher.Flushed, "Flush should be called after WriteHeader")
	})
}
