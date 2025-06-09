package router

import (
	"context"
	"net/http"
	"strings"
	"testing"

	"github.com/Suhaibinator/SRouter/pkg/common"
	"github.com/Suhaibinator/SRouter/pkg/router/internal/mocks"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zaptest"
)

// TestTransactionValidation tests the transaction configuration validation in NewRouter
func TestTransactionValidation(t *testing.T) {
	// Mock auth functions
	mockAuth := func(ctx context.Context, token string) (*TestUser, bool) {
		return &TestUser{ID: "1", Name: "Test"}, true
	}
	mockGetUserID := func(u *TestUser) string {
		return u.ID
	}

	t.Run("GlobalTransaction enabled without TransactionFactory should panic", func(t *testing.T) {
		defer func() {
			r := recover()
			assert.NotNil(t, r, "Expected panic")
			errMsg, ok := r.(string)
			assert.True(t, ok, "Expected string panic message")
			assert.Contains(t, errMsg, "GlobalTransaction.Enabled is true but TransactionFactory is nil")
		}()

		config := RouterConfig{
			Logger: zaptest.NewLogger(t),
			GlobalTransaction: &common.TransactionConfig{
				Enabled: true,
			},
			// TransactionFactory is nil
		}
		
		NewRouter[string, TestUser](config, mockAuth, mockGetUserID)
	})

	t.Run("SubRouter transaction enabled without TransactionFactory should panic", func(t *testing.T) {
		defer func() {
			r := recover()
			assert.NotNil(t, r, "Expected panic")
			errMsg, ok := r.(string)
			assert.True(t, ok, "Expected string panic message")
			assert.Contains(t, errMsg, "SubRouters[0]: Transaction.Enabled is true but TransactionFactory is nil")
		}()

		config := RouterConfig{
			Logger: zaptest.NewLogger(t),
			SubRouters: []SubRouterConfig{
				{
					PathPrefix: "/api",
					Overrides: common.RouteOverrides{
						Transaction: &common.TransactionConfig{
							Enabled: true,
						},
					},
				},
			},
			// TransactionFactory is nil
		}
		
		NewRouter[string, TestUser](config, mockAuth, mockGetUserID)
	})

	t.Run("Route transaction enabled without TransactionFactory should panic", func(t *testing.T) {
		defer func() {
			r := recover()
			assert.NotNil(t, r, "Expected panic")
			errMsg, ok := r.(string)
			assert.True(t, ok, "Expected string panic message")
			assert.Contains(t, errMsg, "SubRouters[0].Routes[0]: Transaction.Enabled is true but TransactionFactory is nil")
		}()

		config := RouterConfig{
			Logger: zaptest.NewLogger(t),
			SubRouters: []SubRouterConfig{
				{
					PathPrefix: "/api",
					Routes: []RouteDefinition{
						RouteConfigBase{
							Path:    "/test",
							Methods: []HttpMethod{MethodGet},
							Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
							Overrides: common.RouteOverrides{
								Transaction: &common.TransactionConfig{
									Enabled: true,
								},
							},
						},
					},
				},
			},
			// TransactionFactory is nil
		}
		
		NewRouter[string, TestUser](config, mockAuth, mockGetUserID)
	})

	t.Run("Nested SubRouter transaction enabled without TransactionFactory should panic", func(t *testing.T) {
		defer func() {
			r := recover()
			assert.NotNil(t, r, "Expected panic")
			errMsg, ok := r.(string)
			assert.True(t, ok, "Expected string panic message")
			assert.Contains(t, errMsg, "SubRouters[0].SubRouters[0]: Transaction.Enabled is true but TransactionFactory is nil")
		}()

		config := RouterConfig{
			Logger: zaptest.NewLogger(t),
			SubRouters: []SubRouterConfig{
				{
					PathPrefix: "/api",
					SubRouters: []SubRouterConfig{
						{
							PathPrefix: "/v1",
							Overrides: common.RouteOverrides{
								Transaction: &common.TransactionConfig{
									Enabled: true,
								},
							},
						},
					},
				},
			},
			// TransactionFactory is nil
		}
		
		NewRouter[string, TestUser](config, mockAuth, mockGetUserID)
	})

	t.Run("Transaction disabled with nil TransactionFactory should not panic", func(t *testing.T) {
		config := RouterConfig{
			Logger: zaptest.NewLogger(t),
			GlobalTransaction: &common.TransactionConfig{
				Enabled: false, // Disabled
			},
			SubRouters: []SubRouterConfig{
				{
					PathPrefix: "/api",
					Overrides: common.RouteOverrides{
						Transaction: &common.TransactionConfig{
							Enabled: false, // Disabled
						},
					},
					Routes: []RouteDefinition{
						RouteConfigBase{
							Path:    "/test",
							Methods: []HttpMethod{MethodGet},
							Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
							Overrides: common.RouteOverrides{
								Transaction: &common.TransactionConfig{
									Enabled: false, // Disabled
								},
							},
						},
					},
				},
			},
			// TransactionFactory is nil
		}
		
		// Should not panic
		r := NewRouter[string, TestUser](config, mockAuth, mockGetUserID)
		assert.NotNil(t, r)
	})

	t.Run("Nil transaction configs with nil TransactionFactory should not panic", func(t *testing.T) {
		config := RouterConfig{
			Logger: zaptest.NewLogger(t),
			// GlobalTransaction is nil
			SubRouters: []SubRouterConfig{
				{
					PathPrefix: "/api",
					// Transaction override is nil
					Routes: []RouteDefinition{
						RouteConfigBase{
							Path:    "/test",
							Methods: []HttpMethod{MethodGet},
							Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
							// Transaction override is nil
						},
					},
				},
			},
			// TransactionFactory is nil
		}
		
		// Should not panic
		r := NewRouter[string, TestUser](config, mockAuth, mockGetUserID)
		assert.NotNil(t, r)
	})

	t.Run("Transaction enabled with valid TransactionFactory should not panic", func(t *testing.T) {
		mockFactory := &mocks.MockTransactionFactory{}
		
		config := RouterConfig{
			Logger: zaptest.NewLogger(t),
			TransactionFactory: mockFactory,
			GlobalTransaction: &common.TransactionConfig{
				Enabled: true,
			},
			SubRouters: []SubRouterConfig{
				{
					PathPrefix: "/api",
					Overrides: common.RouteOverrides{
						Transaction: &common.TransactionConfig{
							Enabled: true,
						},
					},
					Routes: []RouteDefinition{
						RouteConfigBase{
							Path:    "/test",
							Methods: []HttpMethod{MethodGet},
							Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
							Overrides: common.RouteOverrides{
								Transaction: &common.TransactionConfig{
									Enabled: true,
								},
							},
						},
					},
				},
			},
		}
		
		// Should not panic
		r := NewRouter[string, TestUser](config, mockAuth, mockGetUserID)
		assert.NotNil(t, r)
	})

	t.Run("Multiple validation errors should report first error", func(t *testing.T) {
		defer func() {
			r := recover()
			assert.NotNil(t, r, "Expected panic")
			errMsg, ok := r.(string)
			assert.True(t, ok, "Expected string panic message")
			// Should report global error first
			assert.Contains(t, errMsg, "GlobalTransaction.Enabled is true but TransactionFactory is nil")
			// Should not contain sub-router error
			assert.False(t, strings.Contains(errMsg, "SubRouters"), "Should only report first error")
		}()

		config := RouterConfig{
			Logger: zaptest.NewLogger(t),
			GlobalTransaction: &common.TransactionConfig{
				Enabled: true,
			},
			SubRouters: []SubRouterConfig{
				{
					PathPrefix: "/api",
					Overrides: common.RouteOverrides{
						Transaction: &common.TransactionConfig{
							Enabled: true,
						},
					},
				},
			},
			// TransactionFactory is nil
		}
		
		NewRouter[string, TestUser](config, mockAuth, mockGetUserID)
	})
}

// TestDynamicSubRouterValidation tests that dynamically registered sub-routers bypass validation
func TestDynamicSubRouterValidation(t *testing.T) {
	mockAuth := func(ctx context.Context, token string) (*TestUser, bool) {
		return &TestUser{ID: "1", Name: "Test"}, true
	}
	mockGetUserID := func(u *TestUser) string {
		return u.ID
	}

	t.Run("RegisterSubRouter does not validate transactions", func(t *testing.T) {
		// Create router without transaction factory
		config := RouterConfig{
			Logger: zaptest.NewLogger(t),
			// TransactionFactory is nil
		}
		
		r := NewRouter[string, TestUser](config, mockAuth, mockGetUserID)
		
		// Dynamically register a sub-router with transaction enabled
		// This should not panic because validation only happens at startup
		sr := SubRouterConfig{
			PathPrefix: "/api",
			Overrides: common.RouteOverrides{
				Transaction: &common.TransactionConfig{
					Enabled: true,
				},
			},
		}
		
		// Should not panic - dynamic registration bypasses validation
		// The transaction will silently not work, but that's expected behavior
		assert.NotPanics(t, func() {
			r.RegisterSubRouter(sr)
		})
	})
}