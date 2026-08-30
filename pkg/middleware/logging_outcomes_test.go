package middleware

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/common"
	"github.com/Suhaibinator/SRouter/pkg/scontext"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

type rejectingUserProvider struct {
	err error
}

func (p rejectingUserProvider) AuthenticateUser(*http.Request) (*string, error) {
	return nil, p.err
}

type rejectingRateLimiter struct {
	remaining int
	reset     time.Duration
}

func (l rejectingRateLimiter) Allow(string, int, time.Duration) (bool, int, time.Duration) {
	return false, l.remaining, l.reset
}

func TestAuthenticationRejectionsAreInfoWithRequestContext(t *testing.T) {
	tests := []struct {
		name       string
		middleware func(*zap.Logger) common.Middleware
		wantCause  string
		wantReason string
	}{
		{
			name: "ID provider rejects credentials",
			middleware: func(logger *zap.Logger) common.Middleware {
				return AuthenticationWithProvider[string, string](
					&BearerTokenProvider[string]{ValidTokens: map[string]string{}},
					logger,
				)
			},
			wantReason: "credentials rejected",
		},
		{
			name: "user provider returns authentication error",
			middleware: func(logger *zap.Logger) common.Middleware {
				return AuthenticationWithUserProvider[string](
					rejectingUserProvider{err: errors.New("credentials rejected by provider")},
					logger,
				)
			},
			wantCause: "credentials rejected by provider",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			core, logs := observer.New(zap.DebugLevel)
			handlerCalled := false
			handler := tt.middleware(zap.New(core))(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
				handlerCalled = true
			}))
			req := httptest.NewRequest(http.MethodPost, "/sessions", nil)
			req.RemoteAddr = "192.0.2.15:4321"
			req = req.WithContext(scontext.WithTraceID[string, string](req.Context(), "auth-trace"))
			rr := httptest.NewRecorder()

			handler.ServeHTTP(rr, req)

			if handlerCalled {
				t.Fatal("next handler was called after rejected authentication")
			}
			if rr.Code != http.StatusUnauthorized {
				t.Fatalf("response status = %d, want %d", rr.Code, http.StatusUnauthorized)
			}
			entries := logs.AllUntimed()
			if len(entries) != 1 {
				t.Fatalf("log entries = %d, want exactly 1: %#v", len(entries), entries)
			}
			entry := entries[0]
			if entry.Level != zapcore.InfoLevel || entry.Message != "Authentication failed" {
				t.Errorf("log = (%s, %q), want (info, Authentication failed)", entry.Level, entry.Message)
			}
			fields := entry.ContextMap()
			wants := map[string]any{
				"method":      http.MethodPost,
				"path":        "/sessions",
				"remote_addr": "192.0.2.15:4321",
				"status_code": int64(http.StatusUnauthorized),
				"trace_id":    "auth-trace",
			}
			if tt.wantCause != "" {
				wants["error"] = tt.wantCause
			}
			if tt.wantReason != "" {
				wants["reason"] = tt.wantReason
			}
			for key, want := range wants {
				if got := fields[key]; got != want {
					t.Errorf("%s = %#v, want %#v", key, got, want)
				}
			}
		})
	}
}

func TestAuthenticationRejectionGeneratesTraceWhenMissing(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)
	handler := AuthenticationWithProvider[string, any](
		&BearerTokenProvider[string]{ValidTokens: map[string]string{}},
		zap.New(core),
	)(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("next handler called")
	}))

	handler.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/protected", nil))

	entries := logs.AllUntimed()
	if len(entries) != 1 {
		t.Fatalf("log entries = %d, want 1", len(entries))
	}
	if traceID, ok := entries[0].ContextMap()["trace_id"].(string); !ok || traceID == "" {
		t.Errorf("trace_id = %#v, want generated non-empty string", entries[0].ContextMap()["trace_id"])
	}
}

func TestRateLimitExceededIsStructuredWarning(t *testing.T) {
	core, logs := observer.New(zap.DebugLevel)
	config := &common.RateLimitConfig[string, any]{
		BucketName: "login",
		Limit:      5,
		Window:     time.Minute,
		Strategy:   common.StrategyIP,
	}
	limiter := rejectingRateLimiter{
		remaining: 0,
		reset:     1500 * time.Millisecond,
	}
	handler := RateLimit(config, limiter, zap.New(core))(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {
		t.Fatal("next handler called after rate limit rejection")
	}))
	req := httptest.NewRequest(http.MethodPost, "/sessions", nil)
	req = req.WithContext(scontext.WithClientIP[string, any](req.Context(), "198.51.100.7"))
	rr := httptest.NewRecorder()

	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusTooManyRequests {
		t.Fatalf("response status = %d, want %d", rr.Code, http.StatusTooManyRequests)
	}
	if rr.Header().Get("Retry-After") != "1" {
		t.Errorf("Retry-After = %q, want 1", rr.Header().Get("Retry-After"))
	}
	entries := logs.AllUntimed()
	if len(entries) != 1 {
		t.Fatalf("log entries = %d, want exactly 1: %#v", len(entries), entries)
	}
	entry := entries[0]
	if entry.Level != zapcore.WarnLevel || entry.Message != "Rate limit exceeded" {
		t.Errorf("log = (%s, %q), want (warn, Rate limit exceeded)", entry.Level, entry.Message)
	}
	fields := entry.ContextMap()
	wants := map[string]any{
		"bucket":              "login",
		"key":                 "198.51.100.7",
		"strategy":            "IP",
		"limit":               int64(5),
		"remaining":           int64(0),
		"status_code":         int64(http.StatusTooManyRequests),
		"retry_after_seconds": int64(1),
		"method":              http.MethodPost,
		"path":                "/sessions",
	}
	for key, want := range wants {
		if got := fields[key]; got != want {
			t.Errorf("%s = %#v, want %#v", key, got, want)
		}
	}
}

func TestIDGeneratorBlockingAndFallbackPaths(t *testing.T) {
	generator := &IDGenerator{idChan: make(chan string, 1), stop: make(chan struct{})}
	generator.idChan <- "buffered-id"
	if got := generator.GetID(); got != "buffered-id" {
		t.Fatalf("GetID() = %q, want buffered ID", got)
	}

	got := generator.GetIDNonBlocking()
	if got == "" || got == "buffered-id" {
		t.Fatalf("GetIDNonBlocking() fallback = %q, want newly generated ID", got)
	}
}
