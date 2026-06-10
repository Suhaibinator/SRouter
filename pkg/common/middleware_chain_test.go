package common

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMiddlewareChain(t *testing.T) {
	// Create a chain of middleware
	chain := NewMiddlewareChain()

	// Add middleware that adds headers
	chain = chain.Append(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Add("X-Test-1", "value1")
			next.ServeHTTP(w, r)
		})
	})

	chain = chain.Append(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Add("X-Test-2", "value2")
			next.ServeHTTP(w, r)
		})
	})

	// Create a final handler
	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Add("X-Final", "final")
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	// Wrap the final handler with the middleware chain
	handler := chain.Then(finalHandler)

	// Create a test request
	req := httptest.NewRequest("GET", "http://example.com/foo", nil)
	w := httptest.NewRecorder()

	// Call the handler
	handler.ServeHTTP(w, req)

	// Check the response
	if w.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
	}

	// Check the headers
	if w.Header().Get("X-Test-1") != "value1" {
		t.Errorf("Expected X-Test-1 header to be %q, got %q", "value1", w.Header().Get("X-Test-1"))
	}
	if w.Header().Get("X-Test-2") != "value2" {
		t.Errorf("Expected X-Test-2 header to be %q, got %q", "value2", w.Header().Get("X-Test-2"))
	}
	if w.Header().Get("X-Final") != "final" {
		t.Errorf("Expected X-Final header to be %q, got %q", "final", w.Header().Get("X-Final"))
	}

	// Check the body
	if w.Body.String() != "OK" {
		t.Errorf("Expected body %q, got %q", "OK", w.Body.String())
	}
}

func TestMiddlewareChainOrder(t *testing.T) {
	// Create a chain of middleware
	chain := NewMiddlewareChain()

	// Add middleware that modifies the response in order
	var order []string

	chain = chain.Append(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "middleware1-before")
			next.ServeHTTP(w, r)
			order = append(order, "middleware1-after")
		})
	})

	chain = chain.Append(func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			order = append(order, "middleware2-before")
			next.ServeHTTP(w, r)
			order = append(order, "middleware2-after")
		})
	})

	// Create a final handler
	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		order = append(order, "final-handler")
		w.WriteHeader(http.StatusOK)
	})

	// Wrap the final handler with the middleware chain
	handler := chain.Then(finalHandler)

	// Create a test request
	req := httptest.NewRequest("GET", "http://example.com/foo", nil)
	w := httptest.NewRecorder()

	// Call the handler
	handler.ServeHTTP(w, req)

	// Check the order of execution
	expected := []string{
		"middleware1-before",
		"middleware2-before",
		"final-handler",
		"middleware2-after",
		"middleware1-after",
	}

	if len(order) != len(expected) {
		t.Errorf("Expected %d middleware calls, got %d", len(expected), len(order))
	}

	for i, v := range expected {
		if i >= len(order) || order[i] != v {
			t.Errorf("Expected middleware call %d to be %q, got %q", i, v, order[i])
		}
	}
}

func TestEmptyMiddlewareChain(t *testing.T) {
	// Create an empty chain of middleware
	chain := NewMiddlewareChain()

	// Create a final handler
	finalHandler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("OK"))
	})

	// Wrap the final handler with the middleware chain
	handler := chain.Then(finalHandler)

	// Create a test request
	req := httptest.NewRequest("GET", "http://example.com/foo", nil)
	w := httptest.NewRecorder()

	// Call the handler
	handler.ServeHTTP(w, req)

	// Check the response
	if w.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, w.Code)
	}

	// Check the body
	if w.Body.String() != "OK" {
		t.Errorf("Expected body %q, got %q", "OK", w.Body.String())
	}
}

// TestMiddlewareChainAppendDoesNotAliasParent verifies that two Appends on the
// same parent chain don't share a backing array (regression: append(c, ...)
// could let the second Append clobber the first chain's middleware).
func TestMiddlewareChainAppendDoesNotAliasParent(t *testing.T) {
	record := func(tag string, log *[]string) Middleware {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				*log = append(*log, tag)
				next.ServeHTTP(w, r)
			})
		}
	}

	var log []string
	// Build a parent with spare capacity so naive append would share arrays.
	parent := make(MiddlewareChain, 0, 4)
	parent = parent.Append(record("base", &log))

	chainA := parent.Append(record("a", &log))
	chainB := parent.Append(record("b", &log)) // must not overwrite chainA's "a"

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {})

	log = nil
	chainA.Then(handler).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	if len(log) != 2 || log[0] != "base" || log[1] != "a" {
		t.Fatalf("chainA executed %v, want [base a]", log)
	}

	log = nil
	chainB.Then(handler).ServeHTTP(httptest.NewRecorder(), httptest.NewRequest("GET", "/", nil))
	if len(log) != 2 || log[0] != "base" || log[1] != "b" {
		t.Fatalf("chainB executed %v, want [base b]", log)
	}
}
