package router

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/codec"
	"github.com/Suhaibinator/SRouter/pkg/common"
	"go.uber.org/zap"
)

func TestGenericRouteDefinitionDisableTimeoutHonorsGlobalTimeout(t *testing.T) {
	r := NewRouter[string, string](RouterConfig{
		Logger:        zap.NewNop(),
		GlobalTimeout: 25 * time.Millisecond,
		SubRouters: []SubRouterConfig{{
			PathPrefix: "/api",
			Routes: []RouteDefinition{
				NewGenericRouteDefinition[struct{}, map[string]string, string, string](RouteConfig[struct{}, map[string]string]{
					Path:           "/slow",
					Methods:        []HttpMethod{MethodGet},
					Codec:          codec.NewJSONCodec[struct{}, map[string]string](),
					SourceType:     Empty,
					DisableTimeout: true,
					Handler: func(r *http.Request, data struct{}) (map[string]string, error) {
						time.Sleep(60 * time.Millisecond)
						return map[string]string{"ok": "true"}, nil
					},
				}),
			},
		}},
	}, nil, nil)

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/slow", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected disabled timeout generic route to complete with 200, got %d body %q", rr.Code, rr.Body.String())
	}
}

func TestRegisterGenericRouteOnSubRouterDisableTimeoutHonorsSubRouterTimeout(t *testing.T) {
	r := NewRouter[string, string](RouterConfig{
		Logger: zap.NewNop(),
		SubRouters: []SubRouterConfig{{
			PathPrefix: "/api",
			Overrides: common.RouteOverrides{
				Timeout: 25 * time.Millisecond,
			},
		}},
	}, nil, nil)

	err := RegisterGenericRouteOnSubRouter(r, "/api", RouteConfig[struct{}, map[string]string]{
		Path:           "/slow",
		Methods:        []HttpMethod{MethodGet},
		Codec:          codec.NewJSONCodec[struct{}, map[string]string](),
		SourceType:     Empty,
		DisableTimeout: true,
		Handler: func(r *http.Request, data struct{}) (map[string]string, error) {
			time.Sleep(60 * time.Millisecond)
			return map[string]string{"ok": "true"}, nil
		},
	})
	if err != nil {
		t.Fatalf("RegisterGenericRouteOnSubRouter failed: %v", err)
	}

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/slow", nil))
	if rr.Code != http.StatusOK {
		t.Fatalf("expected disabled timeout dynamic generic route to complete with 200, got %d body %q", rr.Code, rr.Body.String())
	}
}
