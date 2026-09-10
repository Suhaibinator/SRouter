package scontext

import (
	"net"
	"strings"
)

// cleanClientIP removes a port from a valid IP socket address while preserving
// the established representation of bracketed and zone-qualified IPv6 values.
func cleanClientIP(ip string) string {
	host, _, err := net.SplitHostPort(ip)
	if err == nil {
		// Zone-qualified IPv6 addresses are stored without brackets.
		if strings.Contains(host, "%") {
			return host
		}
		// Strip a port only when the host is an IP. Hostnames and malformed
		// inputs remain untouched.
		if net.ParseIP(host) != nil {
			if strings.HasPrefix(ip, "[") && strings.Contains(ip, "]") {
				return "[" + host + "]"
			}
			return host
		}
		return ip
	}

	// Bracketed IPv6 without a port and all other non-socket inputs preserve
	// their original representation.
	return ip
}
