package common

import "time"

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

	// Transaction overrides the transaction configuration.
	// A nil value means no override is set.
	Transaction *TransactionConfig
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

// HasTransaction returns true if a transaction override is set (non-nil).
func (ro *RouteOverrides) HasTransaction() bool {
	return ro.Transaction != nil
}

// TransactionConfig defines configuration for automatic transaction management.
// When enabled, the router will automatically begin a transaction before
// executing the handler and commit/rollback based on the handler's success.
type TransactionConfig struct {
	// Enabled indicates whether automatic transaction management is active.
	// When true, a transaction will be started before the handler executes.
	Enabled bool

	// Options provides database-specific configuration options.
	// These are passed to the TransactionFactory when beginning a transaction.
	// Examples might include isolation level, read-only mode, etc.
	Options map[string]any
}