# In-house route table

Status: implemented in `pkg/router/internal/routetree` and used by `Router`. 2026-09-28.

## Purpose

SRouter matches requests through `github.com/julienschmidt/httprouter` v1.3.0,
which has not had a release since 2019. This spec defines a replacement route
table that SRouter owns. The implementation is written from this document, from
SRouter's own docs and tests, and from black-box observation of the current
router's public behaviour. No httprouter source or tests are copied.

The table lives in `pkg/router/internal/routetree`. Path parameters use the
SRouter-owned `scontext.Params` type.

## Goals

- Keep the request-visible behaviour SRouter users rely on today, except for
  the deliberate changes listed under [Compatibility](#compatibility).
- Report every registration problem as an error. Never panic.
- Match static routes without allocating, and match routes with parameters in
  no more than one allocation.
- Allow a static segment and a wildcard at the same position, such as
  `/users/new` next to `/users/:id`.

## Pattern syntax

A pattern is an absolute path. Everything after the leading `/` is split into
segments on `/`. A pattern that ends in `/` has an empty final segment, so
`/dir` and `/dir/` are different patterns.

| Segment | Meaning |
|---|---|
| `name` | Static text. Must match the request segment byte for byte. |
| `:name` | Parameter. Matches exactly one non-empty request segment. |
| `*name` | Catch-all. Allowed only as the final segment. Matches the rest of the path, starting with the `/` in front of it. |

Rules checked when a route is added:

- The pattern starts with `/`.
- A wildcard fills its whole segment. `/user_:name`, `/a/:b:c`, and `/f*path`
  are rejected.
- A static segment contains no `:` or `*`.
- Wildcard names are non-empty and unique within one pattern.
- A catch-all is the final segment.

## Precedence

At each segment, candidates are tried in a fixed order:

1. a static child whose text equals the request segment;
2. the parameter child;
3. the catch-all.

If a candidate fails deeper in the path, matching backtracks and tries the next
one. So with routes `/users/new/settings` and `/users/:id/profile`, a request
for `/users/new/profile` matches `/users/:id/profile` with `id=new`.

The result depends only on the set of patterns, never on registration order.

## Conflicts

Methods have independent tables. Within one method, `Add` returns an error when:

- the same pattern is added twice;
- two patterns put parameters with different names at the same position, such
  as `/users/:id` and `/users/:name`;
- two patterns put catch-alls with different names at the same position.

A static segment never conflicts with a wildcard, and a parameter never
conflicts with a catch-all at the same position.

## Matching a request

The table matches the decoded `req.URL.Path`, so `%20` in a request becomes a
space in the parameter value. An encoded `%2F` decodes to `/` and splits
segments. A parameter never matches an empty segment, so `/users//posts` does
not match `/users/:id/posts`.

A catch-all value keeps the `/` in front of it. For `/files/*path`, the request
`/files/a/b` gives `path=/a/b` and `/files/` gives `path=/`. The value is not
cleaned: `/files/../x` gives `path=/../x`. Handlers that use a catch-all as a
file path must clean it themselves.

Parameters are returned left to right, in pattern order.

## Unmatched requests

When no route matches the request method and path, the table applies these
steps in order and stops at the first that applies:

1. **Trailing-slash redirect.** Skipped for `CONNECT`, for the path `/`, and
   for paths that are not already clean (see step 2). If the same method's
   table matches the path with its trailing slash added or removed, redirect
   there.
2. **Cleaned-path redirect.** Skipped for `CONNECT`. Clean the path: collapse
   repeated slashes, drop `.` segments, and resolve `..` segments without
   going above the root. The cleaned path keeps a trailing slash when the
   original ends in `/` or `/.`, so `/s/x/../` cleans to `/s/`, `/s/.` to `/s/`,
   and `/s/x/..` to `/s`. If the cleaned path differs and the same method's
   table matches it, or matches it with the trailing slash toggled, redirect
   there. Matching is case-sensitive.
3. **Automatic `OPTIONS`.** For an `OPTIONS` request, if any method matches the
   path, respond `200` with an `Allow` header and an empty body. For the
   request target `*`, `Allow` lists every method registered anywhere.
4. **Method not allowed.** If any other method matches the path, respond `405`
   with an `Allow` header and the body `Method Not Allowed`.
5. **Not found.** Respond `404` with the body `404 page not found`.

A redirect is only sent to a clean path that the table has just matched, so
the client always lands on a route. Redirects use `301` for `GET` and `307`
for every other method, and keep the query string. The `Location` path is
percent-encoded, so a request for `/\evil.com/` redirects to `/%5Cevil.com`.
Because targets are clean and rooted, a `Location` never starts with `//` and
never points a browser at another host.

`Allow` lists the matching methods plus `OPTIONS`, sorted and joined with
`", "`. Explicit `OPTIONS` routes never count as a match for steps 3 and 4, so
a catch-all such as `OPTIONS /*path` does not turn every `404` into a `405`.
Steps 3 and 4 match exact paths only. They never follow a redirect. `HEAD` is
not treated as `GET`.

The `404` and `405` bodies come from `http.Error`, so they are sent as
`text/plain; charset=utf-8` with `X-Content-Type-Options: nosniff`.

## Interfaces

```go
package scontext

type Param struct{ Key, Value string }
type Params []Param

func (ps Params) ByName(name string) string
func (ps Params) Get(name string) (string, bool)
```

```go
package routetree

type Handle func(http.ResponseWriter, *http.Request, scontext.Params)

func New() *Table
func (t *Table) Add(method, pattern string, h Handle) error
func (t *Table) Lookup(method, path string, ps scontext.Params) (Match, bool)
func (t *Table) ServeHTTP(w http.ResponseWriter, req *http.Request)
func (t *Table) ServeUnmatched(w http.ResponseWriter, req *http.Request)

type Match struct {
	Handle  Handle
	Pattern string
	Params  scontext.Params
}
```

- `Lookup` appends parameters to `ps`, which may be nil. It never writes a
  response.
- `ServeHTTP` calls the matching `Handle`, or applies the
  [unmatched-request steps](#unmatched-requests).
- `ServeUnmatched` applies only the unmatched-request steps. A caller that
  runs `Lookup` itself, for example to reuse a parameter buffer, uses it for
  misses.
- A `Table` is not safe to modify concurrently. It is safe for concurrent
  reads once it stops changing, which matches SRouter's freeze at `Build`.

## Design

- Each method has its own table. The nine standard methods use fixed slots;
  other methods use a map.
- Fully static patterns live in an open-addressing hash table keyed by the
  whole path. A filter over each pattern's length and last byte lets most
  requests for wildcard routes skip the hash lookup with one load.
- Patterns with wildcards live in a trie. A node holds a run of static text,
  spanning several segments when no route branches in between, plus
  optional parameter and catch-all children. Static children are keyed by
  their first segment: up to eight are scanned by comparing the first eight
  bytes of their text as one word, and larger sets become an open-addressing
  table hashed on that word. Adding a route splits a run where the new
  pattern diverges.
- Matching walks the trie iteratively and only saves a backtrack point where a
  node has more than one candidate for the segment. Segment ends are found
  eight bytes at a time.
- Parameters are recorded as offsets into the path until the route is known,
  then copied into a result slice allocated once at its exact size. Static
  routes never allocate.
- A failed search notes whether the path with its trailing slash added or
  removed would have matched, so the trailing-slash redirect needs no second
  search. Redirects write the same headers and body as `http.Redirect`
  without re-parsing the Location, since targets are already clean.

## Compatibility

A black-box probe registered routes on the old router and recorded status
codes, headers, bodies, parameter values, and registration panics. Every row
below is covered by a test in `routetree`.

A differential test then served 120,000 random requests against 3,000 random
route sets through both routers. Every difference fell into one of the
deliberate changes listed under
[Changed on purpose](#changed-on-purpose). That test and a side-by-side
benchmark suite were removed with the dependency; they remain in the commit
that introduced the route table.

### Kept

| Case | Behaviour |
|---|---|
| `GET /users/hello%20world` on `/users/:id` | `id=hello world` |
| `GET /users/a%2Fb` on `/users/:id` | `404` |
| `GET /files/a/b/c.txt` on `/files/*path` | `path=/a/b/c.txt` |
| `GET /files/` on `/files/*path` | `path=/` |
| `GET /files` on `/files/*path` | `301` to `/files/` |
| `GET /users/` with only `/users/:id` | `404` |
| `POST /users/42` on GET/PUT/DELETE `/users/:id` | `405`, `Allow: DELETE, GET, OPTIONS, PUT` |
| `HEAD /users/42` with no HEAD route | `405` |
| `OPTIONS /users/42` with no OPTIONS route | `200`, `Allow: DELETE, GET, OPTIONS, PUT`, empty body |
| `OPTIONS /nope` | `404` |
| `PUT /x/` when only `OPTIONS /:p/` matches | `404` |
| `OPTIONS *` | `200`, `Allow` lists every registered method |
| `GET /users/42/` on `/users/:id` | `301` to `/users/42` |
| `PUT /users/42/` on PUT `/users/:id` | `307` to `/users/42` |
| `GET /users/42/?q=1` | `301` to `/users/42?q=1` |
| `POST /g/` when only GET `/g` exists | `404`; redirects only use the request method's routes |
| `OPTIONS /users/42/` | `404`; automatic `OPTIONS` needs an exact match |
| `GET //users//42`, `/users/./42`, `/x/../users/42` | `301` to `/users/42` |
| `GET //users//42/` | `301` to `/users/42` |
| `GET /s/.` with `/s` and `/s/` | `301` to `/s/` |
| `GET /s/x/..` with `/s` and `/s/` | `301` to `/s` |
| `GET /..`, `/../`, `/./`, `//` with `/` | `301` to `/` |
| `GET /%5Cevil.com/` on `/:x` | `301` to `/%5Cevil.com` |
| `GET /files/../users/1` on `/files/*path` | `path=/../users/1`; catch-alls match before cleaning |
| `/users/:id` and `/users/:name` on the same method | error |
| `/users/:id` on GET and `/users/:name` on POST | accepted |
| Same method and pattern twice | error |
| Empty wildcard name, catch-all not last, two wildcards in one segment | error |
| `/users/:id`, `/users/:id/x`, `/users` together | accepted |
| `/a` and `/a/` together | accepted |

### Changed on purpose

| Case | Old | New |
|---|---|---|
| `/users/new` next to `/users/:id` | registration panic | accepted; static wins |
| `/f/x` next to `/f/*path` | registration panic | accepted; static wins |
| `/f/:id` next to `/f/*path` | registration panic | accepted; parameter wins |
| `/src/` next to `/src/*p` | registration panic | accepted; `/src/` is static and wins |
| `/*path` next to `/x` | registration panic | accepted; static wins |
| `GET /USERS/42` when `/users/:id` exists | `301` to `/users/42` | `404`; no case-correcting redirect |
| Mid-segment wildcard such as `/user_:name` | accepted | error |
| Repeated wildcard name such as `/a/:id/b/:id` | accepted | error |
| `GET /users//posts/7` on `/users/:id/posts/:post` | match with `id=""` | no match; the cleaned-path redirect to `/users/posts/7` applies if that path exists |
| `GET //evil.com/../` on `/:slug`, over HTTP | `301` to `http://evil.com/..` | no redirect off the host |
| `POST /a/a/a/` on `/:a/:b/:c/:d` | `307` to `/a/a/a`, which matches no route | `404` |
| `OPTIONS /a/..` on OPTIONS `/a/:p/` | `307` to `/`, because `http.Redirect` cleans `/a/../` | no redirect to an unclean target |
| `GET /a` on `/:p/*rest` | `404` or `405` | `301` to `/a/`, as for `/files` on `/files/*path` |
| `GET /a/../users/` with `/:p` and `/:p/` | `301` to `/users` | `301` to `/users/`; the trailing slash is kept |

## Out of scope

- Configurable redirect status codes.
- Matching on the raw, still-encoded path, so that `%2F` stays inside one
  segment.
- Host-based routing.
