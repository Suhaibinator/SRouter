package scontext

import (
	"context"
	"fmt"
	"io"
	"math"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/logkeys"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type namedUintID uint64
type namedStringID string
type stringerID uint64

func (id stringerID) String() string { return fmt.Sprintf("user-%d", uint64(id)) }

type errorID uint64

func (id errorID) Error() string { return fmt.Sprintf("error-%d", uint64(id)) }

type jsonID uint64

func (id jsonID) MarshalJSON() ([]byte, error) { return []byte(`"json-id"`), nil }

type textID uint64

func (id textID) MarshalText() ([]byte, error) { return []byte("text-id"), nil }

type objectID uint64

func (id objectID) MarshalLogObject(enc zapcore.ObjectEncoder) error {
	enc.AddUint64("id", uint64(id))
	return nil
}

type arrayID uint64

func (id arrayID) MarshalLogArray(enc zapcore.ArrayEncoder) error {
	enc.AppendUint64(uint64(id))
	return nil
}

func checkDefaultIDField[T comparable](t *testing.T, id T, want zap.Field) {
	t.Helper()
	source := NewRequestLoggerSource[T](zap.NewNop(), nil)
	field := source.userIDField(id)
	// Checking the field type as well as its value catches accidental Reflect
	// encoding for named primitives, and losing custom marshaling methods.
	if !reflect.DeepEqual(field, want) {
		t.Errorf("%T: field = %#v, want %#v", id, field, want)
	}
}

func TestDefaultUserIDField(t *testing.T) {
	checkDefaultIDField(t, namedUintID(math.MaxUint64), zap.Uint64(logkeys.UserID, math.MaxUint64))
	checkDefaultIDField(t, namedStringID("user-42"), zap.String(logkeys.UserID, "user-42"))
	checkDefaultIDField(t, true, zap.Bool(logkeys.UserID, true))
	checkDefaultIDField(t, int(-42), zap.Int64(logkeys.UserID, -42))
	checkDefaultIDField(t, int8(-42), zap.Int64(logkeys.UserID, -42))
	checkDefaultIDField(t, int16(-42), zap.Int64(logkeys.UserID, -42))
	checkDefaultIDField(t, int32(-42), zap.Int64(logkeys.UserID, -42))
	checkDefaultIDField(t, int64(math.MinInt64), zap.Int64(logkeys.UserID, math.MinInt64))
	checkDefaultIDField(t, uint(42), zap.Uint64(logkeys.UserID, 42))
	checkDefaultIDField(t, uint8(42), zap.Uint64(logkeys.UserID, 42))
	checkDefaultIDField(t, uint16(42), zap.Uint64(logkeys.UserID, 42))
	checkDefaultIDField(t, uint32(42), zap.Uint64(logkeys.UserID, 42))
	checkDefaultIDField(t, uintptr(42), zap.Uint64(logkeys.UserID, 42))
	checkDefaultIDField(t, float32(1.23), zap.Float32(logkeys.UserID, 1.23))
	checkDefaultIDField(t, float64(1.23), zap.Float64(logkeys.UserID, 1.23))
	checkDefaultIDField(t, stringerID(42), zap.Any(logkeys.UserID, stringerID(42)))
	checkDefaultIDField(t, errorID(42), zap.Any(logkeys.UserID, errorID(42)))
	checkDefaultIDField(t, jsonID(42), zap.Any(logkeys.UserID, jsonID(42)))
	checkDefaultIDField(t, textID(42), zap.Any(logkeys.UserID, textID(42)))
	checkDefaultIDField(t, objectID(42), zap.Any(logkeys.UserID, objectID(42)))
	checkDefaultIDField(t, arrayID(42), zap.Any(logkeys.UserID, arrayID(42)))
	checkDefaultIDField(t, time.Second, zap.Duration(logkeys.UserID, time.Second))
	checkDefaultIDField(t, [2]byte{1, 2}, zap.Any(logkeys.UserID, [2]byte{1, 2}))
	checkDefaultIDField[*uint64](t, nil, zap.Any(logkeys.UserID, (*uint64)(nil)))
}

func TestInterfaceUserIDSourceHandlesDifferentDynamicTypes(t *testing.T) {
	source := NewRequestLoggerSource[any](zap.NewNop(), nil)
	for _, id := range []any{nil, uint64(42), "user-42", stringerID(42), objectID(42)} {
		if got, want := source.userIDField(id), zap.Any(logkeys.UserID, id); !reflect.DeepEqual(got, want) {
			t.Errorf("%T: field = %#v, want %#v", id, got, want)
		}
	}
}

func TestRequestLoggerSourceReuseAndDisabledSources(t *testing.T) {
	base, logs := newObservedLogger()
	source := NewRequestLoggerSource[namedUintID](base, nil)
	first := WithRequestLogger[namedUintID, testUser](context.Background(), source)
	first = WithUserID[namedUintID, testUser](first, 1000)
	second := WithRequestLogger[namedUintID, testUser](context.Background(), source)
	second = WithUserID[namedUintID, testUser](second, 2000)
	for i, ctx := range []context.Context{first, second, first} {
		logger, ok := GetLogger[namedUintID, testUser](ctx)
		if !ok {
			t.Fatal("source did not install a logger")
		}
		entry := logAndTake(t, logger, logs, "independent request")
		if got, want := entry.ContextMap()[logkeys.UserID], []uint64{1000, 2000, 1000}[i]; got != want {
			t.Fatalf("user_id = %v, want %v", got, want)
		}
	}
	for _, disabled := range []*RequestLoggerSource[namedUintID]{nil, {}, NewRequestLoggerSource[namedUintID](nil, nil)} {
		first = WithRequestLogger[namedUintID, testUser](first, disabled)
		if logger, ok := GetLogger[namedUintID, testUser](first); ok || logger != nil {
			t.Fatalf("disabled source produced (%v, %v)", logger, ok)
		}
	}
}

func TestGetLoggerDerivationCanReadContext(t *testing.T) {
	for _, stage := range []string{"formatter", "Zap encoder"} {
		t.Run(stage, func(t *testing.T) {
			ctx := WithBuildID[int, testUser](context.Background(), "build-1")
			ctx = WithUserID[int, testUser](ctx, 42)
			read := func() string {
				c, _ := GetCorrelation[int, testUser](ctx)
				return c.BuildID
			}
			base := zap.New(zapcore.NewCore(zapcore.NewJSONEncoder(zap.NewProductionEncoderConfig()), zapcore.AddSync(io.Discard), zapcore.InfoLevel))
			source := NewRequestLoggerSource(base, func(id int) zap.Field {
				if stage == "formatter" {
					return zap.String(logkeys.UserID, read())
				}
				return zap.Object(logkeys.UserID, zapcore.ObjectMarshalerFunc(func(enc zapcore.ObjectEncoder) error {
					enc.AddString("build", read())
					return nil
				}))
			})
			ctx = WithRequestLogger[int, testUser](ctx, source)
			done := make(chan struct{})
			go func() {
				defer close(done)
				if _, ok := GetLogger[int, testUser](ctx); !ok {
					t.Error("no logger")
				}
			}()
			select {
			case <-done:
			case <-time.After(2 * time.Second):
				t.Fatal("derivation deadlocked while reading correlation")
			}
		})
	}
}

func TestGetLoggerPanicDoesNotPublishStaleLogger(t *testing.T) {
	base, logs := newObservedLogger()
	panicOnce := true
	source := NewRequestLoggerSource(base, func(id int) zap.Field {
		if id == 2 && panicOnce {
			panicOnce = false
			panic("formatter failed")
		}
		return zap.Int(logkeys.UserID, id)
	})
	ctx := WithRequestLogger[int, testUser](context.Background(), source)
	ctx = WithUserID[int, testUser](ctx, 1)
	previous, _ := GetLogger[int, testUser](ctx)
	ctx = WithUserID[int, testUser](ctx, 2)
	func() {
		defer func() {
			if recover() == nil {
				t.Error("expected formatter panic")
			}
		}()
		GetLogger[int, testUser](ctx)
	}()
	current, ok := GetLogger[int, testUser](ctx)
	if !ok || current == previous {
		t.Fatal("failed derivation marked the old logger current")
	}
	if got := logAndTake(t, current, logs, "retry").ContextMap()[logkeys.UserID]; got != int64(2) {
		t.Fatalf("retry user_id = %v, want 2", got)
	}
}

func TestGetLoggerDiscardsDerivationAfterConcurrentWrite(t *testing.T) {
	for _, change := range []string{"correlation", "source", "remove source"} {
		t.Run(change, func(t *testing.T) {
			base, logs := newObservedLogger()
			started, release := make(chan struct{}), make(chan struct{})
			unblock := sync.OnceFunc(func() { close(release) })
			defer unblock()
			source := NewRequestLoggerSource(base, func(id int) zap.Field {
				if id == 1 {
					close(started)
					<-release
				}
				return zap.Int(logkeys.UserID, id)
			})
			ctx := WithRequestLogger[int, testUser](context.Background(), source)
			ctx = WithUserID[int, testUser](ctx, 1)
			result := make(chan *zap.Logger, 1)
			go func() {
				logger, _ := GetLogger[int, testUser](ctx)
				result <- logger
			}()
			<-started
			// A writer and a second reader can publish a newer logger while
			// the first reader's application formatter is blocked.
			updated := make(chan *zap.Logger, 1)
			go func() {
				switch change {
				case "correlation":
					WithUserID[int, testUser](ctx, 2)
				case "source":
					WithRequestLogger[int, testUser](ctx, NewRequestLoggerSource[int](base.Named("replacement"), nil))
				case "remove source":
					WithRequestLogger[int, testUser](ctx, nil)
				}
				logger, _ := GetLogger[int, testUser](ctx)
				updated <- logger
			}()
			var current *zap.Logger
			select {
			case current = <-updated:
			case <-time.After(2 * time.Second):
				t.Fatal("blocked formatter prevented concurrent context access")
			}
			unblock()
			if got := <-result; got != current {
				t.Fatal("in-flight derivation returned an obsolete logger")
			}
			if change == "remove source" {
				if current != nil {
					t.Fatal("removed source still returned a logger")
				}
				return
			}
			if current == nil {
				t.Fatal("no logger after write")
			}
			entry := logAndTake(t, current, logs, "after write")
			if change == "correlation" && entry.ContextMap()[logkeys.UserID] != int64(2) {
				t.Fatalf("stale user_id: %v", entry.ContextMap())
			}
			if change == "source" && entry.LoggerName != "replacement" {
				t.Fatalf("stale source: %q", entry.LoggerName)
			}
		})
	}
}

func TestCopySRouterContextCachedLoggerIsIndependent(t *testing.T) {
	base, logs := newObservedLogger()
	src := WithRequestLogger[int, testUser](context.Background(), NewRequestLoggerSource[int](base, nil))
	src = WithUserID[int, testUser](src, 1)
	original, _ := GetLogger[int, testUser](src)
	dst := CopySRouterContext[int, testUser](context.Background(), src)
	copied, _ := GetLogger[int, testUser](dst)
	if copied != original {
		t.Fatal("copy did not retain immutable cached logger")
	}
	WithUserID[int, testUser](src, 2)
	WithRequestLogger[int, testUser](dst, NewRequestLoggerSource[int](base.Named("copy"), nil))
	current, _ := GetLogger[int, testUser](src)
	copied, _ = GetLogger[int, testUser](dst)
	entry := logAndTake(t, current, logs, "source")
	if entry.LoggerName != "" || entry.ContextMap()[logkeys.UserID] != int64(2) {
		t.Fatalf("source changed with copy: %#v", entry)
	}
	entry = logAndTake(t, copied, logs, "copy")
	if entry.LoggerName != "copy" || entry.ContextMap()[logkeys.UserID] != int64(1) {
		t.Fatalf("copy changed with source: %#v", entry)
	}
}

func TestNamedRequestLoggersShareCoreAndPreserveApplicationName(t *testing.T) {
	base, logs := newObservedLogger()
	base = base.Named("app").With(zap.String("region", "west"))
	ctx := WithRequestLogger[int, testUser](context.Background(), NewRequestLoggerSource[int](base, nil))
	ctx = WithTraceID[int, testUser](ctx, "trace-1")
	ctx = WithUserID[int, testUser](ctx, 42)
	request, _ := GetLogger[int, testUser](ctx)
	admin := request.Named("common_service.admin")
	permissions := request.Named("common_service.permission")
	if admin.Core() != request.Core() || permissions.Core() != request.Core() {
		t.Fatal("naming cloned the stamped core")
	}
	for _, logger := range []*zap.Logger{admin, permissions, request} {
		entry := logAndTake(t, logger, logs, "named")
		if entry.LoggerName != logger.Name() || entry.ContextMap()[logkeys.TraceID] != "trace-1" || entry.ContextMap()["region"] != "west" {
			t.Fatalf("name or application/request fields lost: %#v", entry)
		}
		if got := fieldKeys(entry.Context); !reflect.DeepEqual(got, []string{"region", logkeys.TraceID, logkeys.UserID}) {
			t.Fatalf("duplicated correlation or name fields: %v", got)
		}
	}
	if admin.Name() != "app.common_service.admin" || permissions.Name() != "app.common_service.permission" || request.Name() != "app" {
		t.Fatalf("incorrect names: %q, %q, %q", admin.Name(), permissions.Name(), request.Name())
	}
	WithUserID[int, testUser](ctx, 43)
	latest, _ := GetLogger[int, testUser](ctx)
	if got := logAndTake(t, admin, logs, "old child").ContextMap()[logkeys.UserID]; got != int64(42) {
		t.Fatalf("named snapshot changed: %v", got)
	}
	if got := logAndTake(t, latest.Named("common_service.admin"), logs, "new child").ContextMap()[logkeys.UserID]; got != int64(43) {
		t.Fatalf("fresh named snapshot stale: %v", got)
	}
}

func TestGetLoggerDefersFormattingUntilRead(t *testing.T) {
	calls := 0
	source := NewRequestLoggerSource(zap.NewNop(), func(id int) zap.Field {
		calls++
		return zap.Int(logkeys.UserID, id)
	})
	ctx := WithRequestLogger[int, testUser](context.Background(), source)
	ctx = WithTraceID[int, testUser](ctx, "trace-1")
	ctx = WithBuildID[int, testUser](ctx, "build-1")
	ctx = WithConfigID[int, testUser](ctx, "config-1")
	ctx = WithUserID[int, testUser](ctx, 1)
	if calls != 0 {
		t.Fatal("correlation writes invoked formatter before logging")
	}
	first, _ := GetLogger[int, testUser](ctx)
	WithUserAgent[int, testUser](ctx, "agent")
	second, _ := GetLogger[int, testUser](ctx)
	if calls != 1 || first != second {
		t.Fatal("cached read or unrelated write caused another derivation")
	}
}

func TestGetLoggerConcurrentFirstReadersSharePublishedLogger(t *testing.T) {
	const readers = 8
	started, release := make(chan struct{}, readers), make(chan struct{})
	source := NewRequestLoggerSource(zap.NewNop(), func(id int) zap.Field {
		started <- struct{}{}
		<-release
		return zap.Int(logkeys.UserID, id)
	})
	ctx := WithRequestLogger[int, testUser](context.Background(), source)
	ctx = WithUserID[int, testUser](ctx, 42)
	results := make(chan *zap.Logger, readers)
	for range readers {
		go func() {
			logger, _ := GetLogger[int, testUser](ctx)
			results <- logger
		}()
	}
	for range readers {
		select {
		case <-started:
		case <-time.After(2 * time.Second):
			close(release)
			t.Fatal("a blocked formatter prevented concurrent derivation")
		}
	}
	close(release)
	winner := <-results
	for range readers - 1 {
		if logger := <-results; logger != winner {
			t.Fatal("readers did not reuse the published logger")
		}
	}
}

func TestGetLoggerStopsRetryingUnderSustainedWrites(t *testing.T) {
	base, logs := newObservedLogger()
	var ctx context.Context
	var derivations int
	hostile := true
	// The formatter mutates correlation on the same context, which the
	// formatter contract forbids. It is the simplest way to guarantee that a
	// write lands inside every derivation window, simulating a writer that
	// never pauses.
	source := NewRequestLoggerSource(base, func(id int) zap.Field {
		derivations++
		if hostile {
			WithBuildID[int, testUser](ctx, fmt.Sprintf("build-%d", derivations))
		}
		return zap.Int(logkeys.UserID, id)
	})
	ctx = WithRequestLogger[int, testUser](context.Background(), source)
	ctx = WithUserID[int, testUser](ctx, 7)

	logger, ok := GetLogger[int, testUser](ctx)
	if !ok || logger == nil {
		t.Fatal("no logger returned under sustained writes")
	}
	if derivations != maxLoggerDerivations {
		t.Fatalf("derivations = %d, want %d", derivations, maxLoggerDerivations)
	}
	entry := logAndTake(t, logger, logs, "under contention")
	if entry.ContextMap()[logkeys.UserID] != int64(7) {
		t.Fatalf("returned snapshot lost correlation: %v", entry.ContextMap())
	}
	// The final build_id write landed after the last snapshot was taken, so
	// the returned logger carries the previous one.
	if got := entry.ContextMap()[logkeys.BuildID]; got != "build-2" {
		t.Fatalf("build_id = %v, want the pre-final snapshot build-2", got)
	}

	// The uncached result left the cache invalid. Once the writer stops, one
	// more derivation publishes and later calls hit the cache.
	hostile = false
	published, _ := GetLogger[int, testUser](ctx)
	if derivations != maxLoggerDerivations+1 {
		t.Fatalf("derivations after writer stopped = %d, want %d", derivations, maxLoggerDerivations+1)
	}
	if again, _ := GetLogger[int, testUser](ctx); again != published {
		t.Fatal("logger was not cached after the writer stopped")
	}
	entry = logAndTake(t, published, logs, "after contention")
	if got := entry.ContextMap()[logkeys.BuildID]; got != "build-3" {
		t.Fatalf("build_id = %v, want the final write build-3", got)
	}
}
