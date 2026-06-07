package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/codec"
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

func TestGetEffectiveAuthTokenConfigUsesGlobal(t *testing.T) {
	globalAuth := common.AuthTokenConfig{
		Source:     common.AuthTokenSourceCookie,
		CookieName: "auth_token",
	}
	r := NewRouter(RouterConfig{
		Logger:          zap.NewNop(),
		GlobalAuthToken: &globalAuth,
	}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	config := r.getEffectiveAuthTokenConfig(nil, nil)
	if config.Source != common.AuthTokenSourceCookie {
		t.Fatalf("expected cookie auth source, got %v", config.Source)
	}
	if config.CookieName != "auth_token" {
		t.Fatalf("expected cookie name %q, got %q", "auth_token", config.CookieName)
	}
}

func TestAuthRequiredMiddlewareUsesGlobalAuthToken(t *testing.T) {
	globalAuth := common.AuthTokenConfig{
		Source:     common.AuthTokenSourceCookie,
		CookieName: "auth_token",
	}
	r := NewRouter(RouterConfig{
		Logger:          zap.NewNop(),
		GlobalAuthToken: &globalAuth,
	}, tokenAuthFunction, tokenUserIDFromUser)

	handler := r.authRequiredMiddleware(http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	req.AddCookie(&http.Cookie{Name: "auth_token", Value: "valid-token"})
	rr := httptest.NewRecorder()
	handler.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
}

func TestGlobalAuthTokenCookieUsedAcrossRegistrationStyles(t *testing.T) {
	globalAuth := common.AuthTokenConfig{
		Source:     common.AuthTokenSourceCookie,
		CookieName: "auth_token",
	}
	authLevel := authLevelPtr(AuthRequired)
	r := NewRouter(RouterConfig{
		Logger:          zap.NewNop(),
		GlobalAuthToken: &globalAuth,
		SubRouters: []SubRouterConfig{
			{
				PathPrefix: "/api",
				Routes: []RouteDefinition{
					RouteConfigBase{
						Path:      "/sub",
						Methods:   []HttpMethod{MethodGet},
						AuthLevel: authLevel,
						Handler:   okHandler,
					},
					NewGenericRouteDefinition[authTokenTestRequest, authTokenTestResponse, string, string](authTokenGenericRoute("/generic", authLevel)),
				},
				SubRouters: []SubRouterConfig{
					{
						PathPrefix: "/nested",
						Routes: []RouteDefinition{
							RouteConfigBase{
								Path:      "/protected",
								Methods:   []HttpMethod{MethodGet},
								AuthLevel: authLevel,
								Handler:   okHandler,
							},
						},
					},
				},
			},
			{PathPrefix: "/dynamic"},
		},
	}, tokenAuthFunction, tokenUserIDFromUser)

	r.RegisterRoute(RouteConfigBase{
		Path:      "/direct",
		Methods:   []HttpMethod{MethodGet},
		AuthLevel: authLevel,
		Handler:   okHandler,
	})
	RegisterGenericRoute(r, authTokenGenericRoute("/direct-generic", authLevel), 0, 0, nil)

	err := RegisterGenericRouteOnSubRouter(r, "/dynamic", authTokenGenericRoute("/generic", authLevel))
	if err != nil {
		t.Fatalf("RegisterGenericRouteOnSubRouter failed: %v", err)
	}

	tests := []struct {
		method string
		path   string
		body   string
	}{
		{method: http.MethodGet, path: "/direct"},
		{method: http.MethodPost, path: "/direct-generic", body: `{"name":"test"}`},
		{method: http.MethodGet, path: "/api/sub"},
		{method: http.MethodGet, path: "/api/nested/protected"},
		{method: http.MethodPost, path: "/api/generic", body: `{"name":"test"}`},
		{method: http.MethodPost, path: "/dynamic/generic", body: `{"name":"test"}`},
	}

	for _, tt := range tests {
		req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "auth_token", Value: "valid-token"})
		rr := httptest.NewRecorder()

		r.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("%s %s: expected status %d, got %d; body=%s", tt.method, tt.path, http.StatusOK, rr.Code, rr.Body.String())
		}
	}
}

func TestAuthTokenPrecedence(t *testing.T) {
	globalAuth := common.AuthTokenConfig{Source: common.AuthTokenSourceCookie, CookieName: "global_token"}
	parentAuth := common.AuthTokenConfig{Source: common.AuthTokenSourceCookie, CookieName: "parent_token"}
	childAuth := common.AuthTokenConfig{Source: common.AuthTokenSourceCookie, CookieName: "child_token"}
	subAuth := common.AuthTokenConfig{Source: common.AuthTokenSourceCookie, CookieName: "sub_token"}
	routeAuth := common.AuthTokenConfig{Source: common.AuthTokenSourceHeader, HeaderName: "X-Route-Token"}
	authLevel := authLevelPtr(AuthRequired)

	r := NewRouter(RouterConfig{
		Logger:          zap.NewNop(),
		GlobalAuthToken: &globalAuth,
		SubRouters: []SubRouterConfig{
			{
				PathPrefix: "/api",
				Overrides:  common.RouteOverrides{AuthToken: &parentAuth},
				Routes: []RouteDefinition{
					RouteConfigBase{
						Path:      "/route",
						Methods:   []HttpMethod{MethodGet},
						AuthLevel: authLevel,
						Overrides: common.RouteOverrides{AuthToken: &routeAuth},
						Handler:   okHandler,
					},
				},
				SubRouters: []SubRouterConfig{
					{
						PathPrefix: "/sub",
						Overrides:  common.RouteOverrides{AuthToken: &subAuth},
						Routes: []RouteDefinition{
							RouteConfigBase{
								Path:      "/protected",
								Methods:   []HttpMethod{MethodGet},
								AuthLevel: authLevel,
								Handler:   okHandler,
							},
						},
					},
					{
						PathPrefix: "/child",
						Overrides:  common.RouteOverrides{AuthToken: &childAuth},
						Routes: []RouteDefinition{
							RouteConfigBase{
								Path:      "/protected",
								Methods:   []HttpMethod{MethodGet},
								AuthLevel: authLevel,
								Handler:   okHandler,
							},
						},
					},
					{
						PathPrefix: "/inherited",
						Routes: []RouteDefinition{
							RouteConfigBase{
								Path:      "/protected",
								Methods:   []HttpMethod{MethodGet},
								AuthLevel: authLevel,
								Handler:   okHandler,
							},
						},
					},
				},
			},
		},
	}, tokenAuthFunction, tokenUserIDFromUser)
	r.RegisterRoute(RouteConfigBase{
		Path:      "/global",
		Methods:   []HttpMethod{MethodGet},
		AuthLevel: authLevel,
		Handler:   okHandler,
	})

	tests := []struct {
		name       string
		path       string
		headerName string
		cookieName string
	}{
		{name: "route beats sub-router", path: "/api/route", headerName: "X-Route-Token"},
		{name: "sub-router beats inherited parent", path: "/api/sub/protected", cookieName: "sub_token"},
		{name: "child sub-router beats inherited parent", path: "/api/child/protected", cookieName: "child_token"},
		{name: "parent sub-router inherits into child", path: "/api/inherited/protected", cookieName: "parent_token"},
		{name: "global beats built-in default", path: "/global", cookieName: "global_token"},
	}

	for _, tt := range tests {
		req := httptest.NewRequest(http.MethodGet, tt.path, nil)
		if tt.headerName != "" {
			req.Header.Set(tt.headerName, "Bearer valid-token")
		}
		if tt.cookieName != "" {
			req.AddCookie(&http.Cookie{Name: tt.cookieName, Value: "valid-token"})
		}
		rr := httptest.NewRecorder()

		r.ServeHTTP(rr, req)

		if rr.Code != http.StatusOK {
			t.Fatalf("%s: expected status %d, got %d; body=%s", tt.name, http.StatusOK, rr.Code, rr.Body.String())
		}
	}
}

func TestIsolateOverridesPreventsParentAuthInheritanceButKeepsGlobal(t *testing.T) {
	globalAuth := common.AuthTokenConfig{Source: common.AuthTokenSourceCookie, CookieName: "global_token"}
	parentAuth := common.AuthTokenConfig{Source: common.AuthTokenSourceCookie, CookieName: "parent_token"}
	authLevel := authLevelPtr(AuthRequired)

	r := NewRouter(RouterConfig{
		Logger:          zap.NewNop(),
		GlobalAuthToken: &globalAuth,
		SubRouters: []SubRouterConfig{
			{
				PathPrefix: "/api",
				Overrides:  common.RouteOverrides{AuthToken: &parentAuth},
				SubRouters: []SubRouterConfig{
					{
						PathPrefix:       "/isolated",
						IsolateOverrides: true,
						Routes: []RouteDefinition{
							RouteConfigBase{
								Path:      "/protected",
								Methods:   []HttpMethod{MethodGet},
								AuthLevel: authLevel,
								Handler:   okHandler,
							},
						},
					},
				},
			},
		},
	}, tokenAuthFunction, tokenUserIDFromUser)

	reqParent := httptest.NewRequest(http.MethodGet, "/api/isolated/protected", nil)
	reqParent.AddCookie(&http.Cookie{Name: "parent_token", Value: "valid-token"})
	rrParent := httptest.NewRecorder()
	r.ServeHTTP(rrParent, reqParent)
	if rrParent.Code != http.StatusUnauthorized {
		t.Fatalf("expected parent cookie to be ignored with status %d, got %d", http.StatusUnauthorized, rrParent.Code)
	}

	reqGlobal := httptest.NewRequest(http.MethodGet, "/api/isolated/protected", nil)
	reqGlobal.AddCookie(&http.Cookie{Name: "global_token", Value: "valid-token"})
	rrGlobal := httptest.NewRecorder()
	r.ServeHTTP(rrGlobal, reqGlobal)
	if rrGlobal.Code != http.StatusOK {
		t.Fatalf("expected global cookie to pass with status %d, got %d", http.StatusOK, rrGlobal.Code)
	}
}

func TestAuthRequiredBuiltInFallbackWarning(t *testing.T) {
	core, logs := observer.New(zap.WarnLevel)
	logger := zap.New(core)

	NewRouter(RouterConfig{
		Logger: logger,
		SubRouters: []SubRouterConfig{
			{
				PathPrefix: "/api",
				Routes: []RouteDefinition{
					RouteConfigBase{
						Path:      "/protected",
						Methods:   []HttpMethod{MethodGet},
						AuthLevel: authLevelPtr(AuthRequired),
						Handler:   okHandler,
					},
				},
			},
		},
	}, tokenAuthFunction, tokenUserIDFromUser)

	entries := logs.FilterMessage("Auth-required route using built-in default auth token source").All()
	if len(entries) != 1 {
		t.Fatalf("expected 1 built-in fallback warning, got %d", len(entries))
	}
	if entries[0].ContextMap()["path"] != "/api/protected" {
		t.Fatalf("expected path field %q, got %v", "/api/protected", entries[0].ContextMap()["path"])
	}
	if entries[0].ContextMap()["header_name"] != defaultAuthHeaderName {
		t.Fatalf("expected header_name field %q, got %v", defaultAuthHeaderName, entries[0].ContextMap()["header_name"])
	}
}

func TestNoBuiltInFallbackWarningWhenNotRisky(t *testing.T) {
	core, logs := observer.New(zap.WarnLevel)
	logger := zap.New(core)

	NewRouter(RouterConfig{
		Logger: logger,
		SubRouters: []SubRouterConfig{{
			PathPrefix: "/api",
			Routes: []RouteDefinition{
				RouteConfigBase{
					Path:      "/optional",
					Methods:   []HttpMethod{MethodGet},
					AuthLevel: authLevelPtr(AuthOptional),
					Handler:   okHandler,
				},
				RouteConfigBase{
					Path:      "/public",
					Methods:   []HttpMethod{MethodGet},
					AuthLevel: authLevelPtr(NoAuth),
					Handler:   okHandler,
				},
			},
		}},
	}, tokenAuthFunction, tokenUserIDFromUser)

	entries := logs.FilterMessage("Auth-required route using built-in default auth token source").All()
	if len(entries) != 0 {
		t.Fatalf("expected no built-in fallback warnings for optional/noauth routes, got %d", len(entries))
	}

	globalAuth := common.AuthTokenConfig{Source: common.AuthTokenSourceHeader}
	routeAuth := common.AuthTokenConfig{Source: common.AuthTokenSourceHeader, HeaderName: "X-Route-Token"}
	subRouterAuth := common.AuthTokenConfig{Source: common.AuthTokenSourceCookie, CookieName: "sub_token"}
	for _, tt := range []struct {
		name   string
		config RouterConfig
	}{
		{
			name: "global auth token",
			config: RouterConfig{
				Logger:          logger,
				GlobalAuthToken: &globalAuth,
				SubRouters: []SubRouterConfig{{
					PathPrefix: "/api",
					Routes: []RouteDefinition{RouteConfigBase{
						Path:      "/required",
						Methods:   []HttpMethod{MethodGet},
						AuthLevel: authLevelPtr(AuthRequired),
						Handler:   okHandler,
					}},
				}},
			},
		},
		{
			name: "sub-router auth token",
			config: RouterConfig{
				Logger: logger,
				SubRouters: []SubRouterConfig{{
					PathPrefix: "/api",
					Overrides:  common.RouteOverrides{AuthToken: &subRouterAuth},
					Routes: []RouteDefinition{RouteConfigBase{
						Path:      "/required",
						Methods:   []HttpMethod{MethodGet},
						AuthLevel: authLevelPtr(AuthRequired),
						Handler:   okHandler,
					}},
				}},
			},
		},
		{
			name: "route auth token",
			config: RouterConfig{
				Logger: logger,
				SubRouters: []SubRouterConfig{{
					PathPrefix: "/api",
					Routes: []RouteDefinition{RouteConfigBase{
						Path:      "/required",
						Methods:   []HttpMethod{MethodGet},
						AuthLevel: authLevelPtr(AuthRequired),
						Overrides: common.RouteOverrides{AuthToken: &routeAuth},
						Handler:   okHandler,
					}},
				}},
			},
		},
	} {
		before := len(logs.FilterMessage("Auth-required route using built-in default auth token source").All())
		NewRouter(tt.config, tokenAuthFunction, tokenUserIDFromUser)
		after := len(logs.FilterMessage("Auth-required route using built-in default auth token source").All())
		if after != before {
			t.Fatalf("%s: expected no built-in fallback warning, got %d new warnings", tt.name, after-before)
		}
	}
}

func TestResolveSubRouterOverridesInheritsAndIsolates(t *testing.T) {
	parentRateLimit := &common.RateLimitConfig[any, any]{Limit: 10, Window: time.Minute}
	childRateLimit := &common.RateLimitConfig[any, any]{Limit: 20, Window: time.Minute}
	parentAuth := &common.AuthTokenConfig{Source: common.AuthTokenSourceCookie, CookieName: "parent_token"}
	childAuth := &common.AuthTokenConfig{Source: common.AuthTokenSourceCookie, CookieName: "child_token"}

	parent := common.RouteOverrides{
		Timeout:     time.Second,
		MaxBodySize: 100,
		RateLimit:   parentRateLimit,
		AuthToken:   parentAuth,
	}

	inherited := resolveSubRouterOverrides(parent, SubRouterConfig{})
	if inherited.Timeout != parent.Timeout {
		t.Fatalf("expected inherited timeout %v, got %v", parent.Timeout, inherited.Timeout)
	}
	if inherited.MaxBodySize != parent.MaxBodySize {
		t.Fatalf("expected inherited max body size %d, got %d", parent.MaxBodySize, inherited.MaxBodySize)
	}
	if inherited.RateLimit != parentRateLimit {
		t.Fatal("expected inherited rate limit")
	}
	if inherited.AuthToken != parentAuth {
		t.Fatal("expected inherited auth token")
	}

	overridden := resolveSubRouterOverrides(parent, SubRouterConfig{
		Overrides: common.RouteOverrides{
			Timeout:     2 * time.Second,
			MaxBodySize: 200,
			RateLimit:   childRateLimit,
			AuthToken:   childAuth,
		},
	})
	if overridden.Timeout != 2*time.Second || overridden.MaxBodySize != 200 || overridden.RateLimit != childRateLimit || overridden.AuthToken != childAuth {
		t.Fatalf("expected child overrides to win, got %+v", overridden)
	}

	isolated := resolveSubRouterOverrides(parent, SubRouterConfig{IsolateOverrides: true})
	if isolated.Timeout != 0 || isolated.MaxBodySize != 0 || isolated.RateLimit != nil || isolated.AuthToken != nil {
		t.Fatalf("expected isolated overrides to start empty, got %+v", isolated)
	}
}

type authTokenTestRequest struct {
	Name string `json:"name"`
}

type authTokenTestResponse struct {
	Message string `json:"message"`
}

func authTokenGenericRoute(path string, authLevel *AuthLevel) RouteConfig[authTokenTestRequest, authTokenTestResponse] {
	return RouteConfig[authTokenTestRequest, authTokenTestResponse]{
		Path:       path,
		Methods:    []HttpMethod{MethodPost},
		AuthLevel:  authLevel,
		Codec:      codec.NewJSONCodec[authTokenTestRequest, authTokenTestResponse](),
		SourceType: Body,
		Handler: func(req *http.Request, data authTokenTestRequest) (authTokenTestResponse, error) {
			return authTokenTestResponse{Message: data.Name}, nil
		},
	}
}

func tokenAuthFunction(ctx context.Context, token string) (*string, bool) {
	if token == "valid-token" {
		user := "user"
		return &user, true
	}
	return nil, false
}

func tokenUserIDFromUser(user *string) string {
	if user == nil {
		return ""
	}
	return *user
}

func authLevelPtr(level AuthLevel) *AuthLevel {
	return &level
}

func okHandler(w http.ResponseWriter, req *http.Request) {
	w.WriteHeader(http.StatusOK)
}
