package traceid

import (
	"encoding/hex"
	"fmt"
	"sync"
	"uuid"
)

// Generate returns a UUIDv7 as 32 lowercase hexadecimal characters.
func Generate() string {
	id := uuid.NewV7()
	return hex.EncodeToString(id[:])
}

// Generator supplies UUIDv7 IDs safely to concurrent callers. Its zero value
// generates synchronously. A Generator must not be copied after first use.
type Generator struct {
	ids      chan string
	stop     chan struct{}
	done     chan struct{}
	stopOnce sync.Once
}

// NewGenerator creates a generator. Zero generates synchronously; positive
// sizes prefill a buffer and start a background worker. Negative sizes fail.
func NewGenerator(bufferSize int) (*Generator, error) {
	if bufferSize < 0 {
		return nil, fmt.Errorf("trace ID buffer size must not be negative")
	}
	g := &Generator{}
	if bufferSize == 0 {
		return g, nil
	}
	g.ids = make(chan string, bufferSize)
	g.stop = make(chan struct{})
	g.done = make(chan struct{})
	for range bufferSize {
		g.ids <- Generate()
	}
	go func() {
		defer close(g.done)
		for {
			select {
			case <-g.stop:
				return
			case g.ids <- Generate():
			}
		}
	}()
	return g, nil
}

// Next returns a buffered ID or generates synchronously when the buffer is
// empty. It never waits for the worker and remains usable after Stop.
func (g *Generator) Next() string {
	select {
	case id := <-g.ids:
		return id
	default:
		return Generate()
	}
}

// Stop waits for the background worker to exit. It is safe to call repeatedly
// and concurrently with Next or Stop, including for a synchronous generator.
func (g *Generator) Stop() {
	if g.stop == nil {
		return
	}
	g.stopOnce.Do(func() { close(g.stop) })
	<-g.done
}
