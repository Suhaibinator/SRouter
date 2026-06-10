package scontext

import (
	"context"
	"testing"
)

// TestGettersWithoutSRouterContext verifies that every getter degrades
// gracefully when called on a context that has no SRouterContext — e.g. a
// helper invoked outside the router's middleware chain. Each getter must
// return its zero value and report not-found instead of panicking or
// returning stale data.
func TestGettersWithoutSRouterContext(t *testing.T) {
	ctx := context.Background()

	if id, ok := GetUserID[string, any](ctx); ok || id != "" {
		t.Errorf("GetUserID = (%q, %v), want (\"\", false)", id, ok)
	}
	if user, ok := GetUser[string, any](ctx); ok || user != nil {
		t.Errorf("GetUser = (%v, %v), want (nil, false)", user, ok)
	}
	if value, exists := GetFlag[string, any](ctx, "feature"); exists || value {
		t.Errorf("GetFlag = (%v, %v), want (false, false)", value, exists)
	}
	if ip, ok := GetClientIP[string, any](ctx); ok || ip != "" {
		t.Errorf("GetClientIP = (%q, %v), want (\"\", false)", ip, ok)
	}
	if ua, ok := GetUserAgent[string, any](ctx); ok || ua != "" {
		t.Errorf("GetUserAgent = (%q, %v), want (\"\", false)", ua, ok)
	}
	if tx, ok := GetTransaction[string, any](ctx); ok || tx != nil {
		t.Errorf("GetTransaction = (%v, %v), want (nil, false)", tx, ok)
	}
	if traceID := GetTraceIDFromContext[string, any](ctx); traceID != "" {
		t.Errorf("GetTraceIDFromContext = %q, want \"\"", traceID)
	}
	if tmpl, ok := GetRouteTemplateFromContext[string, any](ctx); ok || tmpl != "" {
		t.Errorf("GetRouteTemplateFromContext = (%q, %v), want (\"\", false)", tmpl, ok)
	}
	if params, ok := GetPathParamsFromContext[string, any](ctx); ok || params != nil {
		t.Errorf("GetPathParamsFromContext = (%v, %v), want (nil, false)", params, ok)
	}
	if origin, creds, ok := GetCORSInfo[string, any](ctx); ok || origin != "" || creds {
		t.Errorf("GetCORSInfo = (%q, %v, %v), want (\"\", false, false)", origin, creds, ok)
	}
	if hdrs, ok := GetCORSRequestedHeaders[string, any](ctx); ok || hdrs != "" {
		t.Errorf("GetCORSRequestedHeaders = (%q, %v), want (\"\", false)", hdrs, ok)
	}
	if err, ok := GetHandlerError[string, any](ctx); ok || err != nil {
		t.Errorf("GetHandlerError = (%v, %v), want (nil, false)", err, ok)
	}
}

// TestGettersWithSRouterContextButUnsetValues verifies that getters report
// not-found when an SRouterContext exists but the specific value was never set,
// so callers can distinguish "never set" from a set zero value.
func TestGettersWithSRouterContextButUnsetValues(t *testing.T) {
	ctx := WithSRouterContext(context.Background(), NewSRouterContext[string, any]())

	if id, ok := GetUserID[string, any](ctx); ok || id != "" {
		t.Errorf("GetUserID = (%q, %v), want (\"\", false)", id, ok)
	}
	if value, exists := GetFlag[string, any](ctx, "feature"); exists || value {
		t.Errorf("GetFlag = (%v, %v), want (false, false)", value, exists)
	}
	if traceID := GetTraceIDFromContext[string, any](ctx); traceID != "" {
		t.Errorf("GetTraceIDFromContext = %q, want \"\"", traceID)
	}
	if origin, creds, ok := GetCORSInfo[string, any](ctx); ok || origin != "" || creds {
		t.Errorf("GetCORSInfo = (%q, %v, %v), want (\"\", false, false)", origin, creds, ok)
	}
}
