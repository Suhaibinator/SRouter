// Package middleware provides a collection of HTTP middleware components for the SRouter framework.
package middleware

import (
	"context"
	"net/http"
	"strings"
)

// IPSourceType defines the source for client IP addresses
type IPSourceType string

const (
	// IPSourceRemoteAddr uses the request's RemoteAddr field
	IPSourceRemoteAddr IPSourceType = "remote_addr"

	// IPSourceXForwardedFor uses the X-Forwarded-For header
	IPSourceXForwardedFor IPSourceType = "x_forwarded_for"

	// IPSourceXRealIP uses the X-Real-IP header
	IPSourceXRealIP IPSourceType = "x_real_ip"

	// IPSourceCustomHeader uses a custom header specified in the configuration
	IPSourceCustomHeader IPSourceType = "custom_header"
)

// IPConfig defines configuration for IP extraction
type IPConfig struct {
	// Source specifies where to extract the client IP from
	Source IPSourceType

	// CustomHeader is the name of the custom header to use when Source is IPSourceCustomHeader
	CustomHeader string

	// TrustProxy determines whether to trust proxy headers like X-Forwarded-For
	// If false, RemoteAddr will be used as a fallback for all sources
	TrustProxy bool
}

// DefaultIPConfig returns the default IP configuration
func DefaultIPConfig() *IPConfig {
	return &IPConfig{
		Source:     IPSourceXForwardedFor,
		TrustProxy: true,
	}
}

// contextKey is a type for context keys
type contextKey string

// ClientIPKey is the key used to store the client IP in the request context
const ClientIPKey contextKey = "client_ip"

// ClientIP extracts the client IP from the request context
func ClientIP(r *http.Request) string {
	if ip, ok := r.Context().Value(ClientIPKey).(string); ok {
		return ip
	}
	return ""
}

// ClientIPMiddleware creates a middleware that extracts the client IP from the request
// and adds it to the request context
func ClientIPMiddleware(config *IPConfig) func(http.Handler) http.Handler {
	if config == nil {
		config = DefaultIPConfig()
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract the client IP based on the configured source
			clientIP := extractClientIP(r, config)

			// Add the client IP to the request context
			ctx := context.WithValue(r.Context(), ClientIPKey, clientIP)

			// Call the next handler with the updated request
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// extractClientIP extracts the client IP from the request based on the configuration
func extractClientIP(r *http.Request, config *IPConfig) string {
	var ip string

	switch config.Source {
	case IPSourceXForwardedFor:
		ip = extractIPFromXForwardedFor(r)
	case IPSourceXRealIP:
		ip = r.Header.Get("X-Real-IP")
	case IPSourceCustomHeader:
		ip = r.Header.Get(config.CustomHeader)
	case IPSourceRemoteAddr:
		ip = r.RemoteAddr
	default:
		ip = extractIPFromXForwardedFor(r)
	}

	// If we don't trust proxy headers or couldn't extract an IP, fall back to RemoteAddr
	if !config.TrustProxy || ip == "" {
		ip = r.RemoteAddr
	}

	// Clean up the IP address (remove port if present)
	return cleanIP(ip)
}

// extractIPFromXForwardedFor extracts the client IP from the X-Forwarded-For header
// The X-Forwarded-For header contains a comma-separated list of IPs, with the leftmost being the original client
func extractIPFromXForwardedFor(r *http.Request) string {
	xff := r.Header.Get("X-Forwarded-For")
	if xff == "" {
		return ""
	}

	// The leftmost IP is the original client
	ips := strings.Split(xff, ",")
	if len(ips) > 0 {
		return strings.TrimSpace(ips[0])
	}

	return ""
}

// cleanIP removes the port from an IP address if present
func cleanIP(ip string) string {
	// IPv6 addresses with ports are formatted as [IPv6]:port
	if strings.HasPrefix(ip, "[") {
		end := strings.LastIndex(ip, "]")
		if end > 0 {
			if end+1 < len(ip) && ip[end+1] == ':' {
				return ip[:end+1]
			}
			return ip
		}
	}

	// Check if this is an IPv6 address without brackets (contains multiple colons)
	if strings.Count(ip, ":") > 1 {
		// This is likely an IPv6 address without port, return as is
		return ip
	}

	// IPv4 addresses with ports are formatted as IPv4:port
	end := strings.LastIndex(ip, ":")
	if end > 0 {
		return ip[:end]
	}

	return ip
}
