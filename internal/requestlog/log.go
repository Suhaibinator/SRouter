// Package requestlog emits library records through the shared request logger.
package requestlog

import (
	"context"

	"github.com/Suhaibinator/SRouter/pkg/scontext"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Check resolves the current request logger at the logging point. It does not
// install a source or modify request context; SRouter initializes both before
// middleware runs. A missing source produces no entry. Callers must construct
// event fields only after Check returns a nonnil entry, then call Write once.
// Request metadata, including normalized client IP, must be installed at ingress.
func Check[T comparable, U any](ctx context.Context, level zapcore.Level, message string) *zapcore.CheckedEntry {
	logger, ok := scontext.GetLogger[T](ctx)
	if !ok || !logger.Core().Enabled(level) {
		return nil
	}
	return logger.Named("SRouter").WithOptions(zap.AddCallerSkip(1)).Check(level, message)
}
