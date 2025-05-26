package scontext

import (
	"context"
	"net/http/httptest"
	"testing"
)

// TestUserAgentContext verifies storing and retrieving the user agent from a context.
func TestUserAgentContext(t *testing.T) {
	ctx := context.Background()

	// Should not find a user agent in a new context
	if ua, ok := GetUserAgent[string, any](ctx); ok || ua != "" {
		t.Errorf("expected empty user agent, got %q", ua)
	}

	expected := "TestAgent/1.0"
	ctx = WithUserAgent[string, any](ctx, expected)

	ua, ok := GetUserAgent[string, any](ctx)
	if !ok {
		t.Errorf("expected user agent to be set")
	}
	if ua != expected {
		t.Errorf("expected %q, got %q", expected, ua)
	}

	rc, ok := GetSRouterContext[string, any](ctx)
	if !ok || !rc.UserAgentSet {
		t.Error("UserAgentSet flag not true")
	}
	if rc.UserAgent != expected {
		t.Errorf("expected stored user agent %q, got %q", expected, rc.UserAgent)
	}
}

// TestGetUserAgentFromRequest verifies convenience retrieval from an *http.Request.
func TestGetUserAgentFromRequest(t *testing.T) {
	req := httptest.NewRequest("GET", "/", nil)

	if ua, ok := GetUserAgentFromRequest[string, any](req); ok || ua != "" {
		t.Errorf("expected no user agent, got %q", ua)
	}

	expected := "ReqAgent/2.0"
	ctx := WithUserAgent[string, any](req.Context(), expected)
	req = req.WithContext(ctx)

	ua, ok := GetUserAgentFromRequest[string, any](req)
	if !ok {
		t.Errorf("expected user agent to be set on request")
	}
	if ua != expected {
		t.Errorf("expected %q, got %q", expected, ua)
	}
}
