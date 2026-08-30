package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/common"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func TestRateLimitInvariantViolationsAreSingleStructuredErrors(t *testing.T) {
	tests := []struct {
		name      string
		config    *common.RateLimitConfig[string, any]
		invariant string
		expected  string
		actual    any
		fallback  string
	}{
		{
			name: "missing client IP context uses remote address",
			config: &common.RateLimitConfig[string, any]{
				BucketName: "ip-bucket", Limit: 10, Window: time.Minute, Strategy: common.StrategyIP,
			},
			invariant: "rate_limit_client_ip_context_present",
			expected:  "non-empty client IP in request context",
			actual:    "client IP missing",
			fallback:  "remote_addr",
		},
		{
			name: "custom strategy requires extractor",
			config: &common.RateLimitConfig[string, any]{
				BucketName: "custom-bucket", Limit: 10, Window: time.Minute, Strategy: common.StrategyCustom,
			},
			invariant: "rate_limit_custom_key_extractor_configured",
			expected:  "non-nil custom key extractor",
			actual:    "nil",
			fallback:  "abort request with 500",
		},
		{
			name: "custom extractor must return key",
			config: &common.RateLimitConfig[string, any]{
				BucketName: "custom-bucket", Limit: 10, Window: time.Minute, Strategy: common.StrategyCustom,
				KeyExtractor: func(*http.Request) (string, error) {
					return "", nil
				},
			},
			invariant: "rate_limit_custom_key_nonempty",
			expected:  "non-empty custom rate-limit key",
			actual:    "empty",
			fallback:  "abort request with 500",
		},
		{
			name: "unknown strategy uses IP fallback",
			config: &common.RateLimitConfig[string, any]{
				BucketName: "unknown-bucket", Limit: 10, Window: time.Minute, Strategy: common.RateLimitStrategy(99),
			},
			invariant: "rate_limit_strategy_known",
			expected:  "known rate-limit strategy",
			actual:    int64(99),
			fallback:  "remote_addr",
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			core, observed := observer.New(zapcore.DebugLevel)
			limiter := &captureLimiter{}
			handler := RateLimit(test.config, limiter, zap.New(core))(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(http.StatusNoContent)
			}))
			req := httptest.NewRequest(http.MethodGet, "/limited", nil)
			req.RemoteAddr = "192.0.2.4:1234"

			handler.ServeHTTP(httptest.NewRecorder(), req)

			entries := observed.FilterLevelExact(zapcore.ErrorLevel).AllUntimed()
			if len(entries) != 1 {
				t.Fatalf("Error entries = %d, want exactly 1: %#v", len(entries), observed.AllUntimed())
			}
			fields := entries[0].ContextMap()
			wants := map[string]any{
				"invariant": test.invariant,
				"operation": "rate_limit",
				"expected":  test.expected,
				"actual":    test.actual,
				"fallback":  test.fallback,
				"bucket":    test.config.BucketName,
			}
			for key, want := range wants {
				if got := fields[key]; got != want {
					t.Errorf("%s = %#v, want %#v", key, got, want)
				}
			}
			if fields["stage"] == "" || fields["method"] != http.MethodGet || fields["path"] != "/limited" {
				t.Errorf("missing execution context: %#v", fields)
			}
		})
	}
}
