package core

import (
	"sync"
)

// Clock implements a Lamport logical clock
// It provides monotonically increasing timestamps for causality tracking
type Clock struct {
	mu   sync.Mutex
	time uint64
}

// NewClock creates a new Lamport clock starting at 0
func NewClock() *Clock {
	return &Clock{time: 0}
}

// NewClockWithTime creates a new Lamport clock with an initial time
// Useful for restoring clock state from persistent storage
func NewClockWithTime(initialTime uint64) *Clock {
	return &Clock{time: initialTime}
}

// Tick increments the clock and returns the new time
// Must be called before every local mutation
func (c *Clock) Tick() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.time++
	return c.time
}

// Update merges with a remote timestamp
// Sets local time to max(local, remote) + 1
// Must be called when receiving remote state
func (c *Clock) Update(remoteTime uint64) uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	if remoteTime > c.time {
		c.time = remoteTime
	}
	c.time++
	return c.time
}

// Now returns the current clock time without incrementing
func (c *Clock) Now() uint64 {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.time
}
