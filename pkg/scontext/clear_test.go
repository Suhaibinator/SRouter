package scontext

import (
	"context"
	"errors"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/julienschmidt/httprouter"
	"go.uber.org/zap"
)

type clearCase struct {
	name   string
	clear  func(context.Context) context.Context
	fields []string
	log    bool
}

func clearCases[T comparable, U any]() []clearCase {
	return []clearCase{
		{"UserID", ClearUserID[T, U], []string{"id"}, true},
		{"User", ClearUser[T, U], []string{"user"}, false},
		{"Identity", ClearIdentity[T, U], []string{"id", "user"}, true},
		{"BuildID", ClearBuildID[T, U], []string{"build"}, true},
		{"ConfigID", ClearConfigID[T, U], []string{"config"}, true},
		{"ClientIP", ClearClientIP[T, U], []string{"ip"}, true},
		{"UserAgent", ClearUserAgent[T, U], []string{"ua"}, false},
		{"ClientInfo", ClearClientInfo[T, U], []string{"ip", "ua"}, true},
		{"TraceID", ClearTraceID[T, U], []string{"trace", "tracePresence"}, true},
		{"Transaction", ClearTransaction[T, U], []string{"tx"}, false},
		{"RouteInfo", ClearRouteInfo[T, U], []string{"route", "params"}, false},
		{"CORSInfo", ClearCORSInfo[T, U], []string{"cors"}, false},
		{"CORSRequestedHeaders", ClearCORSRequestedHeaders[T, U], []string{"headers"}, false},
		{"HandlerError", ClearHandlerError[T, U], []string{"error"}, false},
		{"Flag", func(ctx context.Context) context.Context { return ClearFlag[T, U](ctx, "target") }, []string{"flag"}, false},
		{"RequestLogger", ClearRequestLogger[T, U], nil, true},
	}
}

func pair[V any](v V, ok bool) any {
	return struct {
		Value   V
		Present bool
	}{v, ok}
}
func clearState(ctx context.Context) map[string]any {
	origin, credentials, ok := GetCORSInfo(ctx)
	c, _ := GetCorrelation[int](ctx)
	return map[string]any{
		"id": pair(GetUserID[int](ctx)), "user": pair(GetUser[int, testUser](ctx)),
		"build": pair(GetBuildID(ctx)), "config": pair(GetConfigID(ctx)),
		"ip": pair(GetClientIP(ctx)), "ua": pair(GetUserAgent(ctx)),
		"trace": GetTraceID(ctx), "tracePresence": c.TraceIDSet,
		"tx": pair(GetTransaction(ctx)), "route": pair(GetRouteTemplate(ctx)), "params": pair(GetPathParams(ctx)),
		"cors": []any{origin, credentials, ok}, "headers": pair(GetCORSRequestedHeaders(ctx)),
		"error": pair(GetHandlerError(ctx)), "flag": pair(GetFlag(ctx, "target")), "otherFlag": pair(GetFlag(ctx, "other")),
	}
}

func populateClearContext(ctx context.Context, zero bool) context.Context {
	id, text, user, params, err := 7, "value", new(testUser), httprouter.Params{{Key: "id", Value: "7"}}, error(errors.New("handler"))
	var tx DatabaseTransaction = &mockTransaction{}
	if zero {
		id, text, user, params, err, tx = 0, "", nil, nil, nil, nil
	}
	ctx = WithUserID[int, testUser](ctx, id)
	ctx = WithUser[int](ctx, user)
	ctx = WithBuildID[int, testUser](ctx, text)
	ctx = WithConfigID[int, testUser](ctx, text)
	ctx = WithClientInfo[int, testUser](ctx, text, text)
	ctx = SetTraceID[int, testUser](ctx, text)
	ctx = WithTransaction[int, testUser](ctx, tx)
	ctx = WithRouteInfo[int, testUser](ctx, params, text)
	ctx = WithCORSInfo[int, testUser](ctx, text, !zero)
	ctx = WithCORSRequestedHeaders[int, testUser](ctx, text)
	ctx = WithHandlerError[int, testUser](ctx, err)
	ctx = WithFlag[int, testUser](ctx, "target", !zero)
	ctx = WithFlag[int, testUser](ctx, "other", true)
	return WithRequestLogger[int, testUser](ctx, NewRequestLoggerSource[int](zap.NewNop(), nil))
}

func TestClearHelpers(t *testing.T) {
	empty := clearState(context.Background())
	wrongID, wrongUser := clearCases[string, testUser](), clearCases[int, string]()
	for i, tc := range clearCases[int, testUser]() {
		t.Run(tc.name, func(t *testing.T) {
			bare := context.Background()
			if tc.clear(bare) != bare {
				t.Fatal("missing wrapper changed context")
			}
			if _, ok := GetSRouterContext[int, testUser](bare); ok {
				t.Fatal("created wrapper")
			}
			for _, zero := range []bool{false, true} {
				ctx := populateClearContext(context.Background(), zero)
				before := clearState(ctx)
				oldLogger, _ := GetLogger(ctx)
				rc, _ := GetSRouterContext[int, testUser](ctx)
				version := rc.logVersion
				for _, clear := range []func(context.Context) context.Context{wrongID[i].clear, wrongUser[i].clear} {
					if clear(ctx) != ctx || !reflect.DeepEqual(before, clearState(ctx)) {
						t.Fatal("mismatched clear changed state")
					}
				}
				if logger, ok := GetLogger(ctx); !ok || logger != oldLogger || rc.logVersion != version {
					t.Fatal("mismatched clear changed logger")
				}
				want := clearState(ctx)
				for _, field := range tc.fields {
					want[field] = empty[field]
				}
				for n := uint64(1); n <= 2; n++ {
					if tc.clear(ctx) != ctx {
						t.Fatal("clear replaced context")
					}
					if got := clearState(ctx); !reflect.DeepEqual(want, got) {
						t.Fatalf("state after clear: got %#v want %#v", got, want)
					}
					expectedVersion := version
					if tc.log {
						expectedVersion += n
					}
					if rc.logVersion != expectedVersion {
						t.Fatal("wrong logger invalidation count")
					}
					logger, ok := GetLogger(ctx)
					if tc.name == "RequestLogger" {
						if logger != nil || ok {
							t.Fatal("logger not removed")
						}
					} else if !ok || (!tc.log && logger != oldLogger) {
						t.Fatal("unrelated logger changed")
					}
				}
				// Re-populating verifies every cleared bit can be set again, even for zeros.
				populateClearContext(ctx, zero)
				if !reflect.DeepEqual(before, clearState(ctx)) {
					t.Fatal("set after clear failed")
				}
				if _, ok := GetLogger(ctx); !ok {
					t.Fatal("logger reattachment failed")
				}
			}
			_, unset := EnsureSRouterContext[int, testUser](bare)
			tc.clear(unset)
			if !reflect.DeepEqual(empty, clearState(unset)) {
				t.Fatal("clear of absent field changed state")
			}
		})
	}
}

func TestClearIsolation(t *testing.T) {
	type key struct{}
	for _, tc := range clearCases[int, testUser]() {
		t.Run(tc.name, func(t *testing.T) {
			parent, cancel := context.WithTimeout(populateClearContext(context.Background(), false), time.Minute)
			defer cancel()
			parent = context.WithValue(parent, key{}, "retained")
			before := clearState(parent)
			parentLogger, _ := GetLogger(parent)
			for _, copyCtx := range []func(context.Context, context.Context) context.Context{CopySRouterContext[int, testUser], CopySRouterContextOverlay[int, testUser]} {
				child := copyCtx(parent, parent)
				tc.clear(child)
				if !reflect.DeepEqual(before, clearState(parent)) {
					t.Fatal("clear affected parent")
				}
				if l, _ := GetLogger(parent); l != parentLogger {
					t.Fatal("parent logger changed")
				}
				deadline, _ := parent.Deadline()
				childDeadline, _ := child.Deadline()
				if childDeadline != deadline || child.Done() != parent.Done() || child.Value(key{}) != "retained" {
					t.Fatal("context chain changed")
				}
			}
			sibling := context.WithValue(parent, key{}, "sibling")
			tc.clear(sibling)
			if !reflect.DeepEqual(clearState(parent), clearState(sibling)) {
				t.Fatal("shared wrapper not mutated")
			}
			if tc.name == "RequestLogger" {
				if _, ok := GetLogger(parent); ok {
					t.Fatal("shared logger not cleared")
				}
			}
			cancel()
			if sibling.Err() != context.Canceled {
				t.Fatal("cancellation lost")
			}
		})
	}
}

func TestClearConcurrent(t *testing.T) {
	ctx := populateClearContext(context.Background(), false)
	var wg sync.WaitGroup
	for _, tc := range clearCases[int, testUser]() {
		wg.Go(func() {
			for range 100 {
				populateClearContext(ctx, false)
				tc.clear(ctx)
				clearState(ctx)
				GetLogger(ctx)
				CopySRouterContext[int, testUser](ctx, ctx)
				CopySRouterContextOverlay[int, testUser](ctx, ctx)
			}
		})
	}
	wg.Wait()
	for _, tc := range clearCases[int, testUser]() {
		tc.clear(ctx)
	}
}

func TestClearTraceAllowsWithTraceID(t *testing.T) {
	ctx := WithTraceID[int, testUser](context.Background(), "")
	ClearTraceID[int, testUser](ctx)
	WithTraceID[int, testUser](ctx, "replacement")
	if GetTraceID(ctx) != "replacement" {
		t.Fatal("trace presence not cleared")
	}
}

func TestClearLoggerCorrelation(t *testing.T) {
	for _, tc := range []struct {
		name    string
		clear   func(context.Context) context.Context
		removed []string
	}{
		{"user", ClearUserID[int, testUser], []string{"user_id"}},
		{"identity", ClearIdentity[int, testUser], []string{"user_id"}},
		{"trace", ClearTraceID[int, testUser], []string{"trace_id"}},
		{"build", ClearBuildID[int, testUser], []string{"build_id"}},
		{"config", ClearConfigID[int, testUser], []string{"config_id"}},
		{"ip", ClearClientIP[int, testUser], []string{"client_ip"}},
		{"client", ClearClientInfo[int, testUser], []string{"client_ip"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			base, logs := newObservedLogger()
			parent := populateClearContext(context.Background(), false)
			WithRequestLogger[int, testUser](parent, NewRequestLoggerSource[int](base, nil))
			old, _ := GetLogger(parent)
			original := logAndTake(t, old, logs, "original").ContextMap()
			child := CopySRouterContext[int, testUser](parent, parent)
			tc.clear(child)
			logger, _ := GetLogger(child)
			got := logAndTake(t, logger, logs, "cleared").ContextMap()
			for _, key := range tc.removed {
				if _, ok := got[key]; ok {
					t.Fatalf("logger still contains %s", key)
				}
			}
			for key, value := range original {
				removed := false
				for _, k := range tc.removed {
					removed = removed || k == key
				}
				if !removed && !reflect.DeepEqual(value, got[key]) {
					t.Fatalf("unrelated field %s changed", key)
				}
			}
			if got := logAndTake(t, old, logs, "old snapshot").ContextMap(); !reflect.DeepEqual(original, got) {
				t.Fatal("old snapshot changed")
			}
			if logger, _ := GetLogger(parent); logger != old {
				t.Fatal("parent cache invalidated")
			}
		})
	}
}
