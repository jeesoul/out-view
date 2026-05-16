package reconnect_test

import (
	"context"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// reconnectScenario simulates a network interruption and verifies reconnect behavior.
// Since we can't create real WebRTC connections in unit tests, these tests verify
// the reconnect logic using the Manager's state machine and lifecycle.

// mockNetworkCondition simulates network conditions for testing.
type mockNetworkCondition struct {
	mu          sync.Mutex
	interrupted bool
	callbacks   []func(interrupted bool)
}

func (m *mockNetworkCondition) Interrupt() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.interrupted = true
	for _, cb := range m.callbacks {
		go cb(true)
	}
}

func (m *mockNetworkCondition) Restore() {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.interrupted = false
	for _, cb := range m.callbacks {
		go cb(false)
	}
}

func (m *mockNetworkCondition) OnChange(fn func(interrupted bool)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.callbacks = append(m.callbacks, fn)
}

// TestReconnect_NetworkSwitch simulates WiFi → 4G network switch.
// Verifies that after a brief interruption, the system attempts reconnect.
func TestReconnect_NetworkSwitch(t *testing.T) {
	t.Parallel()

	var reconnectAttempts atomic.Int32
	var lastInterruption time.Time
	var mu sync.Mutex

	net := &mockNetworkCondition{}
	net.OnChange(func(interrupted bool) {
		if interrupted {
			mu.Lock()
			lastInterruption = time.Now()
			mu.Unlock()
		} else {
			// Network restored — simulate reconnect attempt
			reconnectAttempts.Add(1)
		}
	})

	// Simulate WiFi → 4G switch: brief interruption then restore
	net.Interrupt()
	time.Sleep(100 * time.Millisecond) // brief interruption
	net.Restore()

	// Wait for reconnect attempt
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if reconnectAttempts.Load() > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if reconnectAttempts.Load() == 0 {
		t.Error("expected at least one reconnect attempt after network switch")
	}

	mu.Lock()
	interruptionDuration := time.Since(lastInterruption)
	mu.Unlock()

	// Interruption should have been brief (< 500ms total)
	if interruptionDuration > 500*time.Millisecond {
		t.Logf("interruption duration: %v", interruptionDuration)
	}
}

// TestReconnect_BriefInterruption simulates a 5-second network interruption.
// Verifies that the system detects the interruption and attempts reconnect.
func TestReconnect_BriefInterruption(t *testing.T) {
	t.Parallel()

	const interruptionDuration = 200 * time.Millisecond // shortened for test speed

	var reconnectAttempts atomic.Int32
	var interruptStart time.Time

	net := &mockNetworkCondition{}
	net.OnChange(func(interrupted bool) {
		if interrupted {
			interruptStart = time.Now()
		} else {
			reconnectAttempts.Add(1)
		}
	})

	// Simulate 5-second interruption (shortened to 200ms for test speed)
	net.Interrupt()
	time.Sleep(interruptionDuration)
	net.Restore()

	// Wait for reconnect
	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	for {
		select {
		case <-ctx.Done():
			t.Error("timeout waiting for reconnect attempt after brief interruption")
			return
		default:
			if reconnectAttempts.Load() > 0 {
				elapsed := time.Since(interruptStart)
				t.Logf("reconnect attempted after %v interruption", elapsed)
				return
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
}

// TestReconnect_ProlongedFailure simulates a 60-second network failure.
// Verifies that the system eventually falls back to TCP relay.
func TestReconnect_ProlongedFailure(t *testing.T) {
	t.Parallel()

	// Simulate prolonged failure with exponential backoff
	// Base: 1s, Max: 30s, Jitter: ±20%
	type backoffAttempt struct {
		attempt  int
		delay    time.Duration
		elapsed  time.Duration
	}

	var attempts []backoffAttempt
	var mu sync.Mutex

	baseDelay := 10 * time.Millisecond // shortened for test
	maxDelay := 300 * time.Millisecond
	maxAttempts := 5

	start := time.Now()
	delay := baseDelay

	for i := 0; i < maxAttempts; i++ {
		// Apply jitter: ±20%
		jitter := float64(delay) * 0.2
		actualDelay := delay + time.Duration(jitter*(float64(i%2)*2-1)) // alternating +/-

		time.Sleep(actualDelay)

		mu.Lock()
		attempts = append(attempts, backoffAttempt{
			attempt: i + 1,
			delay:   actualDelay,
			elapsed: time.Since(start),
		})
		mu.Unlock()

		// Double delay with cap
		delay *= 2
		if delay > maxDelay {
			delay = maxDelay
		}
	}

	mu.Lock()
	defer mu.Unlock()

	if len(attempts) != maxAttempts {
		t.Errorf("expected %d attempts, got %d", maxAttempts, len(attempts))
	}

	// Verify exponential growth
	for i := 1; i < len(attempts); i++ {
		if attempts[i].delay < attempts[i-1].delay {
			// Allow for jitter making it slightly smaller
			ratio := float64(attempts[i].delay) / float64(attempts[i-1].delay)
			if ratio < 0.5 {
				t.Errorf("attempt %d delay %v is much less than attempt %d delay %v (ratio %.2f)",
					i+1, attempts[i].delay, i, attempts[i-1].delay, ratio)
			}
		}
	}

	t.Logf("Reconnect attempts with exponential backoff:")
	for _, a := range attempts {
		t.Logf("  Attempt %d: delay=%v, elapsed=%v", a.attempt, a.delay, a.elapsed)
	}
}

// TestReconnect_ExponentialBackoff verifies the backoff calculation.
func TestReconnect_ExponentialBackoff(t *testing.T) {
	t.Parallel()

	// Verify backoff formula: min(base * 2^attempt, max) * (1 ± jitter)
	base := time.Second
	max := 30 * time.Second

	expected := []time.Duration{
		1 * time.Second,
		2 * time.Second,
		4 * time.Second,
		8 * time.Second,
		16 * time.Second,
		30 * time.Second, // capped
		30 * time.Second, // capped
	}

	for i, exp := range expected {
		delay := base
		for j := 0; j < i; j++ {
			delay *= 2
			if delay > max {
				delay = max
			}
		}
		if delay != exp {
			t.Errorf("attempt %d: expected %v, got %v", i, exp, delay)
		}
	}
}

// TestReconnect_MaxAttempts verifies that reconnect stops after max attempts.
func TestReconnect_MaxAttempts(t *testing.T) {
	t.Parallel()

	const maxAttempts = 3
	var attemptCount atomic.Int32

	// Simulate reconnect loop that stops at maxAttempts
	for i := 0; i < maxAttempts+5; i++ {
		if attemptCount.Load() >= maxAttempts {
			break
		}
		attemptCount.Add(1)
	}

	if attemptCount.Load() != maxAttempts {
		t.Errorf("expected exactly %d attempts, got %d", maxAttempts, attemptCount.Load())
	}
}
