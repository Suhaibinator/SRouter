package routetree

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Suhaibinator/SRouter/pkg/scontext"
)

func FuzzCleanPath(f *testing.F) {
	for _, seed := range []string{"", "/", "//", "/a/../b", "/a/./b/", "/../..", "a/b", "/a/b/.", "/a//b//"} {
		f.Add(seed)
	}
	f.Fuzz(func(t *testing.T, in string) {
		out := cleanPath(in)
		if out == "" || out[0] != '/' {
			t.Fatalf("cleanPath(%q) = %q; want a rooted path", in, out)
		}
		if strings.Contains(out, "//") {
			t.Fatalf("cleanPath(%q) = %q; contains an empty segment", in, out)
		}
		for seg := range strings.SplitSeq(strings.Trim(out, "/"), "/") {
			if seg == "." || seg == ".." {
				t.Fatalf("cleanPath(%q) = %q; contains %q", in, out, seg)
			}
		}
		if isCleanPath(in) != (cleanPath(in) == in) {
			t.Fatalf("isCleanPath(%q) = %v, but cleanPath returns %q", in, isCleanPath(in), cleanPath(in))
		}
		if again := cleanPath(out); again != out {
			t.Fatalf("cleanPath not idempotent: %q -> %q -> %q", in, out, again)
		}
	})
}

// FuzzMatch adds newline-separated patterns, then matches a path. A match
// must rebuild the request path exactly from its pattern and parameters.
func FuzzMatch(f *testing.F) {
	f.Add("/users/:id\n/users/new\n/files/*path", "/users/42")
	f.Add("/a/:x/b\n/a/:x/*rest\n/a/c/b", "/a/c/b/d")
	f.Add("/\n/:x/\n/*all", "/q/")
	f.Fuzz(func(t *testing.T, patterns, path string) {
		table := New()
		for pattern := range strings.SplitSeq(patterns, "\n") {
			_ = table.Add(http.MethodGet, pattern, echoHandle(pattern))
		}

		match, ok := table.Lookup(http.MethodGet, path, nil)
		if ok {
			if got := rebuildPath(t, match.Pattern, match.Params); got != path {
				t.Fatalf("pattern %q with %v rebuilds %q; want %q", match.Pattern, match.Params, got, path)
			}
		}

		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.URL.Path = path
		table.ServeHTTP(httptest.NewRecorder(), req)
	})
}

func rebuildPath(t *testing.T, pattern string, params scontext.Params) string {
	t.Helper()
	parts, _, err := parsePattern(pattern, nil)
	if err != nil {
		t.Fatalf("matched pattern %q does not parse: %v", pattern, err)
	}
	want := 0
	for _, p := range parts {
		if p.kind != staticPart {
			want++
		}
	}
	if len(params) != want {
		t.Fatalf("pattern %q returned %d params; want %d", pattern, len(params), want)
	}
	var b strings.Builder
	for _, p := range parts {
		switch p.kind {
		case staticPart:
			b.WriteString(p.text)
		case paramPart:
			value := params.ByName(p.text)
			if value == "" || strings.Contains(value, "/") {
				t.Fatalf("parameter %q = %q; want one non-empty segment", p.text, value)
			}
			b.WriteByte('/')
			b.WriteString(value)
		case catchAllPart:
			value := params.ByName(p.text)
			if !strings.HasPrefix(value, "/") {
				t.Fatalf("catch-all %q = %q; want a leading '/'", p.text, value)
			}
			b.WriteString(value)
		}
	}
	return b.String()
}
