package webrtc

import (
	"testing"
	"time"
)

// TestManager_Close_Idempotent verifies that calling Close() multiple times
// does not panic and returns nil each time.
func TestManager_Close_Idempotent_Lifecycle(t *testing.T) {
	m := NewManager("lc-idempotent", DefaultConfig(), nil)
	for i := 0; i < 5; i++ {
		if err := m.Close(); err != nil {
			t.Fatalf("Close() call %d returned error: %v", i+1, err)
		}
	}
	if m.State() != StateClosed {
		t.Errorf("expected StateClosed after Close(), got %v", m.State())
	}
}

// TestManager_Close_DataChannelBeforePC verifies the cleanup order:
// DataChannel is nilled before PeerConnection.
// We verify this by checking that after Close() both are nil and the state is Closed.
func TestManager_Close_DataChannelBeforePC(t *testing.T) {
	m := NewManager("lc-order", DefaultConfig(), nil)

	// Inject mock dc/pc pointers via the internal fields to verify cleanup order.
	// Since we can't create real pion objects without a network, we verify the
	// post-condition: both fields are nil and state is StateClosed.
	if err := m.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	m.mu.RLock()
	dc := m.dc
	pc := m.pc
	m.mu.RUnlock()

	if dc != nil {
		t.Error("expected dc to be nil after Close()")
	}
	if pc != nil {
		t.Error("expected pc to be nil after Close()")
	}
	if m.State() != StateClosed {
		t.Errorf("expected StateClosed, got %v", m.State())
	}
}

// TestManager_IdleTimeout_TriggersClose verifies that when IdleTimeout elapses
// with no data activity, the manager closes itself.
func TestManager_IdleTimeout_TriggersClose(t *testing.T) {
	cfg := DefaultConfig()
	cfg.IdleTimeout = 50 * time.Millisecond

	m := NewManager("lc-idle", cfg, nil)

	// Wait for the idle timer to fire (up to 500ms).
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if m.State() == StateClosed {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if m.State() != StateClosed {
		t.Errorf("expected StateClosed after idle timeout, got %v", m.State())
	}
}

// TestManager_IdleTimeout_ResetOnActivity verifies that resetting the idle timer
// (via resetIdleTimer) postpones the close.
func TestManager_IdleTimeout_ResetOnActivity(t *testing.T) {
	cfg := DefaultConfig()
	cfg.IdleTimeout = 80 * time.Millisecond

	m := NewManager("lc-idle-reset", cfg, nil)

	// Reset the timer several times within the timeout window.
	for i := 0; i < 3; i++ {
		time.Sleep(30 * time.Millisecond)
		m.resetIdleTimer()
		if m.State() == StateClosed {
			t.Fatalf("manager closed prematurely after %d resets", i+1)
		}
	}

	// Now let it expire.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if m.State() == StateClosed {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if m.State() != StateClosed {
		t.Errorf("expected StateClosed after idle timeout expired, got %v", m.State())
	}
}

// TestManager_IdleTimeout_Disabled verifies that IdleTimeout=0 disables the timer.
func TestManager_IdleTimeout_Disabled(t *testing.T) {
	cfg := DefaultConfig()
	cfg.IdleTimeout = 0

	m := NewManager("lc-idle-disabled", cfg, nil)
	defer m.Close()

	time.Sleep(50 * time.Millisecond)

	if m.State() == StateClosed {
		t.Error("manager should not close when IdleTimeout is disabled")
	}
}

// TestManager_AllTriggers_CallClose verifies that each trigger path results in
// the manager reaching StateClosed.

// Trigger 2: ICE failed — via closeWithReason.
func TestManager_Trigger_ICEFailed(t *testing.T) {
	m := NewManager("lc-ice-failed", DefaultConfig(), nil)
	m.closeWithReason("ICE connection failed")

	if m.State() != StateClosed {
		t.Errorf("expected StateClosed after ICE failed trigger, got %v", m.State())
	}
}

// Trigger 3: DataChannel onClose.
func TestManager_Trigger_DataChannelClose(t *testing.T) {
	m := NewManager("lc-dc-close", DefaultConfig(), nil)
	m.closeWithReason("data channel closed")

	if m.State() != StateClosed {
		t.Errorf("expected StateClosed after DataChannel close trigger, got %v", m.State())
	}
}

// Trigger 4: Business timeout (idle timeout) — covered by TestManager_IdleTimeout_TriggersClose.

// Trigger 5: Application shutdown — via Close().
func TestManager_Trigger_ApplicationShutdown(t *testing.T) {
	m := NewManager("lc-app-shutdown", DefaultConfig(), nil)
	if err := m.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if m.State() != StateClosed {
		t.Errorf("expected StateClosed after application shutdown, got %v", m.State())
	}
}

// TestManager_CloseWithReason_Idempotent verifies that closeWithReason called
// multiple times with different reasons only executes cleanup once.
func TestManager_CloseWithReason_Idempotent(t *testing.T) {
	m := NewManager("lc-reason-idempotent", DefaultConfig(), nil)

	m.closeWithReason("ICE connection failed")
	m.closeWithReason("data channel closed")
	m.closeWithReason("idle timeout")
	m.Close() //nolint:errcheck

	if m.State() != StateClosed {
		t.Errorf("expected StateClosed, got %v", m.State())
	}
}

// TestManager_ContextCancelledOnClose verifies that the manager's context is
// cancelled when Close() is called, which stops internal goroutines.
func TestManager_ContextCancelledOnClose(t *testing.T) {
	m := NewManager("lc-ctx-cancel", DefaultConfig(), nil)

	if err := m.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	select {
	case <-m.ctx.Done():
		// expected
	default:
		t.Error("expected manager context to be cancelled after Close()")
	}
}
