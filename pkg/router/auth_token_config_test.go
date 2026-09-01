package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

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

func TestBuildAuthTokenExtractor(t *testing.T) {
	t.Run("default header", func(t *testing.T) {
		extractor := buildAuthTokenExtractor(common.AuthTokenConfig{Source: common.AuthTokenSourceHeader})
		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set(defaultAuthHeaderName, "Bearer valid-token")
		token, ok, reason := extractor(req)
		if !ok || reason != "" || token != "valid-token" {
			t.Fatalf("unexpected extraction result token=%q ok=%v reason=%q", token, ok, reason)
		}
	})

	t.Run("cookie without name", func(t *testing.T) {
		extractor := buildAuthTokenExtractor(common.AuthTokenConfig{Source: common.AuthTokenSourceCookie})
		_, ok, reason := extractor(httptest.NewRequest(http.MethodGet, "/test", nil))
		if ok || reason != "auth cookie name not configured" {
			t.Fatalf("unexpected extraction result ok=%v reason=%q", ok, reason)
		}
	})

	t.Run("unsupported source", func(t *testing.T) {
		extractor := buildAuthTokenExtractor(common.AuthTokenConfig{Source: common.AuthTokenSource(99)})
		_, ok, reason := extractor(httptest.NewRequest(http.MethodGet, "/test", nil))
		if ok || reason != "unsupported auth token source" {
			t.Fatalf("unexpected extraction result ok=%v reason=%q", ok, reason)
		}
	})
}

func TestWarnOnInvalidAuthTokenConfigLogs(t *testing.T) {
	core, logs := observer.New(zap.WarnLevel)
	r := NewRouter(RouterConfig{Logger: zap.New(core)}, RouterDependencies[string, string]{Authenticate: mocks.MockAuthFunction, UserID: mocks.MockUserIDFromUser})
	r.authRequiredMiddlewareWithConfig(common.AuthTokenConfig{Source: common.AuthTokenSourceCookie})
	entries := logs.FilterMessage("Auth token cookie name not configured").All()
	if len(entries) != 1 {
		t.Fatalf("expected one warning, got %d", len(entries))
	}
}

func TestInitialAuthTokenConfig(t *testing.T) {
	global := common.AuthTokenConfig{Source: common.AuthTokenSourceCookie, CookieName: "global"}
	r := NewRouter(RouterConfig{Logger: zap.NewNop(), GlobalAuthToken: &global}, RouterDependencies[string, string]{Authenticate: mocks.MockAuthFunction, UserID: mocks.MockUserIDFromUser})

	got := r.initialAuthTokenConfig()
	if got.origin != authTokenOriginGlobal || got.config.CookieName != "global" {
		t.Fatalf("expected global config, got %+v", got)
	}
}

func TestGlobalAuthTokenUsedAcrossRootAndNestedGroups(t *testing.T) {
	global := common.AuthTokenConfig{Source: common.AuthTokenSourceCookie, CookieName: "auth_token"}
	r := NewRouter(RouterConfig{Logger: zap.NewNop(), GlobalAuthToken: &global}, RouterDependencies[string, string]{Authenticate: tokenAuthFunction, UserID: tokenUserIDFromUser})

	r.Route(
		RouteConfigBase{Path: "/direct", Methods: []HttpMethod{MethodGet}, AuthLevel: new(AuthRequired), Handler: okHandler},
		authTokenGenericRoute("/direct-generic", new(AuthRequired)),
	)
	api := r.Group("/api")
	api.Route(
		RouteConfigBase{Path: "/standard", Methods: []HttpMethod{MethodGet}, AuthLevel: new(AuthRequired), Handler: okHandler},
		authTokenGenericRoute("/generic", new(AuthRequired)),
	)
	api.Group("/nested").Route(RouteConfigBase{
		Path: "/protected", Methods: []HttpMethod{MethodGet}, AuthLevel: new(AuthRequired), Handler: okHandler,
	})

	tests := []struct {
		method string
		path   string
		body   string
	}{
		{http.MethodGet, "/direct", ""},
		{http.MethodPost, "/direct-generic", `{"name":"test"}`},
		{http.MethodGet, "/api/standard", ""},
		{http.MethodPost, "/api/generic", `{"name":"test"}`},
		{http.MethodGet, "/api/nested/protected", ""},
	}
	for _, tt := range tests {
		req := httptest.NewRequest(tt.method, tt.path, strings.NewReader(tt.body))
		req.Header.Set("Content-Type", "application/json")
		req.AddCookie(&http.Cookie{Name: "auth_token", Value: "valid-token"})
		rr := httptest.NewRecorder()
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusOK {
			t.Fatalf("%s %s: expected 200, got %d: %s", tt.method, tt.path, rr.Code, rr.Body.String())
		}
	}
}

func TestAuthTokenPrecedenceAcrossGroups(t *testing.T) {
	global := common.AuthTokenConfig{Source: common.AuthTokenSourceCookie, CookieName: "global"}
	parent := common.AuthTokenConfig{Source: common.AuthTokenSourceCookie, CookieName: "parent"}
	child := common.AuthTokenConfig{Source: common.AuthTokenSourceCookie, CookieName: "child"}
	route := common.AuthTokenConfig{Source: common.AuthTokenSourceHeader, HeaderName: "X-Route-Token"}
	r := NewRouter(RouterConfig{Logger: zap.NewNop(), GlobalAuthToken: &global}, RouterDependencies[string, string]{Authenticate: tokenAuthFunction, UserID: tokenUserIDFromUser})

	r.Route(RouteConfigBase{Path: "/global", Methods: []HttpMethod{MethodGet}, AuthLevel: new(AuthRequired), Handler: okHandler})
	api := r.Group("/api").AuthToken(&parent).Auth(AuthRequired)
	api.Route(RouteConfigBase{
		Path: "/route", Methods: []HttpMethod{MethodGet},
		Overrides: common.RouteOverrides{AuthToken: &route}, Handler: okHandler,
	})
	api.Group("/child").AuthToken(&child).Route(RouteConfigBase{Path: "/protected", Methods: []HttpMethod{MethodGet}, Handler: okHandler})
	api.Group("/inherited").Route(RouteConfigBase{Path: "/protected", Methods: []HttpMethod{MethodGet}, Handler: okHandler})

	tests := []struct {
		path       string
		headerName string
		cookieName string
	}{
		{path: "/global", cookieName: "global"},
		{path: "/api/route", headerName: "X-Route-Token"},
		{path: "/api/child/protected", cookieName: "child"},
		{path: "/api/inherited/protected", cookieName: "parent"},
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
			t.Fatalf("%s: expected 200, got %d: %s", tt.path, rr.Code, rr.Body.String())
		}
	}
}

func TestGroupAuthTokenNilResetsInheritedSource(t *testing.T) {
	global := common.AuthTokenConfig{Source: common.AuthTokenSourceCookie, CookieName: "global"}
	parent := common.AuthTokenConfig{Source: common.AuthTokenSourceCookie, CookieName: "parent"}
	r := NewRouter(RouterConfig{Logger: zap.NewNop(), GlobalAuthToken: &global}, RouterDependencies[string, string]{Authenticate: tokenAuthFunction, UserID: tokenUserIDFromUser})
	api := r.Group("/api").AuthToken(&parent)
	api.Group("/reset").AuthToken(nil).Auth(AuthRequired).Route(RouteConfigBase{
		Path: "/protected", Methods: []HttpMethod{MethodGet}, Handler: okHandler,
	})

	req := httptest.NewRequest(http.MethodGet, "/api/reset/protected", nil)
	req.AddCookie(&http.Cookie{Name: "parent", Value: "valid-token"})
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected reset group to ignore parent token source, got %d", rr.Code)
	}

	req = httptest.NewRequest(http.MethodGet, "/api/reset/protected", nil)
	req.Header.Set(defaultAuthHeaderName, "Bearer valid-token")
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected built-in token source after reset, got %d", rr.Code)
	}
}

func TestAuthRequiredBuiltInFallbackWarningAtBuild(t *testing.T) {
	core, logs := observer.New(zap.WarnLevel)
	r := NewRouter(RouterConfig{Logger: zap.New(core)}, RouterDependencies[string, string]{Authenticate: tokenAuthFunction, UserID: tokenUserIDFromUser})
	r.Group("/api").Auth(AuthRequired).Route(RouteConfigBase{
		Path: "/protected", Methods: []HttpMethod{MethodGet}, Handler: okHandler,
	})
	if err := r.Build(); err != nil {
		t.Fatalf("Build failed: %v", err)
	}

	entries := logs.FilterMessage("Auth-required route using built-in default auth token source").All()
	if len(entries) != 1 {
		t.Fatalf("expected one built-in fallback warning, got %d", len(entries))
	}
	if entries[0].ContextMap()["path"] != "/api/protected" {
		t.Fatalf("unexpected warning path: %v", entries[0].ContextMap()["path"])
	}
}

func TestConfiguredAuthTokenDoesNotWarnAtBuild(t *testing.T) {
	core, logs := observer.New(zap.WarnLevel)
	auth := common.AuthTokenConfig{Source: common.AuthTokenSourceCookie, CookieName: "token"}
	r := NewRouter(RouterConfig{Logger: zap.New(core)}, RouterDependencies[string, string]{Authenticate: tokenAuthFunction, UserID: tokenUserIDFromUser})
	r.Group("/api").Auth(AuthRequired).AuthToken(&auth).Route(RouteConfigBase{
		Path: "/protected", Methods: []HttpMethod{MethodGet}, Handler: okHandler,
	})
	if err := r.Build(); err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	if entries := logs.FilterMessage("Auth-required route using built-in default auth token source").All(); len(entries) != 0 {
		t.Fatalf("expected no fallback warning, got %d", len(entries))
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
		Path: path, Methods: []HttpMethod{MethodPost}, AuthLevel: authLevel,
		Codec: codec.NewJSONCodec[authTokenTestRequest, authTokenTestResponse](), SourceType: Body,
		Handler: func(_ *http.Request, data authTokenTestRequest) (authTokenTestResponse, error) {
			return authTokenTestResponse{Message: data.Name}, nil
		},
	}
}

func tokenAuthFunction(_ context.Context, token string) (*string, bool) {
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

func okHandler(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
}
