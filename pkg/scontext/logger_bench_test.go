package scontext

import (
	"context"
	"io"
	"testing"

	"github.com/Suhaibinator/SRouter/pkg/logkeys"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func benchmarkJSONLogger() *zap.Logger {
	return zap.New(zapcore.NewCore(
		zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()),
		zapcore.AddSync(io.Discard), zapcore.InfoLevel,
	)).Named("app")
}

// Includes invalidation and JSON core cloning/encoding; sources are configured
// outside the timed loop, as they are in NewRouter.
func BenchmarkGetLoggerDerivation(b *testing.B) {
	for _, encoder := range []string{"default", "explicit", "zap.Any"} {
		b.Run(encoder, func(b *testing.B) {
			var field func(namedUintID) zap.Field
			switch encoder {
			case "explicit":
				field = func(id namedUintID) zap.Field { return zap.Uint64(logkeys.UserID, uint64(id)) }
			case "zap.Any":
				field = func(id namedUintID) zap.Field { return zap.Any(logkeys.UserID, id) }
			}
			source := NewRequestLoggerSource(benchmarkJSONLogger(), field)
			ctx := WithRequestLogger[namedUintID, testUser](context.Background(), source)
			ctx = WithTraceID[namedUintID, testUser](ctx, "trace-1")
			ctx = WithBuildID[namedUintID, testUser](ctx, "build-1")
			ctx = WithConfigID[namedUintID, testUser](ctx, "config-1")
			b.ReportAllocs()
			for b.Loop() {
				WithUserID[namedUintID, testUser](ctx, 4242)
				loggerSink, _ = GetLogger[namedUintID, testUser](ctx)
			}
		})
	}
}

func BenchmarkRequestLoggerNamedChild(b *testing.B) {
	source := NewRequestLoggerSource[namedUintID](benchmarkJSONLogger(), nil)
	ctx := WithRequestLogger[namedUintID, testUser](context.Background(), source)
	ctx = WithTraceID[namedUintID, testUser](ctx, "trace-1")
	ctx = WithUserID[namedUintID, testUser](ctx, 4242)
	_, _ = GetLogger[namedUintID, testUser](ctx)
	b.ReportAllocs()
	for b.Loop() {
		logger, _ := GetLogger[namedUintID, testUser](ctx)
		loggerSink = logger.Named("common_service.admin")
	}
}
