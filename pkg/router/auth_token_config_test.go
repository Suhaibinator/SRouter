package router

import (
	"net/http/httptest"
	"testing"

	"github.com/Suhaibinator/SRouter/pkg/common"
	"github.com/Suhaibinator/SRouter/pkg/router/internal/mocks"
	"go.uber.org/zap"
	"go.uber.org/zap/zaptest/observer"
)

func TestNormalizeAuthTokenConfigDefaultsHeaderName(t *testing.T) {
	config := common.AuthTokenConfig{Source: common.AuthTokenSourceHeader}
	normalized := normalizeAuthTokenConfig(config)
	if normalized.HeaderName != defaultAuthHeaderName {
		t.Fatalf("expected header name %q, got %q", defaultAuthHeaderName, normalized.HeaderName)
	}
}

func TestBuildAuthTokenExtractorDefaultsHeaderName(t *testing.T) {
	extractor := buildAuthTokenExtractor(common.AuthTokenConfig{Source: common.AuthTokenSourceHeader})
	req := httptest.NewRequest("GET", "/test", nil)
	req.Header.Set(defaultAuthHeaderName, "Bearer valid-token")

	token, ok, reason := extractor(req)
	if !ok {
		t.Fatalf("expected token extraction to succeed, got reason %q", reason)
	}
	if token != "valid-token" {
		t.Fatalf("expected token %q, got %q", "valid-token", token)
	}
}

func TestBuildAuthTokenExtractorCookieMissingName(t *testing.T) {
	extractor := buildAuthTokenExtractor(common.AuthTokenConfig{Source: common.AuthTokenSourceCookie})
	req := httptest.NewRequest("GET", "/test", nil)

	_, ok, reason := extractor(req)
	if ok {
		t.Fatal("expected token extraction to fail")
	}
	if reason != "auth cookie name not configured" {
		t.Fatalf("expected reason %q, got %q", "auth cookie name not configured", reason)
	}
}

func TestBuildAuthTokenExtractorUnsupportedSource(t *testing.T) {
	extractor := buildAuthTokenExtractor(common.AuthTokenConfig{Source: common.AuthTokenSource(99)})
	req := httptest.NewRequest("GET", "/test", nil)

	_, ok, reason := extractor(req)
	if ok {
		t.Fatal("expected token extraction to fail")
	}
	if reason != "unsupported auth token source" {
		t.Fatalf("expected reason %q, got %q", "unsupported auth token source", reason)
	}
}

func TestWarnOnInvalidAuthTokenConfigLogs(t *testing.T) {
	core, logs := observer.New(zap.WarnLevel)
	logger := zap.New(core)
	r := NewRouter(RouterConfig{Logger: logger}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	r.authRequiredMiddlewareWithConfig(common.AuthTokenConfig{
		Source: common.AuthTokenSourceCookie,
	})

	logEntries := logs.All()
	if len(logEntries) != 1 {
		t.Fatalf("expected 1 warning log, got %d", len(logEntries))
	}
	if logEntries[0].Message != "Auth token cookie name not configured" {
		t.Fatalf("expected warning message %q, got %q", "Auth token cookie name not configured", logEntries[0].Message)
	}
}

func TestGetEffectiveAuthTokenConfigUsesSubRouter(t *testing.T) {
	logger := zap.NewNop()
	r := NewRouter(RouterConfig{Logger: logger}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	subRouterAuth := common.AuthTokenConfig{
		Source: common.AuthTokenSourceHeader,
	}

	config := r.getEffectiveAuthTokenConfig(nil, &subRouterAuth)
	if config.HeaderName != defaultAuthHeaderName {
		t.Fatalf("expected header name %q, got %q", defaultAuthHeaderName, config.HeaderName)
	}
}
