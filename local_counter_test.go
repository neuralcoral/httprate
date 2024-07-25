package httprate

import (
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"golang.org/x/sync/errgroup"
)

func TestLocalCounter(t *testing.T) {
	limitCounter := &localCounter{
		latestWindow:     time.Now().UTC().Truncate(time.Second),
		latestCounters:   make(map[uint64]int),
		previousCounters: make(map[uint64]int),
		windowLength:     time.Second,
	}

	currentWindow := time.Now().UTC().Truncate(time.Second)
	previousWindow := currentWindow.Add(-time.Second)

	type test struct {
		name        string        // In each test do the following:
		advanceTime time.Duration // 1. advance time
		incrBy      int           // 2. increase counter
		prev        int           // 3. check previous window counter
		curr        int           //    and current window counter
	}

	tests := []test{
		{
			name: "t=0s: init",
			prev: 0,
			curr: 0,
		},
		{
			name:   "t=0s: increment 1",
			incrBy: 1,
			prev:   0,
			curr:   1,
		},
		{
			name:   "t=0s: increment by 99",
			incrBy: 99,
			prev:   0,
			curr:   100,
		},
		{
			name:        "t=1s: move clock by 1s",
			advanceTime: time.Second,
			prev:        100,
			curr:        0,
		},
		{
			name:   "t=1s: increment by 20",
			incrBy: 20,
			prev:   100,
			curr:   20,
		},
		{
			name:   "t=1s: increment by 20",
			incrBy: 20,
			prev:   100,
			curr:   40,
		},
		{
			name:        "t=2s: move clock by 1s",
			advanceTime: time.Second,
			prev:        40,
			curr:        0,
		},
		{
			name:   "t=2s: incr++",
			incrBy: 1,
			prev:   40,
			curr:   1,
		},
		{
			name:   "t=2s: incr+=9",
			incrBy: 9,
			prev:   40,
			curr:   10,
		},
		{
			name:   "t=2s: incr+=20",
			incrBy: 20,
			prev:   40,
			curr:   30,
		},
		{
			name:        "t=4s: move clock by 2s",
			advanceTime: 2 * time.Second,
			prev:        0,
			curr:        0,
		},
	}

	concurrentRequests := 1000

	for _, tt := range tests {
		if tt.advanceTime > 0 {
			currentWindow = currentWindow.Add(tt.advanceTime)
			previousWindow = previousWindow.Add(tt.advanceTime)
		}

		if tt.incrBy > 0 {
			var g errgroup.Group
			for i := 0; i < concurrentRequests; i++ {
				i := i
				g.Go(func() error {
					key := fmt.Sprintf("key:%v", i)
					return limitCounter.IncrementBy(key, currentWindow, tt.incrBy)
				})
			}
			if err := g.Wait(); err != nil {
				t.Errorf("%s: %v", tt.name, err)
			}
		}

		var g errgroup.Group
		for i := 0; i < concurrentRequests; i++ {
			i := i
			g.Go(func() error {
				key := fmt.Sprintf("key:%v", i)
				curr, prev, err := limitCounter.Get(key, currentWindow, previousWindow)
				if err != nil {
					return fmt.Errorf("%q: %w", key, err)
				}
				if curr != tt.curr {
					return fmt.Errorf("%q: unexpected curr = %v, expected %v", key, curr, tt.curr)
				}
				if prev != tt.prev {
					return fmt.Errorf("%q: unexpected prev = %v, expected %v", key, prev, tt.prev)
				}
				return nil
			})
		}
		if err := g.Wait(); err != nil {
			t.Errorf("%s: %v", tt.name, err)
		}
	}
}

func BenchmarkLocalCounter(b *testing.B) {
	limitCounter := &localCounter{
		latestWindow:     time.Now().UTC().Truncate(time.Second),
		latestCounters:   make(map[uint64]int),
		previousCounters: make(map[uint64]int),
		windowLength:     time.Second,
	}

	currentWindow := time.Now().UTC().Truncate(time.Second)
	previousWindow := currentWindow.Add(-time.Second)

	b.ResetTimer()

	for i := 0; i < b.N; i++ {
		for i := range []int{0, 1, 0, 0, 0, 0, 0, 1, 0, 0, 0, 1, 0, 0, 0, 3, 0, 0, 0, 0, 1, 0} {
			// Simulate time.
			currentWindow.Add(time.Duration(i) * time.Second)
			previousWindow.Add(time.Duration(i) * time.Second)

			wg := sync.WaitGroup{}
			wg.Add(1000)
			for i := 0; i < 1000; i++ {
				// Simulate concurrent requests with different rate-limit keys.
				go func(i int) {
					defer wg.Done()

					_, _, _ = limitCounter.Get(fmt.Sprintf("key-%v", i), currentWindow, previousWindow)
					_ = limitCounter.IncrementBy(fmt.Sprintf("key-%v", i), currentWindow, rand.Intn(100))
				}(i)
			}
			wg.Wait()
		}
	}
}