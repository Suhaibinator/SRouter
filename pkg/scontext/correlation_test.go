package scontext

import (
	"context"
	"fmt"
	"net/http/httptest"
	"sync"
	"testing"

	"go.uber.org/zap"
)

func TestGetCorrelationReturnsEveryValue(t *testing.T) {
	ctx := WithUserID[int, testUser](context.Background(), 123)
	ctx = WithTraceID[int, testUser](ctx, "trace-1")
	ctx = WithBuildID[int, testUser](ctx, "build-1")
	ctx = WithConfigID[int, testUser](ctx, "config-1")

	c, ok := GetCorrelation[int, testUser](ctx)
	if !ok {
		t.Fatal("GetCorrelation returned false, want true")
	}
	if c.TraceID != "trace-1" || !c.TraceIDSet {
		t.Errorf("TraceID = (%q, %v), want (trace-1, true)", c.TraceID, c.TraceIDSet)
	}
	if c.BuildID != "build-1" || !c.BuildIDSet {
		t.Errorf("BuildID = (%q, %v), want (build-1, true)", c.BuildID, c.BuildIDSet)
	}
	if c.ConfigID != "config-1" || !c.ConfigIDSet {
		t.Errorf("ConfigID = (%q, %v), want (config-1, true)", c.ConfigID, c.ConfigIDSet)
	}
	if c.UserID != 123 || !c.UserIDSet {
		t.Errorf("UserID = (%d, %v), want (123, true)", c.UserID, c.UserIDSet)
	}
}

func TestGetCorrelationReportsUnsetValues(t *testing.T) {
	ctx := WithTraceID[int, testUser](context.Background(), "trace-only")

	c, ok := GetCorrelation[int, testUser](ctx)
	if !ok {
		t.Fatal("GetCorrelation returned false, want true")
	}
	if !c.TraceIDSet {
		t.Error("TraceIDSet = false, want true")
	}
	if c.BuildIDSet || c.ConfigIDSet || c.UserIDSet {
		t.Errorf("set flags = %+v, want only the trace ID set", c)
	}
	if c.BuildID != "" || c.ConfigID != "" || c.UserID != 0 {
		t.Errorf("unset values = %+v, want zero", c)
	}
}

func TestGetCorrelationDistinguishesSetEmptyValues(t *testing.T) {
	ctx := WithTraceID[int, testUser](context.Background(), "")
	ctx = WithBuildID[int, testUser](ctx, "")
	ctx = WithConfigID[int, testUser](ctx, "")
	ctx = WithUserID[int, testUser](ctx, 0)

	c, ok := GetCorrelation[int, testUser](ctx)
	if !ok {
		t.Fatal("GetCorrelation returned false, want true")
	}
	if !c.TraceIDSet || !c.BuildIDSet || !c.ConfigIDSet || !c.UserIDSet {
		t.Errorf("set flags = %+v, want all true for explicitly set zero values", c)
	}
}

func TestGetCorrelationMatchesIndividualAccessors(t *testing.T) {
	ctx := createFullSRouterContext()

	c, ok := GetCorrelation[int, testUser](ctx)
	if !ok {
		t.Fatal("GetCorrelation returned false, want true")
	}
	if want := GetTraceIDFromContext[int, testUser](ctx); c.TraceID != want {
		t.Errorf("TraceID = %q, want %q", c.TraceID, want)
	}
	buildID, buildIDSet := GetBuildID[int, testUser](ctx)
	if c.BuildID != buildID || c.BuildIDSet != buildIDSet {
		t.Errorf("BuildID = (%q, %v), want (%q, %v)", c.BuildID, c.BuildIDSet, buildID, buildIDSet)
	}
	configID, configIDSet := GetConfigID[int, testUser](ctx)
	if c.ConfigID != configID || c.ConfigIDSet != configIDSet {
		t.Errorf("ConfigID = (%q, %v), want (%q, %v)", c.ConfigID, c.ConfigIDSet, configID, configIDSet)
	}
	userID, userIDSet := GetUserID[int, testUser](ctx)
	if c.UserID != userID || c.UserIDSet != userIDSet {
		t.Errorf("UserID = (%d, %v), want (%d, %v)", c.UserID, c.UserIDSet, userID, userIDSet)
	}
}

// TestCorrelationDoesNotObserveLaterWrites pins the point-in-time semantics
// the documentation promises.
func TestCorrelationDoesNotObserveLaterWrites(t *testing.T) {
	rc, ctx := EnsureSRouterContext[int, testUser](context.Background())
	ctx = WithBuildID[int, testUser](ctx, "build-1")

	c, _ := GetCorrelation[int, testUser](ctx)

	rc.mu.Lock()
	rc.BuildID = "build-2"
	rc.mu.Unlock()

	if c.BuildID != "build-1" {
		t.Errorf("BuildID = %q after a later write, want the value read at call time", c.BuildID)
	}
}

func TestGetCorrelationWithoutSRouterContext(t *testing.T) {
	c, ok := GetCorrelation[int, testUser](context.Background())
	if ok {
		t.Fatal("GetCorrelation returned true, want false")
	}
	if c != (Correlation[int]{}) {
		t.Errorf("correlation = %+v, want zero value", c)
	}
}

func TestGetCorrelationFromRequest(t *testing.T) {
	ctx := WithTraceID[int, testUser](context.Background(), "trace-req")
	req := httptest.NewRequest("GET", "/", nil).WithContext(ctx)

	c, ok := GetCorrelationFromRequest[int, testUser](req)
	if !ok || c.TraceID != "trace-req" {
		t.Fatalf("GetCorrelationFromRequest = (%q, %v), want (trace-req, true)", c.TraceID, ok)
	}

	if _, ok := GetCorrelationFromRequest[int, testUser](httptest.NewRequest("GET", "/", nil)); ok {
		t.Error("GetCorrelationFromRequest on a bare request returned true, want false")
	}
}

// TestGetCorrelationConcurrentWithWrites exercises the read lock against
// concurrent writes; it is meaningful under -race.
func TestGetCorrelationConcurrentWithWrites(t *testing.T) {
	_, ctx := EnsureSRouterContext[int, testUser](context.Background())

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			WithUserID[int, testUser](ctx, i)
			WithTraceID[int, testUser](ctx, "trace")
			WithBuildID[int, testUser](ctx, "build")
		}(i)
		go func() {
			defer wg.Done()
			GetCorrelation[int, testUser](ctx)
		}()
	}
	wg.Wait()
}

type benchKey int

// benchCorrelationContext returns a context carrying every correlation value,
// wrapped in depth additional context.WithValue layers to model the wrappers a
// real request accumulates between net/http and the handler.
func benchCorrelationContext(depth int) context.Context {
	ctx := WithUserID[int, testUser](context.Background(), 123)
	ctx = WithTraceID[int, testUser](ctx, "trace-1")
	ctx = WithBuildID[int, testUser](ctx, "build-1")
	ctx = WithConfigID[int, testUser](ctx, "config-1")
	for i := 0; i < depth; i++ {
		ctx = context.WithValue(ctx, benchKey(i), i)
	}
	return ctx
}

var benchFields []zap.Field

// BenchmarkCorrelation compares stamping four correlation values onto a log
// entry with the individual accessors against one GetCorrelation call. Both
// arms build the same zap fields, since that is what a caller actually does
// with the values.
func BenchmarkCorrelation(b *testing.B) {
	for _, depth := range []int{0, 5} {
		ctx := benchCorrelationContext(depth)

		b.Run(fmt.Sprintf("individual/depth=%d", depth), func(b *testing.B) {
			for b.Loop() {
				fields := make([]zap.Field, 0, 4)
				if traceID := GetTraceIDFromContext[int, testUser](ctx); traceID != "" {
					fields = append(fields, zap.String("trace_id", traceID))
				}
				if buildID, ok := GetBuildID[int, testUser](ctx); ok {
					fields = append(fields, zap.String("build_id", buildID))
				}
				if configID, ok := GetConfigID[int, testUser](ctx); ok {
					fields = append(fields, zap.String("config_id", configID))
				}
				if userID, ok := GetUserID[int, testUser](ctx); ok {
					fields = append(fields, zap.Int("user_id", userID))
				}
				benchFields = fields
			}
		})

		b.Run(fmt.Sprintf("correlation/depth=%d", depth), func(b *testing.B) {
			for b.Loop() {
				var fields []zap.Field
				if c, ok := GetCorrelation[int, testUser](ctx); ok {
					fields = make([]zap.Field, 0, 4)
					if c.TraceIDSet {
						fields = append(fields, zap.String("trace_id", c.TraceID))
					}
					if c.BuildIDSet {
						fields = append(fields, zap.String("build_id", c.BuildID))
					}
					if c.ConfigIDSet {
						fields = append(fields, zap.String("config_id", c.ConfigID))
					}
					if c.UserIDSet {
						fields = append(fields, zap.Int("user_id", c.UserID))
					}
				}
				benchFields = fields
			}
		})
	}
}
