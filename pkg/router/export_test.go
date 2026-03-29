package router

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/codec"
	"github.com/Suhaibinator/SRouter/pkg/common"
)

type exportReq struct {
	Query string `json:"query"`
}

type exportResp struct {
	Result string `json:"result"`
}

func TestExportSpecIncludesRegisteredRoutes(t *testing.T) {
	subAuth := AuthRequired
	cfg := RouterConfig{
		ServiceName:       "export-test",
		GlobalTimeout:     5 * time.Second,
		GlobalMaxBodySize: 1024,
		CORSConfig: &CORSConfig{
			Origins: []string{"https://example.com"},
			Methods: []string{"GET", "POST"},
			Headers: []string{"Content-Type"},
			MaxAge:  24 * time.Hour,
		},
		SubRouters: []SubRouterConfig{
			{
				PathPrefix: "/api",
				AuthLevel:  &subAuth,
				Routes: []RouteDefinition{
					RouteConfigBase{
						Path:    "/health",
						Methods: []HttpMethod{MethodGet},
						Handler: func(http.ResponseWriter, *http.Request) {},
					},
					NewGenericRouteDefinition[exportReq, exportResp, string, string](RouteConfig[exportReq, exportResp]{
						Path:       "/search",
						Methods:    []HttpMethod{MethodPost},
						Codec:      codec.NewJSONCodec[exportReq, exportResp](),
						SourceType: Body,
						Handler: func(r *http.Request, data exportReq) (exportResp, error) {
							return exportResp{Result: data.Query}, nil
						},
					}),
				},
			},
		},
	}

	r := NewRouter[string, string](cfg, func(context.Context, string) (*string, bool) { return nil, false }, func(*string) string { return "" })
	spec := r.ExportSpec()
	if spec.Version != exportSpecVersion {
		t.Fatalf("unexpected version: %q", spec.Version)
	}
	if len(spec.Routes) != 2 {
		t.Fatalf("expected 2 routes, got %d", len(spec.Routes))
	}
	if spec.Service.Name != "export-test" {
		t.Fatalf("unexpected service name: %q", spec.Service.Name)
	}

	var foundGeneric bool
	for _, route := range spec.Routes {
		if route.Path == "/api/search" {
			foundGeneric = true
			if route.Request == nil || route.Request.Codec != "json" {
				t.Fatalf("expected generic request schema with json codec, got: %+v", route.Request)
			}
			if route.Response == nil || route.Response.Kind != "struct" {
				t.Fatalf("expected response schema, got: %+v", route.Response)
			}
		}
	}
	if !foundGeneric {
		t.Fatal("expected to find /api/search route in export")
	}
}

func TestExportJSONFile(t *testing.T) {
	r := NewRouter[string, string](RouterConfig{}, nil, nil)
	r.RegisterRoute(RouteConfigBase{
		Path:    "/healthz",
		Methods: []HttpMethod{MethodGet},
		Overrides: common.RouteOverrides{
			Timeout: 2 * time.Second,
		},
		Handler: func(http.ResponseWriter, *http.Request) {},
	})

	path := filepath.Join(t.TempDir(), "spec.json")
	if err := r.ExportJSONFile(path); err != nil {
		t.Fatalf("ExportJSONFile returned error: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("failed reading export file: %v", err)
	}

	var spec ExportSpec
	if err := json.Unmarshal(data, &spec); err != nil {
		t.Fatalf("invalid export JSON: %v", err)
	}
	if len(spec.Routes) != 1 || spec.Routes[0].Path != "/healthz" {
		t.Fatalf("unexpected route export: %+v", spec.Routes)
	}
}
