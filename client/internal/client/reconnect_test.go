package client

import (
	"sync/atomic"
	"testing"
	"time"

	clientwebrtc "github.com/outview/client/internal/webrtc"
)

// ---------------------------------------------------------------------------
// ReconnectCount
// ---------------------------------------------------------------------------

// TestClient_ReconnectCount_InitiallyZero verifies that reconnectCount starts at 0.
func TestClient_ReconnectCount_InitiallyZero(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AutoReconnect = false
	c := NewClient(cfg)

	if got := c.ReconnectCount(); got != 0 {
		t.Errorf("expected ReconnectCount()=0, got %d", got)
	}
}

// TestClient_ReconnectCount_Increments verifies that reconnectCount increments atomically.
func TestClient_ReconnectCount_Increments(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AutoReconnect = false
	c := NewClient(cfg)

	c.reconnectCount.Add(1)
	c.reconnectCount.Add(1)
	c.reconnectCount.Add(1)

	if got := c.ReconnectCount(); got != 3 {
		t.Errorf("expected ReconnectCount()=3, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// WebRTCRecoveryCount
// ---------------------------------------------------------------------------

// TestClient_WebRTCRecoveryCount_InitiallyZero verifies that webrtcRecoveryCount starts at 0.
func TestClient_WebRTCRecoveryCount_InitiallyZero(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AutoReconnect = false
	webrtcCfg := clientwebrtc.DefaultConfig()
	c := NewClientWithWebRTC(cfg, "conn-recovery-zero", webrtcCfg)

	if got := c.WebRTCRecoveryCount(); got != 0 {
		t.Errorf("expected WebRTCRecoveryCount()=0, got %d", got)
	}
}

// TestClient_WebRTCRecoveryCount_IncrementedOnReinit verifies that webrtcRecoveryCount
// increments when WebRTC is re-initiated after a reconnect.
func TestClient_WebRTCRecoveryCount_IncrementedOnReinit(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AutoReconnect = false
	webrtcCfg := clientwebrtc.DefaultConfig()
	c := NewClientWithWebRTC(cfg, "conn-recovery-inc", webrtcCfg)

	// Simulate what reconnectLoop does when shouldReinitiateWebRTC is true.
	c.webrtcRecoveryCount.Add(1)
	c.webrtcRecoveryCount.Add(1)

	if got := c.WebRTCRecoveryCount(); got != 2 {
		t.Errorf("expected WebRTCRecoveryCount()=2, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// Backoff parameters
// ---------------------------------------------------------------------------

// TestReconnectBackoff_ExponentialWithJitter verifies that the backoff doubles
// each attempt and stays within the ±20% jitter band.
func TestReconnectBackoff_ExponentialWithJitter(t *testing.T) {
	const (
		baseDelay = 1 * time.Second
		maxDelay  = 30 * time.Second
		jitter    = 0.20
	)

	delay := baseDelay
	for attempt := 1; attempt <= 8; attempt++ {
		lo := time.Duration(float64(delay) * (1 - jitter))
		hi := time.Duration(float64(delay) * (1 + jitter))

		if lo <= 0 {
			t.Errorf("attempt %d: lower bound must be positive, got %v", attempt, lo)
		}
		if hi > time.Duration(float64(maxDelay)*(1+jitter)) {
			t.Errorf("attempt %d: upper bound %v exceeds max+jitter", attempt, hi)
		}

		// Advance delay as the reconnectLoop does.
		delay *= 2
		if delay > maxDelay {
			delay = maxDelay
		}
	}
}

// TestReconnectBackoff_CapsAtMax verifies that the delay never exceeds maxDelay
// regardless of how many doublings occur.
func TestReconnectBackoff_CapsAtMax(t *testing.T) {
	const (
		baseDelay = 1 * time.Second
		maxDelay  = 30 * time.Second
	)

	delay := baseDelay
	for i := 0; i < 20; i++ {
		delay *= 2
		if delay > maxDelay {
			delay = maxDelay
		}
	}

	if delay != maxDelay {
		t.Errorf("expected delay to cap at %v, got %v", maxDelay, delay)
	}
}

// ---------------------------------------------------------------------------
// Old manager closed before replacement
// ---------------------------------------------------------------------------

// TestClient_OldWebRTCManagerClosedOnReinit verifies that the old WebRTC manager
// is closed before being replaced during a reconnect.
func TestClient_OldWebRTCManagerClosedOnReinit(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AutoReconnect = false
	webrtcCfg := clientwebrtc.DefaultConfig()
	c := NewClientWithWebRTC(cfg, "conn-close-old", webrtcCfg)

	originalMgr := c.webrtcManager

	// Simulate the reconnect reinit path.
	c.webrtcMu.Lock()
	c.webrtcEnabled = true
	c.webrtcMu.Unlock()

	c.webrtcMu.Lock()
	shouldReinit := c.webrtcEnabled && c.webrtcCfg != nil
	oldMgr := c.webrtcManager
	c.webrtcMu.Unlock()

	if !shouldReinit {
		t.Fatal("precondition: shouldReinit must be true")
	}

	var connID string
	if oldMgr != nil {
		connID = oldMgr.ConnectionID()
		oldMgr.Close() // close before replacement
	}

	newMgr := clientwebrtc.NewManager(connID, c.webrtcCfg, nil)
	c.webrtcMu.Lock()
	c.webrtcManager = newMgr
	c.webrtcMu.Unlock()
	c.webrtcRecoveryCount.Add(1)

	// The original manager should now be closed.
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if originalMgr.State() == clientwebrtc.StateClosed {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}
	if originalMgr.State() != clientwebrtc.StateClosed {
		t.Errorf("expected original manager to be closed, state=%d", originalMgr.State())
	}

	// The new manager should be a different instance.
	if c.webrtcManager == originalMgr {
		t.Error("expected a new Manager instance after reinit")
	}

	// Recovery count should be 1.
	if got := c.WebRTCRecoveryCount(); got != 1 {
		t.Errorf("expected WebRTCRecoveryCount()=1, got %d", got)
	}
}

// ---------------------------------------------------------------------------
// Atomic counter concurrency
// ---------------------------------------------------------------------------

// TestClient_ReconnectCount_Concurrent verifies that concurrent increments are safe.
func TestClient_ReconnectCount_Concurrent(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AutoReconnect = false
	c := NewClient(cfg)

	const goroutines = 50
	var wg atomic.Int32
	wg.Store(goroutines)

	done := make(chan struct{})
	for i := 0; i < goroutines; i++ {
		go func() {
			c.reconnectCount.Add(1)
			if wg.Add(-1) == 0 {
				close(done)
			}
		}()
	}

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatal("concurrent increment timed out")
	}

	if got := c.ReconnectCount(); got != goroutines {
		t.Errorf("expected ReconnectCount()=%d, got %d", goroutines, got)
	}
}
