// Package middleware provides a collection of HTTP middleware components for the SRouter framework.
package middleware

import (
	"context"
	"encoding/hex"
	"net/http"
	"sync"
	"time"

	"github.com/Suhaibinator/SRouter/pkg/common"
	"github.com/google/uuid"
)

// IDGenerator provides efficient generation of trace IDs by precomputing them
type IDGenerator struct {
	idChan   chan string
	size     int
	initOnce sync.Once
}

// NewIDGenerator creates a new IDGenerator with the specified buffer size
func NewIDGenerator(bufferSize int) *IDGenerator {
	g := &IDGenerator{
		idChan: make(chan string, bufferSize),
		size:   bufferSize,
	}
	g.init()
	return g
}

// init starts the background goroutine that fills the channel with UUIDs
func (g *IDGenerator) init() {
	g.initOnce.Do(func() {
		// First fill the channel
		for range g.size {
			g.idChan <- generateUUID()
		}

		// Then start the background worker to keep it filled
		go func() {
			// Pre-allocate a batch of UUIDs to insert quickly when needed
			const batchSize = 1000
			batchUUIDs := make([]string, 0, batchSize)

			// Used to determine if we need to batch-fill when channel is getting empty
			lastChannelLen := g.size
			emptyThreshold := g.size / 10 // 10% capacity threshold to trigger batch fill

			for {
				// Get current channel capacity
				currentLen := len(g.idChan)

				// If the channel is getting depleted quickly (below threshold),
				// batch-fill it immediately with multiple UUIDs
				if currentLen < emptyThreshold && lastChannelLen > currentLen {
					// Channel is being consumed quickly, pre-generate a batch
					if len(batchUUIDs) == 0 {
						// Refill our batch
						batchUUIDs = batchUUIDs[:0] // Clear without deallocating
						for range batchSize {
							batchUUIDs = append(batchUUIDs, generateUUID())
						}
					}

					// Add from our batch as many as we can without blocking
					for len(batchUUIDs) > 0 {
						select {
						case g.idChan <- batchUUIDs[0]:
							// Successfully added one from batch
							batchUUIDs = batchUUIDs[1:]
						default:
							// Channel is now full, stop adding
							// No need for break here as it would only exit the select, not the for loop
						}
						// Break out of the for loop if the channel is full
						if len(g.idChan) == g.size {
							break
						}
					}

					// Very short sleep to prevent CPU thrashing but still be responsive
					time.Sleep(100 * time.Microsecond) // 100μs instead of 10ms
				} else {
					// Normal case: channel has plenty of capacity, add one at a time
					select {
					case g.idChan <- generateUUID():
						// Successfully added a new UUID
					default:
						// Channel is full, sleep longer to save CPU
						time.Sleep(1 * time.Millisecond) // 1ms instead of 10ms
					}
				}

				// Update our last seen channel length
				lastChannelLen = currentLen
			}
		}()
	})
}

func generateUUID() string {
	// Generate a new UUID and return it as a string
	id := uuid.New()
	return hex.EncodeToString(id[:])
}

// GetID returns a precomputed UUID from the channel.
// This is significantly more efficient than generating UUIDs on-demand
// since the generation happens in a background goroutine and is batched.
// The channel acts as a buffer, ensuring there's always a pool of IDs ready.
// This method will block if the channel is empty until a UUID becomes available.
func (g *IDGenerator) GetID() string {
	return <-g.idChan
}

// GetIDNonBlocking attempts to get a precomputed UUID from the channel without blocking.
// If the channel is empty (which should be rare with proper sizing), it will
// generate a new UUID on the spot rather than waiting.
// This ensures requests are never delayed even during extreme traffic spikes.
func (g *IDGenerator) GetIDNonBlocking() string {
	select {
	case id := <-g.idChan:
		return id
	default:
		// Channel is empty, generate a new UUID on the spot as fallback
		return generateUUID()
	}
}

// WithTraceID adds a trace ID to the SRouterContext in the provided context.
// If no SRouterContext exists, one will be created.
//
// This function is part of the SRouterContext approach for storing values in the context,
// which avoids deep nesting of context values by using a single wrapper structure.
func WithTraceID[T comparable, U any](ctx context.Context, traceID string) context.Context {
	// Get or create the router context
	rc, ok := GetSRouterContext[T, U](ctx)
	if !ok {
		rc = &SRouterContext[T, U]{
			Flags: make(map[string]bool),
		}
	}

	// Store the trace ID in the flags
	rc.TraceID = traceID
	rc.TraceIDSet = true

	// Update the context
	return context.WithValue(ctx, sRouterContextKey{}, rc)
}

// GetTraceIDFromContext extracts the trace ID from a context.
// It first tries to find the trace ID in the SRouterContext using the flags map,
// and falls back to the legacy context key approach for backward compatibility.
// Returns an empty string if no trace ID is found.
func GetTraceIDFromContext(ctx context.Context) string {
	// Try the new way first
	rc, ok := GetSRouterContext[string, any](ctx)
	if !ok {
		return ""
	}

	// Check if the trace ID is set in the SRouterContext
	if rc.TraceIDSet {
		return rc.TraceID
	}
	return ""
}

// GetTraceID extracts the trace ID from the request context.
// Returns an empty string if no trace ID is found.
func GetTraceID(r *http.Request) string {
	return GetTraceIDFromContext(r.Context())
}

// AddTraceIDToRequest adds a trace ID to the request context.
// This is useful for testing or for manually setting a trace ID.
func AddTraceIDToRequest(r *http.Request, traceID string) *http.Request {
	ctx := WithTraceID[string, any](r.Context(), traceID)
	return r.WithContext(ctx)
}

// CreateTraceMiddleware creates a trace middleware with the provided ID generator.
// This is the core implementation used by both traceMiddleware and traceMiddlewareWithConfig.
// It checks for an existing trace ID in the request headers before generating a new one,
// which allows for trace ID propagation across service calls.
func CreateTraceMiddleware(generator *IDGenerator) common.Middleware {
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var traceID string

			// Check if there's already a trace ID in the request headers
			existingTraceID := r.Header.Get("X-Trace-ID")
			if existingTraceID != "" {
				// Use the existing trace ID for propagation
				traceID = existingTraceID
			} else {
				// Generate a new trace ID if none exists
				traceID = generator.GetIDNonBlocking()
			}

			// Add the trace ID to the request context using the new wrapper
			ctx := WithTraceID[string, any](r.Context(), traceID)

			// Add trace ID to the headers for logging or tracing
			w.Header().Set("X-Trace-ID", traceID)

			// Call the next handler with the updated request
			next.ServeHTTP(w, r.WithContext(ctx))
		})
	}
}
