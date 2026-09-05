package scontext

import (
	"context"
	"fmt"
	"net/http/httptest"
	"sync"
	"testing"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// fieldMap indexes fields by key so assertions do not depend on slice order.
func fieldMap(fields []zap.Field) map[string]zap.Field {
	indexed := make(map[string]zap.Field, len(fields))
	for _, field := range fields {
		indexed[field.Key] = field
	}
	return indexed
}

func TestCorrelationFieldsReturnsEveryCorrelationValue(t *testing.T) {
	ctx := WithUserID[int, testUser](context.Background(), 123)
	ctx = WithTraceID[int, testUser](ctx, "trace-1")
	ctx = WithBuildID[int, testUser](ctx, "build-1")
	ctx = WithConfigID[int, testUser](ctx, "config-1")

	fields, userID, userIDSet := CorrelationFields[int, testUser](ctx)
	if !userIDSet || userID != 123 {
		t.Errorf("user ID = (%d, %v), want (123, true)", userID, userIDSet)
	}
	if len(fields) != 3 {
		t.Fatalf("fields = %+v, want 3 entries", fields)
	}

	indexed := fieldMap(fields)
	for key, want := range map[string]string{
		"trace_id":  "trace-1",
		"build_id":  "build-1",
		"config_id": "config-1",
	} {
		field, ok := indexed[key]
		if !ok {
			t.Errorf("field %q missing from %+v", key, fields)
			continue
		}
		if field.Type != zapcore.StringType || field.String != want {
			t.Errorf("field %q = %q (type %v), want %q as a string field", key, field.String, field.Type, want)
		}
	}
}

func TestCorrelationFieldsOmitsUnsetValues(t *testing.T) {
	ctx := WithTraceID[int, testUser](context.Background(), "trace-only")

	fields, _, userIDSet := CorrelationFields[int, testUser](ctx)
	if userIDSet {
		t.Error("userIDSet = true, want false when no user ID was written")
	}
	if len(fields) != 1 || fields[0].Key != "trace_id" {
		t.Fatalf("fields = %+v, want only trace_id", fields)
	}
}

func TestCorrelationFieldsKeepsSetEmptyValues(t *testing.T) {
	ctx := WithTraceID[int, testUser](context.Background(), "")
	ctx = WithBuildID[int, testUser](ctx, "")
	ctx = WithConfigID[int, testUser](ctx, "")
	ctx = WithUserID[int, testUser](ctx, 0)

	fields, userID, userIDSet := CorrelationFields[int, testUser](ctx)
	if !userIDSet || userID != 0 {
		t.Errorf("user ID = (%d, %v), want (0, true) for an explicitly set zero", userID, userIDSet)
	}
	if len(fields) != 3 {
		t.Fatalf("fields = %+v, want 3 entries for explicitly set empty values", fields)
	}
	for _, field := range fields {
		if field.String != "" {
			t.Errorf("field %q = %q, want empty", field.Key, field.String)
		}
	}
}

func TestCorrelationFieldsMatchesIndividualAccessors(t *testing.T) {
	ctx := createFullSRouterContext()

	fields, userID, userIDSet := CorrelationFields[int, testUser](ctx)

	wantUserID, wantUserIDSet := GetUserID[int, testUser](ctx)
	if userID != wantUserID || userIDSet != wantUserIDSet {
		t.Errorf("user ID = (%d, %v), want (%d, %v)", userID, userIDSet, wantUserID, wantUserIDSet)
	}

	indexed := fieldMap(fields)
	if want := GetTraceIDFromContext[int, testUser](ctx); indexed["trace_id"].String != want {
		t.Errorf("trace_id = %q, want %q", indexed["trace_id"].String, want)
	}
	if want, _ := GetBuildID[int, testUser](ctx); indexed["build_id"].String != want {
		t.Errorf("build_id = %q, want %q", indexed["build_id"].String, want)
	}
	if want, _ := GetConfigID[int, testUser](ctx); indexed["config_id"].String != want {
		t.Errorf("config_id = %q, want %q", indexed["config_id"].String, want)
	}
}

// TestCorrelationFieldsLeavesRoomForTheUserIDField pins the spare capacity the
// documented append pattern relies on.
func TestCorrelationFieldsLeavesRoomForTheUserIDField(t *testing.T) {
	ctx := WithUserID[int, testUser](context.Background(), 7)
	ctx = WithTraceID[int, testUser](ctx, "trace-1")
	ctx = WithBuildID[int, testUser](ctx, "build-1")
	ctx = WithConfigID[int, testUser](ctx, "config-1")

	fields, userID, _ := CorrelationFields[int, testUser](ctx)
	if cap(fields) < len(fields)+1 {
		t.Fatalf("cap(fields) = %d with len %d, want room for the user ID field", cap(fields), len(fields))
	}

	before := &fields[0]
	fields = append(fields, zap.Int("user_id", userID))
	if &fields[0] != before {
		t.Error("appending the user ID field reallocated the slice")
	}
}

func TestCorrelationFieldsWithoutSRouterContext(t *testing.T) {
	fields, userID, userIDSet := CorrelationFields[int, testUser](context.Background())
	if fields != nil {
		t.Errorf("fields = %+v, want nil", fields)
	}
	if userIDSet || userID != 0 {
		t.Errorf("user ID = (%d, %v), want (0, false)", userID, userIDSet)
	}
}

func TestCorrelationFieldsFromRequest(t *testing.T) {
	ctx := WithTraceID[int, testUser](context.Background(), "trace-req")
	req := httptest.NewRequest("GET", "/", nil).WithContext(ctx)

	fields, _, _ := CorrelationFieldsFromRequest[int, testUser](req)
	if len(fields) != 1 || fields[0].String != "trace-req" {
		t.Fatalf("fields = %+v, want trace_id=trace-req", fields)
	}

	if fields, _, _ := CorrelationFieldsFromRequest[int, testUser](httptest.NewRequest("GET", "/", nil)); fields != nil {
		t.Errorf("fields = %+v on a bare request, want nil", fields)
	}
}

// TestCorrelationFieldsConcurrentWithWrites exercises the read lock against
// concurrent identity writes; it is meaningful under -race.
func TestCorrelationFieldsConcurrentWithWrites(t *testing.T) {
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
			CorrelationFields[int, testUser](ctx)
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

// BenchmarkCorrelationFields compares stamping four correlation values with
// the individual accessors against one CorrelationFields call.
func BenchmarkCorrelationFields(b *testing.B) {
	for _, depth := range []int{0, 5} {
		ctx := benchCorrelationContext(depth)

		b.Run(fmt.Sprintf("individual/depth=%d", depth), func(b *testing.B) {
			for b.Loop() {
				fields := make([]zap.Field, 0, correlationFieldCapacity)
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
				fields, userID, ok := CorrelationFields[int, testUser](ctx)
				if ok {
					fields = append(fields, zap.Int("user_id", userID))
				}
				benchFields = fields
			}
		})
	}
}
