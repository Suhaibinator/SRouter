package routetree

// Temporary benchmarks against httprouter, used only through its public API.
// Delete this file when the dependency is removed. Compare with:
//
//	go test -run '^$' -bench Compare -benchmem -count 10 . > bench.txt
//	benchstat -col /router bench.txt

import (
	"fmt"
	"net/http"
	"runtime"
	"strings"
	"testing"

	"github.com/Suhaibinator/SRouter/pkg/scontext"
	"github.com/julienschmidt/httprouter"
)

var benchSink string

type benchWriter struct{ header http.Header }

func (w *benchWriter) Header() http.Header         { return w.header }
func (w *benchWriter) Write(b []byte) (int, error) { return len(b), nil }
func (w *benchWriter) WriteHeader(int)             {}

func legacyBenchHandle(_ http.ResponseWriter, _ *http.Request, ps httprouter.Params) {
	if len(ps) > 0 {
		benchSink = ps[0].Value
	}
}

func newBenchHandle(_ http.ResponseWriter, _ *http.Request, ps scontext.Params) {
	if len(ps) > 0 {
		benchSink = ps[0].Value
	}
}

// restRoutes is a 100-route REST API: 20 resources with list, create, read,
// update, and nested-read routes, plus parameter-heavy and catch-all routes.
func restRoutes() []testRoute {
	var routes []testRoute
	for i := range 20 {
		base := fmt.Sprintf("/api/v1/res%d", i)
		routes = append(routes,
			testRoute{http.MethodGet, base},
			testRoute{http.MethodPost, base},
			testRoute{http.MethodGet, base + "/:id"},
			testRoute{http.MethodPut, base + "/:id"},
			testRoute{http.MethodGet, base + "/:id/items/:item"},
		)
	}
	return append(routes,
		testRoute{http.MethodGet, "/p5/:a/:b/:c/:d/:e"},
		testRoute{http.MethodGet, "/static/*filepath"},
	)
}

// githubRoutes is shaped like the GitHub REST API v3, written from its public
// endpoint list.
func githubRoutes() []testRoute {
	spec := `
GET /authorizations
GET /authorizations/:id
POST /authorizations
DELETE /authorizations/:id
GET /applications/:client_id/tokens/:access_token
DELETE /applications/:client_id/tokens
DELETE /applications/:client_id/tokens/:access_token
GET /events
GET /repos/:owner/:repo/events
GET /networks/:owner/:repo/events
GET /orgs/:org/events
GET /users/:user/received_events
GET /users/:user/received_events/public
GET /users/:user/events
GET /users/:user/events/public
GET /users/:user/events/orgs/:org
GET /feeds
GET /notifications
GET /repos/:owner/:repo/notifications
PUT /notifications
PUT /repos/:owner/:repo/notifications
GET /notifications/threads/:id
GET /notifications/threads/:id/subscription
PUT /notifications/threads/:id/subscription
DELETE /notifications/threads/:id/subscription
GET /repos/:owner/:repo/stargazers
GET /users/:user/starred
GET /user/starred
GET /user/starred/:owner/:repo
PUT /user/starred/:owner/:repo
DELETE /user/starred/:owner/:repo
GET /repos/:owner/:repo/subscribers
GET /users/:user/subscriptions
GET /user/subscriptions
GET /repos/:owner/:repo/subscription
PUT /repos/:owner/:repo/subscription
DELETE /repos/:owner/:repo/subscription
GET /user/subscriptions/:owner/:repo
PUT /user/subscriptions/:owner/:repo
DELETE /user/subscriptions/:owner/:repo
GET /users/:user/gists
GET /gists
GET /gists/:id
POST /gists
PUT /gists/:id/star
DELETE /gists/:id/star
GET /gists/:id/star
POST /gists/:id/forks
DELETE /gists/:id
GET /repos/:owner/:repo/git/blobs/:sha
POST /repos/:owner/:repo/git/blobs
GET /repos/:owner/:repo/git/commits/:sha
POST /repos/:owner/:repo/git/commits
GET /repos/:owner/:repo/git/refs
POST /repos/:owner/:repo/git/refs
GET /repos/:owner/:repo/git/tags/:sha
POST /repos/:owner/:repo/git/tags
GET /repos/:owner/:repo/git/trees/:sha
POST /repos/:owner/:repo/git/trees
GET /issues
GET /user/issues
GET /orgs/:org/issues
GET /repos/:owner/:repo/issues
GET /repos/:owner/:repo/issues/:number
POST /repos/:owner/:repo/issues
GET /repos/:owner/:repo/assignees
GET /repos/:owner/:repo/assignees/:assignee
GET /repos/:owner/:repo/issues/:number/comments
POST /repos/:owner/:repo/issues/:number/comments
GET /repos/:owner/:repo/issues/:number/events
GET /repos/:owner/:repo/labels
GET /repos/:owner/:repo/labels/:name
POST /repos/:owner/:repo/labels
DELETE /repos/:owner/:repo/labels/:name
GET /repos/:owner/:repo/issues/:number/labels
POST /repos/:owner/:repo/issues/:number/labels
DELETE /repos/:owner/:repo/issues/:number/labels/:name
PUT /repos/:owner/:repo/issues/:number/labels
DELETE /repos/:owner/:repo/issues/:number/labels
GET /repos/:owner/:repo/milestones/:number/labels
GET /repos/:owner/:repo/milestones
GET /repos/:owner/:repo/milestones/:number
POST /repos/:owner/:repo/milestones
DELETE /repos/:owner/:repo/milestones/:number
GET /emojis
GET /gitignore/templates
GET /gitignore/templates/:name
POST /markdown
POST /markdown/raw
GET /meta
GET /rate_limit
GET /users/:user/orgs
GET /user/orgs
GET /orgs/:org
GET /orgs/:org/members
GET /orgs/:org/members/:user
DELETE /orgs/:org/members/:user
GET /orgs/:org/public_members
GET /orgs/:org/public_members/:user
PUT /orgs/:org/public_members/:user
DELETE /orgs/:org/public_members/:user
GET /orgs/:org/teams
GET /teams/:id
POST /orgs/:org/teams
DELETE /teams/:id
GET /teams/:id/members
GET /teams/:id/members/:user
PUT /teams/:id/members/:user
DELETE /teams/:id/members/:user
GET /teams/:id/repos
GET /teams/:id/repos/:owner/:repo
PUT /teams/:id/repos/:owner/:repo
DELETE /teams/:id/repos/:owner/:repo
GET /user/teams
GET /repos/:owner/:repo/pulls
GET /repos/:owner/:repo/pulls/:number
POST /repos/:owner/:repo/pulls
GET /repos/:owner/:repo/pulls/:number/commits
GET /repos/:owner/:repo/pulls/:number/files
GET /repos/:owner/:repo/pulls/:number/merge
PUT /repos/:owner/:repo/pulls/:number/merge
GET /repos/:owner/:repo/pulls/:number/comments
PUT /repos/:owner/:repo/pulls/:number/comments
GET /user/repos
GET /users/:user/repos
GET /orgs/:org/repos
GET /repositories
POST /user/repos
POST /orgs/:org/repos
GET /repos/:owner/:repo
DELETE /repos/:owner/:repo
GET /repos/:owner/:repo/contributors
GET /repos/:owner/:repo/languages
GET /repos/:owner/:repo/teams
GET /repos/:owner/:repo/tags
GET /repos/:owner/:repo/branches
GET /repos/:owner/:repo/branches/:branch
GET /repos/:owner/:repo/collaborators
GET /repos/:owner/:repo/collaborators/:user
PUT /repos/:owner/:repo/collaborators/:user
DELETE /repos/:owner/:repo/collaborators/:user
GET /repos/:owner/:repo/comments
GET /repos/:owner/:repo/commits/:sha/comments
POST /repos/:owner/:repo/commits/:sha/comments
GET /repos/:owner/:repo/comments/:id
DELETE /repos/:owner/:repo/comments/:id
GET /repos/:owner/:repo/commits
GET /repos/:owner/:repo/commits/:sha
GET /repos/:owner/:repo/readme
GET /repos/:owner/:repo/contents/*path
DELETE /repos/:owner/:repo/contents/*path
GET /repos/:owner/:repo/keys
GET /repos/:owner/:repo/keys/:id
POST /repos/:owner/:repo/keys
DELETE /repos/:owner/:repo/keys/:id
GET /repos/:owner/:repo/downloads
GET /repos/:owner/:repo/downloads/:id
DELETE /repos/:owner/:repo/downloads/:id
GET /repos/:owner/:repo/forks
POST /repos/:owner/:repo/forks
GET /repos/:owner/:repo/hooks
GET /repos/:owner/:repo/hooks/:id
POST /repos/:owner/:repo/hooks
POST /repos/:owner/:repo/hooks/:id/tests
DELETE /repos/:owner/:repo/hooks/:id
POST /repos/:owner/:repo/merges
GET /repos/:owner/:repo/releases
GET /repos/:owner/:repo/releases/:id
POST /repos/:owner/:repo/releases
DELETE /repos/:owner/:repo/releases/:id
GET /repos/:owner/:repo/releases/:id/assets
GET /repos/:owner/:repo/stats/contributors
GET /repos/:owner/:repo/stats/commit_activity
GET /repos/:owner/:repo/stats/code_frequency
GET /repos/:owner/:repo/stats/participation
GET /repos/:owner/:repo/stats/punch_card
GET /repos/:owner/:repo/statuses/:ref
POST /repos/:owner/:repo/statuses/:ref
GET /search/repositories
GET /search/code
GET /search/issues
GET /search/users
GET /legacy/issues/search/:owner/:repository/:state/:keyword
GET /legacy/repos/search/:keyword
GET /legacy/user/search/:keyword
GET /legacy/user/email/:email
GET /users/:user
GET /user
GET /users
GET /user/emails
POST /user/emails
DELETE /user/emails
GET /users/:user/followers
GET /user/followers
GET /users/:user/following
GET /user/following
GET /user/following/:user
GET /users/:user/following/:target_user
PUT /user/following/:user
DELETE /user/following/:user
GET /users/:user/keys
GET /user/keys
GET /user/keys/:id
POST /user/keys
DELETE /user/keys/:id
`
	var routes []testRoute
	for line := range strings.SplitSeq(strings.TrimSpace(spec), "\n") {
		method, pattern, _ := strings.Cut(line, " ")
		routes = append(routes, testRoute{method, pattern})
	}
	return routes
}

func flatRoutes(n int) []testRoute {
	routes := make([]testRoute, n)
	for i := range routes {
		routes[i] = testRoute{http.MethodGet, fmt.Sprintf("/route%d", i)}
	}
	return routes
}

func deepRoutes() []testRoute {
	var routes []testRoute
	for i := range 8 {
		prefix := ""
		for depth := range 8 {
			prefix += fmt.Sprintf("/group-%d", (i+depth)%8)
		}
		routes = append(routes, testRoute{http.MethodGet, prefix + "/target"})
	}
	return routes
}

// fillPattern turns a pattern into a request path.
func fillPattern(pattern string) string {
	var b strings.Builder
	for seg := range strings.SplitSeq(pattern[1:], "/") {
		b.WriteByte('/')
		switch {
		case strings.HasPrefix(seg, ":"):
			b.WriteByte('v')
			b.WriteString(seg[1:])
		case strings.HasPrefix(seg, "*"):
			b.WriteString("a/b/c.txt")
		default:
			b.WriteString(seg)
		}
	}
	return b.String()
}

type benchRouters struct {
	legacy http.Handler
	table  http.Handler
}

func buildBenchRouters(tb testing.TB, routes []testRoute) benchRouters {
	legacy := httprouter.New()
	table := New()
	for _, r := range routes {
		legacy.Handle(r.method, r.pattern, legacyBenchHandle)
		if err := table.Add(r.method, r.pattern, newBenchHandle); err != nil {
			tb.Fatalf("Add(%s %s): %v", r.method, r.pattern, err)
		}
	}
	return benchRouters{legacy: legacy, table: table}
}

func (r benchRouters) each(b *testing.B, name string, run func(*testing.B, http.Handler)) {
	b.Run(name+"/router=httprouter", func(b *testing.B) { run(b, r.legacy) })
	b.Run(name+"/router=routetree", func(b *testing.B) { run(b, r.table) })
}

func serveOne(method, path string) func(*testing.B, http.Handler) {
	return func(b *testing.B, h http.Handler) {
		req, _ := http.NewRequest(method, path, nil)
		w := &benchWriter{header: http.Header{}}
		b.ReportAllocs()
		for b.Loop() {
			// httprouter rewrites req.URL.Path when it redirects.
			req.URL.Path = path
			h.ServeHTTP(w, req)
		}
	}
}

func serveOneParallel(method, path string) func(*testing.B, http.Handler) {
	return func(b *testing.B, h http.Handler) {
		b.ReportAllocs()
		b.RunParallel(func(pb *testing.PB) {
			req, _ := http.NewRequest(method, path, nil)
			w := &benchWriter{header: http.Header{}}
			for pb.Next() {
				req.URL.Path = path
				h.ServeHTTP(w, req)
			}
		})
	}
}

// serveAll issues one request per route on each iteration.
func serveAll(routes []testRoute) func(*testing.B, http.Handler) {
	return func(b *testing.B, h http.Handler) {
		reqs := make([]*http.Request, len(routes))
		paths := make([]string, len(routes))
		for i, r := range routes {
			paths[i] = fillPattern(r.pattern)
			reqs[i], _ = http.NewRequest(r.method, paths[i], nil)
		}
		w := &benchWriter{header: http.Header{}}
		b.ReportAllocs()
		for b.Loop() {
			for i, req := range reqs {
				req.URL.Path = paths[i]
				h.ServeHTTP(w, req)
			}
		}
	}
}

func BenchmarkCompareMatch(b *testing.B) {
	rest := buildBenchRouters(b, restRoutes())
	rest.each(b, "case=rest_static", serveOne(http.MethodGet, "/api/v1/res17"))
	rest.each(b, "case=rest_param1", serveOne(http.MethodGet, "/api/v1/res17/12345"))
	rest.each(b, "case=rest_param2", serveOne(http.MethodGet, "/api/v1/res17/12345/items/678"))
	rest.each(b, "case=rest_param5", serveOne(http.MethodGet, "/p5/a/b/c/d/e"))
	rest.each(b, "case=rest_catchall", serveOne(http.MethodGet, "/static/css/app/main.css"))
	rest.each(b, "case=rest_404", serveOne(http.MethodGet, "/api/v2/nothing/here"))
	rest.each(b, "case=rest_405", serveOne(http.MethodDelete, "/api/v1/res17/12345"))
	rest.each(b, "case=rest_redirect", serveOne(http.MethodGet, "/api/v1/res17/12345/"))
	rest.each(b, "case=rest_static_parallel", serveOneParallel(http.MethodGet, "/api/v1/res17"))
	rest.each(b, "case=rest_param2_parallel", serveOneParallel(http.MethodGet, "/api/v1/res17/12345/items/678"))

	githubList := githubRoutes()
	github := buildBenchRouters(b, githubList)
	github.each(b, "case=github_static", serveOne(http.MethodGet, "/user/repos"))
	github.each(b, "case=github_param1", serveOne(http.MethodGet, "/users/octocat"))
	github.each(b, "case=github_param2", serveOne(http.MethodGet, "/repos/octocat/hello-world"))
	github.each(b, "case=github_param4", serveOne(http.MethodGet, "/legacy/issues/search/o/r/open/bug"))
	github.each(b, "case=github_catchall", serveOne(http.MethodGet, "/repos/octocat/hello/contents/docs/readme.md"))
	github.each(b, "case=github_all", serveAll(githubList))

	flat := buildBenchRouters(b, flatRoutes(1000))
	flat.each(b, "case=flat1000_first", serveOne(http.MethodGet, "/route0"))
	flat.each(b, "case=flat1000_last", serveOne(http.MethodGet, "/route999"))

	deep := buildBenchRouters(b, deepRoutes())
	deep.each(b, "case=deep9_static", serveOne(http.MethodGet, deepRoutes()[5].pattern))
}

func BenchmarkCompareBuild(b *testing.B) {
	sets := []struct {
		name   string
		routes []testRoute
	}{
		{"flat100", flatRoutes(100)},
		{"flat1000", flatRoutes(1000)},
		{"github", githubRoutes()},
	}
	for _, set := range sets {
		b.Run("case="+set.name+"/router=httprouter", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				r := httprouter.New()
				for _, route := range set.routes {
					r.Handle(route.method, route.pattern, legacyBenchHandle)
				}
			}
			reportRetainedHeap(b, func() any {
				r := httprouter.New()
				for _, route := range set.routes {
					r.Handle(route.method, route.pattern, legacyBenchHandle)
				}
				return r
			})
		})
		b.Run("case="+set.name+"/router=routetree", func(b *testing.B) {
			b.ReportAllocs()
			for b.Loop() {
				t := New()
				for _, route := range set.routes {
					_ = t.Add(route.method, route.pattern, newBenchHandle)
				}
			}
			reportRetainedHeap(b, func() any {
				t := New()
				for _, route := range set.routes {
					_ = t.Add(route.method, route.pattern, newBenchHandle)
				}
				return t
			})
		})
	}
}

var retained any

// reportRetainedHeap reports the live heap held by one built router.
func reportRetainedHeap(b *testing.B, build func() any) {
	var before, after runtime.MemStats
	runtime.GC()
	runtime.ReadMemStats(&before)
	retained = build()
	runtime.GC()
	runtime.ReadMemStats(&after)
	b.ReportMetric(float64(int64(after.HeapAlloc)-int64(before.HeapAlloc)), "retained-B")
	retained = nil
}
