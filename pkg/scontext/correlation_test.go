package scontext

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"go.uber.org/zap"
)

func TestGetCorrelationReturnsEveryValue(t *testing.T) {
	ctx := WithUserID[int, testUser](context.Background(), 123)
	ctx = WithTraceID[int, testUser](ctx, "trace-1")
	ctx = WithBuildID[int, testUser](ctx, "build-1")
	ctx = WithConfigID[int, testUser](ctx, "config-1")

	c, ok := GetCorrelation[int](ctx)
	if !ok {
		t.Fatal("GetCorrelation returned false, want true")
	}
	if c.TraceID != "trace-1" || !c.HasTraceID() {
		t.Errorf("TraceID = (%q, %v), want (trace-1, true)", c.TraceID, c.HasTraceID())
	}
	if c.BuildID != "build-1" || !c.HasBuildID() {
		t.Errorf("BuildID = (%q, %v), want (build-1, true)", c.BuildID, c.HasBuildID())
	}
	if c.ConfigID != "config-1" || !c.HasConfigID() {
		t.Errorf("ConfigID = (%q, %v), want (config-1, true)", c.ConfigID, c.HasConfigID())
	}
	if c.UserID != 123 || !c.HasUserID() {
		t.Errorf("UserID = (%d, %v), want (123, true)", c.UserID, c.HasUserID())
	}
}

func TestGetCorrelationReportsUnsetValues(t *testing.T) {
	ctx := WithTraceID[int, testUser](context.Background(), "trace-only")

	c, ok := GetCorrelation[int](ctx)
	if !ok {
		t.Fatal("GetCorrelation returned false, want true")
	}
	if !c.HasTraceID() {
		t.Error("HasTraceID() = false, want true")
	}
	if c.HasBuildID() || c.HasConfigID() || c.HasUserID() {
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

	c, ok := GetCorrelation[int](ctx)
	if !ok {
		t.Fatal("GetCorrelation returned false, want true")
	}
	if !c.HasTraceID() || !c.HasBuildID() || !c.HasConfigID() || !c.HasUserID() {
		t.Errorf("set flags = %+v, want all true for explicitly set zero values", c)
	}
}

func TestGetCorrelationMatchesIndividualAccessors(t *testing.T) {
	ctx := createFullSRouterContext()

	c, ok := GetCorrelation[int](ctx)
	if !ok {
		t.Fatal("GetCorrelation returned false, want true")
	}
	if want := GetTraceID(ctx); c.TraceID != want {
		t.Errorf("TraceID = %q, want %q", c.TraceID, want)
	}
	buildID, buildIDSet := GetBuildID(ctx)
	if c.BuildID != buildID || c.HasBuildID() != buildIDSet {
		t.Errorf("BuildID = (%q, %v), want (%q, %v)", c.BuildID, c.HasBuildID(), buildID, buildIDSet)
	}
	configID, configIDSet := GetConfigID(ctx)
	if c.ConfigID != configID || c.HasConfigID() != configIDSet {
		t.Errorf("ConfigID = (%q, %v), want (%q, %v)", c.ConfigID, c.HasConfigID(), configID, configIDSet)
	}
	userID, userIDSet := GetUserID[int](ctx)
	if c.UserID != userID || c.HasUserID() != userIDSet {
		t.Errorf("UserID = (%d, %v), want (%d, %v)", c.UserID, c.HasUserID(), userID, userIDSet)
	}
}

// TestCorrelationDoesNotObserveLaterWrites pins the point-in-time semantics
// the documentation promises.
func TestCorrelationDoesNotObserveLaterWrites(t *testing.T) {
	_, ctx := EnsureSRouterContext[int, testUser](context.Background())
	ctx = WithBuildID[int, testUser](ctx, "build-1")

	c, _ := GetCorrelation[int](ctx)

	WithBuildID[int, testUser](ctx, "build-2")

	if c.BuildID != "build-1" {
		t.Errorf("BuildID = %q after a later write, want the value read at call time", c.BuildID)
	}
}

func TestGetCorrelationWithoutSRouterContext(t *testing.T) {
	c, ok := GetCorrelation[int](context.Background())
	if ok {
		t.Fatal("GetCorrelation returned true, want false")
	}
	if c != (Correlation[int]{}) {
		t.Errorf("correlation = %+v, want zero value", c)
	}
}

// TestGetCorrelationConcurrentWithWrites exercises the read lock against
// concurrent writes; it is meaningful under -race.
func TestGetCorrelationConcurrentWithWrites(t *testing.T) {
	_, ctx := EnsureSRouterContext[int, testUser](context.Background())

	var wg sync.WaitGroup
	for i := range 8 {
		wg.Add(2)
		go func(i int) {
			defer wg.Done()
			WithUserID[int, testUser](ctx, i)
			WithTraceID[int, testUser](ctx, "trace")
			WithBuildID[int, testUser](ctx, "build")
		}(i)
		go func() {
			defer wg.Done()
			GetCorrelation[int](ctx)
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
	for i := range depth {
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
				if traceID := GetTraceID(ctx); traceID != "" {
					fields = append(fields, zap.String("trace_id", traceID))
				}
				if buildID, ok := GetBuildID(ctx); ok {
					fields = append(fields, zap.String("build_id", buildID))
				}
				if configID, ok := GetConfigID(ctx); ok {
					fields = append(fields, zap.String("config_id", configID))
				}
				if userID, ok := GetUserID[int](ctx); ok {
					fields = append(fields, zap.Int("user_id", userID))
				}
				benchFields = fields
			}
		})

		b.Run(fmt.Sprintf("correlation/depth=%d", depth), func(b *testing.B) {
			for b.Loop() {
				var fields []zap.Field
				if c, ok := GetCorrelation[int](ctx); ok {
					fields = make([]zap.Field, 0, 4)
					if c.HasTraceID() {
						fields = append(fields, zap.String("trace_id", c.TraceID))
					}
					if c.HasBuildID() {
						fields = append(fields, zap.String("build_id", c.BuildID))
					}
					if c.HasConfigID() {
						fields = append(fields, zap.String("config_id", c.ConfigID))
					}
					if c.HasUserID() {
						fields = append(fields, zap.Int("user_id", c.UserID))
					}
				}
				benchFields = fields
			}
		})
	}
}

func TestCorrelationPresenceCombinations(t *testing.T) {
	setters := []func(context.Context) context.Context{
		func(ctx context.Context) context.Context { return WithTraceID[int, testUser](ctx, "") },
		func(ctx context.Context) context.Context { return WithBuildID[int, testUser](ctx, "") },
		func(ctx context.Context) context.Context { return WithConfigID[int, testUser](ctx, "") },
		func(ctx context.Context) context.Context { return WithUserID[int, testUser](ctx, 0) },
	}
	clears := []func(context.Context) context.Context{
		ClearTraceID[int, testUser], ClearBuildID[int, testUser],
		ClearConfigID[int, testUser], ClearUserID[int, testUser],
	}
	presence := func(c Correlation[int]) [4]bool {
		return [4]bool{c.HasTraceID(), c.HasBuildID(), c.HasConfigID(), c.HasUserID()}
	}
	if got := presence(Correlation[int]{}); got != ([4]bool{}) {
		t.Fatalf("zero snapshot presence = %v", got)
	}
	for combination := range 16 {
		_, ctx := EnsureSRouterContext[int, testUser](context.Background())
		var want [4]bool
		for i, set := range setters {
			if combination&(1<<i) != 0 {
				ctx = set(ctx)
				want[i] = true
			}
		}
		snapshot, ok := GetCorrelation[int](ctx)
		if !ok || presence(snapshot) != want {
			t.Fatalf("combination %d: presence = %v, want %v", combination, presence(snapshot), want)
		}
		for i, clear := range clears {
			clear(ctx)
			want[i] = false
			current, _ := GetCorrelation[int](ctx)
			if presence(current) != want {
				t.Fatalf("combination %d, clear %d: presence = %v, want %v", combination, i, presence(current), want)
			}
		}
		for i, has := range presence(snapshot) {
			if has != (combination&(1<<i) != 0) {
				t.Fatal("clear mutated existing snapshot")
			}
		}
	}
}

func TestCorrelationIgnoresUnrelatedPresence(t *testing.T) {
	ctx := WithUserID[int, testUser](context.Background(), 42)
	before, _ := GetCorrelation[int](ctx)
	// Populate every non-correlation presence bit; snapshot equality must depend
	// only on correlation, not other state on the source wrapper.
	WithUser[int, testUser](ctx, new(testUser))
	WithClientInfo[int, testUser](ctx, "127.0.0.1", "agent")
	WithTransaction[int, testUser](ctx, nil)
	WithRouteInfo[int, testUser](ctx, nil, "/users")
	WithCORSInfo[int, testUser](ctx, "", false)
	WithCORSRequestedHeaders[int, testUser](ctx, "")
	WithHandlerError[int, testUser](ctx, nil)
	after, _ := GetCorrelation[int](ctx)
	if before != after {
		t.Fatal("unrelated presence changed correlation snapshot")
	}
	ClearIdentity[int, testUser](ctx)
	cleared, _ := GetCorrelation[int](ctx)
	if cleared != (Correlation[int]{}) {
		t.Fatal("cleared correlation differs from zero snapshot")
	}
}
