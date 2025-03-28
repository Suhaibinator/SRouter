package router

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/middleware"
	"github.com/Suhaibinator/SRouter/pkg/router/internal/mocks" // Use centralized mocks
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// Note: Removed unused imports "context" and "strings"

// --- Tests from trace_test.go ---

// TestTraceIDLogging tests that trace IDs are included in log entries when EnableTraceID is true
func TestTraceIDLogging(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)
	logger := zap.New(core)
	r := NewRouter(RouterConfig{Logger: logger, EnableMetrics: true, EnableTraceID: true}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)
	r.RegisterRoute(RouteConfigBase{Path: "/test", Methods: []string{"GET"}, Handler: func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }})
	req, err := http.NewRequest("GET", "/test", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	traceID := "test-trace-id"
	req = middleware.AddTraceIDToRequest(req, traceID)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	logEntries := logs.All()
	if len(logEntries) == 0 {
		t.Errorf("Expected logs to be recorded")
	}
	found := false
	for _, log := range logEntries {
		for _, field := range log.Context {
			if field.Key == "trace_id" && field.String == traceID {
				found = true
				break
			}
		}
		if found {
			break
		}
	}
	if !found {
		t.Errorf("Expected trace ID to be included in log entries")
	}
}

// TestTraceIDLoggingDisabled tests that trace IDs are not included in log entries when EnableTraceID is false
func TestTraceIDLoggingDisabled(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)
	logger := zap.New(core)
	r := NewRouter(RouterConfig{Logger: logger, EnableMetrics: true, EnableTraceID: false}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)
	r.RegisterRoute(RouteConfigBase{Path: "/test", Methods: []string{"GET"}, Handler: func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }})
	req, err := http.NewRequest("GET", "/test", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	traceID := "test-trace-id"
	req = middleware.AddTraceIDToRequest(req, traceID)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	logEntries := logs.All()
	if len(logEntries) == 0 {
		t.Errorf("Expected logs to be recorded")
	}
	found := false
	for _, log := range logEntries {
		for _, field := range log.Context {
			if field.Key == "trace_id" {
				found = true
				break
			}
		}
		if found {
			break
		}
	}
	if found {
		t.Errorf("Expected trace ID not to be included in log entries")
	}
}

// TestLoggingMiddlewareWithTraceID tests that LoggingMiddleware includes trace IDs in log entries when enableTraceID is true
func TestLoggingMiddlewareWithTraceID(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	loggingMiddleware := LoggingMiddleware(logger, true)
	wrappedHandler := loggingMiddleware(handler)
	req, err := http.NewRequest("GET", "/test", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	traceID := "test-trace-id"
	req = middleware.AddTraceIDToRequest(req, traceID)
	rr := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(rr, req)
	logEntries := logs.All()
	if len(logEntries) == 0 {
		t.Errorf("Expected logs to be recorded")
	}
	found := false
	for _, log := range logEntries {
		for _, field := range log.Context {
			if field.Key == "trace_id" && field.String == traceID {
				found = true
				break
			}
		}
		if found {
			break
		}
	}
	if !found {
		t.Errorf("Expected trace ID to be included in log entries")
	}
}

// TestLoggingMiddlewareWithoutTraceID tests that LoggingMiddleware does not include trace IDs in log entries when enableTraceID is false
func TestLoggingMiddlewareWithoutTraceID(t *testing.T) {
	core, logs := observer.New(zap.InfoLevel)
	logger := zap.New(core)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	loggingMiddleware := LoggingMiddleware(logger, false)
	wrappedHandler := loggingMiddleware(handler)
	req, err := http.NewRequest("GET", "/test", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	traceID := "test-trace-id"
	req = middleware.AddTraceIDToRequest(req, traceID)
	rr := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(rr, req)
	logEntries := logs.All()
	if len(logEntries) == 0 {
		t.Errorf("Expected logs to be recorded")
	}
	found := false
	for _, log := range logEntries {
		for _, field := range log.Context {
			if field.Key == "trace_id" {
				found = true
				break
			}
		}
		if found {
			break
		}
	}
	if found {
		t.Errorf("Expected trace ID not to be included in log entries")
	}
}

// TestHandleErrorWithTraceID tests that handleError includes trace IDs in log entries when EnableTraceID is true
func TestHandleErrorWithTraceID(t *testing.T) {
	core, logs := observer.New(zap.ErrorLevel)
	logger := zap.New(core)
	r := NewRouter(RouterConfig{Logger: logger, EnableTraceID: true}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)
	req, err := http.NewRequest("GET", "/test", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	traceID := "test-trace-id"
	req = middleware.AddTraceIDToRequest(req, traceID)
	rr := httptest.NewRecorder()
	r.handleError(rr, req, err, http.StatusInternalServerError, "Test error") // Pass err from NewRequest
	logEntries := logs.All()
	if len(logEntries) == 0 {
		t.Errorf("Expected logs to be recorded")
	}
	found := false
	for _, log := range logEntries {
		for _, field := range log.Context {
			if field.Key == "trace_id" && field.String == traceID {
				found = true
				break
			}
		}
		if found {
			break
		}
	}
	if !found {
		t.Errorf("Expected trace ID to be included in log entries")
	}
}

// TestHandleErrorWithoutTraceID tests that handleError does not include trace IDs in log entries when EnableTraceID is false
func TestHandleErrorWithoutTraceID(t *testing.T) {
	core, logs := observer.New(zap.ErrorLevel)
	logger := zap.New(core)
	r := NewRouter(RouterConfig{Logger: logger, EnableTraceID: false}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)
	req, err := http.NewRequest("GET", "/test", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	traceID := "test-trace-id"
	req = middleware.AddTraceIDToRequest(req, traceID)
	rr := httptest.NewRecorder()
	r.handleError(rr, req, err, http.StatusInternalServerError, "Test error") // Pass err from NewRequest
	logEntries := logs.All()
	if len(logEntries) == 0 {
		t.Errorf("Expected logs to be recorded")
	}
	found := false
	for _, log := range logEntries {
		for _, field := range log.Context {
			if field.Key == "trace_id" {
				found = true
				break
			}
		}
		if found {
			break
		}
	}
	if found {
		t.Errorf("Expected trace ID not to be included in log entries")
	}
}

// TestRecoveryMiddlewareWithTraceID tests that recoveryMiddleware includes trace IDs in log entries when EnableTraceID is true
func TestRecoveryMiddlewareWithTraceID(t *testing.T) {
	core, logs := observer.New(zap.ErrorLevel)
	logger := zap.New(core)
	r := NewRouter(RouterConfig{Logger: logger, EnableTraceID: true}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { panic("Test panic") })
	wrappedHandler := r.recoveryMiddleware(handler)
	req, err := http.NewRequest("GET", "/test", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	traceID := "test-trace-id"
	req = middleware.AddTraceIDToRequest(req, traceID)
	rr := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(rr, req)
	logEntries := logs.All()
	if len(logEntries) == 0 {
		t.Errorf("Expected logs to be recorded")
	}
	found := false
	for _, log := range logEntries {
		for _, field := range log.Context {
			if field.Key == "trace_id" && field.String == traceID {
				found = true
				break
			}
		}
		if found {
			break
		}
	}
	if !found {
		t.Errorf("Expected trace ID to be included in log entries")
	}
}

// TestRecoveryMiddlewareWithoutTraceID tests that recoveryMiddleware does not include trace IDs in log entries when EnableTraceID is false
func TestRecoveryMiddlewareWithoutTraceID(t *testing.T) {
	core, logs := observer.New(zap.ErrorLevel)
	logger := zap.New(core)
	r := NewRouter(RouterConfig{Logger: logger, EnableTraceID: false}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { panic("Test panic") })
	wrappedHandler := r.recoveryMiddleware(handler)
	req, err := http.NewRequest("GET", "/test", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	traceID := "test-trace-id"
	req = middleware.AddTraceIDToRequest(req, traceID)
	rr := httptest.NewRecorder()
	wrappedHandler.ServeHTTP(rr, req)
	logEntries := logs.All()
	if len(logEntries) == 0 {
		t.Errorf("Expected logs to be recorded")
	}
	found := false
	for _, log := range logEntries {
		for _, field := range log.Context {
			if field.Key == "trace_id" {
				found = true
				break
			}
		}
		if found {
			break
		}
	}
	if found {
		t.Errorf("Expected trace ID not to be included in log entries")
	}
}

// --- Tests from advanced_features_test.go ---

// TestSlowRequestLogging tests that slow requests are logged with a warning
func TestSlowRequestLogging(t *testing.T) {
	core, logs := observer.New(zap.WarnLevel)
	logger := zap.New(core)
	r := NewRouter(RouterConfig{Logger: logger, EnableMetrics: true}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)
	r.RegisterRoute(RouteConfigBase{
		Path:    "/slow",
		Methods: []string{"GET"},
		Handler: func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(1100 * time.Millisecond) // Longer than 1s threshold
			w.WriteHeader(http.StatusOK)
		},
	})
	req, _ := http.NewRequest("GET", "/slow", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rr.Code)
	}
	logEntries := logs.All()
	if len(logEntries) == 0 {
		t.Errorf("Expected warning to be logged")
	}
	found := false
	for _, log := range logEntries {
		if log.Message == "Slow request" {
			found = true
			if log.Context[0].Key != "method" || log.Context[0].String != "GET" {
				t.Errorf("Expected method field GET, got %q", log.Context[0].String)
			}
			if log.Context[1].Key != "path" || log.Context[1].String != "/slow" {
				t.Errorf("Expected path field /slow, got %q", log.Context[1].String)
			}
			break
		}
	}
	if !found {
		t.Errorf("Expected 'Slow request' log message")
	}
}

// TestErrorStatusLogging tests that error status codes are logged appropriately
func TestErrorStatusLogging(t *testing.T) {
	// Test server error (5xx)
	coreErr, logsErr := observer.New(zap.ErrorLevel)
	loggerErr := zap.New(coreErr)
	rErr := NewRouter(RouterConfig{Logger: loggerErr, EnableMetrics: true}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)
	rErr.RegisterRoute(RouteConfigBase{Path: "/server-error", Methods: []string{"GET"}, Handler: func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusInternalServerError) }})
	reqErr, _ := http.NewRequest("GET", "/server-error", nil)
	rrErr := httptest.NewRecorder()
	rErr.ServeHTTP(rrErr, reqErr)
	if rrErr.Code != http.StatusInternalServerError {
		t.Errorf("Expected status code %d, got %d", http.StatusInternalServerError, rrErr.Code)
	}
	logEntriesErr := logsErr.All()
	if len(logEntriesErr) == 0 {
		t.Errorf("Expected error to be logged")
	}
	foundErr := false
	for _, log := range logEntriesErr {
		if log.Message == "Server error" {
			foundErr = true
			break
		}
	}
	if !foundErr {
		t.Errorf("Expected 'Server error' log message")
	}

	// Test client error (4xx)
	coreWarn, logsWarn := observer.New(zap.WarnLevel)
	loggerWarn := zap.New(coreWarn)
	rWarn := NewRouter(RouterConfig{Logger: loggerWarn, EnableMetrics: true}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)
	rWarn.RegisterRoute(RouteConfigBase{Path: "/client-error", Methods: []string{"GET"}, Handler: func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusBadRequest) }})
	reqWarn, _ := http.NewRequest("GET", "/client-error", nil)
	rrWarn := httptest.NewRecorder()
	rWarn.ServeHTTP(rrWarn, reqWarn)
	if rrWarn.Code != http.StatusBadRequest {
		t.Errorf("Expected status code %d, got %d", http.StatusBadRequest, rrWarn.Code)
	}
	logEntriesWarn := logsWarn.All()
	if len(logEntriesWarn) == 0 {
		t.Errorf("Expected warning to be logged")
	}
	foundWarn := false
	for _, log := range logEntriesWarn {
		if log.Message == "Client error" {
			foundWarn = true
			break
		}
	}
	if !foundWarn {
		t.Errorf("Expected 'Client error' log message")
	}
}

// TestLoggingMiddlewareWithFlusher tests the LoggingMiddleware with a response writer that implements http.Flusher
func TestLoggingMiddlewareWithFlusher(t *testing.T) {
	logger := zap.NewNop()
	flushed := false // Track if Flush was called

	// Create a handler that calls Flush
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("Hello, World!"))
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
			flushed = true // Mark as flushed
		} else {
			t.Error("ResponseWriter does not implement http.Flusher")
		}
	})

	// Wrap with LoggingMiddleware
	wrappedHandler := LoggingMiddleware(logger, false)(handler)

	// Create a request and a recorder that supports flushing
	req, _ := http.NewRequest("GET", "/test", nil)
	rr := mocks.NewFlusherRecorder() // Use mock FlusherRecorder

	// Serve the request
	wrappedHandler.ServeHTTP(rr, req)

	// Check status code
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rr.Code)
	}
	// Check response body by accessing the underlying ResponseRecorder's Body
	if rr.ResponseRecorder.Body.String() != "Hello, World!" {
		t.Errorf("Expected response body %q, got %q", "Hello, World!", rr.ResponseRecorder.Body.String())
	}
	// Check if Flush was called (via the mock recorder)
	if !rr.Flushed {
		t.Errorf("Expected Flush to be called")
	}
	// Check the local flushed variable as well (redundant but confirms handler logic)
	if !flushed {
		t.Errorf("Expected Flush to be called within the handler")
	}
}
