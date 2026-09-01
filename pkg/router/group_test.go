package router

import (
	"net/http"
	"net/http/httptest"
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/codec"
	"github.com/Suhaibinator/SRouter/pkg/common"
	"github.com/Suhaibinator/SRouter/pkg/router/internal/mocks"
	"go.uber.org/zap"
)

func TestRouteGroupRegistersNestedRoutes(t *testing.T) {
	r := NewRouter(RouterConfig{Logger: zap.NewNop()}, RouterDependencies[string, string]{Authenticate: mocks.MockAuthFunction, UserID: mocks.MockUserIDFromUser})
	api := r.Group("/api")
	v1 := api.Group("/v1")
	users := v1.Group("/users")
	users.Route(RouteConfigBase{
		Path:    "",
		Methods: []HttpMethod{MethodGet},
		Handler: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		},
	})

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/users", nil))
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected status %d, got %d", http.StatusNoContent, rr.Code)
	}
}

func TestRouteGroupMiddlewareOrder(t *testing.T) {
	var order []string
	middleware := func(name string) common.Middleware {
		return func(next http.Handler) http.Handler {
			return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
				order = append(order, name+" before")
				next.ServeHTTP(w, req)
				order = append(order, name+" after")
			})
		}
	}

	r := NewRouter(RouterConfig{
		Logger:      zap.NewNop(),
		Middlewares: []common.Middleware{middleware("global")},
	}, RouterDependencies[string, string]{Authenticate: mocks.MockAuthFunction, UserID: mocks.MockUserIDFromUser})

	api := r.Group("/api").Use(middleware("api"))
	api.Group("/v1").Use(middleware("v1")).Route(RouteConfigBase{
		Path:        "/ping",
		Methods:     []HttpMethod{MethodGet},
		Middlewares: []common.Middleware{middleware("route")},
		Handler: func(w http.ResponseWriter, _ *http.Request) {
			order = append(order, "handler")
			w.WriteHeader(http.StatusNoContent)
		},
	})

	r.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/api/v1/ping", nil))
	want := []string{
		"global before", "api before", "v1 before", "route before", "handler",
		"route after", "v1 after", "api after", "global after",
	}
	if !reflect.DeepEqual(order, want) {
		t.Fatalf("unexpected middleware order:\n got: %v\nwant: %v", order, want)
	}
}

func TestRouteGroupPolicyInheritanceAndSelectiveDisable(t *testing.T) {
	r := NewRouter(RouterConfig{
		Logger:            zap.NewNop(),
		GlobalTimeout:     time.Nanosecond,
		GlobalMaxBodySize: 1,
		GlobalRateLimit: &common.RateLimitConfig[any, any]{
			Limit:  1,
			Window: time.Hour,
		},
	}, RouterDependencies[string, string]{Authenticate: mocks.MockAuthFunction, UserID: mocks.MockUserIDFromUser})

	api := r.Group("/api").Timeout(0).MaxBodySize(0).RateLimit(nil).Auth(AuthRequired)
	api.Route(RouteConfigBase{
		Path:    "/open",
		Methods: []HttpMethod{MethodPost},
		Handler: func(w http.ResponseWriter, _ *http.Request) {
			time.Sleep(time.Millisecond)
			w.WriteHeader(http.StatusNoContent)
		},
		AuthLevel: new(NoAuth),
	})

	for range 2 {
		rr := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodPost, "/api/open", http.NoBody)
		r.ServeHTTP(rr, req)
		if rr.Code != http.StatusNoContent {
			t.Fatalf("expected disabled inherited policies to allow request, got %d", rr.Code)
		}
	}
}

func TestRouteGroupAuthInheritance(t *testing.T) {
	r := NewRouter(RouterConfig{Logger: zap.NewNop()}, RouterDependencies[string, string]{Authenticate: mocks.MockAuthFunction, UserID: mocks.MockUserIDFromUser})
	api := r.Group("/api").Auth(AuthRequired)
	api.Group("/v1").Route(RouteConfigBase{
		Path:    "/secret",
		Methods: []HttpMethod{MethodGet},
		Handler: func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusNoContent) },
	})

	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, httptest.NewRequest(http.MethodGet, "/api/v1/secret", nil))
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected inherited authentication to reject request, got %d", rr.Code)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/v1/secret", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusNoContent {
		t.Fatalf("expected authenticated request to pass, got %d", rr.Code)
	}
}

func TestBuildRejectsInvalidAndDuplicateRoutes(t *testing.T) {
	tests := []struct {
		name  string
		setup func(*Router[string, string])
	}{
		{
			name: "invalid group prefix",
			setup: func(r *Router[string, string]) {
				r.Group("api").Route(RouteConfigBase{Path: "/x", Methods: []HttpMethod{MethodGet}, Handler: func(http.ResponseWriter, *http.Request) {}})
			},
		},
		{
			name: "missing handler",
			setup: func(r *Router[string, string]) {
				r.Route(RouteConfigBase{Path: "/x", Methods: []HttpMethod{MethodGet}})
			},
		},
		{
			name: "missing typed handler",
			setup: func(r *Router[string, string]) {
				r.Route(RouteConfig[string, string]{
					Path:    "/x",
					Methods: []HttpMethod{MethodPost},
					Codec:   codec.NewJSONCodec[string, string](),
				})
			},
		},
		{
			name: "missing typed codec",
			setup: func(r *Router[string, string]) {
				r.Route(RouteConfig[string, string]{
					Path:    "/x",
					Methods: []HttpMethod{MethodPost},
					Handler: func(*http.Request, string) (string, error) { return "", nil },
				})
			},
		},
		{
			name: "invalid typed source",
			setup: func(r *Router[string, string]) {
				r.Route(RouteConfig[string, string]{
					Path:       "/x",
					Methods:    []HttpMethod{MethodPost},
					Codec:      codec.NewJSONCodec[string, string](),
					Handler:    func(*http.Request, string) (string, error) { return "", nil },
					SourceType: SourceType(999),
				})
			},
		},
		{
			name: "missing typed query source key",
			setup: func(r *Router[string, string]) {
				r.Route(RouteConfig[string, string]{
					Path:       "/x",
					Methods:    []HttpMethod{MethodGet},
					Codec:      codec.NewJSONCodec[string, string](),
					Handler:    func(*http.Request, string) (string, error) { return "", nil },
					SourceType: Base64QueryParameter,
				})
			},
		},
		{
			name: "authentication without dependencies",
			setup: func(r *Router[string, string]) {
				r.Auth(AuthRequired).Route(RouteConfigBase{
					Path:    "/x",
					Methods: []HttpMethod{MethodGet},
					Handler: func(http.ResponseWriter, *http.Request) {},
				})
			},
		},
		{
			name: "duplicate route",
			setup: func(r *Router[string, string]) {
				r.Route(
					RouteConfigBase{Path: "/x", Methods: []HttpMethod{MethodGet}, Handler: func(http.ResponseWriter, *http.Request) {}},
					RouteConfigBase{Path: "/x", Methods: []HttpMethod{MethodGet}, Handler: func(http.ResponseWriter, *http.Request) {}},
				)
			},
		},
		{
			name: "negative group timeout",
			setup: func(r *Router[string, string]) {
				r.Group("/api").Timeout(-time.Second).Route(RouteConfigBase{Path: "/x", Methods: []HttpMethod{MethodGet}, Handler: func(http.ResponseWriter, *http.Request) {}})
			},
		},
		{
			name: "nil group middleware",
			setup: func(r *Router[string, string]) {
				r.Group("/api").Use(nil).Route(RouteConfigBase{Path: "/x", Methods: []HttpMethod{MethodGet}, Handler: func(http.ResponseWriter, *http.Request) {}})
			},
		},
		{
			name: "middleware panic",
			setup: func(r *Router[string, string]) {
				r.Group("/api").Use(func(http.Handler) http.Handler { panic("bad middleware") }).Route(RouteConfigBase{Path: "/x", Methods: []HttpMethod{MethodGet}, Handler: func(http.ResponseWriter, *http.Request) {}})
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRouter[string, string](RouterConfig{Logger: zap.NewNop()}, RouterDependencies[string, string]{})
			tt.setup(r)
			if err := r.Build(); err == nil {
				t.Fatal("expected Build to reject invalid route tree")
			}
		})
	}
}

func TestBuildFreezesRouteTree(t *testing.T) {
	r := NewRouter[string, string](RouterConfig{Logger: zap.NewNop()}, RouterDependencies[string, string]{})
	r.Route(RouteConfigBase{Path: "/ready", Methods: []HttpMethod{MethodGet}, Handler: func(http.ResponseWriter, *http.Request) {}})
	if err := r.Build(); err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	if err := r.Build(); err != nil {
		t.Fatalf("second Build failed: %v", err)
	}

	defer func() {
		if recover() == nil {
			t.Fatal("expected mutation after Build to panic")
		}
	}()
	r.Route(RouteConfigBase{Path: "/late", Methods: []HttpMethod{MethodGet}, Handler: func(http.ResponseWriter, *http.Request) {}})
}

func TestConcurrentFirstRequestBuildsOnce(t *testing.T) {
	r := NewRouter[string, string](RouterConfig{Logger: zap.NewNop()}, RouterDependencies[string, string]{})
	r.Group("/api").Group("/v1").Route(RouteConfigBase{
		Path:    "/ready",
		Methods: []HttpMethod{MethodGet},
		Handler: func(w http.ResponseWriter, _ *http.Request) {
			w.WriteHeader(http.StatusNoContent)
		},
	})

	const requestCount = 32
	start := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(requestCount)
	for range requestCount {
		go func() {
			defer wg.Done()
			<-start
			recorder := httptest.NewRecorder()
			r.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/api/v1/ready", nil))
			if recorder.Code != http.StatusNoContent {
				t.Errorf("expected status %d, got %d", http.StatusNoContent, recorder.Code)
			}
		}()
	}
	close(start)
	wg.Wait()

	if !r.routeTree.ready.Load() {
		t.Fatal("expected route tree to be published after the first requests")
	}
}
