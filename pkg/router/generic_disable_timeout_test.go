package router

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/codec"
	"go.uber.org/zap"
)

// slowGenericHandler sleeps past the test timeout before responding successfully.
func slowGenericHandler(r *http.Request, data struct{}) (map[string]string, error) {
	time.Sleep(60 * time.Millisecond)
	return map[string]string{"ok": "true"}, nil
}

func TestGenericRouteDefinitionDisableTimeoutBypassesGlobalTimeout(t *testing.T) {
	newGenericRoute := func(path string, disableTimeout bool) RouteDefinition {
		return RouteConfig[struct{}, map[string]string]{
			Path:           path,
			Methods:        []HttpMethod{MethodGet},
			Codec:          codec.NewJSONCodec[struct{}, map[string]string](),
			SourceType:     Empty,
			DisableTimeout: disableTimeout,
			Handler:        slowGenericHandler,
		}
	}

	r := NewRouter[string, string](RouterConfig{
		Logger:        zap.NewNop(),
		GlobalTimeout: 25 * time.Millisecond,
	}, RouterDependencies[string, string]{})

	r.Group("/api").Route(
		newGenericRoute("/slow-disabled", true),
		newGenericRoute("/slow-enabled", false),
	)

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/slow-disabled", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected disabled timeout generic route to complete with 200, got %d body %q", rr.Code, rr.Body.String())
	}

	// Negative control: the same route without DisableTimeout must hit the global timeout.
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/slow-enabled", nil))
	if rr.Code != http.StatusRequestTimeout {
		t.Fatalf("expected generic route without DisableTimeout to time out with 408, got %d body %q", rr.Code, rr.Body.String())
	}
}

func TestGroupRouteDisableTimeoutBypassesGroupTimeout(t *testing.T) {
	r := NewRouter[string, string](RouterConfig{
		Logger: zap.NewNop(),
	}, RouterDependencies[string, string]{})

	api := r.Group("/api").Timeout(25 * time.Millisecond)

	registerGenericRoute := func(path string, disableTimeout bool) {
		t.Helper()
		api.Route(RouteConfig[struct{}, map[string]string]{
			Path:           path,
			Methods:        []HttpMethod{MethodGet},
			Codec:          codec.NewJSONCodec[struct{}, map[string]string](),
			SourceType:     Empty,
			DisableTimeout: disableTimeout,
			Handler:        slowGenericHandler,
		})
	}

	registerGenericRoute("/slow-disabled", true)
	registerGenericRoute("/slow-enabled", false)

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/slow-disabled", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected disabled timeout dynamic generic route to complete with 200, got %d body %q", rr.Code, rr.Body.String())
	}

	// Negative control: the same route without DisableTimeout must hit the route group timeout.
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/slow-enabled", nil))
	if rr.Code != http.StatusRequestTimeout {
		t.Fatalf("expected dynamic generic route without DisableTimeout to time out with 408, got %d body %q", rr.Code, rr.Body.String())
	}
}
