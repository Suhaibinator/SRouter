package router

import (
	"testing"

	"github.com/Suhaibinator/SRouter/pkg/common"
	"github.com/Suhaibinator/SRouter/pkg/logkeys"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

func processLogDependencies(buildID, configID func() string) RouterDependencies[string, string] {
	return RouterDependencies[string, string]{
		Authenticate: tokenAuthFunction,
		UserID:       tokenUserIDFromUser,
		BuildID:      buildID,
		ConfigID:     configID,
	}
}

func requireRuntimeIdentities(t *testing.T, logs *observer.ObservedLogs, message, buildID, configID string) {
	t.Helper()
	entries := logs.FilterMessage(message).All()
	if len(entries) != 1 {
		t.Fatalf("expected one %q warning, got %d", message, len(entries))
	}
	fields := entries[0].ContextMap()
	if got := fields[logkeys.BuildID]; got != buildID {
		t.Errorf("%q: expected build_id %q, got %v", message, buildID, got)
	}
	if got := fields[logkeys.ConfigID]; got != configID {
		t.Errorf("%q: expected config_id %q, got %v", message, configID, got)
	}
	if _, ok := fields[logkeys.TraceID]; ok {
		t.Errorf("%q: process warning must not carry trace_id", message)
	}
}

func TestProcessWarningsIncludeRuntimeIdentities(t *testing.T) {
	core, logs := observer.New(zap.WarnLevel)
	r := NewRouter(RouterConfig{
		Logger:     zap.New(core),
		CORSConfig: &CORSConfig{Origins: []string{"*"}, AllowCredentials: true},
	}, processLogDependencies(func() string { return "build-1" }, func() string { return "config-1" }))

	r.Route(authTokenGenericRoute("/typed", nil))
	r.Group("/api").Auth(AuthRequired).Route(RouteConfigBase{
		Path: "/protected", Methods: []HttpMethod{MethodGet}, Handler: okHandler,
	})
	if err := r.Build(); err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	r.authRequiredMiddlewareWithConfig(common.AuthTokenConfig{Source: common.AuthTokenSourceCookie})

	for _, message := range []string{
		"CORS config combines wildcard origin with AllowCredentials; " +
			"credentials are never allowed for wildcard origins per the CORS spec. " +
			"List explicit origins to enable credentials.",
		"Route registered without sanitizer function",
		"Auth-required route using built-in default auth token source",
		"Auth token cookie name not configured",
	} {
		requireRuntimeIdentities(t, logs, message, "build-1", "config-1")
	}
}

func TestProcessWarningSamplesCurrentIdentities(t *testing.T) {
	core, logs := observer.New(zap.WarnLevel)
	configID := "config-1"
	r := NewRouter(RouterConfig{Logger: zap.New(core)},
		processLogDependencies(nil, func() string { return configID }))

	r.warnProcess("first")
	configID = "config-2"
	r.warnProcess("second")

	for message, want := range map[string]string{"first": "config-1", "second": "config-2"} {
		fields := logs.FilterMessage(message).All()[0].ContextMap()
		if got := fields[logkeys.ConfigID]; got != want {
			t.Errorf("%q: expected config_id %q, got %v", message, want, got)
		}
		if _, ok := fields[logkeys.BuildID]; ok {
			t.Errorf("%q: nil BuildID provider must not emit build_id", message)
		}
	}
}

func TestProcessWarningOmitsEmptyIdentities(t *testing.T) {
	core, logs := observer.New(zap.WarnLevel)
	r := NewRouter(RouterConfig{Logger: zap.New(core)},
		processLogDependencies(func() string { return "" }, func() string { return "" }))

	r.warnProcess("empty", zap.String(logkeys.Path, "/p"))

	fields := logs.FilterMessage("empty").All()[0].ContextMap()
	if _, ok := fields[logkeys.BuildID]; ok {
		t.Error("empty build ID must not be emitted")
	}
	if _, ok := fields[logkeys.ConfigID]; ok {
		t.Error("empty config ID must not be emitted")
	}
	if fields[logkeys.Path] != "/p" {
		t.Errorf("expected event field to be kept, got %v", fields)
	}
}

func TestProcessWarningSkipsProvidersWhenDisabled(t *testing.T) {
	core, _ := observer.New(zapcore.ErrorLevel)
	called := false
	provider := func() string {
		called = true
		return "id"
	}
	r := NewRouter(RouterConfig{Logger: zap.New(core)}, processLogDependencies(provider, provider))

	r.warnProcess("disabled")

	if called {
		t.Fatal("providers must not be sampled when Warn is disabled")
	}
}
