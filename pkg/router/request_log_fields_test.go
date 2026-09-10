package router

import (
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/common"
	"github.com/Suhaibinator/SRouter/pkg/logkeys"
	"github.com/Suhaibinator/SRouter/pkg/scontext"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

const deprecatedIPLogField = "ip"

func assertObservedFieldOnce(t *testing.T, entry observer.LoggedEntry, key string, want any) {
	t.Helper()
	if got := entry.ContextMap()[key]; got != want {
		t.Errorf("%s = %#v, want %#v", key, got, want)
	}
	count := 0
	for _, field := range entry.Context {
		if field.Key == key {
			count++
		}
	}
	if count != 1 {
		t.Errorf("%s field count = %d, want 1; fields: %#v", key, count, entry.Context)
	}
}

func assertObservedFieldKeys(t *testing.T, entry observer.LoggedEntry, want []string) {
	t.Helper()
	got := make([]string, 0, len(entry.Context))
	for _, field := range entry.Context {
		got = append(got, field.Key)
	}
	if !slices.Equal(got, want) {
		t.Errorf("field keys = %v, want %v", got, want)
	}
}

// TestRateLimitAndSummaryShareRequestCorrelation reproduces the production
// symptom that prompted the shared-logger change. A rejected IP bucket keeps
// its limiter key while both records carry the same canonical client and trace
// fields exactly once.
func TestRateLimitAndSummaryShareRequestCorrelation(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	r := NewRouter[string, struct{}](RouterConfig{
		Logger:             zap.New(core),
		EnableTraceLogging: true,
		GlobalRateLimit: &common.RateLimitConfig[any, any]{
			BucketName: "request-log-fields",
			Limit:      1,
			Window:     time.Minute,
			Strategy:   common.StrategyIP,
		},
	}, RouterDependencies[string, struct{}]{})
	r.Route(RouteConfigBase{
		Path:    "/limited",
		Methods: []HttpMethod{MethodGet},
		Handler: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		},
	})

	serve := func(trace string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodGet, "/limited", nil)
		req.RemoteAddr = "213.209.159.133:53127"
		req = req.WithContext(scontext.WithTraceID[string, struct{}](req.Context(), trace))
		rec := httptest.NewRecorder()
		r.ServeHTTP(rec, req)
		return rec
	}

	if got := serve("first-trace").Code; got != http.StatusNoContent {
		t.Fatalf("first response status = %d, want %d", got, http.StatusNoContent)
	}
	if got := serve("rejected-trace").Code; got != http.StatusTooManyRequests {
		t.Fatalf("second response status = %d, want %d", got, http.StatusTooManyRequests)
	}

	warnings := logs.FilterMessage("Rate limit exceeded").AllUntimed()
	if len(warnings) != 1 {
		t.Fatalf("rate-limit warnings = %d, want 1: %#v", len(warnings), logs.AllUntimed())
	}
	warning := warnings[0]
	if warning.LoggerName != "SRouter" {
		t.Errorf("rate-limit logger name = %q, want %q", warning.LoggerName, "SRouter")
	}
	assertObservedFieldKeys(t, warning, []string{
		logkeys.TraceID,
		logkeys.ClientIP,
		logkeys.Bucket,
		logkeys.Key,
		logkeys.Strategy,
		logkeys.Limit,
		logkeys.Window,
		logkeys.Remaining,
		logkeys.ResetDuration,
		logkeys.StatusCode,
		logkeys.RetryAfterSeconds,
		logkeys.Method,
		logkeys.Path,
	})
	assertObservedFieldOnce(t, warning, logkeys.Key, "213.209.159.133")
	assertObservedFieldOnce(t, warning, logkeys.ClientIP, "213.209.159.133")
	assertObservedFieldOnce(t, warning, logkeys.TraceID, "rejected-trace")
	assertObservedFieldOnce(t, warning, logkeys.Method, http.MethodGet)
	assertObservedFieldOnce(t, warning, logkeys.Path, "/limited")

	summaries := logs.FilterMessage("Request summary statistics").AllUntimed()
	if len(summaries) != 2 {
		t.Fatalf("request summaries = %d, want 2: %#v", len(summaries), logs.AllUntimed())
	}
	rejectedSummary := summaries[1]
	assertObservedFieldKeys(t, rejectedSummary, []string{
		logkeys.TraceID,
		logkeys.ClientIP,
		logkeys.Method,
		logkeys.Path,
		logkeys.Status,
		logkeys.Duration,
		logkeys.Bytes,
		logkeys.UserAgent,
	})
	assertObservedFieldOnce(t, rejectedSummary, logkeys.ClientIP, "213.209.159.133")
	assertObservedFieldOnce(t, rejectedSummary, logkeys.TraceID, "rejected-trace")
	assertObservedFieldOnce(t, rejectedSummary, logkeys.Method, http.MethodGet)
	assertObservedFieldOnce(t, rejectedSummary, logkeys.Path, "/limited")
	if _, present := rejectedSummary.ContextMap()[deprecatedIPLogField]; present {
		t.Errorf("deprecated %q field is still emitted: %#v", deprecatedIPLogField, rejectedSummary.Context)
	}
}

func TestLazyBuildFailureUsesRequestLoggerInstalledAtBoundary(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	r := NewRouter[string, struct{}](RouterConfig{
		Logger:        zap.New(core),
		GlobalTimeout: -time.Second,
	}, RouterDependencies[string, struct{}]{
		BuildID:  func() string { return "failed-build" },
		ConfigID: func() string { return "failed-config" },
	})
	req := httptest.NewRequest(http.MethodGet, "/before-build", nil)
	req.RemoteAddr = "[2001:db8::20]:8443"
	req = req.WithContext(scontext.WithTraceID[string, struct{}](req.Context(), "build-trace"))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusInternalServerError {
		t.Fatalf("response status = %d, want %d", rec.Code, http.StatusInternalServerError)
	}
	entries := logs.FilterMessage("Failed to build route tree").AllUntimed()
	if len(entries) != 1 {
		t.Fatalf("build-failure logs = %d, want 1: %#v", len(entries), logs.AllUntimed())
	}
	entry := entries[0]
	if entry.LoggerName != "SRouter" {
		t.Errorf("build-failure logger name = %q, want %q", entry.LoggerName, "SRouter")
	}
	assertObservedFieldOnce(t, entry, logkeys.ClientIP, "[2001:db8::20]")
	assertObservedFieldOnce(t, entry, logkeys.TraceID, "build-trace")
	assertObservedFieldOnce(t, entry, logkeys.BuildID, "failed-build")
	assertObservedFieldOnce(t, entry, logkeys.ConfigID, "failed-config")
	assertObservedFieldOnce(t, entry, logkeys.Method, http.MethodGet)
	assertObservedFieldOnce(t, entry, logkeys.Path, "/before-build")
}

func TestUnmatchedRequestSummaryUsesCanonicalRequestFields(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	r := NewRouter[string, struct{}](RouterConfig{
		Logger:             zap.New(core),
		EnableTraceLogging: true,
	}, RouterDependencies[string, struct{}]{})
	req := httptest.NewRequest(http.MethodGet, "/missing", nil)
	req.RemoteAddr = "198.51.100.14:9000"
	req = req.WithContext(scontext.WithTraceID[string, struct{}](req.Context(), "missing-trace"))
	rec := httptest.NewRecorder()

	r.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("response status = %d, want %d", rec.Code, http.StatusNotFound)
	}
	entries := logs.FilterMessage("Request summary statistics").AllUntimed()
	if len(entries) != 1 {
		t.Fatalf("request summaries = %d, want 1: %#v", len(entries), logs.AllUntimed())
	}
	entry := entries[0]
	assertObservedFieldOnce(t, entry, logkeys.ClientIP, "198.51.100.14")
	assertObservedFieldOnce(t, entry, logkeys.TraceID, "missing-trace")
	assertObservedFieldOnce(t, entry, logkeys.Method, http.MethodGet)
	assertObservedFieldOnce(t, entry, logkeys.Path, "/missing")
	if _, present := entry.ContextMap()[deprecatedIPLogField]; present {
		t.Errorf("deprecated %q field is still emitted: %#v", deprecatedIPLogField, entry.Context)
	}
}

func TestRequestSummaryClientIPHonorsProxyTrust(t *testing.T) {
	tests := []struct {
		name      string
		trust     bool
		wantIP    string
		traceID   string
		remote    string
		forwarded string
	}{
		{
			name:      "trusted proxy uses nearest forwarded address",
			trust:     true,
			wantIP:    "198.51.100.8",
			traceID:   "trusted-trace",
			remote:    "192.0.2.10:8443",
			forwarded: "203.0.113.99, 198.51.100.8",
		},
		{
			name:      "untrusted proxy ignores forwarded address",
			trust:     false,
			wantIP:    "192.0.2.10",
			traceID:   "untrusted-trace",
			remote:    "192.0.2.10:8443",
			forwarded: "203.0.113.99, 198.51.100.8",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core, logs := observer.New(zapcore.DebugLevel)
			r := NewRouter[string, struct{}](RouterConfig{
				Logger:             zap.New(core),
				EnableTraceLogging: true,
				IPConfig: &IPConfig{
					Source:     IPSourceXForwardedFor,
					TrustProxy: tt.trust,
				},
			}, RouterDependencies[string, struct{}]{})
			r.Route(RouteConfigBase{
				Path:    "/proxy",
				Methods: []HttpMethod{MethodGet},
				Handler: func(w http.ResponseWriter, _ *http.Request) {
					w.WriteHeader(http.StatusNoContent)
				},
			})
			req := httptest.NewRequest(http.MethodGet, "/proxy", nil)
			req.RemoteAddr = tt.remote
			req.Header.Set("X-Forwarded-For", tt.forwarded)
			req = req.WithContext(scontext.WithTraceID[string, struct{}](req.Context(), tt.traceID))

			r.ServeHTTP(httptest.NewRecorder(), req)

			entries := logs.FilterMessage("Request summary statistics").AllUntimed()
			if len(entries) != 1 {
				t.Fatalf("request summaries = %d, want 1: %#v", len(entries), logs.AllUntimed())
			}
			assertObservedFieldOnce(t, entries[0], logkeys.ClientIP, tt.wantIP)
			assertObservedFieldOnce(t, entries[0], logkeys.TraceID, tt.traceID)
			if _, present := entries[0].ContextMap()[deprecatedIPLogField]; present {
				t.Errorf("deprecated %q field is still emitted: %#v", deprecatedIPLogField, entries[0].Context)
			}
		})
	}
}
