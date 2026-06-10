package middleware

import (
	"testing"
	"time"
)

// TestSlidingWindowRolloverWeightsPreviousWindow verifies the window rollover
// behavior of the sliding-window limiter: the previous window's count keeps
// counting against the limit in proportion to how much it still overlaps the
// sliding window, smoothing bursts at window boundaries.
func TestSlidingWindowRolloverWeightsPreviousWindow(t *testing.T) {
	l := &slidingWindowLimiter{}
	const limit = 2
	window := time.Minute
	t0 := time.Unix(1_000_000, 0)

	// Use up the full limit inside the first window.
	if allowed, remaining, _ := l.allow(limit, window, t0); !allowed || remaining != 1 {
		t.Fatalf("first request: got (allowed=%v, remaining=%d), want (true, 1)", allowed, remaining)
	}
	if allowed, remaining, _ := l.allow(limit, window, t0.Add(time.Second)); !allowed || remaining != 0 {
		t.Fatalf("second request: got (allowed=%v, remaining=%d), want (true, 0)", allowed, remaining)
	}

	// Over the limit: denied, and reset reports the time left in the window.
	allowed, _, reset := l.allow(limit, window, t0.Add(10*time.Second))
	if allowed {
		t.Fatal("third request inside the window should be denied")
	}
	if reset != 50*time.Second {
		t.Errorf("expected reset of 50s (window remainder), got %v", reset)
	}

	// Exactly at the window boundary the previous window still fully overlaps
	// the sliding window, so a burst right at the boundary is still denied.
	allowed, _, reset = l.allow(limit, window, t0.Add(window))
	if allowed {
		t.Fatal("request at the window boundary should still be denied (previous window fully weighted)")
	}
	if reset != window {
		t.Errorf("expected reset of one full window, got %v", reset)
	}

	// Halfway through the second window only half of the previous window's
	// 2 requests count (estimated usage 1 of 2), so one request is allowed.
	if allowed, remaining, _ := l.allow(limit, window, t0.Add(90*time.Second)); !allowed || remaining != 0 {
		t.Fatalf("mid-second-window request: got (allowed=%v, remaining=%d), want (true, 0)", allowed, remaining)
	}

	// After a gap of two or more full windows all history is discarded and the
	// full limit is available again.
	if allowed, remaining, _ := l.allow(limit, window, t0.Add(4*window)); !allowed || remaining != 1 {
		t.Fatalf("request after long idle gap: got (allowed=%v, remaining=%d), want (true, 1)", allowed, remaining)
	}
}

// TestUberRateLimiterAllowNonPositiveLimit verifies that a zero or negative
// limit denies every request immediately and reports the window as the reset.
func TestUberRateLimiterAllowNonPositiveLimit(t *testing.T) {
	limiter := NewUberRateLimiter()

	for _, limit := range []int{0, -1} {
		allowed, remaining, reset := limiter.Allow("nonpositive", limit, time.Minute)
		if allowed {
			t.Errorf("limit %d: request should be denied", limit)
		}
		if remaining != 0 {
			t.Errorf("limit %d: expected 0 remaining, got %d", limit, remaining)
		}
		if reset != time.Minute {
			t.Errorf("limit %d: expected reset equal to window, got %v", limit, reset)
		}
	}
}

// TestUberRateLimiterAllowNonPositiveWindowDefaultsToOneSecond verifies that a
// zero (or negative) window falls back to a one-second window instead of
// producing a degenerate limiter.
func TestUberRateLimiterAllowNonPositiveWindowDefaultsToOneSecond(t *testing.T) {
	limiter := NewUberRateLimiter()

	allowed, remaining, _ := limiter.Allow("zero-window", 1, 0)
	if !allowed || remaining != 0 {
		t.Fatalf("first request: got (allowed=%v, remaining=%d), want (true, 0)", allowed, remaining)
	}

	// The second request must be denied, and the reset must reflect the
	// defaulted one-second window.
	allowed, _, reset := limiter.Allow("zero-window", 1, 0)
	if allowed {
		t.Fatal("second request should be denied within the defaulted window")
	}
	if reset <= 0 || reset > time.Second {
		t.Errorf("expected reset within (0, 1s] for the defaulted window, got %v", reset)
	}
}
