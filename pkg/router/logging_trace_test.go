package router

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/router/internal/mocks" // Use centralized mocks
	"github.com/Suhaibinator/SRouter/pkg/scontext"              // Added import
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

// Note: Removed unused imports "context" and "strings"

// --- Tests from trace_test.go ---

// TestTraceIDLogging tests that trace IDs are included in log entries when EnableTraceID is true
func TestTraceIDLogging(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)
	logger := zap.New(core)
	r := NewRouter(RouterConfig{Logger: logger, EnableMetrics: true, TraceIDBufferSize: 1000}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)
	r.RegisterRoute(RouteConfigBase{Path: "/test", Methods: []HttpMethod{MethodGet}, Handler: func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }}) // Use HttpMethod enum
	req, err := http.NewRequest("GET", "/test", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	traceID := "test-trace-id"
	ctxWithTrace := scontext.WithTraceID[string, string](req.Context(), traceID) // Use scontext
	req = req.WithContext(ctxWithTrace)                                          // Apply context
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	logEntries := logs.All()
	if len(logEntries) == 0 {
		t.Errorf("Expected logs to be recorded")
	}
	//traceID := middleware.GetTraceIDFromContext(req.Context())
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
	r := NewRouter(RouterConfig{Logger: logger, EnableMetrics: true, TraceIDBufferSize: 0, EnableTraceLogging: true}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)
	r.RegisterRoute(RouteConfigBase{Path: "/test", Methods: []HttpMethod{MethodGet}, Handler: func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) }}) // Use HttpMethod enum
	req, err := http.NewRequest("GET", "/test", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	traceID := "test-trace-id"
	ctxWithTrace := scontext.WithTraceID[string, string](req.Context(), traceID) // Use scontext
	req = req.WithContext(ctxWithTrace)                                          // Apply context
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

// TestHandleErrorWithTraceID tests that handleError includes trace IDs in log entries when EnableTraceID is true
func TestHandleErrorWithTraceID(t *testing.T) {
	core, logs := observer.New(zap.ErrorLevel)
	logger := zap.New(core)
	r := NewRouter(RouterConfig{Logger: logger, TraceIDBufferSize: 1000}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)
	req, err := http.NewRequest("GET", "/test", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	traceID := "test-trace-id"
	ctxWithTrace := scontext.WithTraceID[string, string](req.Context(), traceID) // Use scontext
	req = req.WithContext(ctxWithTrace)                                          // Apply context
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
	r := NewRouter(RouterConfig{Logger: logger, TraceIDBufferSize: 0}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)
	req, err := http.NewRequest("GET", "/test", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	traceID := "test-trace-id"
	ctxWithTrace := scontext.WithTraceID[string, string](req.Context(), traceID) // Use scontext
	req = req.WithContext(ctxWithTrace)                                          // Apply context
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
	r := NewRouter(RouterConfig{Logger: logger, TraceIDBufferSize: 1000}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { panic("Test panic") })
	wrappedHandler := r.recoveryMiddleware(handler)
	req, err := http.NewRequest("GET", "/test", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	traceID := "test-trace-id"
	ctxWithTrace := scontext.WithTraceID[string, string](req.Context(), traceID) // Use scontext
	req = req.WithContext(ctxWithTrace)                                          // Apply context
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
	r := NewRouter(RouterConfig{Logger: logger, TraceIDBufferSize: 0}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { panic("Test panic") })
	wrappedHandler := r.recoveryMiddleware(handler)
	req, err := http.NewRequest("GET", "/test", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	traceID := "test-trace-id"
	ctxWithTrace := scontext.WithTraceID[uint64, string](req.Context(), traceID) // Use scontext
	req = req.WithContext(ctxWithTrace)                                          // Apply context
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
	r := NewRouter(RouterConfig{Logger: logger, EnableMetrics: true, EnableTraceLogging: true}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)
	r.RegisterRoute(RouteConfigBase{
		Path:    "/slow",
		Methods: []HttpMethod{MethodGet}, // Use HttpMethod enum
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
	rErr := NewRouter(RouterConfig{Logger: loggerErr, EnableMetrics: true, EnableTraceLogging: true}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)                                                   // Added EnableTraceLogging
	rErr.RegisterRoute(RouteConfigBase{Path: "/server-error", Methods: []HttpMethod{MethodGet}, Handler: func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusInternalServerError) }}) // Use HttpMethod enum
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
	coreWarn, logsWarn := observer.New(zap.InfoLevel)
	loggerWarn := zap.New(coreWarn)
	rWarn := NewRouter(RouterConfig{Logger: loggerWarn, EnableMetrics: true, EnableTraceLogging: true}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)                                         // Added EnableTraceLogging
	rWarn.RegisterRoute(RouteConfigBase{Path: "/client-error", Methods: []HttpMethod{MethodGet}, Handler: func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusBadRequest) }}) // Use HttpMethod enum
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
