// Package requestlog emits library records through the shared request logger.
package requestlog

import (
	"net/http"

	"github.com/Suhaibinator/SRouter/pkg/scontext"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Check resolves the current request logger at the logging point. It does not
// install a source or modify request context. Standalone middleware without a
// source retains its HTTP behavior and emits no logs. Callers must construct
// event fields only after Check returns a nonnil entry, then call Write once.
// Request metadata, including normalized client IP, must be installed at ingress.
func Check[T comparable, U any](req *http.Request, level zapcore.Level, message string) *zapcore.CheckedEntry {
	logger, ok := scontext.GetLogger[T, U](req.Context())
	if !ok || !logger.Core().Enabled(level) {
		return nil
	}
	return logger.Named("SRouter").WithOptions(zap.AddCallerSkip(1)).Check(level, message)
}
