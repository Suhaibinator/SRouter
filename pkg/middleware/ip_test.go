package middleware

import (
	"context"
	"net/http/httptest"
	"testing"

	"go.uber.org/zap"
)

// TestExtractIPFromXForwardedFor tests the extractIPFromXForwardedFor function
func TestExtractIPFromXForwardedFor(t *testing.T) {
	// Test with valid X-Forwarded-For header containing multiple IPs
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.1, 198.51.100.1")
	ip := extractIPFromXForwardedFor(req)
	if ip != "203.0.113.1" {
		t.Errorf("Expected IP '203.0.113.1', got '%s'", ip)
	}

	// Test with valid X-Forwarded-For header containing a single IP
	req = httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.2")
	ip = extractIPFromXForwardedFor(req)
	if ip != "203.0.113.2" {
		t.Errorf("Expected IP '203.0.113.2', got '%s'", ip)
	}

	// Test with empty X-Forwarded-For header
	req = httptest.NewRequest("GET", "/test", nil)
	ip = extractIPFromXForwardedFor(req)
	if ip != "" {
		t.Errorf("Expected empty IP, got '%s'", ip)
	}

	// Test with X-Forwarded-For header containing empty value
	req = httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Forwarded-For", "")
	ip = extractIPFromXForwardedFor(req)
	if ip != "" {
		t.Errorf("Expected empty IP, got '%s'", ip)
	}
}

// TestExtractIP tests the extractIP function
func TestExtractIP(t *testing.T) {
	// Create a nil logger for testing
	var logger *zap.Logger = nil

	// Test with IP in context using legacy method
	req := httptest.NewRequest("GET", "/test", nil)
	ctx := req.Context()
	ctx = context.WithValue(ctx, ClientIPKey, "192.168.1.1")
	req = req.WithContext(ctx)
	ip := extractIP(req, logger)
	if ip != "192.168.1.1" {
		t.Errorf("Expected IP '192.168.1.1', got '%s'", ip)
	}

	// Test with IP in context using SRouterContext wrapper
	req = httptest.NewRequest("GET", "/test", nil)
	ctx = WithClientIP[string, any](req.Context(), "192.168.1.2")
	req = req.WithContext(ctx)
	ip = extractIP(req, logger)
	if ip != "192.168.1.2" {
		t.Errorf("Expected IP '192.168.1.2', got '%s'", ip)
	}

	// Test with X-Forwarded-For header
	req = httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Forwarded-For", "203.0.113.1, 198.51.100.1")
	ip = extractIP(req, logger)
	if ip != "203.0.113.1" {
		t.Errorf("Expected IP '203.0.113.1', got '%s'", ip)
	}

	// Test with X-Real-IP header
	req = httptest.NewRequest("GET", "/test", nil)
	req.Header.Set("X-Real-IP", "203.0.113.2")
	ip = extractIP(req, logger)
	if ip != "203.0.113.2" {
		t.Errorf("Expected IP '203.0.113.2', got '%s'", ip)
	}

	// Test with RemoteAddr
	req = httptest.NewRequest("GET", "/test", nil)
	req.RemoteAddr = "203.0.113.3:1234"
	ip = extractIP(req, logger)
	if ip != "203.0.113.3:1234" {
		t.Errorf("Expected IP '203.0.113.3:1234', got '%s'", ip)
	}
}
