package router

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"go.uber.org/zap"
)

// These tests pin router-level behaviour of the route table through the full
// Router stack. See docs/plans/route-table.md.

func newRouteTableTestRouter(t *testing.T, routes ...RouteConfigBase) *Router[string, string] {
	t.Helper()
	r := NewRouter(RouterConfig{Logger: zap.NewNop()}, RouterDependencies[string, string]{})
	for _, route := range routes {
		r.Route(route)
	}
	if err := r.Build(); err != nil {
		t.Fatalf("Build: %v", err)
	}
	return r
}

// echoRoute writes the route template it was registered with and its
// parameters, so tests can see which route matched.
func echoRoute(path string, methods ...HttpMethod) RouteConfigBase {
	return RouteConfigBase{
		Path:    path,
		Methods: methods,
		Handler: func(w http.ResponseWriter, req *http.Request) {
			var parts []string
			for _, p := range GetParams(req) {
				parts = append(parts, p.Key+"="+p.Value)
			}
			_, _ = fmt.Fprintf(w, "%s %s [%s]", req.Method, path, strings.Join(parts, " "))
		},
	}
}

func serveRouter(r http.Handler, method, target string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	r.ServeHTTP(rec, httptest.NewRequest(method, target, nil))
	return rec
}

func TestRouteTableStaticAndParameterSegmentsCoexist(t *testing.T) {
	r := newRouteTableTestRouter(t,
		echoRoute("/users/:id", MethodGet),
		echoRoute("/users/new", MethodGet),
		echoRoute("/users/:id/profile", MethodGet),
		echoRoute("/files/*path", MethodGet),
	)

	tests := []struct{ target, want string }{
		{"/users/new", "GET /users/new []"},
		{"/users/42", "GET /users/:id [id=42]"},
		{"/users/new/profile", "GET /users/:id/profile [id=new]"},
		{"/files/a/b.txt", "GET /files/*path [path=/a/b.txt]"},
	}
	for _, tt := range tests {
		if rec := serveRouter(r, http.MethodGet, tt.target); rec.Code != http.StatusOK || rec.Body.String() != tt.want {
			t.Errorf("GET %s = %d %q; want 200 %q", tt.target, rec.Code, rec.Body.String(), tt.want)
		}
	}
}

func TestRouteTableUnmatchedRequests(t *testing.T) {
	r := newRouteTableTestRouter(t,
		echoRoute("/health", MethodGet, MethodOptions),
		echoRoute("/items/:id", MethodGet, MethodPut),
		echoRoute("/submit", MethodPost),
	)

	tests := []struct {
		name, method, target string
		code                 int
		location, allow      string
		body                 string
	}{
		{name: "explicit OPTIONS reaches its handler", method: "OPTIONS", target: "/health", code: 200, body: "OPTIONS /health []"},
		{name: "automatic OPTIONS", method: "OPTIONS", target: "/items/1", code: 200, allow: "GET, OPTIONS, PUT"},
		{name: "method not allowed", method: "DELETE", target: "/items/1", code: 405, allow: "GET, OPTIONS, PUT"},
		{name: "method not allowed on static route", method: "GET", target: "/submit", code: 405, allow: "OPTIONS, POST"},
		{name: "not found", method: "GET", target: "/items/1/extra", code: 404},
		{name: "trailing slash GET", method: "GET", target: "/items/1/", code: 301, location: "/items/1"},
		{name: "trailing slash PUT", method: "PUT", target: "/items/1/?q=1", code: 307, location: "/items/1?q=1"},
		{name: "cleaned path", method: "GET", target: "//items//1", code: 301, location: "/items/1"},
		{name: "no case-correcting redirect", method: "GET", target: "/ITEMS/1/extra", code: 404},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			rec := serveRouter(r, tt.method, tt.target)
			if rec.Code != tt.code {
				t.Fatalf("status = %d; want %d (body %q)", rec.Code, tt.code, rec.Body.String())
			}
			if got := rec.Header().Get("Location"); got != tt.location {
				t.Fatalf("Location = %q; want %q", got, tt.location)
			}
			if got := rec.Header().Get("Allow"); got != tt.allow {
				t.Fatalf("Allow = %q; want %q", got, tt.allow)
			}
			if tt.body != "" && rec.Body.String() != tt.body {
				t.Fatalf("body = %q; want %q", rec.Body.String(), tt.body)
			}
		})
	}
}

func TestRouteTableNoOpenRedirect(t *testing.T) {
	r := newRouteTableTestRouter(t, echoRoute("/:slug", MethodGet))
	server := httptest.NewServer(r)
	defer server.Close()
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	for _, raw := range []string{"//evil.com/../", "//evil.com/", "/%5Cevil.com/", "///evil.com/../"} {
		req, err := http.NewRequest(http.MethodGet, server.URL, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.URL.Opaque = raw
		resp, err := client.Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
		location := resp.Header.Get("Location")
		if location == "" {
			continue
		}
		target, err := req.URL.Parse(location)
		if err != nil || target.Host != req.URL.Host {
			t.Fatalf("GET %s: Location %q leaves the host", raw, location)
		}
	}
}

func TestRouteTableBuildErrors(t *testing.T) {
	tests := []struct {
		name   string
		routes []RouteConfigBase
		want   string
	}{
		{"mid-segment wildcard", []RouteConfigBase{echoRoute("/user_:name", MethodGet)}, "must fill the whole segment"},
		{"repeated wildcard name", []RouteConfigBase{echoRoute("/a/:id/b/:id", MethodGet)}, "used more than once"},
		{"catch-all not last", []RouteConfigBase{echoRoute("/f/*path/x", MethodGet)}, "must be the final segment"},
		{"conflicting parameter names", []RouteConfigBase{echoRoute("/u/:id", MethodGet), echoRoute("/u/:name", MethodGet)}, "register GET /u/:name:"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			r := NewRouter(RouterConfig{Logger: zap.NewNop()}, RouterDependencies[string, string]{})
			for _, route := range tt.routes {
				r.Route(route)
			}
			if err := r.Build(); err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Build error = %v; want containing %q", err, tt.want)
			}
		})
	}
}
