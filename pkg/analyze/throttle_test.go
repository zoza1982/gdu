package analyze

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func init() {
	// Quiet down logs during tests
}

// TestIOThrottle_Disabled verifies that nil throttle doesn't block operations
func TestIOThrottle_Disabled(t *testing.T) {
	// Test 1: NewIOThrottle returns nil when both params are 0
	throttle := NewIOThrottle(0, 0)
	assert.Nil(t, throttle, "Expected nil throttle when maxIOPS=0 and ioDelay=0")

	// Test 2: Acquire on nil throttle returns immediately
	start := time.Now()
	err := throttle.Acquire(context.Background())
	elapsed := time.Since(start)

	assert.Nil(t, err, "Expected no error from nil throttle")
	assert.Less(t, elapsed, 1*time.Millisecond, "Nil throttle should not block")

	// Test 3: IsEnabled returns false for nil throttle
	if throttle != nil {
		assert.False(t, throttle.IsEnabled(), "Nil throttle should not be enabled")
	}
}

// TestIOThrottle_IOPSLimit tests IOPS rate limiting accuracy
func TestIOThrottle_IOPSLimit(t *testing.T) {
	tests := []struct {
		name         string
		maxIOPS      int
		numOps       int
		expectedMin  time.Duration
		expectedMax  time.Duration
		description  string
	}{
		{
			name:        "100 IOPS burst allowed",
			maxIOPS:     100,
			numOps:      100, // Exactly the burst size
			expectedMin: 0 * time.Millisecond,
			expectedMax: 200 * time.Millisecond,
			description: "Should complete immediately using burst allowance",
		},
		{
			name:        "100 IOPS rate limiting",
			maxIOPS:     100,
			numOps:      150, // Beyond burst, will rate limit
			expectedMin: 400 * time.Millisecond, // 50 extra ops at 100/sec = 500ms, minus overhead
			expectedMax: 800 * time.Millisecond,
			description: "Should rate limit after consuming burst",
		},
		{
			name:        "1000 IOPS rate limiting",
			maxIOPS:     1000,
			numOps:      1100, // 100 extra ops
			expectedMin: 50 * time.Millisecond, // 100 ops at 1000/sec = 100ms
			expectedMax: 250 * time.Millisecond,
			description: "Should rate limit at 1000 IOPS",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			throttle := NewIOThrottle(tt.maxIOPS, 0)
			assert.NotNil(t, throttle, "Expected non-nil throttle")
			assert.True(t, throttle.IsEnabled(), "Throttle should be enabled")

			// Reset to clear any initial burst allowance
			throttle.Reset()

			start := time.Now()
			ctx := context.Background()

			for i := 0; i < tt.numOps; i++ {
				err := throttle.Acquire(ctx)
				assert.Nil(t, err, "Acquire should not error")
			}

			elapsed := time.Since(start)

			// Verify timing is within expected range
			assert.GreaterOrEqual(t, elapsed, tt.expectedMin,
				"%s: took %v, expected >= %v",
				tt.description, elapsed, tt.expectedMin)
			assert.Less(t, elapsed, tt.expectedMax,
				"%s: took %v, expected < %v",
				tt.description, elapsed, tt.expectedMax)
		})
	}
}

// TestIOThrottle_FixedDelay tests fixed delay timing accuracy
func TestIOThrottle_FixedDelay(t *testing.T) {
	tests := []struct {
		name         string
		ioDelay      time.Duration
		numOps       int
		expectedTime time.Duration
		tolerance    time.Duration
	}{
		{
			name:         "10ms delay",
			ioDelay:      10 * time.Millisecond,
			numOps:       10,
			expectedTime: 100 * time.Millisecond,
			tolerance:    50 * time.Millisecond,
		},
		{
			name:         "50ms delay",
			ioDelay:      50 * time.Millisecond,
			numOps:       5,
			expectedTime: 250 * time.Millisecond,
			tolerance:    100 * time.Millisecond,
		},
		{
			name:         "1ms delay",
			ioDelay:      1 * time.Millisecond,
			numOps:       20,
			expectedTime: 20 * time.Millisecond,
			tolerance:    20 * time.Millisecond,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			throttle := NewIOThrottle(0, tt.ioDelay)
			assert.NotNil(t, throttle, "Expected non-nil throttle")
			assert.True(t, throttle.IsEnabled(), "Throttle should be enabled")

			start := time.Now()
			ctx := context.Background()

			for i := 0; i < tt.numOps; i++ {
				err := throttle.Acquire(ctx)
				assert.Nil(t, err, "Acquire should not error")
			}

			elapsed := time.Since(start)

			// Verify timing is within tolerance
			diff := elapsed - tt.expectedTime
			if diff < 0 {
				diff = -diff
			}
			assert.Less(t, diff, tt.tolerance,
				"Expected ~%v, got %v (diff: %v, tolerance: %v)",
				tt.expectedTime, elapsed, diff, tt.tolerance)
		})
	}
}

// TestIOThrottle_Concurrent tests that multiple goroutines respect shared limit
func TestIOThrottle_Concurrent(t *testing.T) {
	const (
		maxIOPS     = 100
		numWorkers  = 10
		opsPerWorker = 20
		totalOps    = numWorkers * opsPerWorker
	)

	throttle := NewIOThrottle(maxIOPS, 0)
	assert.NotNil(t, throttle, "Expected non-nil throttle")
	throttle.Reset()

	var (
		opsCompleted atomic.Int32
		wg           sync.WaitGroup
	)

	start := time.Now()

	// Launch concurrent workers
	for i := 0; i < numWorkers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			ctx := context.Background()

			for j := 0; j < opsPerWorker; j++ {
				err := throttle.Acquire(ctx)
				assert.Nil(t, err, "Acquire should not error")
				opsCompleted.Add(1)
			}
		}()
	}

	wg.Wait()
	elapsed := time.Since(start)

	// Verify all operations completed
	assert.Equal(t, int32(totalOps), opsCompleted.Load(),
		"All operations should complete")

	// Verify rate limiting occurred
	// With 200 ops at 100 IOPS, should take ~2 seconds (minus initial burst)
	// Allow burst of 100, so 100 more ops should take ~1 second
	expectedMin := 800 * time.Millisecond
	expectedMax := 3 * time.Second
	assert.GreaterOrEqual(t, elapsed, expectedMin,
		"Concurrent operations should be rate limited (took %v, expected >= %v)",
		elapsed, expectedMin)
	assert.Less(t, elapsed, expectedMax,
		"Concurrent operations should not take too long (took %v, expected < %v)",
		elapsed, expectedMax)
}

// TestIOThrottle_Both tests IOPS limit + fixed delay work together
func TestIOThrottle_Both(t *testing.T) {
	const (
		maxIOPS = 100
		ioDelay = 10 * time.Millisecond
		numOps  = 50
	)

	throttle := NewIOThrottle(maxIOPS, ioDelay)
	assert.NotNil(t, throttle, "Expected non-nil throttle")
	assert.True(t, throttle.IsEnabled(), "Throttle should be enabled")
	throttle.Reset()

	start := time.Now()
	ctx := context.Background()

	for i := 0; i < numOps; i++ {
		err := throttle.Acquire(ctx)
		assert.Nil(t, err, "Acquire should not error")
	}

	elapsed := time.Since(start)

	// With both IOPS and delay:
	// - First 100 ops can burst (due to burst allowance)
	// - But each op also has 10ms delay
	// - So 50 ops = 50 * 10ms = 500ms minimum from delay
	// - IOPS limiting adds additional time after burst consumed
	expectedMin := 400 * time.Millisecond // Account for concurrent timer/limiter
	expectedMax := 2 * time.Second

	assert.GreaterOrEqual(t, elapsed, expectedMin,
		"Both throttles should apply (took %v, expected >= %v)",
		elapsed, expectedMin)
	assert.Less(t, elapsed, expectedMax,
		"Should not take too long (took %v, expected < %v)",
		elapsed, expectedMax)
}

// TestIOThrottle_Reset tests that reset clears accumulated tokens
func TestIOThrottle_Reset(t *testing.T) {
	const maxIOPS = 100

	throttle := NewIOThrottle(maxIOPS, 0)
	assert.NotNil(t, throttle, "Expected non-nil throttle")

	// Test 1: Reset creates a fresh limiter with full burst capacity
	throttle.Reset()
	ctx := context.Background()

	// Should be able to acquire burst capacity immediately
	start := time.Now()
	for i := 0; i < maxIOPS; i++ {
		err := throttle.Acquire(ctx)
		assert.Nil(t, err, "Acquire should not error")
	}
	elapsed := time.Since(start)
	assert.Less(t, elapsed, 100*time.Millisecond,
		"Burst capacity should be available immediately after reset (took %v)", elapsed)

	// Test 2: Verify Reset() doesn't panic or deadlock
	throttle.Reset()
	throttle.Reset()
	throttle.Reset()

	// Should still work after multiple resets
	err := throttle.Acquire(ctx)
	assert.Nil(t, err, "Acquire should work after multiple resets")
}

// TestIOThrottle_ContextCancellation tests context cancellation during throttling
func TestIOThrottle_ContextCancellation(t *testing.T) {
	t.Run("Cancel during IOPS wait", func(t *testing.T) {
		// Test that Acquire respects context cancellation
		throttle := NewIOThrottle(10, 0)
		throttle.Reset()

		// Create pre-cancelled context
		ctx, cancel := context.WithCancel(context.Background())
		cancel() // Cancel immediately

		// Acquire should fail quickly with context error
		start := time.Now()
		err := throttle.Acquire(ctx)
		elapsed := time.Since(start)

		assert.NotNil(t, err, "Expected context error")
		assert.Contains(t, err.Error(), "context",
			"Expected context error, got: %v", err)
		assert.Less(t, elapsed, 50*time.Millisecond,
			"Should return immediately when context is cancelled (took %v)", elapsed)
	})

	t.Run("Cancel during delay", func(t *testing.T) {
		throttle := NewIOThrottle(0, 200*time.Millisecond)

		// Create context that cancels during delay
		ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
		defer cancel()

		start := time.Now()
		err := throttle.Acquire(ctx)
		elapsed := time.Since(start)

		assert.NotNil(t, err, "Expected context error")
		assert.Equal(t, context.DeadlineExceeded, err, "Expected deadline exceeded")
		assert.GreaterOrEqual(t, elapsed, 40*time.Millisecond,
			"Should wait until context timeout (took %v)", elapsed)
		assert.Less(t, elapsed, 150*time.Millisecond,
			"Should cancel before full delay (took %v)", elapsed)
	})

	t.Run("Context respected across multiple calls", func(t *testing.T) {
		// Verify that context cancellation works properly during rate limiting
		throttle := NewIOThrottle(100, 0)
		throttle.Reset()

		ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
		defer cancel()

		// Make many calls - some will succeed, some will fail when context expires
		var successCount, errorCount int
		for i := 0; i < 200; i++ {
			err := throttle.Acquire(ctx)
			if err != nil {
				errorCount++
			} else {
				successCount++
			}
		}

		// Should have some successes (burst + rate limited) and some errors (after timeout)
		assert.Greater(t, successCount, 0, "Should have some successful operations")
		assert.Greater(t, errorCount, 0, "Should have some failed operations due to timeout")
	})
}

// TestIOThrottle_ConcurrentReset tests thread safety of Reset
func TestIOThrottle_ConcurrentReset(t *testing.T) {
	throttle := NewIOThrottle(1000, 0)
	assert.NotNil(t, throttle)

	var wg sync.WaitGroup
	ctx := context.Background()

	// Start workers doing Acquire
	for i := 0; i < 10; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < 100; j++ {
				err := throttle.Acquire(ctx)
				assert.Nil(t, err)
			}
		}()
	}

	// Concurrently call Reset
	wg.Add(1)
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			throttle.Reset()
			time.Sleep(10 * time.Millisecond)
		}
	}()

	// Should not deadlock or panic
	wg.Wait()
}

// BenchmarkIOThrottle_NoThrottle measures baseline performance (nil throttle)
func BenchmarkIOThrottle_NoThrottle(b *testing.B) {
	var throttle *IOThrottle
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = throttle.Acquire(ctx)
	}
}

// BenchmarkIOThrottle_IOPSOnly measures overhead of IOPS limiting
func BenchmarkIOThrottle_IOPSOnly(b *testing.B) {
	tests := []struct {
		name    string
		maxIOPS int
	}{
		{"100_IOPS", 100},
		{"1000_IOPS", 1000},
		{"10000_IOPS", 10000},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			throttle := NewIOThrottle(tt.maxIOPS, 0)
			throttle.Reset()
			ctx := context.Background()

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = throttle.Acquire(ctx)
			}
		})
	}
}

// BenchmarkIOThrottle_DelayOnly measures overhead of fixed delay
func BenchmarkIOThrottle_DelayOnly(b *testing.B) {
	tests := []struct {
		name    string
		ioDelay time.Duration
	}{
		{"1ms_delay", 1 * time.Millisecond},
		{"10ms_delay", 10 * time.Millisecond},
		{"50ms_delay", 50 * time.Millisecond},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			throttle := NewIOThrottle(0, tt.ioDelay)
			ctx := context.Background()

			b.ResetTimer()
			for i := 0; i < b.N; i++ {
				_ = throttle.Acquire(ctx)
			}
		})
	}
}

// BenchmarkIOThrottle_Combined measures overhead of both throttles
func BenchmarkIOThrottle_Combined(b *testing.B) {
	throttle := NewIOThrottle(1000, 5*time.Millisecond)
	throttle.Reset()
	ctx := context.Background()

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_ = throttle.Acquire(ctx)
	}
}

// BenchmarkIOThrottle_Concurrent measures concurrent performance
func BenchmarkIOThrottle_Concurrent(b *testing.B) {
	tests := []struct {
		name       string
		maxIOPS    int
		numWorkers int
	}{
		{"100_IOPS_10_workers", 100, 10},
		{"1000_IOPS_10_workers", 1000, 10},
		{"1000_IOPS_100_workers", 1000, 100},
	}

	for _, tt := range tests {
		b.Run(tt.name, func(b *testing.B) {
			throttle := NewIOThrottle(tt.maxIOPS, 0)
			throttle.Reset()
			ctx := context.Background()

			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					_ = throttle.Acquire(ctx)
				}
			})
		})
	}
}

// BenchmarkIOThrottle_Reset measures Reset performance
func BenchmarkIOThrottle_Reset(b *testing.B) {
	throttle := NewIOThrottle(1000, 0)

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		throttle.Reset()
	}
}
