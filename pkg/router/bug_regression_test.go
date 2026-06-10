package router

import (
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/codec"
	"github.com/Suhaibinator/SRouter/pkg/common"
	"github.com/Suhaibinator/SRouter/pkg/router/internal/mocks"
	"go.uber.org/zap"
)

// countingMiddleware returns a middleware that increments counter on every request.
func countingMiddleware(counter *atomic.Int64) common.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, req *http.Request) {
			counter.Add(1)
			next.ServeHTTP(w, req)
		})
	}
}

// Regression test for BUGS.md #1: StrategyUser rate limits configured via
// RouteOverrides must work (previously every request returned 500 because the
// user ID conversion functions were dropped in the [any, any] -> [T, U]
// conversion).
func TestStrategyUserRateLimitViaOverrides(t *testing.T) {
	r := NewRouter(RouterConfig{
		Logger: zap.NewNop(),
		SubRouters: []SubRouterConfig{{
			PathPrefix: "/api",
			Routes: []RouteDefinition{
				RouteConfigBase{
					Path:    "/limited",
					Methods: []HttpMethod{MethodGet},
					Overrides: common.RouteOverrides{
						RateLimit: &common.RateLimitConfig[any, any]{
							BucketName: "user-bucket",
							Limit:      100,
							Window:     time.Minute,
							Strategy:   common.StrategyUser,
						},
					},
					Handler: func(w http.ResponseWriter, req *http.Request) {
						w.WriteHeader(http.StatusOK)
					},
				},
			},
		}},
	}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	req := httptest.NewRequest(http.MethodGet, "/api/limited", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d for user-strategy rate limited route, got %d (body: %s)",
			http.StatusOK, rr.Code, rr.Body.String())
	}
}

// Regression test for BUGS.md #2: RegisterGenericRouteOnSubRouter must not
// apply global middlewares twice.
func TestRegisterGenericRouteOnSubRouterAppliesGlobalsOnce(t *testing.T) {
	var globalCount atomic.Int64

	r := NewRouter(RouterConfig{
		Logger:      zap.NewNop(),
		Middlewares: []common.Middleware{countingMiddleware(&globalCount)},
		SubRouters:  []SubRouterConfig{{PathPrefix: "/api"}},
	}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	err := RegisterGenericRouteOnSubRouter(r, "/api", RouteConfig[map[string]string, map[string]string]{
		Path:    "/echo",
		Methods: []HttpMethod{MethodGet},
		Codec:   codec.NewJSONCodec[map[string]string, map[string]string](),
		Handler: func(req *http.Request, data map[string]string) (map[string]string, error) {
			return map[string]string{"ok": "true"}, nil
		},
		SourceType: Empty,
		Sanitizer:  func(d map[string]string) (map[string]string, error) { return d, nil },
	})
	if err != nil {
		t.Fatalf("RegisterGenericRouteOnSubRouter failed: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/api/echo", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d (body: %s)", http.StatusOK, rr.Code, rr.Body.String())
	}
	if got := globalCount.Load(); got != 1 {
		t.Fatalf("expected global middleware to run exactly once per request, ran %d times", got)
	}
}

// Regression test for BUGS.md #3: nested sub-routers must inherit the parent
// sub-router's middlewares (and AuthLevel) as documented.
func TestNestedSubRouterInheritsParentMiddlewares(t *testing.T) {
	var parentCount, childCount atomic.Int64

	r := NewRouter(RouterConfig{
		Logger: zap.NewNop(),
		SubRouters: []SubRouterConfig{{
			PathPrefix:  "/api",
			Middlewares: []common.Middleware{countingMiddleware(&parentCount)},
			SubRouters: []SubRouterConfig{{
				PathPrefix:  "/v1",
				Middlewares: []common.Middleware{countingMiddleware(&childCount)},
				Routes: []RouteDefinition{
					RouteConfigBase{
						Path:    "/ping",
						Methods: []HttpMethod{MethodGet},
						Handler: func(w http.ResponseWriter, req *http.Request) {
							w.WriteHeader(http.StatusOK)
						},
					},
				},
			}},
		}},
	}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	req := httptest.NewRequest(http.MethodGet, "/api/v1/ping", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)

	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
	if got := parentCount.Load(); got != 1 {
		t.Fatalf("expected parent sub-router middleware to run once for nested route, ran %d times", got)
	}
	if got := childCount.Load(); got != 1 {
		t.Fatalf("expected nested sub-router middleware to run once, ran %d times", got)
	}
}

// Regression test for BUGS.md #3 (AuthLevel part): a nested sub-router without
// its own AuthLevel inherits the parent's.
func TestNestedSubRouterInheritsParentAuthLevel(t *testing.T) {
	authRequired := AuthRequired

	r := NewRouter(RouterConfig{
		Logger: zap.NewNop(),
		SubRouters: []SubRouterConfig{{
			PathPrefix: "/api",
			AuthLevel:  &authRequired,
			SubRouters: []SubRouterConfig{{
				PathPrefix: "/v1",
				Routes: []RouteDefinition{
					RouteConfigBase{
						Path:    "/secret",
						Methods: []HttpMethod{MethodGet},
						Handler: func(w http.ResponseWriter, req *http.Request) {
							w.WriteHeader(http.StatusOK)
						},
					},
				},
			}},
		}},
	}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	// Without credentials: must be rejected because AuthRequired is inherited.
	req := httptest.NewRequest(http.MethodGet, "/api/v1/secret", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusUnauthorized {
		t.Fatalf("expected status %d for unauthenticated request to nested auth-required route, got %d",
			http.StatusUnauthorized, rr.Code)
	}

	// With valid credentials: allowed.
	req = httptest.NewRequest(http.MethodGet, "/api/v1/secret", nil)
	req.Header.Set("Authorization", "Bearer valid-token")
	rr = httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d for authenticated request, got %d", http.StatusOK, rr.Code)
	}
}

// Regression test for BUGS.md #15: sub-routers added via RegisterSubRouter
// after router creation must be discoverable by RegisterGenericRouteOnSubRouter.
func TestRegisterGenericRouteOnDynamicallyAddedSubRouter(t *testing.T) {
	r := NewRouter(RouterConfig{Logger: zap.NewNop()}, mocks.MockAuthFunction, mocks.MockUserIDFromUser)

	r.RegisterSubRouter(SubRouterConfig{PathPrefix: "/late"})

	err := RegisterGenericRouteOnSubRouter(r, "/late", RouteConfig[map[string]string, map[string]string]{
		Path:    "/route",
		Methods: []HttpMethod{MethodGet},
		Codec:   codec.NewJSONCodec[map[string]string, map[string]string](),
		Handler: func(req *http.Request, data map[string]string) (map[string]string, error) {
			return map[string]string{"ok": "true"}, nil
		},
		SourceType: Empty,
		Sanitizer:  func(d map[string]string) (map[string]string, error) { return d, nil },
	})
	if err != nil {
		t.Fatalf("expected dynamically added sub-router to be found, got error: %v", err)
	}

	req := httptest.NewRequest(http.MethodGet, "/late/route", nil)
	rr := httptest.NewRecorder()
	r.ServeHTTP(rr, req)
	if rr.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, rr.Code)
	}
}
