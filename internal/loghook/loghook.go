// Package loghook connects internal/requestlog to unexported scontext logger
// state without adding library-only functions to the scontext public API.
// scontext installs the hooks during package initialization; requestlog
// imports scontext, so they are set before any use.
package loghook

import (
	"context"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// LibraryLogger returns the request logger for SRouter's own records, or nil
// when ctx has no request logger or its source's core disables level.
var LibraryLogger func(ctx context.Context, level zapcore.Level) *zap.Logger
