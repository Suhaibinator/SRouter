package common

import "time"

// AuthTokenSource defines where to extract authentication tokens from.
type AuthTokenSource int

const (
	// AuthTokenSourceHeader reads the token from a request header.
	AuthTokenSourceHeader AuthTokenSource = iota
	// AuthTokenSourceCookie reads the token from a request cookie.
	AuthTokenSourceCookie
)

// AuthTokenConfig defines how to extract authentication tokens from requests.
type AuthTokenConfig struct {
	// Source determines where to look for the token.
	Source AuthTokenSource

	// HeaderName is used when Source is AuthTokenSourceHeader.
	// If empty, defaults to "Authorization".
	HeaderName string

	// CookieName is used when Source is AuthTokenSourceCookie.
	CookieName string
}

// RouteOverrides contains settings that can be overridden at different levels (global, sub-router, route).
// These overrides follow a hierarchy where the most specific setting takes precedence.
type RouteOverrides struct {
	// Timeout overrides the default timeout for requests.
	// A zero value means no override is set.
	Timeout time.Duration

	// MaxBodySize overrides the maximum allowed request body size in bytes.
	// A zero value means no override is set.
	MaxBodySize int64

	// RateLimit overrides the rate limiting configuration.
	// A nil value means no override is set.
	RateLimit *RateLimitConfig[any, any]

	// AuthToken overrides the authentication token source.
	// A nil value means no override is set.
	AuthToken *AuthTokenConfig
}

// HasTimeout returns true if a timeout override is set (non-zero).
func (ro *RouteOverrides) HasTimeout() bool {
	return ro.Timeout > 0
}

// HasMaxBodySize returns true if a max body size override is set (non-zero).
func (ro *RouteOverrides) HasMaxBodySize() bool {
	return ro.MaxBodySize > 0
}

// HasRateLimit returns true if a rate limit override is set (non-nil).
func (ro *RouteOverrides) HasRateLimit() bool {
	return ro.RateLimit != nil
}

// HasAuthToken returns true if an auth token override is set (non-nil).
func (ro *RouteOverrides) HasAuthToken() bool {
	return ro.AuthToken != nil
}
