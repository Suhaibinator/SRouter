package router

import (
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
// The default uses the rightmost X-Forwarded-For entry (the value appended by
// the proxy nearest this server) rather than the client-controlled leftmost
// entry. If the service is exposed directly to the internet (no trusted proxy),
// set Source to IPSourceRemoteAddr or TrustProxy to false so client-supplied
// headers are ignored entirely.
func DefaultIPConfig() *IPConfig {
	return &IPConfig{
		Source:     IPSourceXForwardedFor, // Default to checking X-Forwarded-For
		TrustProxy: true,                  // Trust proxy headers by default
	}
}

// ClientIPMiddleware creates a middleware that extracts the client IP from the request
// and adds it to the SRouterContext. SRouter already initializes this information;
// configure RouterConfig.IPConfig instead of adding this middleware to a router.
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

			// When an SRouterContext already exists, WithClientIP mutates it in
			// place and returns the same context, so cloning the request is
			// unnecessary.
			_, hadSRouterCtx := scontext.GetSRouterContext[T, U](r.Context())

			// Add the client IP to the SRouterContext
			ctx := scontext.WithClientIP[T, U](r.Context(), clientIP)

			// Call the next handler with the updated context
			if !hadSRouterCtx {
				r = r.WithContext(ctx)
			}
			next.ServeHTTP(w, r)
		})
	}
}

// extractClientIP selects the configured address. The context setter normalizes
// it once when storing client information.
func extractClientIP(r *http.Request, config *IPConfig) string {
	var ip string
	if config == nil {
		return r.RemoteAddr
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

	return ip
}

// extractIPFromXForwardedFor extracts the client IP from the X-Forwarded-For header.
// The header contains a comma-separated list of IPs. Earlier (leftmost) entries are
// supplied by the client and are trivially spoofable; the rightmost entry was appended
// by the proxy closest to this server and is the only value the deployment's own
// infrastructure vouches for. Using it prevents clients from rotating fake IPs to
// bypass IP-based rate limiting.
func extractIPFromXForwardedFor(r *http.Request) string {
	xff := r.Header.Get("X-Forwarded-For")
	if xff == "" {
		return ""
	}

	// Use the rightmost (most recently appended, least spoofable) entry.
	// Scan from the end without splitting so no intermediate slice is allocated.
	for {
		comma := strings.LastIndexByte(xff, ',')
		if ip := strings.TrimSpace(xff[comma+1:]); ip != "" {
			return ip
		}
		if comma < 0 {
			return ""
		}
		xff = xff[:comma]
	}
}
