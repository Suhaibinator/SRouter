package scontext

import (
	"net"
	"strings"
)

// cleanClientIP removes a port from a valid IP socket address while preserving
// the established representation of bracketed and zone-qualified IPv6 values.
func cleanClientIP(ip string) string {
	if !mayHavePort(ip) {
		return ip
	}
	host, _, err := net.SplitHostPort(ip)
	if err == nil {
		// Zone-qualified IPv6 addresses are stored without brackets.
		if strings.Contains(host, "%") {
			return host
		}
		// Strip a port only when the host is an IP. Hostnames and malformed
		// inputs remain untouched.
		if net.ParseIP(host) != nil {
			if ip[0] == '[' {
				// SplitHostPort returned ip[1:end] for "[host]:port"; keep the
				// brackets by slicing rather than concatenating.
				return ip[:len(host)+2]
			}
			return host
		}
		return ip
	}

	// Bracketed IPv6 without a port and all other non-socket inputs preserve
	// their original representation.
	return ip
}

// mayHavePort reports false for inputs that net.SplitHostPort always rejects,
// such as bare IPv4, bare IPv6, and bracketed IPv6 without a port. Those are
// returned unchanged either way; skipping SplitHostPort avoids allocating its
// error for the portless values proxy headers usually carry.
func mayHavePort(ip string) bool {
	if ip == "" {
		return false
	}
	if ip[0] == '[' {
		return ip[len(ip)-1] != ']'
	}
	// Without brackets, a socket address has exactly one colon.
	first := strings.IndexByte(ip, ':')
	return first >= 0 && first == strings.LastIndexByte(ip, ':')
}
