package webrtc

import (
	"sync/atomic"
	"testing"
	"time"
)

// TestManager_FallbackCount_InitiallyZero verifies that FallbackCount starts at 0.
func TestManager_FallbackCount_InitiallyZero(t *testing.T) {
	m := NewManager("test-fb-zero", DefaultConfig(), nil)
	defer m.Close()

	if got := m.FallbackCount(); got != 0 {
		t.Errorf("expected FallbackCount()=0, got %d", got)
	}
}

// TestManager_FallbackCount_IncrementOnTrigger verifies that triggerFallback
// increments the counter.
func TestManager_FallbackCount_IncrementOnTrigger(t *testing.T) {
	m := NewManager("test-fb-incr", DefaultConfig(), nil)
	defer m.Close()

	// Drive to a state where fallback is valid (GatheringICE -> WebRTCFailed).
	m.requestStateTransition(StateGatheringICE, "test")
	// Give stateActor time to process.
	time.Sleep(20 * time.Millisecond)

	m.triggerFallback("test reason")

	// Give stateActor time to process the WebRTCFailed transition.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if m.FallbackCount() == 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	if got := m.FallbackCount(); got != 1 {
		t.Errorf("expected FallbackCount()=1 after one triggerFallback, got %d", got)
	}
}

// TestManager_FallbackCount_MultipleIncrements verifies that each triggerFallback
// call increments the counter, even if state transitions are ignored (already failed).
func TestManager_FallbackCount_MultipleIncrements(t *testing.T) {
	m := NewManager("test-fb-multi", DefaultConfig(), nil)
	defer m.Close()

	// Drive to GatheringICE so the first fallback transition is valid.
	m.requestStateTransition(StateGatheringICE, "test")
	time.Sleep(20 * time.Millisecond)

	const n = 3
	for i := 0; i < n; i++ {
		m.triggerFallback("reason")
	}

	// FallbackCount is incremented atomically before the state transition,
	// so all n increments should be visible immediately.
	if got := m.FallbackCount(); got != n {
		t.Errorf("expected FallbackCount()=%d, got %d", n, got)
	}
}

// TestManager_FallbackCount_CallbackFired verifies that the onFallback callback
// is invoked when triggerFallback is called.
func TestManager_FallbackCount_CallbackFired(t *testing.T) {
	m := NewManager("test-fb-cb", DefaultConfig(), nil)
	defer m.Close()

	var callbackCount atomic.Int64
	m.SetOnFallback(func(reason string) {
		callbackCount.Add(1)
	})

	m.requestStateTransition(StateGatheringICE, "test")
	time.Sleep(20 * time.Millisecond)

	m.triggerFallback("ice failed")

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if callbackCount.Load() == 1 {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	if got := callbackCount.Load(); got != 1 {
		t.Errorf("expected onFallback callback to be called once, got %d", got)
	}
	if got := m.FallbackCount(); got != 1 {
		t.Errorf("expected FallbackCount()=1, got %d", got)
	}
}

// TestManager_Stats_FallbackCount verifies that Stats() includes FallbackCount.
func TestManager_Stats_FallbackCount(t *testing.T) {
	m := NewManager("test-stats-fb", DefaultConfig(), nil)
	defer m.Close()

	stats := m.Stats()
	if stats.FallbackCount != 0 {
		t.Errorf("expected Stats().FallbackCount=0 initially, got %d", stats.FallbackCount)
	}

	m.requestStateTransition(StateGatheringICE, "test")
	time.Sleep(20 * time.Millisecond)
	m.triggerFallback("test")

	stats = m.Stats()
	if stats.FallbackCount != 1 {
		t.Errorf("expected Stats().FallbackCount=1 after fallback, got %d", stats.FallbackCount)
	}
}
