package router

import (
	// Import net/http
	"net/http/httptest"
	"testing"

	"github.com/Suhaibinator/SRouter/pkg/scontext" // Import scontext
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

// TestExtractClientIP tests the extractClientIP function with various configurations
func TestExtractClientIP(t *testing.T) {
	// Test cases
	tests := []struct {
		name         string
		config       *IPConfig
		headers      map[string]string
		remoteAddr   string
		expectedIP   string
		expectLog    bool   // Whether a warning log is expected (for fallback)
		expectedUser string // For context test
	}{
		{
			name:       "Default Config (X-Forwarded-For)",
			config:     DefaultIPConfig(),
			headers:    map[string]string{"X-Forwarded-For": "203.0.113.1, 198.51.100.1"},
			remoteAddr: "192.0.2.1:1234",
			expectedIP: "203.0.113.1",
		},
		{
			name:       "X-Real-IP Config",
			config:     &IPConfig{Source: IPSourceXRealIP, TrustProxy: true},
			headers:    map[string]string{"X-Real-IP": "203.0.113.2"},
			remoteAddr: "192.0.2.1:1234",
			expectedIP: "203.0.113.2",
		},
		{
			name:       "Custom Header Config",
			config:     &IPConfig{Source: IPSourceCustomHeader, CustomHeader: "X-Client-IP", TrustProxy: true},
			headers:    map[string]string{"X-Client-IP": "203.0.113.3"},
			remoteAddr: "192.0.2.1:1234",
			expectedIP: "203.0.113.3",
		},
		{
			name:       "RemoteAddr Config",
			config:     &IPConfig{Source: IPSourceRemoteAddr, TrustProxy: true}, // TrustProxy doesn't matter here
			headers:    map[string]string{"X-Forwarded-For": "203.0.113.1"},
			remoteAddr: "192.0.2.1:1234",
			expectedIP: "192.0.2.1", // Expects cleaned RemoteAddr
		},
		{
			name:       "Fallback to RemoteAddr (No Header)",
			config:     DefaultIPConfig(),
			headers:    map[string]string{},
			remoteAddr: "192.0.2.2:5678",
			expectedIP: "192.0.2.2",
		},
		{
			name:       "Fallback to RemoteAddr (TrustProxy=false)",
			config:     &IPConfig{Source: IPSourceXForwardedFor, TrustProxy: false},
			headers:    map[string]string{"X-Forwarded-For": "203.0.113.1"},
			remoteAddr: "192.0.2.3:9012",
			expectedIP: "192.0.2.3",
		},
		{
			name:       "IPv6 X-Forwarded-For",
			config:     DefaultIPConfig(),
			headers:    map[string]string{"X-Forwarded-For": "[2001:db8::1]:54321, 198.51.100.1"},
			remoteAddr: "192.0.2.1:1234",
			expectedIP: "[2001:db8::1]", // Expects cleaned IPv6
		},
		{
			name:       "IPv6 RemoteAddr",
			config:     &IPConfig{Source: IPSourceRemoteAddr, TrustProxy: true},
			headers:    map[string]string{},
			remoteAddr: "[::1]:8080",
			expectedIP: "[::1]", // Expects cleaned IPv6 loopback
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/test", nil)
			for k, v := range tc.headers {
				req.Header.Set(k, v)
			}
			req.RemoteAddr = tc.remoteAddr

			ip := extractClientIP(req, tc.config)
			if ip != tc.expectedIP {
				t.Errorf("Expected IP %q, got %q", tc.expectedIP, ip)
			}
		})
	}
}

// TestClientIPRetrieval tests retrieving the client IP from context using scontext
func TestClientIPRetrieval(t *testing.T) {
	// Test getting IP when it's set in the context
	req := httptest.NewRequest("GET", "/test", nil)
	expectedIP := "10.0.0.1"
	ctx := scontext.WithClientIP[string, any](req.Context(), expectedIP)
	req = req.WithContext(ctx)

	ip, ok := scontext.GetClientIPFromRequest[string, any](req)
	if !ok {
		t.Errorf("Expected to find IP in context, but ok was false")
	}
	if ip != expectedIP {
		t.Errorf("Expected IP %q from context, got %q", expectedIP, ip)
	}

	// Test getting IP when it's not set
	req = httptest.NewRequest("GET", "/test", nil) // Fresh request without IP in context
	ip, ok = scontext.GetClientIPFromRequest[string, any](req)
	if ok {
		t.Errorf("Expected not to find IP in context, but ok was true")
	}
	if ip != "" {
		t.Errorf("Expected empty IP when not set in context, got %q", ip)
	}
}
