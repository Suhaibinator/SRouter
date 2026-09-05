package scontext

import (
	"context"
	"fmt"
	"net/http/httptest"
	"sync"
	"testing"
)

func TestIdentitySnapshotReadsEveryIdentity(t *testing.T) {
	ctx := WithUserID[int, testUser](context.Background(), 123)
	ctx = WithTraceID[int, testUser](ctx, "trace-1")
	ctx = WithBuildID[int, testUser](ctx, "build-1")
	ctx = WithConfigID[int, testUser](ctx, "config-1")
	ctx = WithClientInfo[int, testUser](ctx, "192.0.2.1", "test-agent")

	snapshot, ok := GetIdentitySnapshot[int, testUser](ctx)
	if !ok {
		t.Fatal("GetIdentitySnapshot returned false, want true")
	}
	if snapshot.UserID != 123 || !snapshot.UserIDSet {
		t.Errorf("UserID = (%d, %v), want (123, true)", snapshot.UserID, snapshot.UserIDSet)
	}
	if snapshot.TraceID != "trace-1" || !snapshot.TraceIDSet {
		t.Errorf("TraceID = (%q, %v), want (trace-1, true)", snapshot.TraceID, snapshot.TraceIDSet)
	}
	if snapshot.BuildID != "build-1" || !snapshot.BuildIDSet {
		t.Errorf("BuildID = (%q, %v), want (build-1, true)", snapshot.BuildID, snapshot.BuildIDSet)
	}
	if snapshot.ConfigID != "config-1" || !snapshot.ConfigIDSet {
		t.Errorf("ConfigID = (%q, %v), want (config-1, true)", snapshot.ConfigID, snapshot.ConfigIDSet)
	}
	if snapshot.ClientIP != "192.0.2.1" || !snapshot.ClientIPSet {
		t.Errorf("ClientIP = (%q, %v), want (192.0.2.1, true)", snapshot.ClientIP, snapshot.ClientIPSet)
	}
	if snapshot.UserAgent != "test-agent" || !snapshot.UserAgentSet {
		t.Errorf("UserAgent = (%q, %v), want (test-agent, true)", snapshot.UserAgent, snapshot.UserAgentSet)
	}
}

func TestIdentitySnapshotMatchesIndividualAccessors(t *testing.T) {
	ctx := createFullSRouterContext()

	snapshot, ok := GetIdentitySnapshot[int, testUser](ctx)
	if !ok {
		t.Fatal("GetIdentitySnapshot returned false, want true")
	}

	userID, userIDSet := GetUserID[int, testUser](ctx)
	if snapshot.UserID != userID || snapshot.UserIDSet != userIDSet {
		t.Errorf("UserID = (%d, %v), want (%d, %v)", snapshot.UserID, snapshot.UserIDSet, userID, userIDSet)
	}
	if traceID := GetTraceIDFromContext[int, testUser](ctx); snapshot.TraceID != traceID {
		t.Errorf("TraceID = %q, want %q", snapshot.TraceID, traceID)
	}
	buildID, buildIDSet := GetBuildID[int, testUser](ctx)
	if snapshot.BuildID != buildID || snapshot.BuildIDSet != buildIDSet {
		t.Errorf("BuildID = (%q, %v), want (%q, %v)", snapshot.BuildID, snapshot.BuildIDSet, buildID, buildIDSet)
	}
	configID, configIDSet := GetConfigID[int, testUser](ctx)
	if snapshot.ConfigID != configID || snapshot.ConfigIDSet != configIDSet {
		t.Errorf("ConfigID = (%q, %v), want (%q, %v)", snapshot.ConfigID, snapshot.ConfigIDSet, configID, configIDSet)
	}
	clientIP, clientIPSet := GetClientIP[int, testUser](ctx)
	if snapshot.ClientIP != clientIP || snapshot.ClientIPSet != clientIPSet {
		t.Errorf("ClientIP = (%q, %v), want (%q, %v)", snapshot.ClientIP, snapshot.ClientIPSet, clientIP, clientIPSet)
	}
	userAgent, userAgentSet := GetUserAgent[int, testUser](ctx)
	if snapshot.UserAgent != userAgent || snapshot.UserAgentSet != userAgentSet {
		t.Errorf("UserAgent = (%q, %v), want (%q, %v)", snapshot.UserAgent, snapshot.UserAgentSet, userAgent, userAgentSet)
	}
}

func TestIdentitySnapshotDistinguishesSetEmptyValues(t *testing.T) {
	ctx := WithTraceID[int, testUser](context.Background(), "")
	ctx = WithBuildID[int, testUser](ctx, "")
	ctx = WithConfigID[int, testUser](ctx, "")
	ctx = WithUserID[int, testUser](ctx, 0)

	snapshot, ok := GetIdentitySnapshot[int, testUser](ctx)
	if !ok {
		t.Fatal("GetIdentitySnapshot returned false, want true")
	}
	if !snapshot.TraceIDSet || !snapshot.BuildIDSet || !snapshot.ConfigIDSet || !snapshot.UserIDSet {
		t.Errorf("set flags = %+v, want all true for explicitly set zero values", snapshot)
	}
	if snapshot.ClientIPSet || snapshot.UserAgentSet {
		t.Errorf("ClientIPSet = %v, UserAgentSet = %v, want both false", snapshot.ClientIPSet, snapshot.UserAgentSet)
	}
}

func TestIdentitySnapshotWithoutSRouterContext(t *testing.T) {
	snapshot, ok := GetIdentitySnapshot[int, testUser](context.Background())
	if ok {
		t.Fatal("GetIdentitySnapshot returned true, want false")
	}
	if snapshot != (IdentitySnapshot[int]{}) {
		t.Errorf("snapshot = %+v, want zero value", snapshot)
	}
}

func TestGetIdentitySnapshotFromRequest(t *testing.T) {
	ctx := WithTraceID[int, testUser](context.Background(), "trace-req")
	req := httptest.NewRequest("GET", "/", nil).WithContext(ctx)

	snapshot, ok := GetIdentitySnapshotFromRequest[int, testUser](req)
	if !ok || snapshot.TraceID != "trace-req" {
		t.Fatalf("GetIdentitySnapshotFromRequest = (%q, %v), want (trace-req, true)", snapshot.TraceID, ok)
	}

	if _, ok := GetIdentitySnapshotFromRequest[int, testUser](httptest.NewRequest("GET", "/", nil)); ok {
		t.Error("GetIdentitySnapshotFromRequest on a bare request returned true, want false")
	}
}

// TestIdentitySnapshotConcurrentWithWrites exercises the read lock against
// concurrent identity writes; it is meaningful under -race.
func TestIdentitySnapshotConcurrentWithWrites(t *testing.T) {
	_, ctx := EnsureSRouterContext[int, testUser](context.Background())

	var wg sync.WaitGroup
	for i := 0; i < 8; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			WithUserID[int, testUser](ctx, i)
			WithTraceID[int, testUser](ctx, "trace")
			WithBuildID[int, testUser](ctx, "build")
		}(i)
		wg.Add(1)
		go func() {
			defer wg.Done()
			GetIdentitySnapshot[int, testUser](ctx)
		}()
	}
	wg.Wait()
}

type benchKey int

// benchIdentityContext returns a context carrying every identity, wrapped in
// depth additional context.WithValue layers to model the wrappers a real
// request accumulates between net/http and the handler.
func benchIdentityContext(depth int) context.Context {
	ctx := WithUserID[int, testUser](context.Background(), 123)
	ctx = WithTraceID[int, testUser](ctx, "trace-1")
	ctx = WithBuildID[int, testUser](ctx, "build-1")
	ctx = WithConfigID[int, testUser](ctx, "config-1")
	for i := 0; i < depth; i++ {
		ctx = context.WithValue(ctx, benchKey(i), i)
	}
	return ctx
}

func BenchmarkIdentityAccessors(b *testing.B) {
	for _, depth := range []int{0, 5} {
		ctx := benchIdentityContext(depth)

		b.Run(fmt.Sprintf("individual/depth=%d", depth), func(b *testing.B) {
			for b.Loop() {
				_ = GetTraceIDFromContext[int, testUser](ctx)
				_, _ = GetBuildID[int, testUser](ctx)
				_, _ = GetConfigID[int, testUser](ctx)
				_, _ = GetUserID[int, testUser](ctx)
			}
		})

		b.Run(fmt.Sprintf("snapshot/depth=%d", depth), func(b *testing.B) {
			for b.Loop() {
				_, _ = GetIdentitySnapshot[int, testUser](ctx)
			}
		})
	}
}
