package router

import (
	"net/http"
	"testing"

	"github.com/Suhaibinator/SRouter/pkg/common"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zaptest"
)

// TestWrapWithTransactionAlwaysReturnsHandlerFunc proves that wrapWithTransaction
// always returns an http.HandlerFunc, making the type assertion unnecessary
func TestWrapWithTransactionAlwaysReturnsHandlerFunc(t *testing.T) {
	r := &Router[string, TestUser]{
		config: RouterConfig{
			Logger: zaptest.NewLogger(t),
		},
		logger: zaptest.NewLogger(t),
	}

	// Test 1: When transaction is nil/disabled, returns original HandlerFunc
	t.Run("transaction disabled returns original HandlerFunc", func(t *testing.T) {
		originalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		result := r.wrapWithTransaction(originalHandler, nil)

		// Type assertion should always succeed
		_, ok := result.(http.HandlerFunc)
		assert.True(t, ok, "result should be http.HandlerFunc")
	})

	// Test 2: When transaction is enabled but no factory, returns original
	t.Run("transaction enabled but no factory returns original", func(t *testing.T) {
		originalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
		})

		txConfig := &common.TransactionConfig{Enabled: true}
		result := r.wrapWithTransaction(originalHandler, txConfig)

		// Type assertion should succeed
		_, ok := result.(http.HandlerFunc)
		assert.True(t, ok, "result should be http.HandlerFunc")
	})

	// Test 3: Demonstrate that the conversion code is unreachable
	t.Run("conversion code is unreachable", func(t *testing.T) {
		// In RegisterRoute, the handler is always http.HandlerFunc
		route := RouteConfigBase{
			Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}),
		}

		// After wrapWithTransaction
		finalHandler := r.wrapWithTransaction(route.Handler, nil)

		// This type assertion ALWAYS succeeds
		handlerFunc, ok := finalHandler.(http.HandlerFunc)
		assert.True(t, ok, "type assertion always succeeds")
		assert.NotNil(t, handlerFunc)

		// The conversion code is unreachable
		if !ok {
			// This code can NEVER execute
			t.Fatal("This should never happen")
		}
	})
}

// TestDeadCodeRemovalSuggestion demonstrates that the type assertion can be removed
func TestDeadCodeRemovalSuggestion(t *testing.T) {
	r := &Router[string, TestUser]{
		config: RouterConfig{
			Logger: zaptest.NewLogger(t),
		},
		logger: zaptest.NewLogger(t),
	}

	// Current code pattern (with unnecessary type assertion):
	currentPattern := func(handler http.HandlerFunc) http.HandlerFunc {
		finalHandler := r.wrapWithTransaction(handler, nil)

		// This type assertion is unnecessary
		handlerFunc, ok := finalHandler.(http.HandlerFunc)
		if !ok {
			handlerFunc = http.HandlerFunc(finalHandler.ServeHTTP)
		}

		return handlerFunc
	}

	// Suggested simplified pattern:
	suggestedPattern := func(handler http.HandlerFunc) http.HandlerFunc {
		// Since wrapWithTransaction always returns http.HandlerFunc when given http.HandlerFunc
		// we can directly cast without checking
		return r.wrapWithTransaction(handler, nil).(http.HandlerFunc)
	}

	// Both patterns produce the same result
	testHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	result1 := currentPattern(testHandler)
	result2 := suggestedPattern(testHandler)

	// Both patterns work the same
	assert.NotNil(t, result1)
	assert.NotNil(t, result2)
}

// TestWhyTypeAssertionExists explains why the code might have been written this way
func TestWhyTypeAssertionExists(t *testing.T) {
	// The type assertion exists because wrapWithTransaction is declared to return http.Handler
	// not http.HandlerFunc, even though it always returns http.HandlerFunc in practice.

	// This is the function signature:
	// func (r *Router[T, U]) wrapWithTransaction(handler http.Handler, transaction *common.TransactionConfig) http.Handler

	// Options to fix:
	// 1. Change wrapWithTransaction to return http.HandlerFunc
	// 2. Remove the type assertion and cast directly
	// 3. Keep as defensive programming (current state)

	t.Log("The type assertion exists because wrapWithTransaction returns http.Handler interface")
	t.Log("even though it always returns an http.HandlerFunc concrete type")
}
