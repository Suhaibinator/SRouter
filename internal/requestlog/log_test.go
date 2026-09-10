package requestlog

import (
	"context"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/scontext"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestCheckUsesNormalizedContextIPOnly(t *testing.T) {
	for _, tc := range []struct{ name, contextIP, remote, wantIP string }{
		{"context wins", "198.51.100.9:1234", "192.0.2.1:9000", "198.51.100.9"},
		{"missing context does not infer peer", "", "192.0.2.1:9000", ""},
		{"IPv6 context normalized", "[2001:db8::1]:9000", "192.0.2.1:9000", "[2001:db8::1]"},
		{"missing addresses", "", "", ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			core, logs := observer.New(zapcore.DebugLevel)
			source := scontext.NewRequestLoggerSource[string](zap.New(core, zap.AddCaller()).Named("app").With(zap.String("static", "kept")), nil)
			req := httptest.NewRequest("GET", "/test", nil)
			req.RemoteAddr = tc.remote
			req.Header.Set("X-Forwarded-For", "203.0.113.99")
			ctx := scontext.WithRequestLogger[string, any](req.Context(), source)
			ctx = scontext.WithClientIP[string, any](ctx, tc.contextIP)
			ctx = scontext.WithTraceID[string, any](ctx, "context-trace")
			ctx = scontext.WithBuildID[string, any](ctx, "build-1")
			ctx = scontext.WithUserID[string, any](ctx, "user-1")
			req = req.WithContext(ctx)
			if ce := Check[string, any](req, zapcore.WarnLevel, "test"); ce != nil {
				ce.Write(zap.String("method", req.Method))
			}
			entries := logs.All()
			if len(entries) != 1 {
				t.Fatalf("logs = %d", len(entries))
			}
			entry := entries[0]
			if entry.LoggerName != "app.SRouter" {
				t.Errorf("logger name = %q", entry.LoggerName)
			}
			if !strings.HasSuffix(entry.Caller.File, "log_test.go") {
				t.Errorf("caller = %s", entry.Caller.File)
			}
			got := entry.ContextMap()
			for k, want := range map[string]any{"static": "kept", "trace_id": "context-trace", "build_id": "build-1", "user_id": "user-1", "method": "GET"} {
				if got[k] != want {
					t.Errorf("%s = %#v, want %#v", k, got[k], want)
				}
			}
			if tc.wantIP == "" {
				if _, found := got["client_ip"]; found {
					t.Errorf("unexpected client_ip: %#v", got)
				}
			} else if got["client_ip"] != tc.wantIP {
				t.Errorf("IP = %#v, want %q", got["client_ip"], tc.wantIP)
			}
			if ip, _ := scontext.GetClientIP[string, any](ctx); ip != tc.wantIP {
				t.Errorf("stored client IP = %q, want %q", ip, tc.wantIP)
			}
			seen := map[string]bool{}
			for _, field := range entry.Context {
				if seen[field.Key] {
					t.Errorf("duplicate field %q", field.Key)
				}
				seen[field.Key] = true
			}
		})
	}
}

func TestCheckMissingSourceAndDisabledLevel(t *testing.T) {
	req := httptest.NewRequest("GET", "/test", nil)
	if ce := Check[string, any](req, zapcore.ErrorLevel, "without source"); ce != nil {
		t.Fatal("unexpected entry without source")
	}
	if _, ok := scontext.GetSRouterContext[string, any](req.Context()); ok {
		t.Fatal("logging installed context")
	}
	core, logs := observer.New(zapcore.ErrorLevel)
	ctx := scontext.WithRequestLogger[string, any](req.Context(), scontext.NewRequestLoggerSource[string](zap.New(core), nil))
	if ce := Check[string, any](req.WithContext(ctx), zapcore.InfoLevel, "disabled"); ce != nil {
		t.Fatal("disabled entry was checked")
	}
	if logs.Len() != 0 {
		t.Fatal("disabled level was emitted")
	}
}

func TestCheckConcurrentClientIPWritesDoNotDuplicateFields(t *testing.T) {
	core, logs := observer.New(zapcore.InfoLevel)
	req := httptest.NewRequest("GET", "/test", nil)
	ctx := scontext.WithRequestLogger[string, any](req.Context(), scontext.NewRequestLoggerSource[string](zap.New(core), nil))
	req = req.WithContext(ctx)
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for range 500 {
			scontext.WithClientIP[string, any](ctx, "198.51.100.1")
			scontext.WithClientIP[string, any](ctx, "")
		}
	}()
	for range 500 {
		if ce := Check[string, any](req, zapcore.InfoLevel, "concurrent"); ce != nil {
			ce.Write()
		}
	}
	wg.Wait()
	for _, entry := range logs.All() {
		count := 0
		for _, field := range entry.Context {
			if field.Key == "client_ip" {
				count++
			}
		}
		if count > 1 {
			t.Fatalf("duplicate client_ip: %#v", entry.Context)
		}
	}
}

// Sampling must be checked exactly once, before per-event field construction.
func TestCheckSkipsFieldsForDisabledAndSampledRecords(t *testing.T) {
	core, logs := observer.New(zapcore.InfoLevel)
	sampled := zapcore.NewSamplerWithOptions(core, time.Hour, 1, 0)
	ctx := scontext.WithRequestLogger[string, any](context.Background(), scontext.NewRequestLoggerSource[string](zap.New(sampled), nil))
	ctx = scontext.WithClientIP[string, any](ctx, "192.0.2.1")
	req := httptest.NewRequest("GET", "/sample", nil).WithContext(ctx)
	constructed := 0
	field := func() zap.Field { constructed++; return zap.Int("constructed", constructed) }
	for range 3 {
		if ce := Check[string, any](req, zapcore.DebugLevel, "disabled"); ce != nil {
			ce.Write(field())
		}
		if ce := Check[string, any](req, zapcore.InfoLevel, "sampled"); ce != nil {
			ce.Write(field())
		}
	}
	if constructed != 1 || logs.Len() != 1 {
		t.Fatalf("constructed=%d logs=%d, want one of each", constructed, logs.Len())
	}
}

func TestCheckDisabledWarmPathDoesNotAllocate(t *testing.T) {
	ctx := scontext.WithRequestLogger[string, any](context.Background(), scontext.NewRequestLoggerSource[string](zap.NewNop(), nil))
	ctx = scontext.WithTraceID[string, any](ctx, "trace")
	ctx = scontext.WithClientIP[string, any](ctx, "192.0.2.1")
	req := httptest.NewRequest("GET", "/disabled", nil).WithContext(ctx)
	_ = Check[string, any](req, zapcore.DebugLevel, "disabled")
	allocs := testing.AllocsPerRun(1000, func() {
		if ce := Check[string, any](req, zapcore.DebugLevel, "disabled"); ce != nil {
			panic("unexpected enabled entry")
		}
	})
	if allocs != 0 {
		t.Fatalf("disabled warm path allocations = %v, want 0", allocs)
	}
}

func TestCheckReusesCachedLoggerWithoutClientIP(t *testing.T) {
	core, logs := observer.New(zapcore.InfoLevel)
	encodes := 0
	source := scontext.NewRequestLoggerSource[string](zap.New(core), func(id string) zap.Field {
		encodes++
		return zap.String("user_id", id)
	})
	ctx := scontext.WithRequestLogger[string, any](context.Background(), source)
	ctx = scontext.WithUserID[string, any](ctx, "user")
	warmed, _ := scontext.GetLogger[string, any](ctx)
	req := httptest.NewRequest("GET", "/test", nil).WithContext(ctx)
	req.RemoteAddr = "192.0.2.1:1234"
	for range 5 {
		if ce := Check[string, any](req, zapcore.InfoLevel, "event"); ce != nil {
			ce.Write()
		}
	}
	current, _ := scontext.GetLogger[string, any](ctx)
	if encodes != 1 || current != warmed {
		t.Fatalf("logging rederived cached fields: encodes=%d", encodes)
	}
	if _, ok := scontext.GetClientIP[string, any](ctx); ok {
		t.Fatal("logging installed a peer IP")
	}
	for _, entry := range logs.All() {
		if _, ok := entry.ContextMap()["client_ip"]; ok {
			t.Fatal("logging inferred a peer IP")
		}
	}
}
