package router

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"testing"

	"github.com/Suhaibinator/SRouter/pkg/logkeys"
	"github.com/Suhaibinator/SRouter/pkg/scontext"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"go.uber.org/zap/zaptest/observer"
)

// reqLogUser is the user object used by the request-scoped logger tests.
type reqLogUser struct {
	id   uint64
	name string
}

const reqLogValidToken = "valid-token"

// reqLogUint64Deps builds dependencies for a uint64 user ID with the runtime
// identity providers the request logger stamps onto every entry.
func reqLogUint64Deps(userIDField func(uint64) zap.Field) RouterDependencies[uint64, reqLogUser] {
	return RouterDependencies[uint64, reqLogUser]{
		Authenticate: func(_ context.Context, token string) (*reqLogUser, bool) {
			if token != reqLogValidToken {
				return nil, false
			}
			return &reqLogUser{id: 4242, name: "ada"}, true
		},
		UserID: func(u *reqLogUser) uint64 {
			if u == nil {
				return 0
			}
			return u.id
		},
		BuildID:     func() string { return "build-1" },
		ConfigID:    func() string { return "config-1" },
		UserIDField: userIDField,
	}
}

// reqLogFieldKeys returns the field keys of an observed entry in order.
func reqLogFieldKeys(entry observer.LoggedEntry) []string {
	keys := make([]string, 0, len(entry.Context))
	for _, field := range entry.Context {
		keys = append(keys, field.Key)
	}
	return keys
}

// reqLogSingleEntry returns the one observed entry with the given message.
func reqLogSingleEntry(t *testing.T, logs *observer.ObservedLogs, message string) observer.LoggedEntry {
	t.Helper()
	entries := logs.FilterMessage(message).All()
	if len(entries) != 1 {
		t.Fatalf("entries with message %q = %d, want exactly 1: %#v", message, len(entries), logs.All())
	}
	return entries[0]
}

// TestRequestLoggerCarriesCorrelationOnAuthRequiredRoute verifies that the
// handler's request-scoped logger carries every correlation value in the
// documented order, and that it is distinct from the router's own "SRouter"
// logger.
func TestRequestLoggerCarriesCorrelationOnAuthRequiredRoute(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	r := NewRouter(RouterConfig{
		Logger:            zap.New(core),
		TraceIDBufferSize: 10,
	}, reqLogUint64Deps(func(id uint64) zap.Field {
		return zap.Uint64(logkeys.UserID, id)
	}))

	authLevel := AuthRequired
	handlerRan := false
	r.Route(RouteConfigBase{
		Path:      "/protected",
		Methods:   []HttpMethod{MethodGet},
		AuthLevel: &authLevel,
		Handler: func(w http.ResponseWriter, req *http.Request) {
			handlerRan = true
			logger, ok := scontext.GetLogger[uint64, reqLogUser](req.Context())
			if !ok || logger == nil {
				t.Errorf("GetLogger = (%v, %v), want a non-nil logger", logger, ok)
				return
			}
			if name := logger.Name(); name != "" {
				t.Errorf("request logger name = %q, want the unnamed application logger", name)
			}
			logger.Info("handler line")
			w.WriteHeader(http.StatusNoContent)
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+reqLogValidToken)
	req.Header.Set("X-Trace-ID", "abc123")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if !handlerRan {
		t.Fatalf("handler did not run; response status = %d", rec.Code)
	}
	if rec.Code != http.StatusNoContent {
		t.Fatalf("response status = %d, want %d", rec.Code, http.StatusNoContent)
	}

	entry := reqLogSingleEntry(t, logs, "handler line")

	wantKeys := []string{logkeys.TraceID, logkeys.BuildID, logkeys.ConfigID, logkeys.UserID}
	if got := reqLogFieldKeys(entry); !slices.Equal(got, wantKeys) {
		t.Errorf("handler line field keys = %v, want %v", got, wantKeys)
	}

	fields := entry.ContextMap()
	if got := fields[logkeys.TraceID]; got != "abc123" {
		t.Errorf("trace_id = %#v, want %q", got, "abc123")
	}
	if got := fields[logkeys.BuildID]; got != "build-1" {
		t.Errorf("build_id = %#v, want %q", got, "build-1")
	}
	if got := fields[logkeys.ConfigID]; got != "config-1" {
		t.Errorf("config_id = %#v, want %q", got, "config-1")
	}
	userID, ok := fields[logkeys.UserID].(uint64)
	if !ok {
		t.Fatalf("user_id = %#v (%T), want a uint64", fields[logkeys.UserID], fields[logkeys.UserID])
	}
	if userID != 4242 {
		t.Errorf("user_id = %d, want %d", userID, 4242)
	}

	if entry.LoggerName != "" {
		t.Errorf("handler line logger name = %q, want the unnamed application logger", entry.LoggerName)
	}
	if _, present := fields["logger"]; present {
		t.Errorf("handler line carries a %q field: %#v", "logger", fields["logger"])
	}

	// The router's own lines keep the "SRouter" name, proving the request
	// logger is derived from the unnamed base rather than the router logger.
	namedFound := false
	for _, logged := range logs.All() {
		if logged.LoggerName == "SRouter" {
			namedFound = true
			break
		}
	}
	if !namedFound {
		t.Errorf("no observed entry has logger name %q: %#v", "SRouter", logs.All())
	}
}

// TestRequestLoggerOmitsUserIDOnAuthOptionalWithoutToken verifies that an
// unauthenticated request produces a logger with no user_id field.
func TestRequestLoggerOmitsUserIDOnAuthOptionalWithoutToken(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	r := NewRouter(RouterConfig{
		Logger:            zap.New(core),
		TraceIDBufferSize: 10,
	}, reqLogUint64Deps(func(id uint64) zap.Field {
		return zap.Uint64(logkeys.UserID, id)
	}))

	authLevel := AuthOptional
	handlerRan := false
	r.Route(RouteConfigBase{
		Path:      "/optional",
		Methods:   []HttpMethod{MethodGet},
		AuthLevel: &authLevel,
		Handler: func(w http.ResponseWriter, req *http.Request) {
			handlerRan = true
			logger, ok := scontext.GetLogger[uint64, reqLogUser](req.Context())
			if !ok || logger == nil {
				t.Errorf("GetLogger = (%v, %v), want a non-nil logger", logger, ok)
				return
			}
			logger.Info("handler line")
			w.WriteHeader(http.StatusNoContent)
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/optional", nil)
	req.Header.Set("X-Trace-ID", "abc123")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if !handlerRan {
		t.Fatalf("handler did not run; response status = %d", rec.Code)
	}

	entry := reqLogSingleEntry(t, logs, "handler line")

	wantKeys := []string{logkeys.TraceID, logkeys.BuildID, logkeys.ConfigID}
	if got := reqLogFieldKeys(entry); !slices.Equal(got, wantKeys) {
		t.Errorf("handler line field keys = %v, want %v", got, wantKeys)
	}

	fields := entry.ContextMap()
	if got := fields[logkeys.TraceID]; got != "abc123" {
		t.Errorf("trace_id = %#v, want %q", got, "abc123")
	}
	if got := fields[logkeys.BuildID]; got != "build-1" {
		t.Errorf("build_id = %#v, want %q", got, "build-1")
	}
	if got := fields[logkeys.ConfigID]; got != "config-1" {
		t.Errorf("config_id = %#v, want %q", got, "config-1")
	}
	if got, present := fields[logkeys.UserID]; present {
		t.Errorf("user_id = %#v, want absent for an unauthenticated request", got)
	}
}

// TestRequestLoggerFallsBackToDefaultLoggerWhenConfigLoggerNil verifies that a
// router built without a configured logger still installs a request logger,
// derived from the unnamed fallback rather than the router's named logger.
func TestRequestLoggerFallsBackToDefaultLoggerWhenConfigLoggerNil(t *testing.T) {
	r := NewRouter(RouterConfig{}, RouterDependencies[string, reqLogUser]{})

	handlerRan := false
	r.Route(RouteConfigBase{
		Path:    "/open",
		Methods: []HttpMethod{MethodGet},
		Handler: func(w http.ResponseWriter, req *http.Request) {
			handlerRan = true
			logger, ok := scontext.GetLogger[string, reqLogUser](req.Context())
			if !ok {
				t.Error("GetLogger reported no request logger, want one derived from the fallback logger")
				return
			}
			if logger == nil {
				t.Error("GetLogger returned a nil logger with ok=true")
				return
			}
			if name := logger.Name(); name != "" {
				t.Errorf("request logger name = %q, want the unnamed fallback logger", name)
			}
			if !logger.Core().Enabled(zapcore.InfoLevel) {
				t.Error("fallback request logger has info level disabled, want the production default")
			}
			// Debug is below the production level, so this writes nothing to
			// stderr while still exercising the logger.
			logger.Debug("handler line")
			w.WriteHeader(http.StatusNoContent)
		},
	})

	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/open", nil))

	if !handlerRan {
		t.Fatalf("handler did not run; response status = %d", rec.Code)
	}
	if rec.Code != http.StatusNoContent {
		t.Fatalf("response status = %d, want %d", rec.Code, http.StatusNoContent)
	}
}

// TestRequestLoggerUserIDFieldNilUsesZapAny verifies that a nil UserIDField
// falls back to zap.Any, which renders a string user ID as a string field.
func TestRequestLoggerUserIDFieldNilUsesZapAny(t *testing.T) {
	core, logs := observer.New(zapcore.DebugLevel)
	r := NewRouter(RouterConfig{
		Logger:            zap.New(core),
		TraceIDBufferSize: 10,
	}, RouterDependencies[string, reqLogUser]{
		Authenticate: func(_ context.Context, token string) (*reqLogUser, bool) {
			if token != reqLogValidToken {
				return nil, false
			}
			return &reqLogUser{id: 4242, name: "ada"}, true
		},
		UserID: func(u *reqLogUser) string {
			if u == nil {
				return ""
			}
			return u.name
		},
		BuildID:  func() string { return "build-1" },
		ConfigID: func() string { return "config-1" },
		// UserIDField intentionally nil: the logger falls back to zap.Any.
	})

	authLevel := AuthRequired
	handlerRan := false
	r.Route(RouteConfigBase{
		Path:      "/protected",
		Methods:   []HttpMethod{MethodGet},
		AuthLevel: &authLevel,
		Handler: func(w http.ResponseWriter, req *http.Request) {
			handlerRan = true
			logger, ok := scontext.GetLogger[string, reqLogUser](req.Context())
			if !ok || logger == nil {
				t.Errorf("GetLogger = (%v, %v), want a non-nil logger", logger, ok)
				return
			}
			logger.Info("handler line")
			w.WriteHeader(http.StatusNoContent)
		},
	})

	req := httptest.NewRequest(http.MethodGet, "/protected", nil)
	req.Header.Set("Authorization", "Bearer "+reqLogValidToken)
	req.Header.Set("X-Trace-ID", "abc123")
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, req)

	if !handlerRan {
		t.Fatalf("handler did not run; response status = %d", rec.Code)
	}

	entry := reqLogSingleEntry(t, logs, "handler line")

	wantKeys := []string{logkeys.TraceID, logkeys.BuildID, logkeys.ConfigID, logkeys.UserID}
	if got := reqLogFieldKeys(entry); !slices.Equal(got, wantKeys) {
		t.Errorf("handler line field keys = %v, want %v", got, wantKeys)
	}

	fields := entry.ContextMap()
	userID, ok := fields[logkeys.UserID].(string)
	if !ok {
		t.Fatalf("user_id = %#v (%T), want a string", fields[logkeys.UserID], fields[logkeys.UserID])
	}
	if userID != "ada" {
		t.Errorf("user_id = %q, want %q", userID, "ada")
	}
}
