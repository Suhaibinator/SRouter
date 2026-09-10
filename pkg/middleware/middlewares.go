package middleware

import (
	"github.com/Suhaibinator/SRouter/pkg/common"
)

// Middleware is an alias for the common.Middleware type.
// It represents a function that wraps an http.Handler to provide additional functionality.
type Middleware = common.Middleware

var (
	// MaxBodySize creates middleware that limits request-body reads to maxSize
	// bytes with http.MaxBytesReader. Callers should pass a positive size; the
	// router applies an equivalent limit directly for positive route policies.
	MaxBodySize = maxBodySize
)
