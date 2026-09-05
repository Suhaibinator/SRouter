package scontext

import (
	"context"
	"slices"
	"sync"
	"testing"

	"github.com/Suhaibinator/SRouter/pkg/logkeys"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// newObservedLogger returns a logger writing into an observer core that
// captures every level, plus the observed log store.
func newObservedLogger() (*zap.Logger, *observer.ObservedLogs) {
	core, logs := observer.New(zapcore.DebugLevel)
	return zap.New(core), logs
}

// logAndTake emits one line through logger and returns the single entry it
// produced, draining the observer so later assertions start clean.
func logAndTake(t *testing.T, logger *zap.Logger, logs *observer.ObservedLogs, msg string) observer.LoggedEntry {
	t.Helper()
	logger.Info(msg)
	entries := logs.TakeAll()
	if len(entries) != 1 {
		t.Fatalf("observed %d entries for %q, want 1", len(entries), msg)
	}
	return entries[0]
}

// fieldKeys returns the field keys of an entry in the order they were stamped.
func fieldKeys(fields []zapcore.Field) []string {
	keys := make([]string, len(fields))
	for i, f := range fields {
		keys[i] = f.Key
	}
	return keys
}

func TestGetLoggerWithoutSRouterContext(t *testing.T) {
	logger, ok := GetLogger[int, testUser](context.Background())
	if ok {
		t.Error("GetLogger returned true on a plain context, want false")
	}
	if logger != nil {
		t.Errorf("logger = %v on a plain context, want nil", logger)
	}
}

// TestGetLoggerWithoutBase covers a context that carries an SRouterContext but
// never had a base logger installed: correlation writes must not conjure one.
func TestGetLoggerWithoutBase(t *testing.T) {
	_, ctx := EnsureSRouterContext[int, testUser](context.Background())

	if logger, ok := GetLogger[int, testUser](ctx); ok || logger != nil {
		t.Fatalf("GetLogger = (%v, %v) before correlation writes, want (nil, false)", logger, ok)
	}

	ctx = WithTraceID[int, testUser](ctx, "trace-1")
	ctx = WithBuildID[int, testUser](ctx, "build-1")
	ctx = WithConfigID[int, testUser](ctx, "config-1")
	ctx = WithUserID[int, testUser](ctx, 123)

	if logger, ok := GetLogger[int, testUser](ctx); ok || logger != nil {
		t.Fatalf("GetLogger = (%v, %v) after correlation writes, want (nil, false)", logger, ok)
	}
}

func TestGetLoggerWithBaseAndNoCorrelation(t *testing.T) {
	base, logs := newObservedLogger()
	source := NewRequestLoggerSource[int](base, nil)
	ctx := WithRequestLogger[int, testUser](context.Background(), source)

	logger, ok := GetLogger[int, testUser](ctx)
	if !ok {
		t.Fatal("GetLogger returned false with a base installed, want true")
	}
	if logger == nil {
		t.Fatal("GetLogger returned a nil logger with a base installed")
	}

	if logger != base {
		t.Fatal("empty correlation should reuse the application logger")
	}
	if allocs := testing.AllocsPerRun(1000, func() {
		WithRequestLogger[int, testUser](ctx, source)
		loggerSink, _ = GetLogger[int, testUser](ctx)
	}); allocs != 0 {
		t.Errorf("empty correlation derivation = %.1f allocs/op, want 0", allocs)
	}

	entry := logAndTake(t, logger, logs, "no correlation")
	if got := fieldKeys(entry.Context); len(got) != 0 {
		t.Errorf("entry fields = %v, want none", got)
	}
}

// TestGetLoggerStampsCorrelationInOrder walks the four correlation writers in
// turn. Each write must be visible on the next GetLogger, in the documented
// field order, and must leave the previously returned logger untouched.
func TestGetLoggerStampsCorrelationInOrder(t *testing.T) {
	base, logs := newObservedLogger()
	ctx := WithRequestLogger[int, testUser](context.Background(), NewRequestLoggerSource[int](base, nil))

	previous, ok := GetLogger[int, testUser](ctx)
	if !ok {
		t.Fatal("GetLogger returned false with a base installed, want true")
	}

	steps := []struct {
		name  string
		write func(context.Context) context.Context
		key   string
		want  []string
	}{
		{
			name:  "trace ID",
			write: func(c context.Context) context.Context { return WithTraceID[int, testUser](c, "trace-1") },
			key:   logkeys.TraceID,
			want:  []string{logkeys.TraceID},
		},
		{
			name:  "build ID",
			write: func(c context.Context) context.Context { return WithBuildID[int, testUser](c, "build-1") },
			key:   logkeys.BuildID,
			want:  []string{logkeys.TraceID, logkeys.BuildID},
		},
		{
			name:  "config ID",
			write: func(c context.Context) context.Context { return WithConfigID[int, testUser](c, "config-1") },
			key:   logkeys.ConfigID,
			want:  []string{logkeys.TraceID, logkeys.BuildID, logkeys.ConfigID},
		},
		{
			name:  "user ID",
			write: func(c context.Context) context.Context { return WithUserID[int, testUser](c, 123) },
			key:   logkeys.UserID,
			want:  []string{logkeys.TraceID, logkeys.BuildID, logkeys.ConfigID, logkeys.UserID},
		},
	}

	for _, step := range steps {
		ctx = step.write(ctx)

		logger, ok := GetLogger[int, testUser](ctx)
		if !ok {
			t.Fatalf("%s: GetLogger returned false, want true", step.name)
		}

		entry := logAndTake(t, logger, logs, step.name)
		if got := fieldKeys(entry.Context); !slices.Equal(got, step.want) {
			t.Errorf("%s: entry fields = %v, want %v", step.name, got, step.want)
		}

		// The logger handed out before this write is immutable: it must not
		// have picked up the field written since.
		staleEntry := logAndTake(t, previous, logs, step.name+" (previous logger)")
		if slices.Contains(fieldKeys(staleEntry.Context), step.key) {
			t.Errorf("%s: previously returned logger carries %q, want it unchanged", step.name, step.key)
		}

		previous = logger
	}

	final := logAndTake(t, previous, logs, "final")
	wantValues := map[string]any{
		logkeys.TraceID:  "trace-1",
		logkeys.BuildID:  "build-1",
		logkeys.ConfigID: "config-1",
		logkeys.UserID:   int64(123),
	}
	got := final.ContextMap()
	for key, want := range wantValues {
		if got[key] != want {
			t.Errorf("field %q = %#v, want %#v", key, got[key], want)
		}
	}
}

// TestGetLoggerTraceIDPreservedDoesNotRebuild pins that WithTraceID's
// preserve-existing branch leaves the logger alone: no stale mark, so the same
// pointer comes back and the original trace ID stays stamped.
func TestGetLoggerTraceIDPreservedDoesNotRebuild(t *testing.T) {
	base, logs := newObservedLogger()
	ctx := WithRequestLogger[int, testUser](context.Background(), NewRequestLoggerSource[int](base, nil))
	ctx = WithTraceID[int, testUser](ctx, "trace-original")

	first, ok := GetLogger[int, testUser](ctx)
	if !ok {
		t.Fatal("GetLogger returned false, want true")
	}

	ctx = WithTraceID[int, testUser](ctx, "trace-second")

	second, ok := GetLogger[int, testUser](ctx)
	if !ok {
		t.Fatal("GetLogger returned false after the second trace ID write, want true")
	}
	if first != second {
		t.Errorf("GetLogger returned a rebuilt logger (%p, then %p), want the same pointer", first, second)
	}

	entry := logAndTake(t, second, logs, "preserved trace")
	if got := entry.ContextMap()[logkeys.TraceID]; got != "trace-original" {
		t.Errorf("%s = %#v, want %q", logkeys.TraceID, got, "trace-original")
	}
}

// TestGetLoggerStampsExplicitlyEmptyTraceID mirrors GetCorrelation: the Set
// flag decides presence, so a deliberately empty trace ID is still stamped.
func TestGetLoggerStampsExplicitlyEmptyTraceID(t *testing.T) {
	base, logs := newObservedLogger()
	ctx := WithRequestLogger[int, testUser](context.Background(), NewRequestLoggerSource[int](base, nil))
	ctx = WithTraceID[int, testUser](ctx, "")

	logger, ok := GetLogger[int, testUser](ctx)
	if !ok {
		t.Fatal("GetLogger returned false, want true")
	}

	entry := logAndTake(t, logger, logs, "empty trace")
	value, present := entry.ContextMap()[logkeys.TraceID]
	if !present {
		t.Fatalf("entry fields = %v, want %q present", fieldKeys(entry.Context), logkeys.TraceID)
	}
	if value != "" {
		t.Errorf("%s = %#v, want the empty string", logkeys.TraceID, value)
	}
}

// TestGetLoggerUsesUserIDField proves the supplied hook renders the user ID,
// by having it write a key SRouter would never choose on its own.
func TestGetLoggerUsesUserIDField(t *testing.T) {
	base, logs := newObservedLogger()
	userIDField := func(id uint64) zap.Field { return zap.Uint64("actor_id", id) }
	ctx := WithRequestLogger[uint64, testUser](context.Background(), NewRequestLoggerSource(base, userIDField))
	ctx = WithUserID[uint64, testUser](ctx, 42)

	logger, ok := GetLogger[uint64, testUser](ctx)
	if !ok {
		t.Fatal("GetLogger returned false, want true")
	}

	entry := logAndTake(t, logger, logs, "user ID hook")
	fields := entry.ContextMap()
	if got, want := fields["actor_id"], uint64(42); got != want {
		t.Errorf("actor_id = %#v (%T), want %#v", got, got, want)
	}
	if _, present := fields[logkeys.UserID]; present {
		t.Errorf("entry carries %q, want only the hook's key", logkeys.UserID)
	}
}

// TestGetLoggerUserIDDefaultUint64 covers the default encoder for a builtin numeric T.
func TestGetLoggerUserIDDefaultUint64(t *testing.T) {
	base, logs := newObservedLogger()
	ctx := WithRequestLogger[uint64, testUser](context.Background(), NewRequestLoggerSource[uint64](base, nil))
	ctx = WithUserID[uint64, testUser](ctx, 42)

	logger, ok := GetLogger[uint64, testUser](ctx)
	if !ok {
		t.Fatal("GetLogger returned false, want true")
	}

	entry := logAndTake(t, logger, logs, "uint64 user ID")
	got := entry.ContextMap()[logkeys.UserID]
	if want := uint64(42); got != want {
		t.Errorf("%s = %#v (%T), want %#v (uint64)", logkeys.UserID, got, got, want)
	}
}

// TestGetLoggerUserIDDefaultString covers the nil-hook path for a string T.
func TestGetLoggerUserIDDefaultString(t *testing.T) {
	base, logs := newObservedLogger()
	ctx := WithRequestLogger[string, testUser](context.Background(), NewRequestLoggerSource[string](base, nil))
	ctx = WithUserID[string, testUser](ctx, "user-42")

	logger, ok := GetLogger[string, testUser](ctx)
	if !ok {
		t.Fatal("GetLogger returned false, want true")
	}

	entry := logAndTake(t, logger, logs, "string user ID")
	got := entry.ContextMap()[logkeys.UserID]
	if want := "user-42"; got != want {
		t.Errorf("%s = %#v (%T), want %#v (string)", logkeys.UserID, got, got, want)
	}
}

// TestWithRequestLoggerNilRemovesLogger covers the documented removal path.
func TestWithRequestLoggerNilRemovesLogger(t *testing.T) {
	base, _ := newObservedLogger()
	ctx := WithRequestLogger[int, testUser](context.Background(), NewRequestLoggerSource[int](base, nil))
	ctx = WithTraceID[int, testUser](ctx, "trace-1")

	if _, ok := GetLogger[int, testUser](ctx); !ok {
		t.Fatal("GetLogger returned false with a base installed, want true")
	}

	ctx = WithRequestLogger[int, testUser](ctx, nil)

	logger, ok := GetLogger[int, testUser](ctx)
	if ok || logger != nil {
		t.Fatalf("GetLogger = (%v, %v) after removal, want (nil, false)", logger, ok)
	}
}

// TestGetLoggerConcurrentWithWrites exercises cache publication
// against concurrent correlation writes; it is meaningful under -race.
func TestGetLoggerConcurrentWithWrites(t *testing.T) {
	base, _ := newObservedLogger()
	ctx := WithRequestLogger[int, testUser](context.Background(), NewRequestLoggerSource[int](base, nil))
	ctx = WithTraceID[int, testUser](ctx, "trace-1")

	var readers sync.WaitGroup
	var writer sync.WaitGroup

	for range 8 {
		readers.Go(func() {
			for range 200 {
				logger, ok := GetLogger[int, testUser](ctx)
				if !ok || logger == nil {
					t.Errorf("GetLogger = (%v, %v) during concurrent writes, want a logger", logger, ok)
					return
				}
				logger.Debug("concurrent read")
			}
		})
	}

	writer.Go(func() {
		for i := range 1000 {
			WithUserID[int, testUser](ctx, i)
		}
	})

	readers.Wait()
	writer.Wait()
}

// TestCopySRouterContextPreservesLogger pins that a cloned wrapper keeps the
// request logger and its stamped fields.
func TestCopySRouterContextPreservesLogger(t *testing.T) {
	base, logs := newObservedLogger()
	src := WithRequestLogger[int, testUser](context.Background(), NewRequestLoggerSource[int](base, nil))
	src = WithTraceID[int, testUser](src, "trace-1")
	src = WithBuildID[int, testUser](src, "build-1")
	src = WithConfigID[int, testUser](src, "config-1")
	src = WithUserID[int, testUser](src, 123)

	dst := CopySRouterContext[int, testUser](context.Background(), src)

	logger, ok := GetLogger[int, testUser](dst)
	if !ok {
		t.Fatal("GetLogger on the copy returned false, want true")
	}

	entry := logAndTake(t, logger, logs, "copied context")
	want := []string{logkeys.TraceID, logkeys.BuildID, logkeys.ConfigID, logkeys.UserID}
	if got := fieldKeys(entry.Context); !slices.Equal(got, want) {
		t.Errorf("entry fields = %v, want %v", got, want)
	}
}

// loggerSink keeps the benchmark and allocation test from having their
// GetLogger result optimized away.
var loggerSink *zap.Logger

// benchLoggerContext returns a context with a base logger installed and every
// correlation value written, with the derived logger already warmed.
func benchLoggerContext() context.Context {
	core, _ := observer.New(zapcore.DebugLevel)
	ctx := WithRequestLogger[uint64, testUser](context.Background(), NewRequestLoggerSource[uint64](zap.New(core), nil))
	ctx = WithTraceID[uint64, testUser](ctx, "trace-1")
	ctx = WithBuildID[uint64, testUser](ctx, "build-1")
	ctx = WithConfigID[uint64, testUser](ctx, "config-1")
	ctx = WithUserID[uint64, testUser](ctx, 123)
	loggerSink, _ = GetLogger[uint64, testUser](ctx)
	return ctx
}

// TestGetLoggerFastPathAllocs pins the documented promise that a warmed
// GetLogger is allocation free.
func TestGetLoggerFastPathAllocs(t *testing.T) {
	ctx := benchLoggerContext()
	if loggerSink == nil {
		t.Fatal("GetLogger returned nil while warming, want a logger")
	}

	allocs := testing.AllocsPerRun(1000, func() {
		loggerSink, _ = GetLogger[uint64, testUser](ctx)
	})
	if allocs != 0 {
		t.Errorf("GetLogger fast path = %.1f allocs/op, want 0", allocs)
	}
}

// BenchmarkGetLoggerFastPath measures the read path once the derived logger is
// current: one context walk, one read lock, one pointer copy.
func BenchmarkGetLoggerFastPath(b *testing.B) {
	ctx := benchLoggerContext()

	b.ReportAllocs()
	for b.Loop() {
		loggerSink, _ = GetLogger[uint64, testUser](ctx)
	}
}
