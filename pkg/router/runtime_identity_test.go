package router

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Suhaibinator/SRouter/pkg/scontext"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func requireLogFieldOnce(t *testing.T, entry observer.LoggedEntry, key string, want any) {
	t.Helper()
	if got := entry.ContextMap()[key]; got != want {
		t.Fatalf("field %q = %#v, want %#v", key, got, want)
	}
	count := 0
	for _, field := range entry.Context {
		if field.Key == key {
			count++
		}
	}
	if count != 1 {
		t.Fatalf("field %q occurred %d times, want exactly once", key, count)
	}
}

func TestRuntimeIdentityProvidersSampleOncePerRequest(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	buildID, configID := "build-1", "config-1"
	buildCalls, configCalls := 0, 0
	var seen [][2]string
	r := NewRouter[string, struct{}](RouterConfig{
		Logger:             zap.New(core),
		EnableTraceLogging: true,
	}, RouterDependencies[string, struct{}]{
		BuildID: func() string {
			buildCalls++
			return buildID
		},
		ConfigID: func() string {
			configCalls++
			return configID
		},
	})
	r.Route(RouteConfigBase{
		Path:    "/identities",
		Methods: []HttpMethod{MethodGet},
		Handler: func(w http.ResponseWriter, req *http.Request) {
			gotBuildID, _ := scontext.GetBuildID[string, struct{}](req.Context())
			gotConfigID, _ := scontext.GetConfigID[string, struct{}](req.Context())
			seen = append(seen, [2]string{gotBuildID, gotConfigID})
			w.WriteHeader(http.StatusNoContent)
		},
	})

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(string(MethodGet), "/identities", nil))
	buildID, configID = "build-2", "config-2"
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(string(MethodGet), "/identities", nil))

	if buildCalls != 2 || configCalls != 2 {
		t.Fatalf("provider calls = (%d, %d), want (2, 2)", buildCalls, configCalls)
	}
	if len(seen) != 2 || seen[0] != [2]string{"build-1", "config-1"} || seen[1] != [2]string{"build-2", "config-2"} {
		t.Fatalf("handler identities = %#v", seen)
	}
	summaries := logs.FilterMessage("Request summary statistics").All()
	if len(summaries) != 2 {
		t.Fatalf("request summaries = %d, want 2", len(summaries))
	}
	requireLogFieldOnce(t, summaries[0], "build_id", "build-1")
	requireLogFieldOnce(t, summaries[0], "config_id", "config-1")
	requireLogFieldOnce(t, summaries[1], "build_id", "build-2")
	requireLogFieldOnce(t, summaries[1], "config_id", "config-2")
}

func TestRuntimeIdentityProvidersReplaceOnlyNonEmptyValues(t *testing.T) {
	r := NewRouter(RouterConfig{}, RouterDependencies[string, struct{}]{
		BuildID:  func() string { return "local-build" },
		ConfigID: func() string { return "" },
	})
	r.Route(RouteConfigBase{
		Path:    "/identities",
		Methods: []HttpMethod{MethodGet},
		Handler: func(w http.ResponseWriter, req *http.Request) {
			buildID, buildOK := scontext.GetBuildID[string, struct{}](req.Context())
			configID, configOK := scontext.GetConfigID[string, struct{}](req.Context())
			if !buildOK || buildID != "local-build" {
				t.Errorf("build identity = (%q, %v)", buildID, buildOK)
			}
			if !configOK || configID != "inherited-config" {
				t.Errorf("config identity = (%q, %v)", configID, configOK)
			}
			w.WriteHeader(http.StatusNoContent)
		},
	})

	ctx := scontext.WithBuildID[string, struct{}](context.Background(), "inherited-build")
	ctx = scontext.WithConfigID[string, struct{}](ctx, "inherited-config")
	req := httptest.NewRequest(http.MethodGet, "/identities", nil).WithContext(ctx)
	r.ServeHTTP(httptest.NewRecorder(), req)
}

func TestRuntimeIdentityProvidersLeaveAbsentValuesUnset(t *testing.T) {
	buildCalls := 0
	r := NewRouter(RouterConfig{}, RouterDependencies[string, struct{}]{
		BuildID: func() string {
			buildCalls++
			return ""
		},
	})
	r.Route(RouteConfigBase{
		Path:    "/identities",
		Methods: []HttpMethod{MethodGet},
		Handler: func(w http.ResponseWriter, req *http.Request) {
			if buildID, ok := scontext.GetBuildID[string, struct{}](req.Context()); ok || buildID != "" {
				t.Errorf("build identity = (%q, %v), want absent", buildID, ok)
			}
			if configID, ok := scontext.GetConfigID[string, struct{}](req.Context()); ok || configID != "" {
				t.Errorf("config identity = (%q, %v), want absent", configID, ok)
			}
			w.WriteHeader(http.StatusNoContent)
		},
	})

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/identities", nil))
	if buildCalls != 1 {
		t.Fatalf("empty build identity provider calls = %d, want 1", buildCalls)
	}
}

func TestRuntimeIdentitiesEnrichAuthenticationAndErrorLogs(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	auth := AuthRequired
	r := NewRouter[string, struct{}](RouterConfig{
		Logger: zap.New(core),
	}, RouterDependencies[string, struct{}]{
		Authenticate: func(context.Context, string) (*struct{}, bool) {
			return nil, false
		},
		UserID: func(*struct{}) string {
			return "user"
		},
		BuildID:  func() string { return "build-auth" },
		ConfigID: func() string { return "config-auth" },
	})
	r.Route(RouteConfigBase{
		Path:      "/protected",
		Methods:   []HttpMethod{MethodGet},
		AuthLevel: &auth,
		Handler: func(http.ResponseWriter, *http.Request) {
			t.Fatal("protected handler must not run")
		},
	})
	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/protected", nil))

	authEntries := logs.FilterMessage("Authentication failed").All()
	if len(authEntries) != 1 {
		t.Fatalf("authentication failure logs = %d, want 1", len(authEntries))
	}
	requireLogFieldOnce(t, authEntries[0], "build_id", "build-auth")
	requireLogFieldOnce(t, authEntries[0], "config_id", "config-auth")

	ctx := scontext.WithBuildID[string, struct{}](context.Background(), "build-error")
	ctx = scontext.WithConfigID[string, struct{}](ctx, "config-error")
	req := httptest.NewRequest(http.MethodGet, "/error", nil).WithContext(ctx)
	r.handleError(httptest.NewRecorder(), req, errors.New("boom"), http.StatusInternalServerError, "failed")
	errorEntries := logs.FilterMessage("failed").All()
	if len(errorEntries) != 1 {
		t.Fatalf("handled error logs = %d, want 1", len(errorEntries))
	}
	requireLogFieldOnce(t, errorEntries[0], "build_id", "build-error")
	requireLogFieldOnce(t, errorEntries[0], "config_id", "config-error")
}
