package middleware

import (
	"fmt"
	"strconv"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// Tests in this file cover eviction of stale rate-limiter entries: staleness
// detection, sweep scheduling, the evicted-entry retry path in Allow, and the
// guarantee that eviction never changes a rate-limit decision.

// fakeClock is a manually advanced clock for driving the limiter's sweep and
// staleness logic deterministically.
type fakeClock struct {
	nanos atomic.Int64
}

func newFakeClock(start time.Time) *fakeClock {
	c := &fakeClock{}
	c.nanos.Store(start.UnixNano())
	return c
}

func (c *fakeClock) now() time.Time          { return time.Unix(0, c.nanos.Load()) }
func (c *fakeClock) advance(d time.Duration) { c.nanos.Add(int64(d)) }

// limiterEntryCount counts the entries currently stored in the limiter map.
func limiterEntryCount(u *UberRateLimiter) int {
	n := 0
	u.limiters.Range(func(_, _ any) bool {
		n++
		return true
	})
	return n
}

// limiterCompositeKey mirrors the composite key format Allow builds.
func limiterCompositeKey(key string, limit int, window time.Duration) string {
	return key + "|" + strconv.Itoa(limit) + "|" + strconv.FormatInt(int64(window), 10)
}

// TestSweepEvictsStaleEntries verifies that a sweep removes every entry that
// has been idle for at least two of its own windows, including entries idle
// for exactly two windows (the boundary is inclusive because at that point
// the next request would zero both counters anyway).
func TestSweepEvictsStaleEntries(t *testing.T) {
	clock := newFakeClock(time.Unix(1_000_000, 0))
	limiter := &UberRateLimiter{nowFunc: clock.now}
	window := time.Minute

	const keys = 100
	for i := range keys {
		limiter.Allow(fmt.Sprintf("ip-%d", i), 5, window)
	}
	if got := limiterEntryCount(limiter); got != keys {
		t.Fatalf("expected %d entries after creation, got %d", keys, got)
	}

	// One window of idleness is not enough: prev/curr still carry history.
	clock.advance(window)
	limiter.sweep(clock.now())
	if got := limiterEntryCount(limiter); got != keys {
		t.Fatalf("entries idle for one window must survive, got %d of %d", got, keys)
	}

	// Exactly two windows idle: stale, everything goes.
	clock.advance(window)
	limiter.sweep(clock.now())
	if got := limiterEntryCount(limiter); got != 0 {
		t.Fatalf("expected all entries evicted after 2 windows idle, got %d", got)
	}
}

// TestSweepKeepsActiveEntriesAndTheirCounts verifies that a sweep only removes
// stale entries, and that a surviving entry keeps its window counts intact.
func TestSweepKeepsActiveEntriesAndTheirCounts(t *testing.T) {
	clock := newFakeClock(time.Unix(1_000_000, 0))
	limiter := &UberRateLimiter{nowFunc: clock.now}
	window := time.Minute
	const limit = 3

	// keyOld is used once at t0 and then goes idle.
	limiter.Allow("key-old", limit, window)

	// keyNew is used once at t0 + 1.5w.
	clock.advance(3 * window / 2)
	if allowed, remaining, _ := limiter.Allow("key-new", limit, window); !allowed || remaining != limit-1 {
		t.Fatalf("key-new first request: got (allowed=%v, remaining=%d), want (true, %d)", allowed, remaining, limit-1)
	}

	// At t0 + 2w: keyOld is 2 windows idle (stale), keyNew only 0.5w idle.
	clock.advance(window / 2)
	limiter.sweep(clock.now())

	if _, ok := limiter.limiters.Load(limiterCompositeKey("key-old", limit, window)); ok {
		t.Error("key-old should have been evicted")
	}
	if _, ok := limiter.limiters.Load(limiterCompositeKey("key-new", limit, window)); !ok {
		t.Fatal("key-new must survive the sweep")
	}

	// keyNew's earlier request must still count: same window, so remaining
	// reflects one prior use (limit - 1 used - 1 for this request).
	if allowed, remaining, _ := limiter.Allow("key-new", limit, window); !allowed || remaining != limit-2 {
		t.Errorf("key-new after sweep: got (allowed=%v, remaining=%d), want (true, %d) — prior count was lost",
			allowed, remaining, limit-2)
	}

	// keyOld was legitimately idle for 2 windows, so recreation grants the
	// full limit — the same answer a non-evicted entry would have given.
	if allowed, remaining, _ := limiter.Allow("key-old", limit, window); !allowed || remaining != limit-1 {
		t.Errorf("key-old after eviction: got (allowed=%v, remaining=%d), want (true, %d)", allowed, remaining, limit-1)
	}
}

// TestEvictionIsLossless runs the same scripted request sequence against two
// limiters sharing one clock — one swept aggressively, one never swept — and
// requires identical results (allowed, remaining, and reset) at every step.
// This is the core guarantee: eviction must never change a decision.
func TestEvictionIsLossless(t *testing.T) {
	clock := newFakeClock(time.Unix(1_000_000, 0))
	swept := &UberRateLimiter{nowFunc: clock.now}
	control := &UberRateLimiter{nowFunc: clock.now}
	window := time.Minute
	const limit = 3

	step := func(label string) {
		t.Helper()
		a1, r1, reset1 := swept.Allow("k", limit, window)
		a2, r2, reset2 := control.Allow("k", limit, window)
		if a1 != a2 || r1 != r2 || reset1 != reset2 {
			t.Fatalf("%s: swept limiter diverged: got (%v, %d, %v), control says (%v, %d, %v)",
				label, a1, r1, reset1, a2, r2, reset2)
		}
	}

	// Exhaust the limit, then one denial.
	for i := range limit + 1 {
		step(fmt.Sprintf("initial request %d", i+1))
	}

	// Advance exactly two windows (a multiple of the window keeps the two
	// limiters' window phases aligned) and evict on the swept limiter only.
	clock.advance(2 * window)
	swept.sweep(clock.now())
	if got := limiterEntryCount(swept); got != 0 {
		t.Fatalf("expected swept limiter to be empty, got %d entries", got)
	}

	// Both must now agree on a fresh burst: allow, allow, allow, deny.
	for i := range limit + 1 {
		step(fmt.Sprintf("post-eviction request %d", i+1))
	}

	// And on partial-window behavior afterwards.
	clock.advance(window / 2)
	step("half-window later")
	clock.advance(window)
	swept.sweep(clock.now()) // not stale (recent use): must be a no-op
	step("one more window later")
}

// TestAllowRetriesWhenEntryEvictedMidFlight pins the race where a request
// loads an entry just before the sweeper marks it evicted: Allow must detect
// the tombstone, replace the entry, and count against a fresh one rather than
// losing the count or hanging.
func TestAllowRetriesWhenEntryEvictedMidFlight(t *testing.T) {
	clock := newFakeClock(time.Unix(1_000_000, 0))
	limiter := &UberRateLimiter{nowFunc: clock.now}
	window := time.Minute
	const limit = 5

	limiter.Allow("victim", limit, window)
	compositeKey := limiterCompositeKey("victim", limit, window)

	v, ok := limiter.limiters.Load(compositeKey)
	if !ok {
		t.Fatal("expected entry to exist")
	}
	old := v.(*slidingWindowLimiter)

	// Simulate a sweeper paused between marking the entry and deleting it
	// from the map: the tombstone is set but the entry is still loadable.
	old.mu.Lock()
	old.evicted = true
	old.mu.Unlock()

	done := make(chan struct{})
	var allowed bool
	var remaining int
	go func() {
		defer close(done)
		allowed, remaining, _ = limiter.Allow("victim", limit, window)
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("Allow hung on an evicted-but-not-deleted entry")
	}

	// The retry must have created a fresh entry; the count lands there.
	if !allowed || remaining != limit-1 {
		t.Errorf("got (allowed=%v, remaining=%d), want (true, %d) against a fresh entry", allowed, remaining, limit-1)
	}
	v, ok = limiter.limiters.Load(compositeKey)
	if !ok {
		t.Fatal("expected a replacement entry in the map")
	}
	if v.(*slidingWindowLimiter) == old {
		t.Error("evicted entry is still in the map; Allow must replace it")
	}
}

// TestMaybeSweepThrottlesToOnePerInterval verifies the sweep-scheduling gate:
// entry creations within limiterSweepInterval of the last sweep do not start
// another one, and a creation after the interval does.
func TestMaybeSweepThrottlesToOnePerInterval(t *testing.T) {
	start := time.Unix(1_000_000, 0)
	clock := newFakeClock(start)
	limiter := &UberRateLimiter{nowFunc: clock.now}
	// 2*window must exceed every clock advance below so async sweeps from
	// maybeSweep can never evict anything while the test runs.
	window := 24 * time.Hour

	// First creation claims the first sweep slot.
	limiter.Allow("a", 1, window)
	if got := limiter.lastSweep.Load(); got != start.UnixNano() {
		t.Fatalf("expected first creation to claim a sweep at %d, got %d", start.UnixNano(), got)
	}

	// Creations inside the interval must not claim another sweep.
	clock.advance(limiterSweepInterval - time.Second)
	limiter.Allow("b", 1, window)
	limiter.Allow("c", 1, window)
	if got := limiter.lastSweep.Load(); got != start.UnixNano() {
		t.Errorf("creation inside the interval rescheduled a sweep: lastSweep moved to %d", got)
	}

	// A creation after the interval claims the next sweep, even with many
	// concurrent creators racing for it.
	clock.advance(2 * time.Second)
	want := clock.now().UnixNano()
	var wg sync.WaitGroup
	for i := range 32 {
		wg.Go(func() {
			limiter.Allow(fmt.Sprintf("racer-%d", i), 1, window)
		})
	}
	wg.Wait()
	if got := limiter.lastSweep.Load(); got != want {
		t.Errorf("expected one sweep claimed at %d after the interval, got %d", want, got)
	}

	// Repeat Allows on existing keys never touch the gate (fast path).
	clock.advance(2 * limiterSweepInterval)
	limiter.Allow("a", 1, window)
	if got := limiter.lastSweep.Load(); got != want {
		t.Errorf("fast-path Allow scheduled a sweep: lastSweep moved to %d", got)
	}
}

// TestEvictionEndToEndThroughPublicAPI exercises the whole pipeline with no
// internal calls except clock injection: a stale key is evicted by the
// background sweep that a new key's creation triggers.
func TestEvictionEndToEndThroughPublicAPI(t *testing.T) {
	clock := newFakeClock(time.Unix(1_000_000, 0))
	limiter := &UberRateLimiter{nowFunc: clock.now}
	window := time.Minute
	const limit = 2

	// Exhaust the victim's limit.
	limiter.Allow("victim", limit, window)
	limiter.Allow("victim", limit, window)
	if allowed, _, _ := limiter.Allow("victim", limit, window); allowed {
		t.Fatal("victim should be rate limited")
	}

	// Go idle long enough to be stale (2w) and to reopen the sweep gate
	// (limiterSweepInterval); then a brand-new key triggers the async sweep.
	advance := 2 * window
	if advance <= limiterSweepInterval {
		advance = limiterSweepInterval + 2*window
	}
	clock.advance(advance)
	limiter.Allow("trigger", limit, window)

	// The sweep runs in a goroutine; wait for the victim entry to vanish.
	victimKey := limiterCompositeKey("victim", limit, window)
	deadline := time.Now().Add(2 * time.Second)
	for {
		if _, ok := limiter.limiters.Load(victimKey); !ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("victim entry was not evicted by the background sweep")
		}
		time.Sleep(time.Millisecond)
	}

	// After legitimate 2w idleness the full limit is available again.
	if allowed, remaining, _ := limiter.Allow("victim", limit, window); !allowed || remaining != limit-1 {
		t.Errorf("victim after eviction: got (allowed=%v, remaining=%d), want (true, %d)", allowed, remaining, limit-1)
	}
}

// TestMapSizeStaysBoundedUnderKeyChurn simulates the attack the eviction
// exists for — a flood of never-repeating keys — and asserts the map returns
// to baseline once the flood ages out, rather than growing monotonically.
func TestMapSizeStaysBoundedUnderKeyChurn(t *testing.T) {
	clock := newFakeClock(time.Unix(1_000_000, 0))
	limiter := &UberRateLimiter{nowFunc: clock.now}
	window := time.Minute

	const perWave = 1000
	for wave := range 3 {
		for i := range perWave {
			limiter.Allow(fmt.Sprintf("wave%d-ip%d", wave, i), 10, window)
		}
		// Each wave then ages past staleness before the next arrives.
		clock.advance(2 * window)
		limiter.sweep(clock.now())

		if got := limiterEntryCount(limiter); got != 0 {
			t.Fatalf("after wave %d: expected 0 surviving entries, got %d (map is leaking)", wave, got)
		}
	}
}

// TestSweepDoesNotEvictConcurrentlyCreatedEntry verifies that an entry created
// at the sweep's own timestamp is never considered stale (its windowStart is
// initialized at creation, so a racing sweep sees zero idle time).
func TestSweepDoesNotEvictConcurrentlyCreatedEntry(t *testing.T) {
	clock := newFakeClock(time.Unix(1_000_000, 0))
	limiter := &UberRateLimiter{nowFunc: clock.now}
	window := time.Minute

	limiter.Allow("fresh", 5, window)
	limiter.sweep(clock.now())

	if _, ok := limiter.limiters.Load(limiterCompositeKey("fresh", 5, window)); !ok {
		t.Fatal("a just-created entry must survive a sweep at the same instant")
	}
}

// TestSweepIgnoresZeroValueEntries verifies the sweeper's defensive guards: an
// entry with no window or no windowStart (impossible via Allow, possible via
// direct construction) is left alone rather than evicted by arithmetic on
// zero values.
func TestSweepIgnoresZeroValueEntries(t *testing.T) {
	clock := newFakeClock(time.Unix(1_000_000, 0))
	limiter := &UberRateLimiter{nowFunc: clock.now}

	limiter.limiters.Store("bare", &slidingWindowLimiter{})
	limiter.limiters.Store("no-start", &slidingWindowLimiter{window: time.Minute})
	limiter.limiters.Store("no-window", &slidingWindowLimiter{windowStart: clock.now().Add(-time.Hour)})

	limiter.sweep(clock.now())

	for _, key := range []string{"bare", "no-start", "no-window"} {
		if _, ok := limiter.limiters.Load(key); !ok {
			t.Errorf("zero-value entry %q must not be evicted", key)
		}
	}
}

// TestNoOverAdmissionWhileSweepsRun hammers one key from many goroutines with
// the clock frozen — so nothing is ever stale and every count must be kept —
// while sweeps run continuously in the background. The total number of allowed
// requests must be exactly the limit; one extra admission would mean a sweep
// raced a count away.
func TestNoOverAdmissionWhileSweepsRun(t *testing.T) {
	clock := newFakeClock(time.Unix(1_000_000, 0))
	limiter := &UberRateLimiter{nowFunc: clock.now}
	window := time.Minute
	const limit = 50
	const workers = 8
	const requestsPerWorker = 100

	stopSweeps := make(chan struct{})
	var sweepWG sync.WaitGroup
	sweepWG.Go(func() {
		for {
			select {
			case <-stopSweeps:
				return
			default:
				limiter.sweep(clock.now())
			}
		}
	})

	var allowedTotal atomic.Int64
	var wg sync.WaitGroup
	for range workers {
		wg.Go(func() {
			for range requestsPerWorker {
				allowed, remaining, _ := limiter.Allow("contended", limit, window)
				if allowed {
					allowedTotal.Add(1)
				}
				if remaining < 0 || remaining >= limit {
					t.Errorf("remaining out of range: %d", remaining)
				}
			}
		})
	}
	wg.Wait()
	close(stopSweeps)
	sweepWG.Wait()

	if got := allowedTotal.Load(); got != limit {
		t.Errorf("expected exactly %d admissions with a frozen clock, got %d", limit, got)
	}
}

// TestEvictionUnderConcurrentChaos races Allow calls on a churning key set
// against continuous clock advancement and sweeping. It asserts liveness (no
// hang on the retry path), sane return values, and a race-free run under
// -race; the eviction/retry machinery is being exercised constantly because
// every clock jump makes existing entries stale.
func TestEvictionUnderConcurrentChaos(t *testing.T) {
	clock := newFakeClock(time.Unix(1_000_000, 0))
	limiter := &UberRateLimiter{nowFunc: clock.now}
	window := 10 * time.Millisecond
	const limit = 5
	const workers = 8
	const iterations = 500

	stopChaos := make(chan struct{})
	var chaosWG sync.WaitGroup
	chaosWG.Go(func() {
		for {
			select {
			case <-stopChaos:
				return
			default:
				// Every jump staleness-es all existing entries, then the
				// sweep evicts them out from under in-flight Allows.
				clock.advance(2 * window)
				limiter.sweep(clock.now())
			}
		}
	})

	var wg sync.WaitGroup
	for w := range workers {
		wg.Go(func() {
			for i := range iterations {
				key := fmt.Sprintf("chaos-%d", (w*iterations+i)%16)
				allowed, remaining, reset := limiter.Allow(key, limit, window)
				if remaining < 0 || remaining >= limit {
					t.Errorf("remaining out of range: %d", remaining)
				}
				if !allowed && (reset <= 0 || reset > window) {
					t.Errorf("denied request got reset %v outside (0, %v]", reset, window)
				}
			}
		})
	}

	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(30 * time.Second):
		t.Fatal("workers hung — likely a livelock in the eviction retry path")
	}
	close(stopChaos)
	chaosWG.Wait()
}

// BenchmarkUberRateLimiterAllowHotPath measures the steady-state path (entry
// already exists). Eviction must not add work here: the sweep gate is only
// consulted on entry creation.
func BenchmarkUberRateLimiterAllowHotPath(b *testing.B) {
	limiter := NewUberRateLimiter()
	limiter.Allow("bench", 1_000_000_000, time.Minute)

	b.ReportAllocs()
	b.ResetTimer()
	for b.Loop() {
		limiter.Allow("bench", 1_000_000_000, time.Minute)
	}
}
