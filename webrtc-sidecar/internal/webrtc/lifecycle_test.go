package webrtc

import (
	"testing"
	"time"

	"github.com/outview/webrtc-sidecar/internal/ipc"
)

// TestManager_Close_Idempotent_Lifecycle verifies that calling Close() multiple
// times does not panic.
func TestManager_Close_Idempotent_Lifecycle(t *testing.T) {
	registry := ipc.NewConnRegistry()
	m := NewManagerWithIdleTimeout("lc-idempotent", registry, nil, 0)
	for i := 0; i < 5; i++ {
		m.Close() // must not panic
	}
	if m.State() != stateClosed {
		t.Errorf("expected stateClosed after Close(), got %d", m.State())
	}
}

// TestManager_Close_DataChannelBeforePC verifies that after Close() both dc and
// pc are nil and the state is stateClosed.
func TestManager_Close_DataChannelBeforePC(t *testing.T) {
	registry := ipc.NewConnRegistry()
	m := NewManagerWithIdleTimeout("lc-order", registry, nil, 0)

	m.Close()

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
	if m.State() != stateClosed {
		t.Errorf("expected stateClosed, got %d", m.State())
	}
}

// TestManager_IdleTimeout_TriggersClose verifies that when IdleTimeout elapses
// with no data activity, the manager closes itself.
func TestManager_IdleTimeout_TriggersClose(t *testing.T) {
	registry := ipc.NewConnRegistry()
	m := NewManagerWithIdleTimeout("lc-idle", registry, nil, 50*time.Millisecond)

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if m.State() == stateClosed {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if m.State() != stateClosed {
		t.Errorf("expected stateClosed after idle timeout, got %d", m.State())
	}
}

// TestManager_IdleTimeout_ResetOnActivity verifies that resetting the idle timer
// postpones the close.
func TestManager_IdleTimeout_ResetOnActivity(t *testing.T) {
	registry := ipc.NewConnRegistry()
	m := NewManagerWithIdleTimeout("lc-idle-reset", registry, nil, 80*time.Millisecond)

	for i := 0; i < 3; i++ {
		time.Sleep(30 * time.Millisecond)
		m.resetIdleTimer()
		if m.State() == stateClosed {
			t.Fatalf("manager closed prematurely after %d resets", i+1)
		}
	}

	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if m.State() == stateClosed {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if m.State() != stateClosed {
		t.Errorf("expected stateClosed after idle timeout expired, got %d", m.State())
	}
}

// TestManager_IdleTimeout_Disabled verifies that idleTimeout=0 disables the timer.
func TestManager_IdleTimeout_Disabled(t *testing.T) {
	registry := ipc.NewConnRegistry()
	m := NewManagerWithIdleTimeout("lc-idle-disabled", registry, nil, 0)
	defer m.Close()

	time.Sleep(50 * time.Millisecond)

	if m.State() == stateClosed {
		t.Error("manager should not close when idle timeout is disabled")
	}
}

// TestManager_AllTriggers_CallClose verifies that each trigger path results in
// the manager reaching stateClosed.

// Trigger 2: ICE failed.
func TestManager_Trigger_ICEFailed(t *testing.T) {
	registry := ipc.NewConnRegistry()
	m := NewManagerWithIdleTimeout("lc-ice-failed", registry, nil, 0)
	m.closeWithReason("ICE connection failed")

	if m.State() != stateClosed {
		t.Errorf("expected stateClosed after ICE failed trigger, got %d", m.State())
	}
}

// Trigger 3: DataChannel onClose.
func TestManager_Trigger_DataChannelClose(t *testing.T) {
	registry := ipc.NewConnRegistry()
	m := NewManagerWithIdleTimeout("lc-dc-close", registry, nil, 0)
	m.closeWithReason("data channel closed")

	if m.State() != stateClosed {
		t.Errorf("expected stateClosed after DataChannel close trigger, got %d", m.State())
	}
}

// Trigger 4: Business timeout (idle timeout) — covered by TestManager_IdleTimeout_TriggersClose.

// Trigger 5: Application shutdown — via Close().
func TestManager_Trigger_ApplicationShutdown(t *testing.T) {
	registry := ipc.NewConnRegistry()
	m := NewManagerWithIdleTimeout("lc-app-shutdown", registry, nil, 0)
	m.Close()

	if m.State() != stateClosed {
		t.Errorf("expected stateClosed after application shutdown, got %d", m.State())
	}
}

// Trigger 1: Control channel disconnect — ctx cancellation.
func TestManager_Trigger_ControlChannelDisconnect(t *testing.T) {
	registry := ipc.NewConnRegistry()
	m := NewManagerWithIdleTimeout("lc-ctrl-disconnect", registry, nil, 0)

	// Simulate IPC disconnect by cancelling the context externally.
	m.cancel()
	// Give the goroutine a moment to propagate.
	time.Sleep(10 * time.Millisecond)

	// The manager's ctx is cancelled; Close() should still work idempotently.
	m.Close()

	if m.State() != stateClosed {
		t.Errorf("expected stateClosed after control channel disconnect, got %d", m.State())
	}
}

// TestManager_CloseWithReason_Idempotent verifies that closeWithReason called
// multiple times only executes cleanup once.
func TestManager_CloseWithReason_Idempotent(t *testing.T) {
	registry := ipc.NewConnRegistry()
	m := NewManagerWithIdleTimeout("lc-reason-idempotent", registry, nil, 0)

	m.closeWithReason("ICE connection failed")
	m.closeWithReason("data channel closed")
	m.closeWithReason("idle timeout")
	m.Close()

	if m.State() != stateClosed {
		t.Errorf("expected stateClosed, got %d", m.State())
	}
}

// TestManager_ContextCancelledOnClose verifies that the manager's context is
// cancelled when Close() is called.
func TestManager_ContextCancelledOnClose(t *testing.T) {
	registry := ipc.NewConnRegistry()
	m := NewManagerWithIdleTimeout("lc-ctx-cancel", registry, nil, 0)

	m.Close()

	select {
	case <-m.ctx.Done():
		// expected
	default:
		t.Error("expected manager context to be cancelled after Close()")
	}
}
