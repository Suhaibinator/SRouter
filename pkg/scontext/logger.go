package scontext

import (
	"context"
	"encoding"
	"encoding/json"
	"fmt"
	"reflect"

	"github.com/Suhaibinator/SRouter/pkg/logkeys"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// RequestLoggerSource holds immutable application logging configuration shared
// by request and job contexts. Construct it once with NewRequestLoggerSource.
// A nil source or its zero value disables request logging.
type RequestLoggerSource[T comparable] struct {
	base        *zap.Logger
	userIDField func(T) zap.Field
}

// NewRequestLoggerSource configures request logging at application startup.
// base owns the application name, fields, sinks, and options; it must not already
// contain request correlation. A nil base returns a nil source.
//
// A nil userIDField selects an encoder once from T's static type. Strings, bools,
// integers, and floats (including named types) use typed Zap fields. Types with
// Zap/JSON/text marshaling, error, or fmt.Stringer methods, and all other types,
// retain zap.Any semantics. Interface T uses zap.Any for each value's dynamic
// type.
//
// An explicit userIDField overrides that default, including the field key. It
// must be safe for concurrent calls and should be fast and free of side effects:
// concurrent derivations may call it more than once. It runs without the context
// lock and may read context values, but must not recursively call GetLogger or
// mutate correlation. Panics propagate; unsuccessful derivations are not cached.
func NewRequestLoggerSource[T comparable](base *zap.Logger, userIDField func(T) zap.Field) *RequestLoggerSource[T] {
	if base == nil {
		return nil
	}
	if userIDField == nil {
		userIDField = defaultUserIDField[T]()
	}
	return &RequestLoggerSource[T]{base: base, userIDField: userIDField}
}

// WithRequestLogger attaches the application's preconfigured source to a request
// or job. The router does this automatically; other boundaries reuse a source
// created at startup alongside their WithBuildID and WithConfigID calls.
// Passing nil removes the request logger. T is the user ID type, U the user type.
func WithRequestLogger[T comparable, U any](ctx context.Context, source *RequestLoggerSource[T]) context.Context {
	rc, ctx := EnsureSRouterContext[T, U](ctx)
	rc.mu.Lock()
	rc.logSource = source
	rc.logger = nil
	rc.logVersion++
	rc.mu.Unlock()
	return ctx
}

// GetLogger returns a logger stamped with the current trace_id, build_id,
// config_id, and user_id, each only when set. It returns nil, false if no source
// is attached. Services can apply Named with their relative component name to
// share the stamped core, and reuse that child throughout an operation.
//
// Derivation is lazy, so requests that never call GetLogger do not encode fields.
// The cached path takes one context walk and one read lock with no allocations.
// Concurrent first readers may derive independently; only a logger matching the
// current correlation/source version is published. Formatting and Zap core work
// run outside the context lock. A panic leaves the cache invalidated, so the
// next call derives again.
//
// If a correlation or source write lands during derivation, GetLogger discards
// the result and tries again, up to maxLoggerDerivations times in total. After
// that it returns the last snapshot it built without caching it, so a caller
// racing a sustained stream of writes still terminates and still receives a
// logger that matched the correlation at some point during the call.
//
// The returned logger is an immutable snapshot. A later correlation write is
// visible only through another GetLogger call, including for named children.
// T is the user ID type, U the user type.
func GetLogger[T comparable, U any](ctx context.Context) (*zap.Logger, bool) {
	rc, ok := GetSRouterContext[T, U](ctx)
	if !ok {
		return nil, false
	}
	var logger *zap.Logger
	for range maxLoggerDerivations {
		rc.mu.RLock()
		if rc.loggerVersion == rc.logVersion {
			logger = rc.logger
			rc.mu.RUnlock()
			return logger, logger != nil
		}
		source := rc.logSource
		if source == nil || source.base == nil {
			rc.mu.RUnlock()
			return nil, false
		}
		version, correlation := rc.logVersion, rc.correlationLocked()
		rc.mu.RUnlock()

		logger = source.derive(correlation)

		rc.mu.Lock()
		if rc.logVersion != version {
			// A write raced this derivation; the result is already obsolete.
			rc.mu.Unlock()
			continue
		}
		if rc.loggerVersion != version {
			rc.logger = logger
			rc.loggerVersion = version
		}
		logger = rc.logger
		rc.mu.Unlock()
		return logger, true
	}
	// Every attempt was invalidated by a concurrent write. Return the most
	// recent snapshot uncached rather than spin; the cache stays invalid, so
	// the next call derives from whatever the writer finally settles on.
	// Deriving under the write lock instead would deadlock a formatter that
	// reads the context, which the formatter contract allows.
	return logger, true
}

// maxLoggerDerivations bounds how many times GetLogger derives a logger in one
// call before it stops retrying against concurrent correlation writes.
const maxLoggerDerivations = 3

func (source *RequestLoggerSource[T]) derive(c Correlation[T]) *zap.Logger {
	if !c.TraceIDSet && !c.BuildIDSet && !c.ConfigIDSet && !c.UserIDSet {
		return source.base
	}
	var fields [4]zap.Field
	n := 0
	if c.TraceIDSet {
		fields[n] = zap.String(logkeys.TraceID, c.TraceID)
		n++
	}
	if c.BuildIDSet {
		fields[n] = zap.String(logkeys.BuildID, c.BuildID)
		n++
	}
	if c.ConfigIDSet {
		fields[n] = zap.String(logkeys.ConfigID, c.ConfigID)
		n++
	}
	if c.UserIDSet {
		fields[n] = source.userIDField(c.UserID)
		n++
	}
	return source.base.With(fields[:n]...)
}

func defaultUserIDField[T comparable]() func(T) zap.Field {
	fallback := func(id T) zap.Field { return zap.Any(logkeys.UserID, id) }
	t := reflect.TypeFor[T]()
	// Preserve application-defined rendering before considering the underlying
	// kind. Inspect T itself, not its zero value: interface IDs may vary at runtime.
	if t.Implements(reflect.TypeFor[zapcore.ObjectMarshaler]()) ||
		t.Implements(reflect.TypeFor[zapcore.ArrayMarshaler]()) ||
		t.Implements(reflect.TypeFor[error]()) ||
		t.Implements(reflect.TypeFor[fmt.Stringer]()) ||
		t.Implements(reflect.TypeFor[json.Marshaler]()) ||
		t.Implements(reflect.TypeFor[encoding.TextMarshaler]()) {
		return fallback
	}
	// ValueOf's kind accessors safely handle named primitive types. No unsafe
	// representation assumptions or per-derivation type switch are needed.
	switch t.Kind() {
	case reflect.String:
		return func(id T) zap.Field { return zap.String(logkeys.UserID, reflect.ValueOf(id).String()) }
	case reflect.Bool:
		return func(id T) zap.Field { return zap.Bool(logkeys.UserID, reflect.ValueOf(id).Bool()) }
	case reflect.Int, reflect.Int8, reflect.Int16, reflect.Int32, reflect.Int64:
		return func(id T) zap.Field { return zap.Int64(logkeys.UserID, reflect.ValueOf(id).Int()) }
	case reflect.Uint, reflect.Uint8, reflect.Uint16, reflect.Uint32, reflect.Uint64, reflect.Uintptr:
		return func(id T) zap.Field { return zap.Uint64(logkeys.UserID, reflect.ValueOf(id).Uint()) }
	case reflect.Float32:
		return func(id T) zap.Field { return zap.Float32(logkeys.UserID, float32(reflect.ValueOf(id).Float())) }
	case reflect.Float64:
		return func(id T) zap.Field { return zap.Float64(logkeys.UserID, reflect.ValueOf(id).Float()) }
	default:
		return fallback
	}
}
