package router

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/codec"
	"github.com/Suhaibinator/SRouter/pkg/common"
	"github.com/Suhaibinator/SRouter/pkg/router/internal/mocks"
	"github.com/Suhaibinator/SRouter/pkg/scontext"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zaptest"
	"gorm.io/gorm"
)

// TestUser is used for testing with generic types
type TestUser struct {
	ID   string
	Name string
}


func TestGetEffectiveTransaction(t *testing.T) {
	r := &Router[string, TestUser]{
		config: RouterConfig{
			GlobalTransaction: &common.TransactionConfig{
				Enabled: true,
				Options: map[string]any{"global": true},
			},
		},
	}

	tests := []struct {
		name                 string
		routeTransaction     *common.TransactionConfig
		subRouterTransaction *common.TransactionConfig
		want                 *common.TransactionConfig
	}{
		{
			name: "route override takes precedence",
			routeTransaction: &common.TransactionConfig{
				Enabled: false,
				Options: map[string]any{"route": true},
			},
			subRouterTransaction: &common.TransactionConfig{
				Enabled: true,
				Options: map[string]any{"subrouter": true},
			},
			want: &common.TransactionConfig{
				Enabled: false,
				Options: map[string]any{"route": true},
			},
		},
		{
			name:             "subrouter override when no route override",
			routeTransaction: nil,
			subRouterTransaction: &common.TransactionConfig{
				Enabled: true,
				Options: map[string]any{"subrouter": true},
			},
			want: &common.TransactionConfig{
				Enabled: true,
				Options: map[string]any{"subrouter": true},
			},
		},
		{
			name:                 "global config when no overrides",
			routeTransaction:     nil,
			subRouterTransaction: nil,
			want: &common.TransactionConfig{
				Enabled: true,
				Options: map[string]any{"global": true},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := r.getEffectiveTransaction(tt.routeTransaction, tt.subRouterTransaction)
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestTransactionHandling_StandardRoute(t *testing.T) {
	tests := []struct {
		name               string
		handlerStatus      int
		handlerError       bool
		expectCommit       bool
		expectRollback     bool
		transactionEnabled bool
		factoryError       bool
	}{
		{
			name:               "successful handler commits transaction",
			handlerStatus:      http.StatusOK,
			handlerError:       false,
			expectCommit:       true,
			expectRollback:     false,
			transactionEnabled: true,
		},
		{
			name:               "3xx status still commits",
			handlerStatus:      http.StatusMovedPermanently,
			handlerError:       false,
			expectCommit:       true,
			expectRollback:     false,
			transactionEnabled: true,
		},
		{
			name:               "4xx status rolls back",
			handlerStatus:      http.StatusBadRequest,
			handlerError:       false,
			expectCommit:       false,
			expectRollback:     true,
			transactionEnabled: true,
		},
		{
			name:               "5xx status rolls back",
			handlerStatus:      http.StatusInternalServerError,
			handlerError:       false,
			expectCommit:       false,
			expectRollback:     true,
			transactionEnabled: true,
		},
		{
			name:               "no transaction when disabled",
			handlerStatus:      http.StatusOK,
			handlerError:       false,
			expectCommit:       false,
			expectRollback:     false,
			transactionEnabled: false,
		},
		{
			name:               "factory error returns 500",
			handlerStatus:      http.StatusOK,
			handlerError:       false,
			expectCommit:       false,
			expectRollback:     false,
			transactionEnabled: true,
			factoryError:       true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock transaction
			mockTx := &mocks.MockTransaction{}
			
			// Create mock factory
			mockFactory := &mocks.MockTransactionFactory{}
			if tt.factoryError {
				mockFactory.BeginFunc = func(ctx context.Context, options map[string]any) (scontext.DatabaseTransaction, error) {
					return nil, errors.New("factory error")
				}
			} else {
				mockFactory.BeginFunc = func(ctx context.Context, options map[string]any) (scontext.DatabaseTransaction, error) {
					return mockTx, nil
				}
			}

			// Create router with transaction factory
			r := NewRouter[string, TestUser](RouterConfig{
				Logger:             zaptest.NewLogger(t),
				TransactionFactory: mockFactory,
			}, nil, nil)

			// Handler that writes specific status
			handler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				// Verify transaction is in context if enabled
				if tt.transactionEnabled && !tt.factoryError {
					tx, ok := scontext.GetTransaction[string, TestUser](req.Context())
					assert.True(t, ok, "transaction should be in context")
					assert.NotNil(t, tx)
				}
				w.WriteHeader(tt.handlerStatus)
			})

			// Register route with transaction
			r.RegisterRoute(RouteConfigBase{
				Path:    "/test",
				Methods: []HttpMethod{MethodGet},
				Handler: handler,
				Overrides: common.RouteOverrides{
					Transaction: &common.TransactionConfig{
						Enabled: tt.transactionEnabled,
					},
				},
			})

			// Make request
			req := httptest.NewRequest("GET", "/test", nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			// Check response
			if tt.factoryError {
				assert.Equal(t, http.StatusInternalServerError, w.Code)
			} else {
				assert.Equal(t, tt.handlerStatus, w.Code)
			}

			// Verify transaction calls
			if tt.transactionEnabled && !tt.factoryError {
				assert.Equal(t, tt.expectCommit, mockTx.IsCommitCalled())
				assert.Equal(t, tt.expectRollback, mockTx.IsRollbackCalled())
			} else {
				assert.False(t, mockTx.IsCommitCalled())
				assert.False(t, mockTx.IsRollbackCalled())
			}
		})
	}
}

func TestTransactionHandling_GenericRoute(t *testing.T) {
	type Request struct {
		Name string `json:"name"`
	}
	type Response struct {
		Message string `json:"message"`
		Status  int    `json:"status"`
	}

	tests := []struct {
		name               string
		handlerFunc        GenericHandler[Request, Response]
		expectCommit       bool
		expectRollback     bool
		transactionEnabled bool
	}{
		{
			name: "successful handler commits transaction",
			handlerFunc: func(r *http.Request, data Request) (Response, error) {
				// Verify transaction is in context
				tx, ok := scontext.GetTransaction[string, TestUser](r.Context())
				assert.True(t, ok)
				assert.NotNil(t, tx)
				return Response{Message: "success", Status: 200}, nil
			},
			expectCommit:       true,
			expectRollback:     false,
			transactionEnabled: true,
		},
		{
			name: "handler error rolls back transaction",
			handlerFunc: func(r *http.Request, data Request) (Response, error) {
				return Response{}, errors.New("handler error")
			},
			expectCommit:       false,
			expectRollback:     true,
			transactionEnabled: true,
		},
		{
			name: "HTTPError with 4xx rolls back",
			handlerFunc: func(r *http.Request, data Request) (Response, error) {
				return Response{}, NewHTTPError(http.StatusBadRequest, "bad request")
			},
			expectCommit:       false,
			expectRollback:     true,
			transactionEnabled: true,
		},
		{
			name: "HTTPError with 2xx still commits",
			handlerFunc: func(r *http.Request, data Request) (Response, error) {
				// This is unusual but possible
				return Response{}, NewHTTPError(http.StatusAccepted, "accepted")
			},
			expectCommit:       true,
			expectRollback:     false,
			transactionEnabled: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create mock transaction
			mockTx := &mocks.MockTransaction{}
			
			// Create mock factory
			mockFactory := &mocks.MockTransactionFactory{
				BeginFunc: func(ctx context.Context, options map[string]any) (scontext.DatabaseTransaction, error) {
					return mockTx, nil
				},
			}

			// Create router
			r := NewRouter[string, TestUser](RouterConfig{
				Logger:             zaptest.NewLogger(t),
				TransactionFactory: mockFactory,
				SubRouters: []SubRouterConfig{
					{
						PathPrefix: "/api",
						Routes: []RouteDefinition{
							NewGenericRouteDefinition[Request, Response, string, TestUser](
								RouteConfig[Request, Response]{
									Path:    "/test",
									Methods: []HttpMethod{MethodPost},
									Codec:   codec.NewJSONCodec[Request, Response](),
									Handler: tt.handlerFunc,
									Overrides: common.RouteOverrides{
										Transaction: &common.TransactionConfig{
											Enabled: tt.transactionEnabled,
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
			req := httptest.NewRequest("POST", "/api/test", strings.NewReader(reqBody))
			req.Header.Set("Content-Type", "application/json")
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			// Verify transaction calls
			if tt.transactionEnabled {
				assert.Equal(t, tt.expectCommit, mockTx.IsCommitCalled())
				assert.Equal(t, tt.expectRollback, mockTx.IsRollbackCalled())
			} else {
				assert.False(t, mockTx.IsCommitCalled())
				assert.False(t, mockTx.IsRollbackCalled())
			}
		})
	}
}

func TestTransactionHandling_PanicRollback(t *testing.T) {
	// Create mock transaction
	mockTx := &mocks.MockTransaction{}
	
	// Create mock factory
	mockFactory := &mocks.MockTransactionFactory{
		BeginFunc: func(ctx context.Context, options map[string]any) (scontext.DatabaseTransaction, error) {
			return mockTx, nil
		},
	}

	// Create router
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
		panic("test panic")
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

	// Check response
	assert.Equal(t, http.StatusInternalServerError, w.Code)

	// Verify transaction was rolled back
	assert.False(t, mockTx.IsCommitCalled())
	assert.True(t, mockTx.IsRollbackCalled())
}

func TestTransactionHandling_WithMiddleware(t *testing.T) {
	// Create mock transaction
	mockTx := &mocks.MockTransaction{}
	
	// Create mock factory
	mockFactory := &mocks.MockTransactionFactory{
		BeginFunc: func(ctx context.Context, options map[string]any) (scontext.DatabaseTransaction, error) {
			return mockTx, nil
		},
	}

	// Create router
	r := NewRouter[string, TestUser](RouterConfig{
		Logger:             zaptest.NewLogger(t),
		TransactionFactory: mockFactory,
	}, nil, nil)

	// Variable to check if transaction was available in handler
	var txAvailable bool
	var txInHandler scontext.DatabaseTransaction

	// Handler that checks for transaction
	handler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		txInHandler, txAvailable = scontext.GetTransaction[string, TestUser](req.Context())
		w.WriteHeader(http.StatusOK)
	})

	// Register route with transaction and middleware
	r.RegisterRoute(RouteConfigBase{
		Path:        "/test",
		Methods:     []HttpMethod{MethodGet},
		Handler:     handler,
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
	assert.True(t, txAvailable, "transaction should be available in handler")
	assert.NotNil(t, txInHandler)
	assert.True(t, mockTx.IsCommitCalled())
}

func TestTransactionHandling_Hierarchy(t *testing.T) {
	// Test transaction config hierarchy: route > subrouter > global
	
	// Create mock factory that tracks options
	var capturedOptions map[string]any
	mockFactory := &mocks.MockTransactionFactory{
		BeginFunc: func(ctx context.Context, options map[string]any) (scontext.DatabaseTransaction, error) {
			capturedOptions = options
			return &mocks.MockTransaction{}, nil
		},
	}

	// Create router with global transaction config
	r := NewRouter[string, TestUser](RouterConfig{
		Logger: zaptest.NewLogger(t),
		GlobalTransaction: &common.TransactionConfig{
			Enabled: true,
			Options: map[string]any{"level": "global"},
		},
		TransactionFactory: mockFactory,
		SubRouters: []SubRouterConfig{
			{
				PathPrefix: "/sub",
				Overrides: common.RouteOverrides{
					Transaction: &common.TransactionConfig{
						Enabled: true,
						Options: map[string]any{"level": "subrouter"},
					},
				},
				Routes: []RouteDefinition{
					RouteConfigBase{
						Path:    "/route1",
						Methods: []HttpMethod{MethodGet},
						Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							w.WriteHeader(http.StatusOK)
						}),
						// This route uses subrouter config
					},
					RouteConfigBase{
						Path:    "/route2",
						Methods: []HttpMethod{MethodGet},
						Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
							w.WriteHeader(http.StatusOK)
						}),
						Overrides: common.RouteOverrides{
							Transaction: &common.TransactionConfig{
								Enabled: true,
								Options: map[string]any{"level": "route"},
							},
						},
					},
				},
			},
		},
	}, nil, nil)

	// Test route without override - should use subrouter config
	req1 := httptest.NewRequest("GET", "/sub/route1", nil)
	w1 := httptest.NewRecorder()
	r.ServeHTTP(w1, req1)
	assert.Equal(t, "subrouter", capturedOptions["level"])

	// Test route with override - should use route config
	req2 := httptest.NewRequest("GET", "/sub/route2", nil)
	w2 := httptest.NewRecorder()
	r.ServeHTTP(w2, req2)
	assert.Equal(t, "route", capturedOptions["level"])

	// Test global route - should use global config
	r.RegisterRoute(RouteConfigBase{
		Path:    "/global",
		Methods: []HttpMethod{MethodGet},
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		}),
	})
	req3 := httptest.NewRequest("GET", "/global", nil)
	w3 := httptest.NewRecorder()
	r.ServeHTTP(w3, req3)
	assert.Equal(t, "global", capturedOptions["level"])
}

func TestTransactionHandling_ConcurrentRequests(t *testing.T) {
	// Test that concurrent requests get separate transactions
	txCount := 0
	var mu sync.Mutex
	
	mockFactory := &mocks.MockTransactionFactory{
		BeginFunc: func(ctx context.Context, options map[string]any) (scontext.DatabaseTransaction, error) {
			mu.Lock()
			txCount++
			currentTx := txCount
			mu.Unlock()
			
			return &mocks.MockTransaction{
				GetDBFunc: func() *gorm.DB {
					// Return a unique value to identify this transaction
					return &gorm.DB{Config: &gorm.Config{DryRun: true, SkipDefaultTransaction: true}, Error: errors.New(fmt.Sprintf("tx-%d", currentTx))}
				},
			}, nil
		},
	}

	r := NewRouter[string, TestUser](RouterConfig{
		Logger:             zaptest.NewLogger(t),
		TransactionFactory: mockFactory,
	}, nil, nil)

	// Handler that checks transaction uniqueness
	handler := http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		tx, ok := scontext.GetTransaction[string, TestUser](req.Context())
		assert.True(t, ok)
		
		// Sleep a bit to ensure concurrent execution
		time.Sleep(10 * time.Millisecond)
		
		// Write the transaction identifier
		db := tx.GetDB()
		if db != nil && db.Error != nil {
			w.Write([]byte(db.Error.Error()))
		}
	})

	r.RegisterRoute(RouteConfigBase{
		Path:    "/concurrent",
		Methods: []HttpMethod{MethodGet},
		Handler: handler,
		Overrides: common.RouteOverrides{
			Transaction: &common.TransactionConfig{
				Enabled: true,
			},
		},
	})

	// Make concurrent requests
	const numRequests = 5
	results := make(chan string, numRequests)
	
	for i := 0; i < numRequests; i++ {
		go func() {
			req := httptest.NewRequest("GET", "/concurrent", nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)
			results <- w.Body.String()
		}()
	}

	// Collect results
	seen := make(map[string]bool)
	for i := 0; i < numRequests; i++ {
		result := <-results
		assert.False(t, seen[result], "each request should get a unique transaction")
		seen[result] = true
	}
	
	assert.Equal(t, numRequests, len(seen))
}