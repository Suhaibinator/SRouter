package routetree

// Temporary differential test against httprouter, used only through its public
// API as a black box. Delete this file when the dependency is removed.

import (
	"fmt"
	"math/rand/v2"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/Suhaibinator/SRouter/pkg/scontext"
	"github.com/julienschmidt/httprouter"
)

func legacyHandle(pattern string) httprouter.Handle {
	return func(w http.ResponseWriter, _ *http.Request, ps httprouter.Params) {
		params := make(scontext.Params, len(ps))
		for i, p := range ps {
			params[i] = scontext.Param{Key: p.Key, Value: p.Value}
		}
		_, _ = fmt.Fprint(w, describeMatch(pattern, params))
	}
}

func legacyAdd(r *httprouter.Router, method, pattern string) (ok bool) {
	defer func() {
		if recover() != nil {
			ok = false
		}
	}()
	r.Handle(method, pattern, legacyHandle(pattern))
	return true
}

var (
	compareMethods  = []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodOptions}
	compareStatic   = []string{"a", "b", "users", "v1"}
	compareRequests = []string{"a", "b", "users", "v1", "42", "x", "", ".", ".."}
)

// randomPattern builds a pattern from a small alphabet. Parameter names depend
// only on position, so the generator avoids name conflicts, which both routers
// reject.
func randomPattern(rng *rand.Rand) string {
	var b strings.Builder
	depth := 1 + rng.IntN(4)
	for i := range depth {
		b.WriteByte('/')
		switch n := rng.IntN(10); {
		case n < 6:
			b.WriteString(compareStatic[rng.IntN(len(compareStatic))])
		case n < 9:
			fmt.Fprintf(&b, ":p%d", i)
		default:
			fmt.Fprintf(&b, "*c%d", i)
			return b.String()
		}
	}
	if rng.IntN(4) == 0 {
		b.WriteByte('/')
	}
	return b.String()
}

func randomRequestPath(rng *rand.Rand) string {
	var b strings.Builder
	if rng.IntN(8) == 0 {
		b.WriteByte('/')
	}
	depth := rng.IntN(6)
	for range depth {
		b.WriteByte('/')
		b.WriteString(compareRequests[rng.IntN(len(compareRequests))])
	}
	if depth == 0 || rng.IntN(4) == 0 {
		b.WriteByte('/')
	}
	return b.String()
}

// legacyEmptyParam reports whether the legacy router matches path, or a
// redirect candidate for it, by binding a parameter to an empty segment. The
// spec deliberately stops doing that, so such requests may differ.
func legacyEmptyParam(legacy *httprouter.Router, path string) bool {
	candidates := []string{path, cleanPath(path)}
	for _, c := range candidates[:2] {
		if alternate, ok := toggleTrailingSlash(c); ok {
			candidates = append(candidates, alternate)
		}
	}
	for _, method := range compareMethods {
		for _, candidate := range candidates {
			if h, ps, _ := legacy.Lookup(method, candidate); h != nil {
				for _, p := range ps {
					if p.Value == "" {
						return true
					}
				}
			}
		}
	}
	return false
}

// legacyRedirectsNowhere reports whether the legacy router sent a redirect to
// a Location that none of its routes for method match. The spec only
// redirects to verified routes.
func legacyRedirectsNowhere(legacy *httprouter.Router, method string, rec *httptest.ResponseRecorder) bool {
	location := rec.Header().Get("Location")
	if location == "" {
		return false
	}
	target, err := url.Parse(location)
	if err != nil {
		return true
	}
	h, _, _ := legacy.Lookup(method, target.Path)
	return h == nil
}

// explainDifference names the deliberate spec change behind a difference, or
// returns "" when the difference is unexplained.
func explainDifference(legacy *httprouter.Router, method, path string, want, got *httptest.ResponseRecorder) string {
	switch {
	case strings.HasPrefix(want.Header().Get("Location"), "//"):
		return "legacy open redirect"
	case legacyEmptyParam(legacy, path):
		return "legacy empty parameter"
	case legacyRedirectsNowhere(legacy, method, want):
		return "legacy redirect to unmatched path"
	case want.Header().Get("Location") == "" && legacyServesLocation(legacy, method, got):
		return "legacy missed trailing-slash redirect"
	case want.Code == got.Code && legacyServesLocation(legacy, method, got) &&
		strings.TrimSuffix(want.Header().Get("Location"), "/") == strings.TrimSuffix(got.Header().Get("Location"), "/"):
		return "legacy dropped trailing slash of cleaned path"
	}
	return ""
}

// legacyServesLocation reports whether the new table redirected to a path the
// legacy router also serves for method.
func legacyServesLocation(legacy *httprouter.Router, method string, rec *httptest.ResponseRecorder) bool {
	location := rec.Header().Get("Location")
	if location == "" {
		return false
	}
	target, err := url.Parse(location)
	if err != nil {
		return false
	}
	h, _, _ := legacy.Lookup(method, target.Path)
	return h != nil
}

func newPathRequest(method, path string) *http.Request {
	req := httptest.NewRequest(method, "/", nil)
	req.URL.Path = path
	return req
}

func TestDifferentialAgainstHTTPRouter(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	compared, skipped := 0, map[string]int{}
	for set := range 3000 {
		legacy := httprouter.New()
		table := New()
		var accepted []string
		for range 1 + rng.IntN(12) {
			method := compareMethods[rng.IntN(len(compareMethods))]
			pattern := randomPattern(rng)
			if !legacyAdd(legacy, method, pattern) {
				continue
			}
			if err := table.Add(method, pattern, echoHandle(pattern)); err != nil {
				t.Fatalf("set %d: legacy accepted %s %s but Add failed: %v", set, method, pattern, err)
			}
			accepted = append(accepted, method+" "+pattern)
		}

		for range 40 {
			method := compareMethods[rng.IntN(len(compareMethods))]
			path := randomRequestPath(rng)
			// Each router gets its own request: the legacy router rewrites
			// req.URL.Path in place when it redirects.
			want, got := httptest.NewRecorder(), httptest.NewRecorder()
			legacy.ServeHTTP(want, newPathRequest(method, path))
			table.ServeHTTP(got, newPathRequest(method, path))

			if want.Code != got.Code ||
				want.Header().Get("Location") != got.Header().Get("Location") ||
				want.Header().Get("Allow") != got.Header().Get("Allow") ||
				want.Body.String() != got.Body.String() {
				if reason := explainDifference(legacy, method, path, want, got); reason != "" {
					skipped[reason]++
					continue
				}
				t.Fatalf("set %d routes %q\n%s %q:\n legacy %d Location=%q Allow=%q body=%q\n    new %d Location=%q Allow=%q body=%q",
					set, accepted, method, path,
					want.Code, want.Header().Get("Location"), want.Header().Get("Allow"), want.Body.String(),
					got.Code, got.Header().Get("Location"), got.Header().Get("Allow"), got.Body.String())
			}
			compared++
		}
	}
	t.Logf("%d requests matched exactly; differences explained by deliberate changes: %v", compared, skipped)
}
