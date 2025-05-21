package router

import (
	"net"
	"net/http"
	"strings"

	"github.com/Suhaibinator/SRouter/pkg/scontext" // Updated import
)

// IPSourceType defines the source for client IP addresses.
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

// IPConfig defines configuration for IP extraction.
type IPConfig struct {
	// Source specifies where to extract the client IP from.
	Source IPSourceType

	// CustomHeader is the name of the custom header to use when Source is IPSourceCustomHeader
	CustomHeader string

	// TrustProxy determines whether to trust proxy headers like X-Forwarded-For
	// If false, RemoteAddr will be used as a fallback for all sources
	TrustProxy bool
}

// DefaultIPConfig returns the default IP configuration.
func DefaultIPConfig() *IPConfig {
	return &IPConfig{
		Source:     IPSourceXForwardedFor, // Default to checking X-Forwarded-For
		TrustProxy: false,                 // Trust proxy headers by default
	}
}

// ClientIPMiddleware creates a middleware that extracts the client IP from the request
// and adds it to the SRouterContext.
// T is the User ID type (comparable), U is the User object type (any).
// It stores the IP address in the SRouterContext.
func ClientIPMiddleware[T comparable, U any](config *IPConfig) func(http.Handler) http.Handler {
	// Use default config if none provided
	if config == nil {
		config = DefaultIPConfig()
	}

	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			// Extract the client IP based on the configuration
			clientIP := extractClientIP(r, config)

			// Add the client IP to the SRouterContext
			ctx := scontext.WithClientIP[T, U](r.Context(), clientIP) // Use scontext

			// Call the next handler with the updated context
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}

// extractClientIP extracts the client IP address from the request based on the IPConfig.
func extractClientIP(r *http.Request, config *IPConfig) string {
	var ip string
	if config == nil {
		return cleanIP(r.RemoteAddr)
	}
	// Determine IP based on configured source
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
	host, _, err := net.SplitHostPort(ip)
	if err == nil {
		return host
	}
	// If SplitHostPort fails, it might be an IP without a port or an invalid format
	// For IPv6 without port but with brackets, e.g. "[::1]"
	if strings.HasPrefix(ip, "[") && strings.HasSuffix(ip, "]") {
		return ip
	}
	// For IPs without port or other cases, return the original string if SplitHostPort failed
	// This maintains previous behavior for IPs that don't have a port.
	return ip
}
