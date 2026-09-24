// Package requestlog emits library records through the shared request logger.
package requestlog

import (
	"context"

	"github.com/Suhaibinator/SRouter/internal/loghook"
	// scontext installs loghook.LibraryLogger during initialization.
	_ "github.com/Suhaibinator/SRouter/pkg/scontext"
	"go.uber.org/zap/zapcore"
)

// Check resolves the current request logger at the logging point. It does not
// install a source or modify request context; SRouter initializes both before
// middleware runs. A missing source produces no entry. Callers must construct
// event fields only after Check returns a nonnil entry, then call Write once.
// Request metadata, including normalized client IP, must be installed at ingress.
//
// A level the source's core disables returns nil before the correlated logger
// is derived, so disabled records cost no field encoding.
func Check[T comparable, U any](ctx context.Context, level zapcore.Level, message string) *zapcore.CheckedEntry {
	logger := loghook.LibraryLogger(ctx, level)
	if logger == nil {
		return nil
	}
	return logger.Check(level, message)
}
