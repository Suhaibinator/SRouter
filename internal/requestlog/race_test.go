//go:build race

package requestlog

// raceEnabled reports whether the race detector, which makes sync.Pool drop
// items at random, is compiled in.
const raceEnabled = true
