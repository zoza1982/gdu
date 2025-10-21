package analyze

import (
	"context"
	"sync"
	"time"

	"golang.org/x/time/rate"
)

// IOThrottle limits I/O operations to prevent storage overload.
//
// Design: Token Bucket Rate Limiting
// ------------------------------------
// This implementation uses golang.org/x/time/rate for IOPS limiting and optionally
// adds a fixed delay between operations. The design supports two throttling modes:
//
// 1. IOPS Limiting (--max-iops):
//   - Uses token bucket algorithm via rate.Limiter
//   - Limits directory scans to N operations per second
//   - Thread-safe across multiple goroutines (rate.Limiter handles synchronization)
//   - Burst allowance = maxIOPS (allows short bursts up to the limit)
//
// 2. Fixed Delay (--io-delay):
//   - Adds constant delay between each operation
//   - Simpler but less flexible than IOPS limiting
//   - Can be combined with IOPS limiting for maximum protection
//
// Integration Points:
// -------------------
// - Added to IncrementalAnalyzer struct as optional field
// - Called in processDir() before os.ReadDir() operations
// - Nil throttle = no throttling (zero overhead)
//
// Thread Safety:
// --------------
// - rate.Limiter is internally synchronized (thread-safe)
// - No additional locking needed for basic operations
// - Mutex protects Reset() to prevent race during limiter recreation
//
// Context Support:
// ----------------
// - Acquire() respects context cancellation
// - Allows graceful shutdown during throttled operations
// - Returns context.Err() if cancelled
type IOThrottle struct {
	maxIOPS int           // Maximum I/O operations per second (0 = unlimited)
	ioDelay time.Duration // Fixed delay between operations (0 = no delay)
	limiter *rate.Limiter // Token bucket rate limiter (nil if maxIOPS=0)
	mu      sync.Mutex    // Protects limiter recreation in Reset()
}

// NewIOThrottle creates a throttle with IOPS limit and/or fixed delay.
//
// Parameters:
//   - maxIOPS: Maximum I/O operations per second (0 = no IOPS limiting)
//   - ioDelay: Fixed delay between operations (0 = no delay)
//
// If both parameters are 0, the function returns nil (no throttling).
// Caller should check for nil before calling Acquire().
//
// IOPS Limiting:
//   - Uses rate.NewLimiter(rate.Limit(maxIOPS), maxIOPS)
//   - Burst allowance equals maxIOPS (allows short bursts)
//   - Token bucket refills at maxIOPS rate
//
// Example Usage:
//
//	throttle := NewIOThrottle(1000, 10*time.Millisecond)  // 1000 IOPS + 10ms delay
//	throttle := NewIOThrottle(500, 0)                      // 500 IOPS only
//	throttle := NewIOThrottle(0, 20*time.Millisecond)      // 20ms delay only
//	throttle := NewIOThrottle(0, 0)                        // returns nil (no throttling)
func NewIOThrottle(maxIOPS int, ioDelay time.Duration) *IOThrottle {
	// No throttling if both parameters are zero/disabled
	if maxIOPS <= 0 && ioDelay <= 0 {
		return nil
	}

	throttle := &IOThrottle{
		maxIOPS: maxIOPS,
		ioDelay: ioDelay,
	}

	// Create rate limiter if IOPS limiting is enabled
	if maxIOPS > 0 {
		// rate.Limit(maxIOPS) = tokens per second
		// maxIOPS = burst capacity (allows short bursts up to this limit)
		throttle.limiter = rate.NewLimiter(rate.Limit(maxIOPS), maxIOPS)
	}

	return throttle
}

// Acquire blocks until the I/O operation is allowed.
//
// Behavior:
//   - If t == nil, returns immediately (no throttling)
//   - If IOPS limiting enabled, waits for token from rate limiter
//   - If fixed delay enabled, sleeps for the configured duration
//   - Both delays are applied sequentially if both are configured
//   - Respects context cancellation throughout
//
// Returns:
//   - nil if operation is allowed to proceed
//   - context.Err() if context is cancelled during wait
//
// Thread Safety:
//   - Safe to call from multiple goroutines concurrently
//   - Mutex protects access to limiter field
//   - rate.Limiter handles synchronization internally
//
// Performance:
//   - Zero overhead if throttle is nil (caller should check)
//   - Minimal overhead for IOPS limiting (atomic operations)
//   - Fixed delay uses efficient context-aware sleep
func (t *IOThrottle) Acquire(ctx context.Context) error {
	// Fast path: no throttling
	if t == nil {
		return nil
	}

	// Apply IOPS limiting first (if enabled)
	// Acquire a snapshot of the limiter under lock to avoid race with Reset()
	t.mu.Lock()
	limiter := t.limiter
	t.mu.Unlock()

	if limiter != nil {
		// Wait for a token from the rate limiter
		// This blocks until:
		// 1. A token becomes available, OR
		// 2. Context is cancelled
		if err := limiter.Wait(ctx); err != nil {
			return err // Context cancelled
		}
	}

	// Apply fixed delay (if enabled)
	if t.ioDelay > 0 {
		// Use timer with context to allow cancellation during sleep
		timer := time.NewTimer(t.ioDelay)
		defer timer.Stop()

		select {
		case <-timer.C:
			// Delay completed normally
			return nil
		case <-ctx.Done():
			// Context cancelled during delay
			return ctx.Err()
		}
	}

	return nil
}

// Reset resets the rate limiter state.
//
// This clears any accumulated tokens in the rate limiter, effectively
// resetting the throttling state to initial conditions.
//
// Use Cases:
//   - Starting a new scan (clear previous scan's token accumulation)
//   - Testing (reset to known state between tests)
//   - Recovery after long idle period (prevent burst on resume)
//
// Thread Safety:
//   - Protected by mutex to prevent race during limiter recreation
//   - Safe to call concurrently with Acquire()
//
// Note: This is primarily useful for testing. In production, token accumulation
// is usually desired behavior (allows bursts after idle periods).
func (t *IOThrottle) Reset() {
	if t == nil {
		return
	}

	t.mu.Lock()
	defer t.mu.Unlock()

	// Recreate limiter with same parameters to clear token bucket
	if t.maxIOPS > 0 {
		t.limiter = rate.NewLimiter(rate.Limit(t.maxIOPS), t.maxIOPS)
	}
}

// IsEnabled returns true if throttling is active.
//
// Returns true if either IOPS limiting or fixed delay is configured.
// Used for logging/diagnostics and conditional behavior.
//
// Example:
//
//	if throttle != nil && throttle.IsEnabled() {
//	    log.Infof("Throttling enabled: %d IOPS, %v delay", ...)
//	}
func (t *IOThrottle) IsEnabled() bool {
	if t == nil {
		return false
	}
	return t.maxIOPS > 0 || t.ioDelay > 0
}
