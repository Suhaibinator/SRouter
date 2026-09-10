// Package clientip contains address normalization shared by routing and logging.
package clientip

import (
	"net"
	"strings"
)

// Clean removes the port from an IP address if present
func Clean(ip string) string {
	host, _, err := net.SplitHostPort(ip)
	if err == nil {
		// If the host contains a zone identifier, return without brackets
		if strings.Contains(host, "%") {
			return host
		}
		// Only return the host portion if it parses as a valid IP
		if net.ParseIP(host) != nil {
			// Preserve brackets if the original string contained them
			if strings.HasPrefix(ip, "[") && strings.Contains(ip, "]") {
				return "[" + host + "]"
			}
			return host
		}
		// If host isn't a valid IP, fall back to the original string
		return ip
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
