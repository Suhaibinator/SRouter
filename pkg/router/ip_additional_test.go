package router

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Suhaibinator/SRouter/pkg/scontext" // Added import
)

// TestDefaultIPConfig tests the DefaultIPConfig function
func TestDefaultIPConfig(t *testing.T) {
	config := DefaultIPConfig()
	if config == nil {
		t.Fatal("Expected non-nil config")
	}
	if config.Source != IPSourceXForwardedFor {
		t.Errorf("Expected Source to be %s, got %s", IPSourceXForwardedFor, config.Source)
	}
	if !config.TrustProxy {
		t.Error("Expected TrustProxy to be true")
	}
}

// TestClientIPMiddleware tests the ClientIPMiddleware function (now a variable)
func TestClientIPMiddleware(t *testing.T) {
	// Test with nil config (should use default)
	middleware := ClientIPMiddleware[uint64, any](nil) // Use the variable
	if middleware == nil {
		t.Fatal("Expected non-nil middleware")
	}

	// Create a test handler
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		ip, _ := scontext.GetClientIPFromRequest[uint64, any](r) // Use scontext function
		_, _ = w.Write([]byte(ip))
	})

	// Wrap the handler with the middleware
	wrappedHandler := middleware(handler)

	// Create a test request with X-Forwarded-For header
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Forwarded-For", "192.0.2.1")
	rec := httptest.NewRecorder()

	// Call the handler
	wrappedHandler.ServeHTTP(rec, req)

	// Check that the response body contains the IP
	if rec.Body.String() != "192.0.2.1" {
		t.Errorf("Expected body '192.0.2.1', got '%s'", rec.Body.String())
	}

	// Test with custom config
	config := &IPConfig{
		Source:     IPSourceXRealIP,
		TrustProxy: true,
	}
	middleware = ClientIPMiddleware[uint64, any](config) // Use the variable
	wrappedHandler = middleware(handler)

	// Create a test request with X-Real-IP header
	req = httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Real-IP", "192.0.2.2")
	rec = httptest.NewRecorder()

	// Call the handler
	wrappedHandler.ServeHTTP(rec, req)

	// Check that the response body contains the IP
	if rec.Body.String() != "192.0.2.2" {
		t.Errorf("Expected body '192.0.2.2', got '%s'", rec.Body.String())
	}

	// Test with custom header
	config = &IPConfig{
		Source:       IPSourceCustomHeader,
		CustomHeader: "X-Custom-IP",
		TrustProxy:   true,
	}
	middleware = ClientIPMiddleware[uint64, any](config) // Use the variable
	wrappedHandler = middleware(handler)

	// Create a test request with custom header
	req = httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Custom-IP", "192.0.2.3")
	rec = httptest.NewRecorder()

	// Call the handler
	wrappedHandler.ServeHTTP(rec, req)

	// Check that the response body contains the IP
	if rec.Body.String() != "192.0.2.3" {
		t.Errorf("Expected body '192.0.2.3', got '%s'", rec.Body.String())
	}

	// Test with RemoteAddr
	config = &IPConfig{
		Source:     IPSourceRemoteAddr,
		TrustProxy: true,
	}
	middleware = ClientIPMiddleware[uint64, any](config) // Use the variable
	wrappedHandler = middleware(handler)

	// Create a test request with RemoteAddr
	req = httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "192.0.2.4:1234"
	rec = httptest.NewRecorder()

	// Call the handler
	wrappedHandler.ServeHTTP(rec, req)

	// Check that the response body contains the IP
	if rec.Body.String() != "192.0.2.4" {
		t.Errorf("Expected body '192.0.2.4', got '%s'", rec.Body.String())
	}

	// Test with unknown source (should fall back to X-Forwarded-For)
	config = &IPConfig{
		Source:     IPSourceType("unknown"),
		TrustProxy: true,
	}
	middleware = ClientIPMiddleware[uint64, any](config) // Use the variable
	wrappedHandler = middleware(handler)

	// Create a test request with X-Forwarded-For header
	req = httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Forwarded-For", "192.0.2.5")
	rec = httptest.NewRecorder()

	// Call the handler
	wrappedHandler.ServeHTTP(rec, req)

	// Check that the response body contains the IP
	if rec.Body.String() != "192.0.2.5" {
		t.Errorf("Expected body '192.0.2.5', got '%s'", rec.Body.String())
	}

	// Test with TrustProxy=false (should fall back to RemoteAddr)
	config = &IPConfig{
		Source:     IPSourceXForwardedFor,
		TrustProxy: false,
	}
	middleware = ClientIPMiddleware[uint64, any](config) // Use the variable
	wrappedHandler = middleware(handler)

	// Create a test request with X-Forwarded-For header and RemoteAddr
	req = httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Forwarded-For", "192.0.2.6")
	req.RemoteAddr = "192.0.2.7:1234"
	rec = httptest.NewRecorder()

	// Call the handler
	wrappedHandler.ServeHTTP(rec, req)

	// Check that the response body contains the RemoteAddr IP
	if rec.Body.String() != "192.0.2.7" {
		t.Errorf("Expected body '192.0.2.7', got '%s'", rec.Body.String())
	}
}

// TestCleanIP tests the cleanIP function
func TestCleanIP(t *testing.T) {
	// Test IPv4 with port
	ip := cleanIP("192.0.2.1:1234")
	if ip != "192.0.2.1" {
		t.Errorf("Expected '192.0.2.1', got '%s'", ip)
	}

	// Test IPv4 without port
	ip = cleanIP("192.0.2.1")
	if ip != "192.0.2.1" {
		t.Errorf("Expected '192.0.2.1', got '%s'", ip)
	}

	// Test IPv6 with port
	ip = cleanIP("[2001:db8::1]:1234")
	if ip != "[2001:db8::1]" {
		t.Errorf("Expected '[2001:db8::1]', got '%s'", ip)
	}

	// Test IPv6 without port (no brackets)
	ip = cleanIP("2001:db8::1")
	if ip != "2001:db8::1" {
		t.Errorf("Expected '2001:db8::1', got '%s'", ip)
	}

	// Test IPv6 without port (with brackets) - net.SplitHostPort might not handle this if no port, so cleanIP's fallback kicks in
	ip = cleanIP("[2001:db8::1]")
	if ip != "[2001:db8::1]" {
		t.Errorf("Expected '[2001:db8::1]', got '%s'", ip)
	}

	// Test empty string
	ip = cleanIP("")
	if ip != "" {
		t.Errorf("Expected '', got '%s'", ip)
	}

	// Test malformed host part
	ip = cleanIP("invalid-address:8080")
	if ip != "invalid-address:8080" {
		t.Errorf("Expected 'invalid-address:8080', got '%s'", ip)
	}

	// Test IPv6 with zone identifier and port
	ip = cleanIP("[fe80::1%eth0]:80")
	if ip != "fe80::1%eth0" {
		t.Errorf("Expected 'fe80::1%%eth0', got '%s'", ip)
	}

	// Test IPv6 with zone identifier without port
	ip = cleanIP("fe80::1%eth0")
	if ip != "fe80::1%eth0" {
		t.Errorf("Expected 'fe80::1%%eth0', got '%s'", ip)
	}
}
