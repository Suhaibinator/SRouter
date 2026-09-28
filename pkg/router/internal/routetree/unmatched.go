package routetree

import (
	"bytes"
	"html"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"unicode/utf8"
)

// ServeUnmatched responds to a request that Lookup did not match. It applies
// the spec's steps in order: trailing-slash redirect, cleaned-path redirect,
// automatic OPTIONS, 405 Method Not Allowed, then 404 Not Found.
func (t *Table) ServeUnmatched(w http.ResponseWriter, req *http.Request) {
	t.serveUnmatched(w, req, nil, false, false)
}

// serveUnmatched is ServeUnmatched for a caller that may already have
// searched the request method's table. searched is that table, or nil, and
// tsrAdd and tsrRemove are the hints from that search.
func (t *Table) serveUnmatched(w http.ResponseWriter, req *http.Request, searched *methodTable, tsrAdd, tsrRemove bool) {
	method, path := req.Method, req.URL.Path

	if target, ok := t.redirectTarget(method, path, searched, tsrAdd, tsrRemove); ok {
		redirect(w, req, target)
		return
	}

	if method == http.MethodOptions {
		if allow := t.allowed(path, method); allow != "" {
			w.Header().Set("Allow", allow)
			w.WriteHeader(http.StatusOK)
			return
		}
	} else if allow := t.allowed(path, method); allow != "" {
		w.Header().Set("Allow", allow)
		http.Error(w, http.StatusText(http.StatusMethodNotAllowed), http.StatusMethodNotAllowed)
		return
	}

	http.NotFound(w, req)
}

// redirectTarget returns the path that the request method's own routes match
// once the trailing slash is toggled or the path is cleaned.
func (t *Table) redirectTarget(method, path string, searched *methodTable, tsrAdd, tsrRemove bool) (string, bool) {
	if method == http.MethodConnect || path == "" || path[0] != '/' {
		return "", false
	}
	mt := t.table(method)
	if mt == nil {
		return "", false
	}

	// Only clean targets qualify: browsers and proxies may clean a Location
	// themselves, which would send the client somewhere this table never
	// checked.
	clean := isCleanPath(path)
	if clean && path != "/" {
		if searched != mt {
			_, tsrAdd, tsrRemove = mt.has(path)
		}
		if mt.hasToggled(path, tsrAdd, tsrRemove) {
			return toggleTrailingSlash(path)
		}
	}
	if !clean {
		cleaned := cleanPath(path)
		found, tsrAdd, tsrRemove := mt.has(cleaned)
		if found {
			return cleaned, true
		}
		if cleaned != "/" && mt.hasToggled(cleaned, tsrAdd, tsrRemove) {
			return toggleTrailingSlash(cleaned)
		}
	}
	return "", false
}

// redirect sends the client to target, a clean rooted path, producing the
// same headers and body as http.Redirect. That function parses and cleans
// the Location again, which target does not need, so this saves the parse.
func redirect(w http.ResponseWriter, req *http.Request, target string) {
	code := http.StatusTemporaryRedirect
	if req.Method == http.MethodGet {
		code = http.StatusMovedPermanently
	}
	// Encoding the path keeps characters such as '\' from turning the
	// Location into a URL that browsers resolve against another host.
	location := target
	if !pathSafe(target) {
		location = (&url.URL{Path: target}).EscapedPath()
	}
	if q := req.URL.RawQuery; q != "" {
		location += "?" + q
	}

	h := w.Header()
	// http.Redirect adds a short HTML body for old user agents unless the
	// handler already chose a content type.
	_, hadCT := h["Content-Type"]
	h.Set("Location", hexEscapeNonASCII(location))
	if !hadCT && (req.Method == http.MethodGet || req.Method == http.MethodHead) {
		h.Set("Content-Type", "text/html; charset=utf-8")
	}
	w.WriteHeader(code)
	if !hadCT && req.Method == http.MethodGet {
		body := "<a href=\"" + htmlEscape(location) + "\">" + http.StatusText(code) + "</a>.\n\n"
		_, _ = io.WriteString(w, body)
	}
}

// hexEscapeNonASCII percent-encodes bytes outside ASCII, as net/http does
// for a Location header. The path part is already encoded, so only a query
// can need it.
func hexEscapeNonASCII(s string) string {
	for i := 0; i < len(s); i++ {
		if s[i] >= utf8.RuneSelf {
			var b strings.Builder
			b.Grow(len(s) + 2*(len(s)-i))
			b.WriteString(s[:i])
			for ; i < len(s); i++ {
				if s[i] >= utf8.RuneSelf {
					b.WriteByte('%')
					b.WriteString(strconv.FormatInt(int64(s[i]), 16))
				} else {
					b.WriteByte(s[i])
				}
			}
			return b.String()
		}
	}
	return s
}

// htmlEscape escapes the five characters that net/http escapes in the body
// of a redirect, with the same replacements.
func htmlEscape(s string) string {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '&', '<', '>', '"', '\'':
			return html.EscapeString(s)
		}
	}
	return s
}

// pathSafeBytes marks, by byte value, the characters that URL path encoding
// leaves unchanged: letters, digits, and "-._~/$&+,:;=@".
const pathSafeBytes = "" +
	"\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00" +
	"\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00" +
	"\x00\x00\x00\x00\x01\x00\x01\x00\x00\x00\x00\x01\x01\x01\x01\x01" + // $ & + , - . /
	"\x01\x01\x01\x01\x01\x01\x01\x01\x01\x01\x01\x01\x00\x01\x00\x00" + // 0-9 : ; =
	"\x01\x01\x01\x01\x01\x01\x01\x01\x01\x01\x01\x01\x01\x01\x01\x01" + // @ A-O
	"\x01\x01\x01\x01\x01\x01\x01\x01\x01\x01\x01\x00\x00\x00\x00\x01" + // P-Z _
	"\x00\x01\x01\x01\x01\x01\x01\x01\x01\x01\x01\x01\x01\x01\x01\x01" + // a-o
	"\x01\x01\x01\x01\x01\x01\x01\x01\x01\x01\x01\x00\x00\x00\x01\x00" + // p-z ~
	"\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00" +
	"\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00" +
	"\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00" +
	"\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00" +
	"\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00" +
	"\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00" +
	"\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00" +
	"\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00"

// pathSafe reports whether path contains only bytes that URL path encoding
// leaves unchanged, so it can be used in a Location header as-is.
func pathSafe(path string) bool {
	for i := 0; i < len(path); i++ {
		if pathSafeBytes[path[i]] == 0 {
			return false
		}
	}
	return true
}

// allowed returns the Allow header value for path: every method other than
// exclude whose routes match path exactly, plus OPTIONS, sorted and joined. It
// returns "" when no method matches. OPTIONS routes never count as a match, so
// a catch-all such as "OPTIONS /*path" does not turn every 404 into a 405. The
// OPTIONS target "*" lists every method with a route.
func (t *Table) allowed(path, exclude string) string {
	var buf [standardSlots + 1]string
	methods := buf[:0]
	everyMethod := path == "*" && exclude == http.MethodOptions
	for _, method := range t.methods {
		if method == exclude || method == http.MethodOptions {
			continue
		}
		if !everyMethod {
			if found, _, _ := t.table(method).has(path); !found {
				continue
			}
		}
		methods = append(methods, method)
	}
	if len(methods) == 0 {
		return ""
	}
	if i, found := slices.BinarySearch(methods, http.MethodOptions); !found {
		methods = slices.Insert(methods, i, http.MethodOptions)
	}
	return strings.Join(methods, ", ")
}

// toggleTrailingSlash adds or removes the final slash. The root path has no
// alternate.
func toggleTrailingSlash(path string) (string, bool) {
	if len(path) <= 1 {
		return "", false
	}
	if path[len(path)-1] == '/' {
		return path[:len(path)-1], true
	}
	return path + "/", true
}

// isCleanPath reports whether cleanPath would return path unchanged: it is
// rooted and has no empty, ".", or ".." segments, except that it may end in
// '/'. Every unclean path contains "//" or "/." somewhere, so one pass over
// the bytes decides the common case.
func isCleanPath(path string) bool {
	if path == "" || path[0] != '/' {
		return false
	}
	suspect := false
	for i := 1; i < len(path); i++ {
		if path[i-1] == '/' && (path[i] == '/' || path[i] == '.') {
			suspect = true
			break
		}
	}
	if !suspect {
		return true
	}
	for i := 0; i < len(path); {
		end := i + 1
		for end < len(path) && path[end] != '/' {
			end++
		}
		switch path[i+1 : end] {
		case ".", "..":
			return false
		case "":
			if end < len(path) {
				return false
			}
		}
		i = end
	}
	return true
}

// cleanPath returns the canonical form of path: rooted, with repeated slashes
// collapsed, "." segments removed, and ".." segments resolved without going
// above the root. It keeps a trailing slash when path ends in "/" or "/.".
func cleanPath(path string) string {
	buf := make([]byte, 1, len(path)+2)
	buf[0] = '/'

	for i := 0; i < len(path); {
		if path[i] == '/' {
			i++
			continue
		}
		end := i
		for end < len(path) && path[end] != '/' {
			end++
		}
		switch segment := path[i:end]; segment {
		case ".":
		case "..":
			last := bytes.LastIndexByte(buf, '/')
			buf = buf[:max(last, 1)]
		default:
			if len(buf) > 1 {
				buf = append(buf, '/')
			}
			buf = append(buf, segment...)
		}
		i = end
	}

	if len(buf) > 1 && (strings.HasSuffix(path, "/") || strings.HasSuffix(path, "/.")) {
		buf = append(buf, '/')
	}
	return string(buf)
}
