package routetree

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Suhaibinator/SRouter/pkg/scontext"
)

// These tests cover paths the spec tests in table_test.go do not reach:
// hashed child sets, very long routes, caller-supplied parameter buffers,
// and ServeUnmatched called directly.

// hashedChildren returns routes that give one trie node more static children
// than indexThreshold, so its children are hashed rather than scanned.
func hashedChildren() []testRoute {
	var routes []testRoute
	for i := range indexThreshold * 2 {
		routes = append(routes, get(fmt.Sprintf("/h/c%d/:id", i)))
	}
	return routes
}

func TestHashedChildrenMissAndReuse(t *testing.T) {
	routes := hashedChildren()
	// A second pattern through an existing hashed child must reuse it.
	routes = append(routes, get("/h/c3/:id/more"))
	table := newTestTable(t, routes...)

	tests := []struct {
		target string
		code   int
		body   string
	}{
		{"/h/c3/7", 200, `/h/c3/:id [id="7"]`},
		{"/h/c3/7/more", 200, `/h/c3/:id/more [id="7"]`},
		{"/h/missing/7", 404, ""},
		{"/h/c99/7", 404, ""},
	}
	for _, tt := range tests {
		rec := serve(table, http.MethodGet, tt.target)
		if rec.Code != tt.code || (tt.body != "" && rec.Body.String() != tt.body) {
			t.Fatalf("GET %s = %d %q; want %d %q", tt.target, rec.Code, rec.Body.String(), tt.code, tt.body)
		}
	}
}

func TestRouteWithMoreParametersThanTheStackBuffer(t *testing.T) {
	n := stackParams + 3
	var pattern, path strings.Builder
	var want []string
	for i := range n {
		fmt.Fprintf(&pattern, "/:p%d", i)
		fmt.Fprintf(&path, "/v%d", i)
		want = append(want, fmt.Sprintf("p%d=%q", i, fmt.Sprintf("v%d", i)))
	}
	// The sibling makes the matcher backtrack through the extra parameters
	// before it finds the long route.
	table := newTestTable(t, get(pattern.String()), get(pattern.String()+"/tail"))

	match, ok := table.Lookup(http.MethodGet, path.String(), nil)
	if !ok || len(match.Params) != n {
		t.Fatalf("Lookup = %+v, %v; want %d params", match, ok, n)
	}
	if got := describeMatch(match.Pattern, match.Params); got != pattern.String()+" ["+strings.Join(want, " ")+"]" {
		t.Fatalf("match = %s", got)
	}
	if _, ok := table.Lookup(http.MethodGet, path.String()+"/other", nil); ok {
		t.Fatal("Lookup matched an unregistered path")
	}
}

func TestLookupAppendsToAFullBuffer(t *testing.T) {
	table := newTestTable(t, get("/users/:id/posts/:post"))
	existing := scontext.Params{{Key: "keep", Value: "me"}}

	match, ok := table.Lookup(http.MethodGet, "/users/1/posts/2", existing)
	if !ok {
		t.Fatal("Lookup did not match")
	}
	want := scontext.Params{{Key: "keep", Value: "me"}, {Key: "id", Value: "1"}, {Key: "post", Value: "2"}}
	if fmt.Sprint(match.Params) != fmt.Sprint(want) {
		t.Fatalf("Params = %v; want %v", match.Params, want)
	}
}

func TestTrailingSlashHintFromSibling(t *testing.T) {
	// The static tail "/dir/" hangs below a parameter, so the trie, not the
	// static map, must notice that /v/dir only lacks a trailing slash.
	table := newTestTable(t, get("/:x/dir/"), get("/:x/other"))
	rec := serve(table, http.MethodGet, "/v/dir")
	if rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != "/v/dir/" {
		t.Fatalf("GET /v/dir = %d Location=%q; want 301 /v/dir/", rec.Code, rec.Header().Get("Location"))
	}
}

func TestServeUnmatchedDirectly(t *testing.T) {
	table := newTestTable(t, get("/users/:id"), testRoute{http.MethodPost, "/users/:id"})
	tests := []struct {
		method, target string
		code           int
		location       string
	}{
		{http.MethodGet, "/users/1/", http.StatusMovedPermanently, "/users/1"},
		{http.MethodPost, "//users/1", http.StatusTemporaryRedirect, "/users/1"},
		{http.MethodPut, "/users/1", http.StatusMethodNotAllowed, ""},
		{http.MethodGet, "/nothing", http.StatusNotFound, ""},
	}
	for _, tt := range tests {
		rec := httptest.NewRecorder()
		table.ServeUnmatched(rec, httptest.NewRequest(tt.method, tt.target, nil))
		if rec.Code != tt.code || rec.Header().Get("Location") != tt.location {
			t.Fatalf("%s %s = %d Location=%q; want %d %q", tt.method, tt.target, rec.Code, rec.Header().Get("Location"), tt.code, tt.location)
		}
	}
}

func TestTraceMethod(t *testing.T) {
	table := newTestTable(t, testRoute{http.MethodTrace, "/debug/:id"})
	if rec := serve(table, http.MethodTrace, "/debug/1"); rec.Body.String() != `/debug/:id [id="1"]` {
		t.Fatalf("TRACE /debug/1 = %d %q", rec.Code, rec.Body.String())
	}
}

func TestCleanedRootWithoutRootRoute(t *testing.T) {
	// "/./" cleans to "/", which has no route and no trailing-slash variant.
	table := newTestTable(t, get("/a"))
	if rec := serve(table, http.MethodGet, "/./"); rec.Code != http.StatusNotFound {
		t.Fatalf("GET /./ = %d; want 404", rec.Code)
	}
}

func TestIsCleanPathWithDotPrefixedSegments(t *testing.T) {
	for _, path := range []string{"/.well-known/x", "/a/.b", "/a/..b/"} {
		if !isCleanPath(path) {
			t.Fatalf("isCleanPath(%q) = false; want true", path)
		}
	}
}

func TestRedirectEscapesNonASCIIQueryBytes(t *testing.T) {
	table := newTestTable(t, get("/users/:id"))
	req := httptest.NewRequest(http.MethodGet, "/users/1/", nil)
	req.URL.RawQuery = "q=é&x=1"
	rec := httptest.NewRecorder()
	table.ServeHTTP(rec, req)
	if got, want := rec.Header().Get("Location"), "/users/1?q=%c3%a9&x=1"; got != want {
		t.Fatalf("Location = %q; want %q", got, want)
	}
}
