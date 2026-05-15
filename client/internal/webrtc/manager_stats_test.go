package webrtc

import (
	"testing"
	"time"
)

func TestManager_IsConnected_InitiallyFalse(t *testing.T) {
	m := NewManager("test-isconn-1", DefaultConfig(), nil)
	defer m.Close()

	if m.IsConnected() {
		t.Error("expected IsConnected() == false for a new manager")
	}
}

func TestManager_IsConnected_AfterStateTransition(t *testing.T) {
	m := NewManager("test-isconn-2", DefaultConfig(), nil)
	defer m.Close()

	// Manually drive the state to StateWebRTCConnected via the state channel.
	// We need to go through valid transitions: Idle -> GatheringICE -> Connecting -> WebRTCConnected.
	m.requestStateTransition(StateGatheringICE, "test")
	m.requestStateTransition(StateConnecting, "test")
	m.requestStateTransition(StateWebRTCConnected, "test")

	// Give the stateActor goroutine time to process.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if m.IsConnected() {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	if !m.IsConnected() {
		t.Errorf("expected IsConnected() == true after transitioning to StateWebRTCConnected, got state=%v", m.State())
	}
}

func TestManager_IsConnected_FalseAfterClose(t *testing.T) {
	m := NewManager("test-isconn-3", DefaultConfig(), nil)

	m.requestStateTransition(StateGatheringICE, "test")
	m.requestStateTransition(StateConnecting, "test")
	m.requestStateTransition(StateWebRTCConnected, "test")

	// Wait for connected
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if m.IsConnected() {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	m.Close()

	if m.IsConnected() {
		t.Error("expected IsConnected() == false after Close()")
	}
}

func TestManager_Stats_InitialState(t *testing.T) {
	m := NewManager("test-stats-1", DefaultConfig(), nil)
	defer m.Close()

	stats := m.Stats()

	if stats.ConnectionID != "test-stats-1" {
		t.Errorf("expected ConnectionID=%q, got %q", "test-stats-1", stats.ConnectionID)
	}
	if stats.State != StateIdle {
		t.Errorf("expected State=StateIdle, got %v", stats.State)
	}
	if stats.Uptime != 0 {
		t.Errorf("expected Uptime=0 before connection, got %v", stats.Uptime)
	}
}

func TestManager_Stats_UptimeAfterConnected(t *testing.T) {
	m := NewManager("test-stats-2", DefaultConfig(), nil)
	defer m.Close()

	before := time.Now()

	m.requestStateTransition(StateGatheringICE, "test")
	m.requestStateTransition(StateConnecting, "test")
	m.requestStateTransition(StateWebRTCConnected, "test")

	// Wait for the state actor to process the transition.
	deadline := time.Now().Add(500 * time.Millisecond)
	for time.Now().Before(deadline) {
		if m.IsConnected() {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	after := time.Now()
	stats := m.Stats()

	if stats.State != StateWebRTCConnected {
		t.Errorf("expected State=StateWebRTCConnected, got %v", stats.State)
	}
	if stats.Uptime <= 0 {
		t.Errorf("expected positive Uptime after connection, got %v", stats.Uptime)
	}
	// Uptime should be at most the elapsed wall time since before the transitions.
	elapsed := after.Sub(before) + 100*time.Millisecond // small tolerance
	if stats.Uptime > elapsed {
		t.Errorf("Uptime %v exceeds expected max %v", stats.Uptime, elapsed)
	}
}

func TestManager_Stats_ConnectionID(t *testing.T) {
	id := "my-unique-conn-id"
	m := NewManager(id, DefaultConfig(), nil)
	defer m.Close()

	if m.Stats().ConnectionID != id {
		t.Errorf("Stats().ConnectionID = %q, want %q", m.Stats().ConnectionID, id)
	}
}
