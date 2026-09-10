// Package requestlog emits library records through the shared request logger.
package requestlog

import (
	"net/http"

	"github.com/Suhaibinator/SRouter/internal/clientip"
	"github.com/Suhaibinator/SRouter/pkg/scontext"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// Check resolves the current request logger at the logging point. It does not
// install a source or modify request context. Standalone middleware without a
// source retains its HTTP behavior and emits no logs. Callers must construct
// event fields only after Check returns a nonnil entry, then call Write once.
func Check[T comparable, U any](req *http.Request, level zapcore.Level, message string) *zapcore.CheckedEntry {
	logger, ok := scontext.GetLogger[T, U](req.Context())
	if !ok || !logger.Core().Enabled(level) {
		return nil
	}
	// The shared logger stamps context IPs. A standalone request may only have
	// a peer address; derive a log-local fallback without changing limiter keys.
	if ip, _ := scontext.GetClientIP[T, U](req.Context()); ip == "" {
		if ip := clientip.Clean(req.RemoteAddr); ip != "" {
			// Work on an isolated snapshot: a concurrent client-IP write must
			// not produce duplicate fields or change the request's limiter key.
			ctx := scontext.CopySRouterContext[T, U](req.Context(), req.Context())
			if current, _ := scontext.GetClientIP[T, U](ctx); current == "" {
				ctx = scontext.WithClientIP[T, U](ctx, ip)
			}
			logger, ok = scontext.GetLogger[T, U](ctx)
			if !ok {
				return nil
			}
		}
	}
	return logger.Named("SRouter").WithOptions(zap.AddCallerSkip(1)).Check(level, message)
}
