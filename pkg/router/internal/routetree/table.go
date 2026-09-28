// Package routetree matches HTTP methods and paths to route handles.
//
// The behaviour is specified in docs/plans/route-table.md. Fully static
// patterns are stored in a hash table per method; patterns with wildcards
// are stored in a trie keyed by path segment.
package routetree

import (
	"errors"
	"hash/maphash"
	"net/http"
	"slices"

	"github.com/Suhaibinator/SRouter/pkg/scontext"
)

// Handle serves a matched route. params holds the captured path parameters in
// pattern order and is nil for fully static patterns.
type Handle func(w http.ResponseWriter, req *http.Request, params scontext.Params)

// Match is the result of a successful Lookup.
type Match struct {
	Handle  Handle
	Pattern string
	Params  scontext.Params
}

// Standard methods get a fixed slot so the common lookup avoids hashing the
// method name.
const (
	slotGet = iota
	slotHead
	slotPost
	slotPut
	slotPatch
	slotDelete
	slotOptions
	slotConnect
	slotTrace
	standardSlots
)

func methodSlot(method string) int {
	// GET is by far the most common method, so it skips the switch.
	if method == http.MethodGet {
		return slotGet
	}
	switch method {
	case http.MethodHead:
		return slotHead
	case http.MethodPost:
		return slotPost
	case http.MethodPut:
		return slotPut
	case http.MethodPatch:
		return slotPatch
	case http.MethodDelete:
		return slotDelete
	case http.MethodOptions:
		return slotOptions
	case http.MethodConnect:
		return slotConnect
	case http.MethodTrace:
		return slotTrace
	}
	return -1
}

// methodTable holds every route registered for one method. Fully static
// patterns win outright: under the spec's precedence a static segment beats a
// wildcard at the first position where two matching patterns differ.
type methodTable struct {
	static staticTable
	root   *node
}

// match finds the route for path in the trie. It reports the trailing-slash
// hint through m on a miss.
func (mt *methodTable) match(path string, m *matcher) *node {
	if mt.root == nil || path == "" || path[0] != '/' {
		return nil
	}
	m.path = path
	return mt.root.match(path, m)
}

// has reports whether path matches a route. On a miss it also reports
// whether path with a trailing slash added or removed matches a trie route.
func (mt *methodTable) has(path string) (found, tsrAdd, tsrRemove bool) {
	if mt.static.lookup(path) != nil {
		return true, false, false
	}
	m := matcher{discard: true}
	if mt.match(path, &m) != nil {
		return true, false, false
	}
	return false, m.tsrAdd, m.tsrRemove
}

// hasToggled reports whether path with its trailing slash toggled matches a
// route. tsrAdd and tsrRemove come from a search for path itself, which
// already covers the trie, so only the static table is left to check.
func (mt *methodTable) hasToggled(path string, tsrAdd, tsrRemove bool) bool {
	if path[len(path)-1] == '/' {
		return tsrRemove || mt.static.lookup(path[:len(path)-1]) != nil
	}
	return tsrAdd || mt.static.lookupWithSlash(path)
}

// Table maps methods and paths to handles. Add must not run concurrently with
// any other method; once registration stops, all other methods are safe for
// concurrent use.
type Table struct {
	standard [standardSlots]*methodTable
	custom   map[string]*methodTable
	methods  []string // sorted names of every method with a route
}

// New returns an empty Table.
func New() *Table {
	return &Table{}
}

func (t *Table) table(method string) *methodTable {
	if slot := methodSlot(method); slot >= 0 {
		return t.standard[slot]
	}
	return t.custom[method]
}

func (t *Table) ensureTable(method string) *methodTable {
	if mt := t.table(method); mt != nil {
		return mt
	}
	mt := &methodTable{}
	if slot := methodSlot(method); slot >= 0 {
		t.standard[slot] = mt
	} else {
		if t.custom == nil {
			t.custom = make(map[string]*methodTable)
		}
		t.custom[method] = mt
	}
	i, _ := slices.BinarySearch(t.methods, method)
	t.methods = slices.Insert(t.methods, i, method)
	return mt
}

// Add registers h for method and pattern. It returns an error for invalid
// syntax, a duplicate pattern, or a wildcard-name conflict, and leaves the
// table unchanged in that case.
func (t *Table) Add(method, pattern string, h Handle) error {
	if method == "" {
		return errors.New("method must not be empty")
	}
	if h == nil {
		return errors.New("handle must not be nil")
	}
	var buf [8]part
	parts, static, err := parsePattern(pattern, buf[:0])
	if err != nil {
		return err
	}

	if static {
		mt := t.table(method)
		if mt != nil && mt.static.lookup(pattern) != nil {
			return duplicateError(pattern)
		}
		if mt == nil {
			mt = t.ensureTable(method)
		}
		mt.static.add(pattern, h)
		return nil
	}

	if mt := t.table(method); mt != nil && mt.root != nil {
		if err := mt.root.checkInsert(pattern, parts); err != nil {
			return err
		}
	}
	mt := t.ensureTable(method)
	if mt.root == nil {
		mt.root = &node{}
	}
	mt.root.insert(parts, pattern, h)
	return nil
}

// Lookup finds the route for method and path, appending captured parameters
// to params, which may be nil. It never writes a response.
func (t *Table) Lookup(method, path string, params scontext.Params) (Match, bool) {
	mt := t.table(method)
	if mt == nil {
		return Match{}, false
	}
	if r := mt.static.lookup(path); r != nil {
		return Match{Handle: r.handle, Pattern: r.pattern, Params: params}, true
	}
	var m matcher
	n := mt.match(path, &m)
	if n == nil {
		return Match{}, false
	}
	return Match{Handle: n.handle, Pattern: n.pattern, Params: m.result(params)}, true
}

// ServeHTTP calls the handle for the request's method and path, or responds
// with a redirect, an automatic OPTIONS response, 405, or 404.
func (t *Table) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	mt := t.table(req.Method)
	if mt == nil {
		t.serveUnmatched(w, req, nil, false, false)
		return
	}
	path := req.URL.Path
	if r := mt.static.lookup(path); r != nil {
		r.handle(w, req, nil)
		return
	}
	var m matcher
	if n := mt.match(path, &m); n != nil {
		n.handle(w, req, m.result(nil))
		return
	}
	t.serveUnmatched(w, req, mt, m.tsrAdd, m.tsrRemove)
}

// staticRoute is a fully static pattern and its handle.
type staticRoute struct {
	handle  Handle
	pattern string
}

// staticTable is an open-addressing hash table of fully static patterns. It
// grows by doubling and never shrinks. A filter over each pattern's length
// and last byte lets most requests for wildcard routes, whose paths tend to
// end in a value, skip the hash lookup with one load.
type staticTable struct {
	routes []staticRoute
	slots  []int32 // positions in routes; -1 is empty; len is a power of two
	filter [64]uint64
	seed   maphash.Seed
}

func (st *staticTable) hash(path string) uint32 {
	return uint32(maphash.String(st.seed, path))
}

// filterBit returns the filter word for a path's length and the bit for
// its last byte.
func filterBit(path string) (word int, bit uint64) {
	return min(len(path), len(staticTable{}.filter)-1), 1 << (path[len(path)-1] & 63)
}

func (st *staticTable) lookup(path string) *staticRoute {
	if path == "" {
		return nil
	}
	if word, bit := filterBit(path); st.filter[word]&bit == 0 {
		return nil
	}
	mask := uint32(len(st.slots) - 1)
	for slot := st.hash(path) & mask; ; slot = (slot + 1) & mask {
		i := st.slots[slot]
		if i < 0 {
			return nil
		}
		if r := &st.routes[i]; r.pattern == path {
			return r
		}
	}
}

// lookupWithSlash reports whether path+"/" is registered, without building
// the longer string for common path lengths.
func (st *staticTable) lookupWithSlash(path string) bool {
	var buf [128]byte
	key := append(append(buf[:0], path...), '/')
	return st.lookup(string(key)) != nil
}

func (st *staticTable) add(pattern string, h Handle) {
	if st.routes == nil {
		st.seed = maphash.MakeSeed()
	}
	st.routes = append(st.routes, staticRoute{handle: h, pattern: pattern})
	word, bit := filterBit(pattern)
	st.filter[word] |= bit
	if 2*len(st.routes) > len(st.slots) {
		st.rebuild()
		return
	}
	st.place(int32(len(st.routes) - 1))
}

// rebuild sizes the slots to at least twice the number of routes, so probes
// stay short.
func (st *staticTable) rebuild() {
	size := 8
	for size < 2*len(st.routes) {
		size <<= 1
	}
	st.slots = make([]int32, size)
	for i := range st.slots {
		st.slots[i] = -1
	}
	for i := range st.routes {
		st.place(int32(i))
	}
}

func (st *staticTable) place(i int32) {
	mask := uint32(len(st.slots) - 1)
	slot := st.hash(st.routes[i].pattern) & mask
	for st.slots[slot] >= 0 {
		slot = (slot + 1) & mask
	}
	st.slots[slot] = i
}
