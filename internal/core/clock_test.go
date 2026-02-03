package core

import (
	"sync"
	"testing"
)

func TestNewClock(t *testing.T) {
	c := NewClock()
	if c.Now() != 0 {
		t.Errorf("expected new clock to be at 0, got %d", c.Now())
	}
}

func TestNewClockWithTime(t *testing.T) {
	c := NewClockWithTime(100)
	if c.Now() != 100 {
		t.Errorf("expected clock to be at 100, got %d", c.Now())
	}
}

func TestTick(t *testing.T) {
	c := NewClock()
	
	t1 := c.Tick()
	if t1 != 1 {
		t.Errorf("expected first tick to be 1, got %d", t1)
	}
	
	t2 := c.Tick()
	if t2 != 2 {
		t.Errorf("expected second tick to be 2, got %d", t2)
	}
	
	if c.Now() != 2 {
		t.Errorf("expected current time to be 2, got %d", c.Now())
	}
}

func TestUpdate(t *testing.T) {
	tests := []struct {
		name       string
		localTime  uint64
		remoteTime uint64
		expected   uint64
	}{
		{
			name:       "remote is higher",
			localTime:  5,
			remoteTime: 10,
			expected:   11, // max(5, 10) + 1
		},
		{
			name:       "local is higher",
			localTime:  15,
			remoteTime: 10,
			expected:   16, // max(15, 10) + 1
		},
		{
			name:       "equal times",
			localTime:  10,
			remoteTime: 10,
			expected:   11, // max(10, 10) + 1
		},
	}
	
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			c := NewClockWithTime(tt.localTime)
			result := c.Update(tt.remoteTime)
			if result != tt.expected {
				t.Errorf("expected %d, got %d", tt.expected, result)
			}
		})
	}
}

func TestClockConcurrency(t *testing.T) {
	c := NewClock()
	var wg sync.WaitGroup
	numGoroutines := 100
	ticksPerGoroutine := 100
	
	wg.Add(numGoroutines)
	for i := 0; i < numGoroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < ticksPerGoroutine; j++ {
				c.Tick()
			}
		}()
	}
	wg.Wait()
	
	expected := uint64(numGoroutines * ticksPerGoroutine)
	if c.Now() != expected {
		t.Errorf("expected clock to be at %d after concurrent ticks, got %d", expected, c.Now())
	}
}

func TestClockMonotonicity(t *testing.T) {
	c := NewClock()
	var prev uint64 = 0
	
	for i := 0; i < 1000; i++ {
		curr := c.Tick()
		if curr <= prev {
			t.Errorf("clock is not monotonic: prev=%d, curr=%d", prev, curr)
		}
		prev = curr
	}
}
