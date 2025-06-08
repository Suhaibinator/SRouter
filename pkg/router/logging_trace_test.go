package router

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/router/internal/mocks" // Use centralized mocks
	"github.com/Suhaibinator/SRouter/pkg/scontext"              // Added import
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// Note: Removed unused imports "context" and "strings"

// --- Tests from trace_test.go ---

// TestTraceIDLogging tests that trace IDs are included in log entries when TraceIDBufferSize > 0
func TestTraceIDLogging(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)
	logger := zap.New(core)
	r := NewRouter(RouterConfig{Logger: logger, TraceIDBufferSize: 1000}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)
	r.RegisterRoute(RouteConfigBase{Path: "/test", Methods: []HttpMethod{MethodGet}, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })}) // Use HttpMethod enum
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

// TestTraceIDLoggingDisabled tests that trace IDs are not included in log entries when TraceIDBufferSize is 0
func TestTraceIDLoggingDisabled(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)
	logger := zap.New(core)
	r := NewRouter(RouterConfig{Logger: logger, TraceIDBufferSize: 0, EnableTraceLogging: true}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)
	r.RegisterRoute(RouteConfigBase{Path: "/test", Methods: []HttpMethod{MethodGet}, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })}) // Use HttpMethod enum
	req, err := http.NewRequest("GET", "/test", nil)
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}
	traceID := "test-trace-id"
	ctxWithTrace := scontext.WithTraceID[string, string](req.Context(), traceID) // Use scontext
	req = req.WithContext(ctxWithTrace)                                          // Apply context
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	// With TraceIDBufferSize = 0, the entire deferred logging block in ServeHTTP is skipped.
	// Therefore, we expect NO log entries from this specific logger setup.
	logEntries := logs.AllUntimed() // Use AllUntimed() for consistency
	if len(logEntries) != 0 {
		t.Errorf("Expected 0 log entries when TraceIDBufferSize is 0, but got %d", len(logEntries))
		// Log the unexpected entries for debugging
		for i, entry := range logEntries {
			t.Logf("Unexpected log entry %d: %v", i, entry)
		}
	}
	// The previous loop checking for the absence of 'trace_id' is no longer needed.
}

// TestHandleErrorWithTraceID tests that handleError includes trace IDs in log entries when TraceIDBufferSize > 0
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

// TestHandleErrorWithoutTraceID tests that handleError does not include trace IDs in log entries when TraceIDBufferSize is 0
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

// TestRecoveryMiddlewareWithTraceID tests that recoveryMiddleware includes trace IDs in log entries when TraceIDBufferSize > 0
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

// TestRecoveryMiddlewareWithoutTraceID tests that recoveryMiddleware does not include trace IDs in log entries when TraceIDBufferSize is 0
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
	core, logs := observer.New(zap.WarnLevel) // Observe WarnLevel logs
	logger := zap.New(core)
	// Enable tracing so the logging block runs. Use TraceIDBufferSize > 0.
	r := NewRouter(RouterConfig{Logger: logger, TraceIDBufferSize: 1}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)
	r.RegisterRoute(RouteConfigBase{
		Path:    "/slow",
		Methods: []HttpMethod{MethodGet}, // Use HttpMethod enum
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			time.Sleep(600 * time.Millisecond) // Longer than 500ms threshold
			w.WriteHeader(http.StatusOK)
		}),
	})
	req, _ := http.NewRequest("GET", "/slow", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Errorf("Expected status code %d, got %d", http.StatusOK, rr.Code)
	}

	// Check for the unified log message at Warn level
	logEntries := logs.FilterMessage("Request summary statistics").AllUntimed()
	if len(logEntries) == 0 {
		t.Fatalf("Expected 'Request summary statistics' log entry at WarnLevel, but none found")
	}

	found := false
	for _, log := range logEntries {
		if log.Level == zapcore.WarnLevel {
			found = true
			// Optionally check context fields like method and path
			methodFound := false
			pathFound := false
			for _, field := range log.Context {
				if field.Key == "method" && field.String == "GET" {
					methodFound = true
				}
				if field.Key == "path" && field.String == "/slow" {
					pathFound = true
				}
			}
			if !methodFound || !pathFound {
				t.Errorf("Expected method=GET and path=/slow in log context, fields: %v", log.Context)
			}
			break // Found the correct log entry
		}
	}
	if !found {
		t.Errorf("Expected 'Request summary statistics' log message at WarnLevel due to slow request")
	}
}

// TestErrorStatusLogging tests that error status codes are logged appropriately
func TestErrorStatusLogging(t *testing.T) {
	// Test server error (5xx) -> ERROR level
	coreErr, logsErr := observer.New(zap.ErrorLevel) // Observe ErrorLevel logs
	loggerErr := zap.New(coreErr)
	// Enable tracing so the logging block runs. Use TraceIDBufferSize > 0.
	rErr := NewRouter(RouterConfig{Logger: loggerErr, TraceIDBufferSize: 1}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)
	rErr.RegisterRoute(RouteConfigBase{Path: "/server-error", Methods: []HttpMethod{MethodGet}, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusInternalServerError) })}) // Use HttpMethod enum
	reqErr, _ := http.NewRequest("GET", "/server-error", nil)
	rrErr := httptest.NewRecorder()
	rErr.ServeHTTP(rrErr, reqErr)
	if rrErr.Code != http.StatusInternalServerError {
		t.Errorf("Expected status code %d, got %d", http.StatusInternalServerError, rrErr.Code)
	}

	// Check for the unified log message at Error level
	logEntriesErr := logsErr.FilterMessage("Request summary statistics").AllUntimed()
	if len(logEntriesErr) == 0 {
		t.Fatalf("Expected 'Request summary statistics' log entry at ErrorLevel, but none found")
	}
	foundErr := false
	for _, log := range logEntriesErr {
		if log.Level == zapcore.ErrorLevel {
			foundErr = true
			break
		}
	}
	if !foundErr {
		t.Errorf("Expected 'Request summary statistics' log message at ErrorLevel for 5xx status")
	}

	// Test client error (4xx) -> WARN level
	coreWarn, logsWarn := observer.New(zap.WarnLevel) // Observe WarnLevel logs
	loggerWarn := zap.New(coreWarn)
	// Enable tracing so the logging block runs. Use TraceIDBufferSize > 0.
	rWarn := NewRouter(RouterConfig{Logger: loggerWarn, TraceIDBufferSize: 1}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)
	rWarn.RegisterRoute(RouteConfigBase{Path: "/client-error", Methods: []HttpMethod{MethodGet}, Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusBadRequest) })}) // Use HttpMethod enum
	reqWarn, _ := http.NewRequest("GET", "/client-error", nil)
	rrWarn := httptest.NewRecorder()
	rWarn.ServeHTTP(rrWarn, reqWarn)
	if rrWarn.Code != http.StatusBadRequest {
		t.Errorf("Expected status code %d, got %d", http.StatusBadRequest, rrWarn.Code)
	}

	// Check for the unified log message at Warn level
	logEntriesWarn := logsWarn.FilterMessage("Request summary statistics").AllUntimed()
	if len(logEntriesWarn) == 0 {
		t.Fatalf("Expected 'Request summary statistics' log entry at WarnLevel, but none found")
	}
	foundWarn := false
	for _, log := range logEntriesWarn {
		if log.Level == zapcore.WarnLevel {
			foundWarn = true
			break
		}
	}
	if !foundWarn {
		t.Errorf("Expected 'Request summary statistics' log message at WarnLevel for 4xx status")
	}
}
