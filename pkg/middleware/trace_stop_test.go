package middleware

import (
	"testing"
	"time"
)

// TestIDGeneratorStopDuringBatchFill ensures Stop terminates the generator when batching.
func TestIDGeneratorStopDuringBatchFill(t *testing.T) {
	bufferSize := 50
	g := NewIDGenerator(bufferSize)
	// Drain the buffer to force batch refill
	for i := 0; i < bufferSize; i++ {
		_ = g.GetIDNonBlocking()
	}

	time.Sleep(1 * time.Millisecond) // allow background filler to start batching
	g.Stop()
	g.Stop() // second call should be safe
}

// TestIDGeneratorStopDuringNormalFill ensures Stop works during normal generation.
func TestIDGeneratorStopDuringNormalFill(t *testing.T) {
	g := NewIDGenerator(1)
	_ = g.GetIDNonBlocking() // drain so filler runs normally
	time.Sleep(1 * time.Millisecond)
	g.Stop()
	g.Stop()
}
