package traceid

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestHeaders(t *testing.T) {
	for _, tc := range []struct{ got, want string }{
		{HeaderXTraceID, "X-Trace-ID"}, {HeaderXRequestID, "X-Request-ID"},
		{HeaderXCorrelationID, "X-Correlation-ID"}, {HeaderCloudflareRayID, "CF-Ray"},
		{HeaderB3TraceID, "X-B3-TraceId"}, {HeaderTraceparent, "traceparent"},
	} {
		if tc.got != tc.want {
			t.Errorf("constant = %q, want %q", tc.got, tc.want)
		}
		r := httptest.NewRequest("GET", "/", nil)
		source := FromHeader(strings.ToLower(tc.got))
		if id, ok := source(r); id != "" || ok {
			t.Fatal("absent header accepted")
		}
		r.Header.Set(tc.got, "")
		if _, ok := source(r); ok {
			t.Fatal("empty header accepted")
		}
		r.Header.Set(tc.got, " raw.value ")
		r.Header.Add(tc.got, "second")
		if id, ok := source(r); id != " raw.value " || !ok {
			t.Fatalf("raw header = %q, %v", id, ok)
		}
		if len(r.Header.Values(tc.got)) != 2 {
			t.Fatal("source modified headers")
		}
	}
}

func TestIsValid(t *testing.T) {
	for _, id := range []string{"a", "0123456789", "abcXYZ-123_", strings.Repeat("a", 64), Generate()} {
		if !IsValid(id) {
			t.Errorf("rejected %q", id)
		}
	}
	for _, id := range []string{"", strings.Repeat("a", 65), "a.b", "a/b", "a b", "a\t", "a\n", "a\r", "a\x00", "a\x7f", "é"} {
		if IsValid(id) {
			t.Errorf("accepted %q", id)
		}
	}
}

func TestTraceparent(t *testing.T) {
	const trace = "4bf92f3577b34da6a3ce929d0e0e4736"
	const base = "00-" + trace + "-00f067aa0ba902b7-01"
	for _, tc := range []struct {
		value string
		valid bool
	}{
		{base, true}, {base[:53] + "00", true}, {base[:53] + "ff", true},
		{"01" + base[2:], true}, {"fe" + base[2:] + "-opaque", true}, {"01" + base[2:] + "-", true},
		{"", false}, {base[:54], false}, {base + "-extra", false},
		{"ff" + base[2:], false}, {"gg" + base[2:], false}, {"0A" + base[2:], false},
		{"0-" + base[3:], false}, {"00_" + base[3:], false},
		{base[:35] + "_" + base[36:], false}, {base[:52] + "_01", false},
		{"00-" + strings.Repeat("0", 32) + base[35:], false},
		{base[:36] + strings.Repeat("0", 16) + "-01", false},
		{"00-" + strings.ToUpper(trace) + base[35:], false},
		{base[:36] + "G" + base[37:], false}, {base[:53] + "0G", false},
		{" " + base, false}, {base + " ", false}, {base + "," + base, false},
		{"01" + base[2:] + "extra", false}, {"01" + base[2:] + "-a b", false},
		{"01" + base[2:] + "-a,b", false}, {"01" + base[2:] + "-\x7f", false},
	} {
		t.Run(tc.value, func(t *testing.T) {
			r := httptest.NewRequest("GET", "/", nil)
			r.Header.Set(HeaderTraceparent, tc.value)
			id, ok := FromTraceparent(r)
			if ok != tc.valid || (ok && id != trace) || (!ok && id != "") {
				t.Fatalf("got %q, %v", id, ok)
			}
		})
	}
	r := httptest.NewRequest("GET", "/", nil)
	if _, ok := FromTraceparent(r); ok {
		t.Fatal("accepted absent header")
	}
	r.Header.Add(HeaderTraceparent, base)
	r.Header.Add(HeaderTraceparent, base)
	if _, ok := FromTraceparent(r); ok {
		t.Fatal("accepted duplicate headers")
	}
}

func assertUUIDv7(t *testing.T, id string) {
	t.Helper()
	if len(id) != 32 || !lowerHex(id) || id[12] != '7' || !strings.ContainsRune("89ab", rune(id[16])) {
		t.Errorf("invalid UUIDv7: %q", id)
	}
}

func TestGenerator(t *testing.T) {
	if g, err := NewGenerator(-1); g != nil || err == nil {
		t.Fatal("negative buffer accepted")
	}
	assertUUIDv7(t, Generate())
	var zero Generator
	zero.Stop()
	assertUUIDv7(t, zero.Next())
	for _, size := range []int{0, 1, 64} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			g, err := NewGenerator(size)
			if err != nil {
				t.Fatal(err)
			}
			defer g.Stop()
			if size == 0 && (g.ids != nil || g.stop != nil || g.done != nil) {
				t.Fatal("synchronous generator has worker state")
			}
			if size > 0 && len(g.ids) != size {
				t.Fatal("buffer not prefilled")
			}
			var seen sync.Map
			var wg sync.WaitGroup
			for range 16 {
				wg.Go(func() {
					for range 100 {
						id := g.Next()
						assertUUIDv7(t, id)
						if _, exists := seen.LoadOrStore(id, true); exists {
							t.Errorf("duplicate %s", id)
						}
					}
				})
			}
			wg.Go(g.Stop)
			wg.Go(g.Stop)
			wg.Wait()
			if g.done != nil {
				select {
				case <-g.done:
				default:
					t.Fatal("worker still running")
				}
			}
			for range size + 1 {
				assertUUIDv7(t, g.Next())
			}
		})
	}
}

func TestGeneratorBufferedAndFallback(t *testing.T) {
	g := &Generator{ids: make(chan string, 1)}
	g.ids <- "buffered"
	if got := g.Next(); got != "buffered" {
		t.Fatalf("got %q", got)
	}
	assertUUIDv7(t, g.Next())
}
