package routetree

import (
	"fmt"
	"math/bits"
	"strings"

	"github.com/Suhaibinator/SRouter/pkg/scontext"
)

// indexThreshold is the number of static children up to which a node keeps
// them in a dense slice and scans it. Beyond it, the slice becomes an
// open-addressing hash table keyed by the child's first segment.
const indexThreshold = 8

// node is one position in the trie that holds patterns with wildcards.
// Fully static patterns live in staticTable instead.
type node struct {
	// text is the static text this node matches, including its leading
	// '/'. It spans several segments when no route branches in between.
	// For a wildcard node it is the wildcard's name instead.
	text string

	// kids holds the static children; no two share a first segment. Up to
	// indexThreshold children are packed at the front. More than that
	// turns kids into a power-of-two hash table with nil entries for empty
	// slots, so len(kids) alone says which layout is in use.
	kids []kid

	param    *node
	catchAll *node

	handle  Handle // nil when no route ends here
	pattern string
}

// kid is a static child together with the first bytes of its text, so a
// scan can reject most children by comparing one word without loading them.
type kid struct {
	node *node
	word uint64 // the first eight bytes of node.text, little-endian, zero-padded
}

func newKid(c *node) kid {
	return kid{node: c, word: loadWord(c.text)}
}

// matches reports whether w starts with the child's word. The padding of a
// short text is shifted out, so the compare covers the bytes up to the
// text's last non-zero byte and accept checks any after it.
func (k *kid) matches(w uint64) bool {
	return (w^k.word)<<(bits.LeadingZeros64(k.word)&^7) == 0
}

// compared returns how many leading bytes of the text matches covers.
func (k *kid) compared() int {
	return 8 - bits.LeadingZeros64(k.word)/8
}

// wordMask selects the low n bytes of a word, for n from 1 to 8.
func wordMask(n uint8) uint64 {
	return ^uint64(0) >> (64 - 8*uint(n))
}

// load8 returns the first eight bytes of s as a little-endian word. The
// compiler turns it into a single load.
func load8(s string) uint64 {
	_ = s[7]
	return uint64(s[0]) | uint64(s[1])<<8 | uint64(s[2])<<16 | uint64(s[3])<<24 |
		uint64(s[4])<<32 | uint64(s[5])<<40 | uint64(s[6])<<48 | uint64(s[7])<<56
}

// loadWord returns up to the first eight bytes of s as a little-endian word,
// zero-padded when s is shorter.
func loadWord(s string) uint64 {
	if len(s) >= 8 {
		return load8(s)
	}
	return loadShort(s)
}

func loadShort(s string) uint64 {
	var w uint64
	for i := range len(s) {
		w |= uint64(s[i]) << (8 * i)
	}
	return w
}

const (
	slashBytes = 0x2f2f2f2f2f2f2f2f
	lowBits    = 0x0101010101010101
	highBits   = 0x8080808080808080
)

// slashMask returns a word whose high bit is set in every byte where w
// holds a '/'. Bytes above the lowest such byte may hold false positives.
func slashMask(w uint64) uint64 {
	w ^= slashBytes
	return (w - lowBits) &^ w & highBits
}

// segmentWord cuts the word of a text or path that starts with '/' at the
// next '/', so that equal first segments give equal words.
func segmentWord(w uint64) uint64 {
	if zero := slashMask(w | 0xff); zero != 0 {
		return w & wordMask(uint8(bits.TrailingZeros64(zero)/8))
	}
	return w
}

// kidHash hashes a first-segment word. Every byte of the word reaches the
// low bits of the result, which pick the slot, through the two multiplies.
func kidHash(seg uint64) uint32 {
	h := seg * 0x9e3779b97f4a7c15
	h ^= h >> 32
	h *= 0xbf58476d1ce4e5b9
	return uint32(h >> 32)
}

func (n *node) indexed() bool { return len(n.kids) > indexThreshold }

// segmentEnd returns the index just past the first segment of a path that
// starts with '/'. It checks the eight bytes after the '/' at once, which
// covers most segments, and scans the rest byte by byte.
func segmentEnd(path string) int {
	if len(path) >= 9 {
		if zero := slashMask(load8(path[1:])); zero != 0 {
			return 1 + bits.TrailingZeros64(zero)/8
		}
		return segmentEndFrom(path, 9)
	}
	return segmentEndFrom(path, 1)
}

// segmentEndFrom scans path for its next '/' starting at index from.
func segmentEndFrom(path string, from int) int {
	for from < len(path) && path[from] != '/' {
		from++
	}
	return from
}

// firstSegment returns the first segment of n.text.
func (n *node) firstSegment() string {
	return n.text[1:segmentEnd(n.text)]
}

// accept reports whether path starts with the child's text at a segment
// boundary, once the child's word has matched, and returns the rest of the
// path.
func (k *kid) accept(path string) (string, bool) {
	text := k.node.text
	if len(path) < len(text) || (len(path) > len(text) && path[len(text)] != '/') {
		return "", false
	}
	if from := k.compared(); len(text) > from && !tailEqual(path, text, from, len(text)) {
		return "", false
	}
	return path[len(text):], true
}

// tailEqual reports whether path and text agree on bytes from to to. A
// short tail is compared in a loop, which beats the call for a bulk compare.
func tailEqual(path, text string, from, to int) bool {
	if to-from > 8 {
		return path[from:to] == text[from:to]
	}
	for i := from; i < to; i++ {
		if path[i] != text[i] {
			return false
		}
	}
	return true
}

// indexedChild returns the child of a node with hashed children whose text
// starts path at a segment boundary, and the path that remains after it. w
// is the first word of path. Packed children are scanned inline in match.
func (n *node) indexedChild(path string, w uint64) (*node, string) {
	mask := uint32(len(n.kids) - 1)
	for slot := kidHash(segmentWord(w)) & mask; ; slot = (slot + 1) & mask {
		k := &n.kids[slot]
		if k.node == nil {
			return nil, ""
		}
		if k.matches(w) {
			if rest, ok := k.accept(path); ok {
				return k.node, rest
			}
		}
	}
}

// kidEntry returns the entry of the child whose first segment equals the
// first segment of text, if any. The pointer is valid until the next
// addKid.
func (n *node) kidEntry(text string) *kid {
	segment := text[1:segmentEnd(text)]
	if !n.indexed() {
		for i := range n.kids {
			if n.kids[i].node.firstSegment() == segment {
				return &n.kids[i]
			}
		}
		return nil
	}
	mask := uint32(len(n.kids) - 1)
	for slot := kidHash(segmentWord(loadWord(text))) & mask; ; slot = (slot + 1) & mask {
		k := &n.kids[slot]
		if k.node == nil {
			return nil
		}
		if k.node.firstSegment() == segment {
			return k
		}
	}
}

// kidForText returns the child whose first segment equals the first segment
// of text, if any.
func (n *node) kidForText(text string) *node {
	if k := n.kidEntry(text); k != nil {
		return k.node
	}
	return nil
}

func (n *node) addKid(c *node) {
	entry := newKid(c)
	if !n.indexed() {
		if len(n.kids) < indexThreshold {
			n.kids = append(n.kids, entry)
			return
		}
		n.rehashKids(4*indexThreshold, entry)
		return
	}
	count := 0
	for i := range n.kids {
		if n.kids[i].node != nil {
			count++
		}
	}
	if 2*(count+1) > len(n.kids) {
		n.rehashKids(2*len(n.kids), entry)
		return
	}
	n.placeKid(entry)
}

// rehashKids moves the children into a hash table of size slots, which
// keeps the load at or below one half so probes stay short, then adds
// entry.
func (n *node) rehashKids(size int, entry kid) {
	old := n.kids
	n.kids = make([]kid, size)
	for i := range old {
		if old[i].node != nil {
			n.placeKid(old[i])
		}
	}
	n.placeKid(entry)
}

func (n *node) placeKid(entry kid) {
	mask := uint32(len(n.kids) - 1)
	slot := kidHash(segmentWord(entry.word)) & mask
	for n.kids[slot].node != nil {
		slot = (slot + 1) & mask
	}
	n.kids[slot] = entry
}

// split turns the child in entry k into a node for the first length bytes
// of its text and moves everything else into a single new grandchild. The
// entry is refreshed because its word covers the child's text.
func (k *kid) split(length int) {
	n := k.node
	tail := *n
	tail.text = n.text[length:]
	*n = node{text: n.text[:length]}
	n.addKid(&tail)
	*k = newKid(n)
}

// commonBoundary returns the length of the longest common prefix of a and b
// that ends at the end of a string or in front of a '/'. Both start with
// '/', so the result is never negative.
func commonBoundary(a, b string) int {
	i := 0
	for i < len(a) && i < len(b) && a[i] == b[i] {
		i++
	}
	if (i == len(a) || a[i] == '/') && (i == len(b) || b[i] == '/') {
		return i
	}
	return strings.LastIndexByte(a[:i], '/')
}

// checkInsert reports whether inserting parts below n would conflict with
// the routes already there. It never modifies the trie, so a failed Add
// leaves no partial state behind.
func (n *node) checkInsert(pattern string, parts []part) error {
	current := n
	for _, p := range parts {
		switch p.kind {
		case staticPart:
			text := p.text
			for text != "" {
				c := current.kidForText(text)
				if c == nil {
					return nil
				}
				length := commonBoundary(c.text, text)
				if length < len(c.text) {
					return nil
				}
				text, current = text[length:], c
			}
		case paramPart:
			if current.param == nil {
				return nil
			}
			if current.param.text != p.text {
				return fmt.Errorf("parameter %q in pattern %q conflicts with parameter %q in pattern %q",
					":"+p.text, pattern, ":"+current.param.text, current.param.firstPattern())
			}
			current = current.param
		case catchAllPart:
			if current.catchAll == nil {
				return nil
			}
			if current.catchAll.text != p.text {
				return fmt.Errorf("catch-all %q in pattern %q conflicts with catch-all %q in pattern %q",
					"*"+p.text, pattern, "*"+current.catchAll.text, current.catchAll.pattern)
			}
			return duplicateError(pattern)
		}
	}
	if current.handle != nil {
		return duplicateError(pattern)
	}
	return nil
}

// firstPattern returns a registered pattern that passes through n, for
// error messages.
func (n *node) firstPattern() string {
	for n.handle == nil {
		next := n.param
		if next == nil {
			next = n.catchAll
		}
		for i := range n.kids {
			if n.kids[i].node != nil {
				next = n.kids[i].node
				break
			}
		}
		n = next
	}
	return n.pattern
}

// insert adds the route below n. The caller must have called checkInsert
// first.
func (n *node) insert(parts []part, pattern string, h Handle) {
	current := n
	for _, p := range parts {
		switch p.kind {
		case staticPart:
			current = current.insertStatic(p.text)
		case paramPart:
			if current.param == nil {
				current.param = &node{text: p.text}
			}
			current = current.param
		case catchAllPart:
			current.catchAll = &node{text: p.text}
			current = current.catchAll
		}
	}
	current.handle, current.pattern = h, pattern
}

// insertStatic returns the node at the end of the static run text, adding
// or splitting nodes as needed.
func (n *node) insertStatic(text string) *node {
	for text != "" {
		k := n.kidEntry(text)
		if k == nil {
			c := &node{text: text}
			n.addKid(c)
			return c
		}
		c := k.node
		length := commonBoundary(c.text, text)
		if length < len(c.text) {
			k.split(length)
		}
		text, n = text[length:], c
	}
	return n
}

func duplicateError(pattern string) error {
	return fmt.Errorf("pattern %q is already registered", pattern)
}

// stackParams is the number of parameters a lookup captures without a heap
// allocation before the route is known.
const stackParams = 8

// matcher collects parameters while the trie is searched. It records the
// wildcard node and the value's offsets in the path, which keeps it small
// to zero, and builds the result once the route is known so the result is
// allocated at its exact size. The matcher never points into itself, which
// keeps it on the caller's stack. With discard set, it only checks whether
// a route exists.
type matcher struct {
	path    string // the full path; values are slices of it
	n       int
	nodes   [stackParams]*node
	bounds  [stackParams][2]int
	more    []scontext.Param // parameters beyond stackParams, for very long routes
	discard bool

	// tsrAdd and tsrRemove are set when a failed search saw that the path
	// with a trailing slash added or removed would match a route. The
	// unmatched-request steps use them to redirect without a second search.
	tsrAdd, tsrRemove bool
}

// push records that wild captured path[start:end].
func (m *matcher) push(wild *node, start, end int) {
	if m.discard {
		return
	}
	if m.n < stackParams {
		m.nodes[m.n] = wild
		m.bounds[m.n] = [2]int{start, end}
	} else {
		m.more = append(m.more, scontext.Param{Key: wild.text, Value: m.path[start:end]})
	}
	m.n++
}

// reset drops the parameters captured since the count was mark.
func (m *matcher) reset(mark int) {
	m.n = mark
	m.more = m.more[:max(mark-stackParams, 0)]
}

// result appends the captured parameters to dst, allocating exactly once
// when dst has no room for them.
func (m *matcher) result(dst scontext.Params) scontext.Params {
	if cap(dst)-len(dst) < m.n {
		grown := make(scontext.Params, len(dst), len(dst)+m.n)
		if len(dst) > 0 {
			copy(grown, dst)
		}
		dst = grown
	}
	start := len(dst)
	dst = dst[:start+min(m.n, stackParams)]
	for i := range dst[start:] {
		dst[start+i] = scontext.Param{Key: m.nodes[i].text, Value: m.path[m.bounds[i][0]:m.bounds[i][1]]}
	}
	return append(dst, m.more...)
}

// slashKid reports whether n has a child for the empty segment that ends a
// route, so that path+"/" matches where path matched n exactly.
func (n *node) slashKid() bool {
	c := n.kidForText("/")
	return c != nil && c.text == "/" && c.handle != nil
}

// slashSibling reports whether a child of n is exactly rest+"/" and ends a
// route. It runs when no child matched rest, so the child that would match
// rest+"/" has not been loaded yet.
func (n *node) slashSibling(rest string) bool {
	c := n.kidForText(rest)
	return c != nil && c.handle != nil && len(c.text) == len(rest)+1 && c.text[:len(rest)] == rest
}

// match finds the route for path below n. path is a suffix of m.path: empty
// once every segment has been consumed, otherwise starting with '/'.
// Candidates are tried in the spec's order (static, parameter, catch-all).
// A node only saves a backtrack point when it has more than one candidate
// for the segment.
func (n *node) match(path string, m *matcher) *node {
	for {
		if path == "" {
			if n.handle != nil {
				return n
			}
			if n.catchAll != nil || n.slashKid() {
				m.tsrAdd = true
			}
			return nil
		}

		if len(n.kids) > 0 {
			// The packed scan is written out here to save a call per level.
			var child *node
			var rest string
			var w uint64
			if len(path) >= 8 {
				w = load8(path)
			} else {
				w = loadShort(path)
			}
			if !n.indexed() {
				for i := range n.kids {
					k := &n.kids[i]
					if k.matches(w) {
						if r, ok := k.accept(path); ok {
							child, rest = k.node, r
							break
						}
					}
				}
			} else {
				child, rest = n.indexedChild(path, w)
			}
			if child == nil {
				if n.slashSibling(path) {
					m.tsrAdd = true
				}
			} else if n.param == nil && n.catchAll == nil {
				n, path = child, rest
				continue
			} else {
				mark := m.n
				if r := child.match(rest, m); r != nil {
					return r
				}
				m.reset(mark)
			}
		}
		if n.param != nil {
			// segmentEnd, written out to save a call per parameter. A short
			// tail of a longer path is read from the eight bytes that end the
			// full path and shifted into place, so only tiny paths loop.
			end := 1
			if len(path) >= 9 {
				if zero := slashMask(load8(path[1:])); zero != 0 {
					end += bits.TrailingZeros64(zero) / 8
				} else {
					end = segmentEndFrom(path, 9)
				}
			} else if len(m.path) >= 8 {
				w := load8(m.path[len(m.path)-8:]) >> (8 * (9 - len(path)))
				if zero := slashMask(w); zero != 0 {
					end += bits.TrailingZeros64(zero) / 8
				} else {
					end = len(path)
				}
			} else {
				end = segmentEndFrom(path, 1)
			}
			if end > 1 {
				offset := len(m.path) - len(path)
				if n.catchAll == nil {
					m.push(n.param, offset+1, offset+end)
					n, path = n.param, path[end:]
					continue
				}
				mark := m.n
				m.push(n.param, offset+1, offset+end)
				if r := n.param.match(path[end:], m); r != nil {
					return r
				}
				m.reset(mark)
			}
		}
		if n.catchAll != nil {
			m.push(n.catchAll, len(m.path)-len(path), len(m.path))
			return n.catchAll
		}
		if path == "/" && n.handle != nil {
			m.tsrRemove = true
		}
		return nil
	}
}
