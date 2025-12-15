package middleware

import (
	"sync"
	"testing"
	"time"
)

// TestIDGeneratorStopDuringBatchFill ensures Stop terminates the generator when batching.
func TestIDGeneratorStopDuringBatchFill(t *testing.T) {
	bufferSize := 50
	g := NewIDGenerator(bufferSize)
	// Drain the buffer to force batch refill
	for range bufferSize {
		_ = g.GetIDNonBlocking()
	}

	time.Sleep(1 * time.Millisecond) // allow background filler to start batching
	g.Stop()
	g.Stop() // second call should be safe
}

// TestIDGeneratorStopDuringNormalFill ensures Stop works during normal generation.
// This covers the `case <-g.stop: return` in the normal fill select.
func TestIDGeneratorStopDuringNormalFill(t *testing.T) {
	// Use a larger buffer so normal fill runs longer (batch mode won't trigger)
	// With size 50, emptyThreshold = 5, so we need currentLen < 5 for batch mode
	// If we keep channel above 5 items, we stay in normal fill mode
	g := NewIDGenerator(50)

	// Wait for initial fill
	time.Sleep(50 * time.Millisecond)

	// Start draining to keep the worker active in normal fill mode
	done := make(chan struct{})
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for {
			select {
			case <-done:
				return
			default:
				_ = g.GetIDNonBlocking()
				// Keep some items in buffer so we don't trigger batch mode
				if len(g.idChan) < 40 {
					time.Sleep(100 * time.Microsecond)
				}
			}
		}
	}()

	// Let the drain run briefly
	time.Sleep(5 * time.Millisecond)

	// Stop while normal fill is running
	g.Stop()

	close(done)
	wg.Wait()

	// Second stop should be safe
	g.Stop()
}

// TestIDGeneratorStopWhileBatchLoopActive tests stopping during active batch insertion.
// This specifically covers the `case <-g.stop: return` inside the batch fill inner loop.
func TestIDGeneratorStopWhileBatchLoopActive(t *testing.T) {
	// Use a larger buffer with low threshold to keep batch loop running longer
	bufferSize := 100
	g := NewIDGenerator(bufferSize)

	// Wait for initial fill
	time.Sleep(50 * time.Millisecond)

	// Use multiple goroutines to aggressively drain and keep batch fill active
	done := make(chan struct{})
	var wg sync.WaitGroup

	// Multiple consumers to drain aggressively
	for range 5 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for {
				select {
				case <-done:
					return
				default:
					_ = g.GetIDNonBlocking()
				}
			}
		}()
	}

	// Let consumers drain aggressively to trigger and maintain batch fill mode
	time.Sleep(10 * time.Millisecond)

	// Stop while batch loop is actively running
	g.Stop()

	// Signal drain goroutines to stop
	close(done)
	wg.Wait()
}

// TestIDGeneratorBatchFillChannelFull tests the default case in batch fill loop.
// This covers the `default` branch that exits batch insertion when the channel fills.
func TestIDGeneratorBatchFillChannelFull(t *testing.T) {
	// Use a very small buffer - the batch fill precomputes 1000 UUIDs,
	// so a small buffer will fill up quickly and trigger the default case
	bufferSize := 10
	g := NewIDGenerator(bufferSize)

	// Wait for initial fill
	time.Sleep(50 * time.Millisecond)

	// Drain buffer rapidly to trigger batch fill mode
	// (channel must be below 10% threshold AND decreasing)
	for range bufferSize {
		_ = g.GetIDNonBlocking()
	}

	// Give the batch filler time to:
	// 1. Detect the depletion (currentLen < emptyThreshold && lastChannelLen > currentLen)
	// 2. Start batch fill
	// 3. Hit the default case when channel fills up (buffer is only 10, batch has 1000)
	time.Sleep(10 * time.Millisecond)

	// Verify buffer is refilled (proves batch fill ran and hit the full condition)
	finalLen := len(g.idChan)
	if finalLen != bufferSize {
		t.Errorf("Expected buffer to be full (%d), got %d", bufferSize, finalLen)
	}

	g.Stop()
}
