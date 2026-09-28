package routetree

import (
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Suhaibinator/SRouter/pkg/scontext"
)

type testRoute struct{ method, pattern string }

// echoHandle writes the matched pattern and parameters so tests can assert on
// the response body.
func echoHandle(pattern string) Handle {
	return func(w http.ResponseWriter, _ *http.Request, params scontext.Params) {
		_, _ = fmt.Fprint(w, describeMatch(pattern, params))
	}
}

func describeMatch(pattern string, params scontext.Params) string {
	parts := make([]string, 0, len(params))
	for _, p := range params {
		parts = append(parts, fmt.Sprintf("%s=%q", p.Key, p.Value))
	}
	return pattern + " [" + strings.Join(parts, " ") + "]"
}

func newTestTable(t *testing.T, routes ...testRoute) *Table {
	t.Helper()
	table := New()
	for _, r := range routes {
		if err := table.Add(r.method, r.pattern, echoHandle(r.pattern)); err != nil {
			t.Fatalf("Add(%s %s): %v", r.method, r.pattern, err)
		}
	}
	return table
}

func get(pattern string) testRoute { return testRoute{http.MethodGet, pattern} }

func serve(table http.Handler, method, target string) *httptest.ResponseRecorder {
	rec := httptest.NewRecorder()
	table.ServeHTTP(rec, httptest.NewRequest(method, target, nil))
	return rec
}

func TestAddRejectsInvalidPatterns(t *testing.T) {
	tests := []struct {
		pattern string
		want    string
	}{
		{"", "must begin with '/'"},
		{"users", "must begin with '/'"},
		{"/users/:", "non-empty name"},
		{"/files/*", "non-empty name"},
		{"/files/*path/x", "must be the final segment"},
		{"/f*path", "must fill the whole segment"},
		{"/user_:name", "must fill the whole segment"},
		{"/v1/items:batch", "must fill the whole segment"},
		{"/a/:b:c", "only one wildcard"},
		{"/a/:b*c", "only one wildcard"},
		{"/a/:id/b/:id", "used more than once"},
		{"/a/:id/*id", "used more than once"},
	}
	for _, tt := range tests {
		t.Run(tt.pattern, func(t *testing.T) {
			err := New().Add(http.MethodGet, tt.pattern, echoHandle(tt.pattern))
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("Add(%q) error = %v; want containing %q", tt.pattern, err, tt.want)
			}
		})
	}
}

func TestAddRejectsEmptyMethodAndNilHandle(t *testing.T) {
	if err := New().Add("", "/x", echoHandle("/x")); err == nil {
		t.Fatal("Add with empty method succeeded")
	}
	if err := New().Add(http.MethodGet, "/x", nil); err == nil {
		t.Fatal("Add with nil handle succeeded")
	}
}

func TestAddConflicts(t *testing.T) {
	tests := []struct {
		name   string
		routes []testRoute
		want   string // "" means every route is accepted
	}{
		{"parameter names", []testRoute{get("/users/:id"), get("/users/:name")}, `conflicts with parameter ":id" in pattern "/users/:id"`},
		{"parameter names deeper", []testRoute{get("/a/:x/b"), get("/a/:y/c")}, "conflicts with parameter"},
		{"catch-all names", []testRoute{get("/f/*path"), get("/f/*rest")}, `conflicts with catch-all "*path"`},
		{"duplicate static", []testRoute{get("/x"), get("/x")}, "already registered"},
		{"duplicate wildcard", []testRoute{get("/users/:id"), get("/users/:id")}, "already registered"},
		{"duplicate catch-all", []testRoute{get("/f/*path"), get("/f/*path")}, "already registered"},
		{"parameter names on different methods", []testRoute{get("/users/:id"), {http.MethodPost, "/users/:name"}}, ""},
		{"static next to parameter", []testRoute{get("/users/:id"), get("/users/new")}, ""},
		{"static before parameter", []testRoute{get("/users/new"), get("/users/:id")}, ""},
		{"parameter next to catch-all", []testRoute{get("/f/:id"), get("/f/*path")}, ""},
		{"static next to catch-all", []testRoute{get("/f/x"), get("/f/*path")}, ""},
		{"directory next to catch-all", []testRoute{get("/src/"), get("/src/*p")}, ""},
		{"root catch-all next to static", []testRoute{get("/*path"), get("/x")}, ""},
		{"parameter prefixes", []testRoute{get("/users/:id"), get("/users/:id/x"), get("/users")}, ""},
		{"trailing slash variants", []testRoute{get("/a"), get("/a/")}, ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			table := New()
			var err error
			for _, r := range tt.routes {
				if err = table.Add(r.method, r.pattern, echoHandle(r.pattern)); err != nil {
					break
				}
			}
			if tt.want == "" {
				if err != nil {
					t.Fatalf("unexpected error: %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), tt.want) {
				t.Fatalf("error = %v; want containing %q", err, tt.want)
			}
		})
	}
}

func TestFailedAddLeavesTableUnchanged(t *testing.T) {
	table := newTestTable(t, get("/a/:x/b"))
	if err := table.Add(http.MethodGet, "/a/:y/c/d", echoHandle("/a/:y/c/d")); err == nil {
		t.Fatal("conflicting Add succeeded")
	}
	// The rejected pattern must not have left a partial branch that later
	// makes a valid pattern with the original name look like a duplicate.
	if err := table.Add(http.MethodGet, "/a/:x/c/d", echoHandle("/a/:x/c/d")); err != nil {
		t.Fatalf("valid Add after rejected Add: %v", err)
	}
}

func TestMatching(t *testing.T) {
	table := newTestTable(t,
		get("/"),
		get("/users/:id"),
		get("/users/new"),
		get("/users/new/settings"),
		get("/users/:id/profile"),
		get("/users/:id/posts/:post"),
		get("/files/*path"),
		get("/files/readme"),
		get("/dir/"),
		get("/f/:id"),
		get("/f/*rest"),
		get("/a/:x/deep/end"),
		get("/a/:x/deep/:y"),
		get("/a/fixed/deep/other"),
	)

	tests := []struct {
		target string
		want   string
	}{
		{"/", "/ []"},
		{"/users/42", `/users/:id [id="42"]`},
		{"/users/hello%20world", `/users/:id [id="hello world"]`},
		{"/users/new", "/users/new []"},
		{"/users/new/settings", "/users/new/settings []"},
		{"/users/new/profile", `/users/:id/profile [id="new"]`},
		{"/users/42/posts/7", `/users/:id/posts/:post [id="42" post="7"]`},
		{"/files/a/b/c.txt", `/files/*path [path="/a/b/c.txt"]`},
		{"/files/", `/files/*path [path="/"]`},
		{"/files/readme", "/files/readme []"},
		{"/files/readme/more", `/files/*path [path="/readme/more"]`},
		{"/files/../users/1", `/files/*path [path="/../users/1"]`},
		{"/files//x", `/files/*path [path="//x"]`},
		{"/dir/", "/dir/ []"},
		{"/f/1", `/f/:id [id="1"]`},
		{"/f/1/2", `/f/*rest [rest="/1/2"]`},
		{"/f/", `/f/*rest [rest="/"]`},
		{"/a/fixed/deep/end", `/a/:x/deep/end [x="fixed"]`},
		{"/a/fixed/deep/other", "/a/fixed/deep/other []"},
		{"/a/fixed/deep/zzz", `/a/:x/deep/:y [x="fixed" y="zzz"]`},
	}
	for _, tt := range tests {
		t.Run(tt.target, func(t *testing.T) {
			rec := serve(table, http.MethodGet, tt.target)
			if rec.Code != http.StatusOK || rec.Body.String() != tt.want {
				t.Fatalf("GET %s = %d %q; want 200 %q", tt.target, rec.Code, rec.Body.String(), tt.want)
			}
		})
	}
}

func TestMatchingIgnoresRegistrationOrder(t *testing.T) {
	routes := []testRoute{get("/users/:id"), get("/users/new"), get("/users/:id/x"), get("/users/*rest")}
	reversed := []testRoute{routes[3], routes[2], routes[1], routes[0]}
	forward, backward := newTestTable(t, routes...), newTestTable(t, reversed...)

	for _, target := range []string{"/users/new", "/users/1", "/users/1/x", "/users/1/y", "/users/"} {
		a, b := serve(forward, http.MethodGet, target), serve(backward, http.MethodGet, target)
		if a.Code != b.Code || a.Body.String() != b.Body.String() {
			t.Fatalf("GET %s differs by registration order: %d %q vs %d %q",
				target, a.Code, a.Body.String(), b.Code, b.Body.String())
		}
	}
}

func TestStaticIndexAfterThreshold(t *testing.T) {
	var routes []testRoute
	for i := range indexThreshold * 3 {
		routes = append(routes, get(fmt.Sprintf("/r/s%d/:id", i)))
	}
	table := newTestTable(t, routes...)
	for i := range indexThreshold * 3 {
		target := fmt.Sprintf("/r/s%d/7", i)
		want := fmt.Sprintf(`/r/s%d/:id [id="7"]`, i)
		if rec := serve(table, http.MethodGet, target); rec.Body.String() != want {
			t.Fatalf("GET %s = %q; want %q", target, rec.Body.String(), want)
		}
	}
}

func TestLookup(t *testing.T) {
	table := newTestTable(t, get("/static"), get("/users/:id"))

	match, ok := table.Lookup(http.MethodGet, "/static", nil)
	if !ok || match.Pattern != "/static" || match.Params != nil {
		t.Fatalf("static Lookup = %+v, %v", match, ok)
	}

	buf := make(scontext.Params, 0, 4)
	match, ok = table.Lookup(http.MethodGet, "/users/9", buf)
	if !ok || match.Pattern != "/users/:id" || match.Params.ByName("id") != "9" {
		t.Fatalf("param Lookup = %+v, %v", match, ok)
	}
	if &match.Params[:1][0] != &buf[:1][0] {
		t.Fatal("Lookup did not reuse the supplied params buffer")
	}

	for _, tc := range []struct{ method, path string }{
		{http.MethodPost, "/static"},
		{http.MethodGet, "/missing"},
		{http.MethodGet, ""},
		{http.MethodGet, "*"},
	} {
		if match, ok := table.Lookup(tc.method, tc.path, nil); ok {
			t.Fatalf("Lookup(%s, %q) = %+v; want no match", tc.method, tc.path, match)
		}
	}
}

func TestCustomMethods(t *testing.T) {
	table := newTestTable(t, testRoute{"PROPFIND", "/dav/:name"}, get("/dav/:name"))
	if rec := serve(table, "PROPFIND", "/dav/x"); rec.Body.String() != `/dav/:name [name="x"]` {
		t.Fatalf("PROPFIND = %d %q", rec.Code, rec.Body.String())
	}
	rec := serve(table, "PURGE", "/dav/x")
	if rec.Code != http.StatusMethodNotAllowed || rec.Header().Get("Allow") != "GET, OPTIONS, PROPFIND" {
		t.Fatalf("PURGE = %d Allow=%q", rec.Code, rec.Header().Get("Allow"))
	}
}

// TestUnmatched covers the "Kept" and "Changed on purpose" rows of the
// compatibility section in docs/plans/route-table.md.
func TestUnmatched(t *testing.T) {
	table := newTestTable(t,
		get("/"),
		get("/users/:id"),
		testRoute{http.MethodPut, "/users/:id"},
		testRoute{http.MethodDelete, "/users/:id"},
		get("/users/:id/posts/:post"),
		get("/files/*path"),
		get("/dir/"),
		testRoute{http.MethodPost, "/submit"},
		get("/g"),
		get("/opts"),
		testRoute{http.MethodOptions, "/opts"},
		get("/s"),
		get("/s/"),
		get("/q"),
		get("/r/"),
		get("/a/b"),
		get("/Mixed/Case"),
		get("/users/posts/:post"),
	)

	const html = "text/html; charset=utf-8"
	tests := []struct {
		method, target string
		code           int
		location       string
		allow          string
		body           string
	}{
		{method: "GET", target: "/users/a%2Fb", code: 404, body: "404 page not found\n"},
		{method: "GET", target: "/files", code: 301, location: "/files/"},
		{method: "GET", target: "/users/", code: 404},
		{method: "POST", target: "/users/42", code: 405, allow: "DELETE, GET, OPTIONS, PUT", body: "Method Not Allowed\n"},
		{method: "PATCH", target: "/users/42", code: 405, allow: "DELETE, GET, OPTIONS, PUT"},
		{method: "HEAD", target: "/users/42", code: 405, allow: "DELETE, GET, OPTIONS, PUT"},
		{method: "GET", target: "/submit", code: 405, allow: "OPTIONS, POST"},
		{method: "OPTIONS", target: "/users/42", code: 200, allow: "DELETE, GET, OPTIONS, PUT", body: ""},
		{method: "OPTIONS", target: "/opts", code: 200, body: "/opts []"},
		{method: "OPTIONS", target: "/nope", code: 404},
		{method: "OPTIONS", target: "*", code: 200, allow: "DELETE, GET, OPTIONS, POST, PUT"},
		{method: "GET", target: "*", code: 404},
		{method: "OPTIONS", target: "/users/42/", code: 404},
		{method: "GET", target: "/users/42/", code: 301, location: "/users/42"},
		{method: "GET", target: "/dir", code: 301, location: "/dir/"},
		{method: "PUT", target: "/users/42/", code: 307, location: "/users/42"},
		{method: "POST", target: "/submit/", code: 307, location: "/submit"},
		{method: "GET", target: "/users/42/?q=1", code: 301, location: "/users/42?q=1"},
		{method: "GET", target: "/dir?q=1", code: 301, location: "/dir/?q=1"},
		{method: "POST", target: "/g/", code: 404},
		{method: "GET", target: "//users//42", code: 301, location: "/users/42"},
		{method: "GET", target: "/users/./42", code: 301, location: "/users/42"},
		{method: "GET", target: "/x/../users/42", code: 301, location: "/users/42"},
		{method: "GET", target: "//users//42/", code: 301, location: "/users/42"},
		{method: "GET", target: "//users//42?q=1", code: 301, location: "/users/42?q=1"},
		{method: "POST", target: "//submit", code: 307, location: "/submit"},
		{method: "DELETE", target: "//users//42", code: 307, location: "/users/42"},
		{method: "GET", target: "/s/.", code: 301, location: "/s/"},
		{method: "GET", target: "/s/./", code: 301, location: "/s/"},
		{method: "GET", target: "/s/x/..", code: 301, location: "/s"},
		{method: "GET", target: "/s/x/../", code: 301, location: "/s/"},
		{method: "GET", target: "/q/x/..", code: 301, location: "/q"},
		{method: "GET", target: "/r/x/..", code: 301, location: "/r/"},
		{method: "GET", target: "/..", code: 301, location: "/"},
		{method: "GET", target: "/../", code: 301, location: "/"},
		{method: "GET", target: "/./", code: 301, location: "/"},
		{method: "GET", target: "//", code: 301, location: "/"},
		{method: "GET", target: "/a/b/.", code: 301, location: "/a/b"},
		{method: "GET", target: "/a//b", code: 301, location: "/a/b"},
		{method: "GET", target: "/a/%2e/b", code: 301, location: "/a/b"},
		{method: "GET", target: "/.././a/../a/b", code: 301, location: "/a/b"},
		{method: "GET", target: "/a/b/..", code: 404},
		{method: "GET", target: "/nothing", code: 404},
		{method: "GET", target: "/users/42/posts", code: 404},
		{method: "CONNECT", target: "/users/42/", code: 404},
		// Changed on purpose.
		{method: "GET", target: "/USERS/42", code: 404},
		{method: "GET", target: "/mixed/case", code: 404},
		{method: "GET", target: "/users//posts/7", code: 301, location: "/users/posts/7"},
	}
	for _, tt := range tests {
		t.Run(tt.method+" "+tt.target, func(t *testing.T) {
			rec := serve(table, tt.method, tt.target)
			if rec.Code != tt.code {
				t.Fatalf("status = %d; want %d (body %q)", rec.Code, tt.code, rec.Body.String())
			}
			if got := rec.Header().Get("Location"); got != tt.location {
				t.Fatalf("Location = %q; want %q", got, tt.location)
			}
			if got := rec.Header().Get("Allow"); got != tt.allow {
				t.Fatalf("Allow = %q; want %q", got, tt.allow)
			}
			if tt.body != "" || (tt.code == 200 && tt.allow != "") {
				if got := rec.Body.String(); got != tt.body {
					t.Fatalf("body = %q; want %q", got, tt.body)
				}
			}
			if tt.code == 301 && rec.Header().Get("Content-Type") != html {
				t.Fatalf("Content-Type = %q; want %q", rec.Header().Get("Content-Type"), html)
			}
			if (tt.code == 404 || tt.code == 405) && rec.Header().Get("X-Content-Type-Options") != "nosniff" {
				t.Fatal("missing X-Content-Type-Options: nosniff")
			}
		})
	}
}

func TestRedirectLocationIsEscaped(t *testing.T) {
	table := newTestTable(t, get("/:x"), get("/users/:id"))
	tests := []struct{ target, location string }{
		{"/%5Cevil.com/", "/%5Cevil.com"},
		{"/%5C%5Cevil.com/", "/%5C%5Cevil.com"},
		{"/evil.com/", "/evil.com"},
		{"/users/hello%20world/", "/users/hello%20world"},
	}
	for _, tt := range tests {
		rec := serve(table, http.MethodGet, tt.target)
		if rec.Code != http.StatusMovedPermanently || rec.Header().Get("Location") != tt.location {
			t.Fatalf("GET %s = %d Location=%q; want 301 %q", tt.target, rec.Code, rec.Header().Get("Location"), tt.location)
		}
	}
}

func TestUnmatchedWithEmptyOrRelativePath(t *testing.T) {
	table := newTestTable(t, get("/"))
	for _, path := range []string{"", "relative"} {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.URL.Path = path
		rec := httptest.NewRecorder()
		table.ServeHTTP(rec, req)
		if rec.Code != http.StatusNotFound {
			t.Fatalf("path %q = %d; want 404", path, rec.Code)
		}
	}
}

func TestEmptyTable(t *testing.T) {
	table := New()
	for _, method := range []string{http.MethodGet, http.MethodOptions} {
		for _, target := range []string{"/", "*", "/x/"} {
			if rec := serve(table, method, target); rec.Code != http.StatusNotFound {
				t.Fatalf("%s %s = %d; want 404", method, target, rec.Code)
			}
		}
	}
}

func TestCleanPath(t *testing.T) {
	tests := []struct{ in, want string }{
		{"", "/"},
		{"/", "/"},
		{"//", "/"},
		{"/.", "/"},
		{"/..", "/"},
		{"/./", "/"},
		{"/../", "/"},
		{"a", "/a"},
		{"a/b/", "/a/b/"},
		{"/a/b", "/a/b"},
		{"/a/b/", "/a/b/"},
		{"/a//b", "/a/b"},
		{"/a/./b", "/a/b"},
		{"/a/b/.", "/a/b/"},
		{"/a/b/..", "/a"},
		{"/a/b/../", "/a/"},
		{"/a/../../b", "/b"},
		{"/../a", "/a"},
		{"/a/b/c/../../d", "/a/d"},
		{"/a/.../b", "/a/.../b"},
		{"/a/..b/c", "/a/..b/c"},
		{"/a/b.", "/a/b."},
	}
	for _, tt := range tests {
		if got := cleanPath(tt.in); got != tt.want {
			t.Errorf("cleanPath(%q) = %q; want %q", tt.in, got, tt.want)
		}
	}
}

// TestNoOpenRedirect sends raw request targets through a real server, so the
// path is parsed exactly as in production. No redirect may leave the host.
func TestNoOpenRedirect(t *testing.T) {
	table := newTestTable(t, get("/:slug"), get("/:slug/:b/"), testRoute{http.MethodPost, "/*rest"})
	server := httptest.NewServer(table)
	defer server.Close()
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error {
		return http.ErrUseLastResponse
	}}

	for _, raw := range []string{
		"//evil.com/../", "//evil.com/..", "//evil.com/", "//evil.com",
		"/%2Fevil.com/../", "/%5Cevil.com/", "/%5C%5Cevil.com/", "///evil.com/../",
		"/./evil.com/", "//evil.com/x/../", "/evil.com//",
	} {
		for _, method := range []string{http.MethodGet, http.MethodPost} {
			req, err := http.NewRequest(method, server.URL, nil)
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
			if err != nil {
				t.Fatalf("%s %s: unparsable Location %q", method, raw, location)
			}
			if target.Host != req.URL.Host || strings.HasPrefix(location, "//") || strings.Contains(location, `\`) {
				t.Fatalf("%s %s: Location %q leaves the host", method, raw, location)
			}
		}
	}
}

func TestPathSafeMatchesURLEncoding(t *testing.T) {
	for c := range 256 {
		path := "/a" + string(rune(c)) + "b"
		if c >= 0x80 {
			path = "/a" + string([]byte{byte(c)}) + "b"
		}
		if pathSafe(path) && (&url.URL{Path: path}).EscapedPath() != path {
			t.Fatalf("pathSafe accepts byte %#x, but URL encoding changes %q", c, path)
		}
	}
}

// TestRedirectMatchesHTTPRedirect checks that the table's redirect writes the
// same status, headers, and body as http.Redirect for the same Location.
func TestRedirectMatchesHTTPRedirect(t *testing.T) {
	targets := []string{"/", "/users/42", "/users/hello world/", "/a&b/'c'/<d>", "/x/\\evil.com", "/caf\u00e9"}
	queries := []string{"", "q=1", "q=<a>&b='c'", "q=caf\u00e9", "q=%zz"}
	methods := []string{http.MethodGet, http.MethodHead, http.MethodPost}
	for _, target := range targets {
		for _, query := range queries {
			for _, method := range methods {
				for _, hadCT := range []bool{false, true} {
					newReq := func() *http.Request {
						req := httptest.NewRequest(method, "/", nil)
						req.URL.Path = target
						req.URL.RawQuery = query
						return req
					}
					code := http.StatusTemporaryRedirect
					if method == http.MethodGet {
						code = http.StatusMovedPermanently
					}
					location := (&url.URL{Path: target}).EscapedPath()
					if query != "" {
						location += "?" + query
					}
					want, got := httptest.NewRecorder(), httptest.NewRecorder()
					if hadCT {
						want.Header().Set("Content-Type", "text/plain")
						got.Header().Set("Content-Type", "text/plain")
					}
					http.Redirect(want, newReq(), location, code)
					redirect(got, newReq(), target)
					if want.Code != got.Code || want.Body.String() != got.Body.String() ||
						fmt.Sprint(want.Header()) != fmt.Sprint(got.Header()) {
						t.Fatalf("%s %q?%s hadCT=%v:\n http.Redirect %d %v %q\n     redirect %d %v %q",
							method, target, query, hadCT,
							want.Code, want.Header(), want.Body.String(),
							got.Code, got.Header(), got.Body.String())
					}
				}
			}
		}
	}
}

// TestStaticTextWithNULBytes checks children whose text has NUL bytes, which
// look like the zero padding of a short word in the child index.
func TestStaticTextWithNULBytes(t *testing.T) {
	table := newTestTable(t, get("/a\x00/:x"), get("/ab/:x"), get("/a\x00\x00\x00\x00\x00\x00\x00b/:x"))
	tests := []struct{ target, want string }{
		{"/a%00/1", "/a\x00/:x [x=\"1\"]"},
		{"/ab/1", "/ab/:x [x=\"1\"]"},
		{"/a%00%00%00%00%00%00%00b/1", "/a\x00\x00\x00\x00\x00\x00\x00b/:x [x=\"1\"]"},
		{"/a%00%00%00%00%00%00%00c/1", "404 page not found\n"},
		{"/a%00%00/1", "404 page not found\n"},
	}
	for _, tt := range tests {
		if rec := serve(table, http.MethodGet, tt.target); rec.Body.String() != tt.want {
			t.Errorf("GET %s = %q; want %q", tt.target, rec.Body.String(), tt.want)
		}
	}
}
