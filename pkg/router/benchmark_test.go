package router

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/codec"
	"github.com/Suhaibinator/SRouter/pkg/common" // Re-add common import
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest"
)

// --- Helpers ---

func nopAuthFunc(_ context.Context, _ string) (string, bool) {
	return "user", true // Always authenticate as "user"
}

func userIDFromString(user string) string {
	return user
}

func simpleHandler(w http.ResponseWriter, r *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("OK"))
}

type ctxKey string

const testCtxKey ctxKey = "testKey"

// mockAuthMiddleware checks for a specific header
func mockAuthMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("X-Mock-Auth") != "valid" {
			http.Error(w, "Unauthorized", http.StatusUnauthorized)
			return
		}
		next.ServeHTTP(w, r)
	})
}

// contextModifierMiddleware adds a value to the context
func contextModifierMiddleware(key any, value any) common.Middleware { // Qualify Middleware
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			ctx := context.WithValue(r.Context(), key, value)
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// simpleLoggingMiddleware is a basic logging middleware example
func simpleLoggingMiddleware(logger *zap.Logger) common.Middleware { // Qualify Middleware
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			// Use a response writer wrapper if status code is needed after handler execution
			next.ServeHTTP(w, r)
			logger.Debug("Request handled", zap.String("method", r.Method), zap.String("path", r.URL.Path), zap.Duration("duration", time.Since(start))) // Use zap field constructors
		})
	}
}

// --- Benchmarks ---

// BenchmarkSimpleRoute measures the baseline overhead for a simple GET request.
func BenchmarkSimpleRoute(b *testing.B) {
	logger := zaptest.NewLogger(b)
	r := NewRouter(RouterConfig{Logger: logger}, nopAuthFunc, userIDFromString)
	r.RegisterRoute(RouteConfigBase{
		Path:    "/hello",
		Methods: []HttpMethod{MethodGet}, // Use HttpMethod enum
		Handler: simpleHandler,
	})

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		// Create request and recorder inside the loop for parallel safety
		for pb.Next() {
			req, _ := http.NewRequest(http.MethodGet, "/hello", nil)
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)
		}
	})
}

// BenchmarkRouteWithParams measures the overhead of path parameter extraction.
func BenchmarkRouteWithParams(b *testing.B) {
	logger := zaptest.NewLogger(b)
	r := NewRouter(RouterConfig{Logger: logger}, nopAuthFunc, userIDFromString)
	r.RegisterRoute(RouteConfigBase{
		Path:    "/users/:id/posts/:postId", // Using existing path with two params
		Methods: []HttpMethod{MethodGet},    // Use HttpMethod enum
		Handler: func(w http.ResponseWriter, r *http.Request) {
			id := GetParam(r, "id")
			postId := GetParam(r, "postId")
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte(fmt.Sprintf("User ID: %s, Post ID: %s", id, postId)))
		},
	})

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		// Create request and recorder inside the loop for parallel safety
		for pb.Next() {
			req, _ := http.NewRequest(http.MethodGet, "/users/123/posts/456", nil)
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)
		}
	})
}

// BenchmarkMiddlewareStack measures the cumulative overhead of a middleware chain.
func BenchmarkMiddlewareStack(b *testing.B) {
	logger := zaptest.NewLogger(b)
	r := NewRouter(RouterConfig{
		Logger: logger,
		Middlewares: []common.Middleware{ // Qualify Middleware
			simpleLoggingMiddleware(logger),
		},
	}, nopAuthFunc, userIDFromString)

	// Define route-specific middleware stack
	routeMiddleware := []common.Middleware{ // Qualify Middleware
		mockAuthMiddleware, // Checks X-Mock-Auth header
		contextModifierMiddleware(testCtxKey, "testValue"),
		// Add a mock rate limiter if needed, e.g., just a time.Sleep(1 * time.Microsecond)
	}

	r.RegisterRoute(RouteConfigBase{
		Path:        "/secure",
		Methods:     []HttpMethod{MethodGet}, // Use HttpMethod enum
		Middlewares: routeMiddleware,         // Already qualified above
		Handler: func(w http.ResponseWriter, r *http.Request) {
			// Check if context value was set
			if val := r.Context().Value(testCtxKey); val != "testValue" {
				http.Error(w, "Context value not found", http.StatusInternalServerError)
				return
			}
			simpleHandler(w, r)
		},
	})

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		// Create request and recorder inside the loop for parallel safety
		for pb.Next() {
			req, _ := http.NewRequest(http.MethodGet, "/secure", nil)
			req.Header.Set("X-Mock-Auth", "valid") // Satisfy mock auth middleware
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)
		}
	})
}

// --- Generic Route Benchmarks ---

type GenericRequestData struct {
	Name  string `json:"name"`
	Value int    `json:"value"`
}

type GenericResponseData struct {
	Message string `json:"message"`
	ID      string `json:"id"` // Assuming ID comes from path or context
}

// BenchmarkGenericRouteBody measures overhead of generic route with JSON body decoding.
func BenchmarkGenericRouteBody(b *testing.B) {
	logger := zaptest.NewLogger(b)
	r := NewRouter(RouterConfig{Logger: logger}, nopAuthFunc, userIDFromString)
	jsonCodec := codec.NewJSONCodec[GenericRequestData, GenericResponseData]() // Add type arguments

	// Remove err assignment, use RouteConfig, add missing args, use correct SourceType
	// Use RouteConfig directly, not embedding RouteConfigBase. Rename GenericHandler to Handler.
	RegisterGenericRoute(r, RouteConfig[GenericRequestData, GenericResponseData]{
		Path:       "/generic/body",          // Direct field
		Methods:    []HttpMethod{MethodPost}, // Use HttpMethod enum
		Codec:      jsonCodec,
		SourceType: Body, // Use correct constant
		Handler: func(r *http.Request, req GenericRequestData) (GenericResponseData, error) { // Correct signature
			// Minimal work: access decoded data and return response struct
			return GenericResponseData{ // Return value, not pointer
				Message: fmt.Sprintf("Received: %s, %d", req.Name, req.Value),
				ID:      "some-id",
			}, nil
		},
	}, time.Duration(0), int64(0), nil) // Add missing arguments
	// require.NoError(b, err) // Remove error check

	// Prepare request body outside the parallel loop if possible, ensure it's thread-safe to read
	requestBodyBytes, _ := json.Marshal(GenericRequestData{Name: "Test", Value: 123})

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			// Create a new reader for the body in each iteration
			bodyReader := bytes.NewReader(requestBodyBytes)
			req, _ := http.NewRequest(http.MethodPost, "/generic/body", bodyReader)
			req.Header.Set("Content-Type", "application/json")
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)
		}
	})
}

type GenericPathParamData struct {
	Data string `param:"data"` // Tag matches the path parameter name
}

// BenchmarkGenericRoutePathParam measures overhead of generic route with Base64 path param decoding.
func BenchmarkGenericRoutePathParam(b *testing.B) {
	logger := zaptest.NewLogger(b)
	r := NewRouter(RouterConfig{Logger: logger}, nopAuthFunc, userIDFromString)
	// Assuming a simple codec that can decode struct fields based on tags (like schema or a custom one)
	// For this benchmark, we'll simulate the decoding within the handler for simplicity,
	// but ideally, the codec handles this based on SourceType.
	// Let's use a NopCodec and handle decoding manually in the handler for now.

	// Remove err assignment, use RouteConfig, add missing args, use correct SourceType
	// Use RouteConfig directly, not embedding RouteConfigBase. Rename GenericHandler to Handler.
	RegisterGenericRoute(r, RouteConfig[GenericPathParamData, GenericResponseData]{
		Path:    "/generic/param/:data",  // Direct field
		Methods: []HttpMethod{MethodGet}, // Use HttpMethod enum
		// Codec:      codec.NewNopCodec(), // Or a codec designed for path params
		SourceType: Base64PathParameter, // Use correct constant
		Handler: func(r *http.Request, req GenericPathParamData) (GenericResponseData, error) { // Correct signature
			// Simulate decoding that the codec would do based on SourceType
			// In a real scenario, the framework+codec handles this binding before the handler.
			// Here, we manually extract, decode, and check.
			params := GetParams(r) // Use GetParams helper
			encodedData := ""
			for _, p := range params {
				if p.Key == "data" {
					encodedData = p.Value
					break
				}
			}
			// params := ctx.Value(httprouter.ParamsKey).(httprouter.Params) // Old way
			// encodedData := params.ByName("data") // Old way

			if encodedData == "" {
				// This shouldn't happen if the route matched, but handle defensively
				return GenericResponseData{}, NewHTTPError(http.StatusBadRequest, "Missing path parameter 'data'")
			}

			decodedBytes, err := base64.URLEncoding.DecodeString(encodedData)
			if err != nil {
				return GenericResponseData{}, NewHTTPError(http.StatusBadRequest, "Invalid base64 data") // Return empty struct on error
			}
			// Assume the decoded data is what should have been in req.Data
			// We don't actually populate req.Data here as the framework would.
			// Just accessing the decoded data is enough for benchmark overhead.
			_ = string(decodedBytes) // Use the decoded data

			return GenericResponseData{ // Return value, not pointer
				Message: "Processed param",
				ID:      "param-id",
			}, nil
		},
		// Need to explicitly tell the router how to bind path params to the struct
		// if the codec doesn't handle it automatically. This might involve custom logic
		// or a specific codec implementation. For benchmark, manual extraction is okay.
	}, time.Duration(0), int64(0), nil) // Add missing arguments
	// require.NoError(b, err) // Remove error check

	// Prepare encoded path parameter value
	originalData := "some-data-to-encode"
	encodedData := base64.URLEncoding.EncodeToString([]byte(originalData))
	requestPath := fmt.Sprintf("/generic/param/%s", encodedData)

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req, _ := http.NewRequest(http.MethodGet, requestPath, nil)
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)
		}
	})
}

// --- Existing Benchmarks (Potentially Keep or Remove) ---

// BenchmarkRouterWithTimeout benchmarks a router with a timeout
// Keeping this as it tests a specific feature. Add ReportAllocs/RunParallel.
func BenchmarkRouterWithTimeout(b *testing.B) {
	logger := zaptest.NewLogger(b)
	r := NewRouter(RouterConfig{
		Logger:        logger,
		GlobalTimeout: 100 * time.Millisecond, // Shorter timeout for benchmark relevance
	}, nopAuthFunc, userIDFromString)

	r.RegisterRoute(RouteConfigBase{
		Path:    "/timeout",
		Methods: []HttpMethod{MethodGet}, // Use HttpMethod enum
		Handler: simpleHandler,
	})

	b.ReportAllocs()
	b.ResetTimer()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req, _ := http.NewRequest(http.MethodGet, "/timeout", nil)
			rr := httptest.NewRecorder()
			r.ServeHTTP(rr, req)
		}
	})
}

// BenchmarkMemoryUsage benchmarks the memory usage of the router after adding many routes.
// This is different from per-request allocs. Keep for now.
func BenchmarkMemoryUsage(b *testing.B) {
	// Create a logger
	logger := zaptest.NewLogger(b) // Use zaptest logger

	// Create a router with string as both the user ID and user type
	r := NewRouter(RouterConfig{
		Logger: logger,
	}, nopAuthFunc, userIDFromString) // Use nop funcs

	// Register many routes
	routeCount := 1000
	for i := 0; i < routeCount; i++ {
		path := fmt.Sprintf("/route%d", i)
		r.RegisterRoute(RouteConfigBase{
			Path:    path,
			Methods: []HttpMethod{MethodGet}, // Use HttpMethod enum
			Handler: simpleHandler,           // Use helper
		})
	}

	// Measure memory *after* setup
	runtime.GC() // Run GC first to get a cleaner measurement
	var mStart runtime.MemStats
	runtime.ReadMemStats(&mStart)

	// The original benchmark ran ServeHTTP b.N times, which isn't the goal here.
	// We want memory usage *of the router structure itself*.
	// Let's just report the memory after setup.
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		// We need *something* in the loop for the benchmark to run.
		// Accessing a route is minimal overhead compared to setup.
		req, _ := http.NewRequest(http.MethodGet, "/route0", nil)
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
	}
	b.StopTimer() // Stop timer before final memory measurement

	runtime.GC() // Run GC again
	var mEnd runtime.MemStats
	runtime.ReadMemStats(&mEnd)

	// Report the difference or the final state
	// Using Alloc is generally preferred for heap size
	b.ReportMetric(float64(mEnd.Alloc-mStart.Alloc), "alloc_diff_bytes")
	b.ReportMetric(float64(mEnd.Alloc), "final_alloc_bytes")
	b.ReportMetric(float64(mEnd.Sys), "final_sys_bytes")
	b.ReportMetric(float64(mEnd.NumGC-mStart.NumGC), "gc_runs")

	// Optional: Log detailed stats if needed
	// b.Logf("Final Alloc = %v MiB", bToMb(mEnd.Alloc))
	// b.Logf("Final TotalAlloc = %v MiB", bToMb(mEnd.TotalAlloc))
	// b.Logf("Final Sys = %v MiB", bToMb(mEnd.Sys))
	// b.Logf("GC Runs = %v", mEnd.NumGC - mStart.NumGC)
}
