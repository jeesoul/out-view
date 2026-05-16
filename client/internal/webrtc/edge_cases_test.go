package webrtc

import (
	"context"
	"sync"
	"testing"
	"time"

	pionwebrtc "github.com/pion/webrtc/v4"
)

// TestManager_SendData_ChannelNil verifies SendData returns an error when the
// DataChannel has not been created yet (dc is nil).
func TestManager_SendData_ChannelNil(t *testing.T) {
	m := NewManager("edge-send-nil", DefaultConfig(), nil)
	defer m.Close()

	err := m.SendData(context.Background(), []byte("payload"))
	if err == nil {
		t.Fatal("expected error when dc is nil, got nil")
	}
	if err.Error() != "data channel not ready" {
		t.Errorf("expected \"data channel not ready\", got %q", err.Error())
	}
}

// TestManager_SendData_ContextCancelled_NilChannel verifies that SendData with
// an already-cancelled context still returns an error (without blocking) when
// the DataChannel is nil. The dc-nil check fires before context inspection,
// which is the correct fast-fail behaviour.
func TestManager_SendData_ContextCancelled_NilChannel(t *testing.T) {
	m := NewManager("edge-send-cancel", DefaultConfig(), nil)
	defer m.Close()

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	done := make(chan error, 1)
	go func() {
		done <- m.SendData(ctx, []byte("payload"))
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Error("expected error from SendData with cancelled context")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("SendData did not return — possible blocking with cancelled ctx")
	}
}

// TestManager_SendData_ManagerClosed verifies SendData returns an error after
// the manager has been closed. The close path nils the DataChannel, so the
// dc-nil guard fires first and produces a deterministic error.
func TestManager_SendData_ManagerClosed(t *testing.T) {
	m := NewManager("edge-send-closed", DefaultConfig(), nil)
	if err := m.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}

	done := make(chan error, 1)
	go func() {
		done <- m.SendData(context.Background(), []byte("payload"))
	}()

	select {
	case err := <-done:
		if err == nil {
			t.Error("expected error from SendData on closed manager")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("SendData blocked on closed manager")
	}
}

// TestManager_RequestStateTransition_InvalidIgnored verifies that
// requestStateTransition with an invalid target is silently ignored by the
// stateActor and the state remains unchanged.
func TestManager_RequestStateTransition_InvalidIgnored(t *testing.T) {
	m := NewManager("edge-state-invalid", DefaultConfig(), nil)
	defer m.Close()

	// StateIdle -> StateWebRTCConnected is not a valid transition.
	if isValidTransition(StateIdle, StateWebRTCConnected) {
		t.Fatal("test precondition broken: StateIdle -> StateWebRTCConnected should be invalid")
	}

	if got := m.State(); got != StateIdle {
		t.Fatalf("precondition: expected StateIdle, got %v", got)
	}

	m.requestStateTransition(StateWebRTCConnected, "test invalid")

	// Give the stateActor time to process and reject the transition.
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if m.State() != StateIdle {
			t.Fatalf("invalid transition was applied: state=%v", m.State())
		}
		time.Sleep(10 * time.Millisecond)
	}

	if m.State() != StateIdle {
		t.Errorf("expected StateIdle after invalid transition, got %v", m.State())
	}
}

// TestManager_ConcurrentClose verifies that 100 goroutines calling Close()
// concurrently does not panic and the final state is StateClosed.
func TestManager_ConcurrentClose(t *testing.T) {
	m := NewManager("edge-concurrent-close", DefaultConfig(), nil)

	const goroutines = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			if err := m.Close(); err != nil {
				t.Errorf("Close returned error: %v", err)
			}
		}()
	}
	wg.Wait()

	if m.State() != StateClosed {
		t.Errorf("expected StateClosed, got %v", m.State())
	}
}

// TestManager_AddICECandidate_NilPC verifies that AddICECandidate returns an
// error when remoteSet is true but pc is nil. This case can occur if the
// PeerConnection was torn down between SetRemoteDescription and the candidate
// being delivered.
func TestManager_AddICECandidate_NilPC(t *testing.T) {
	m := NewManager("edge-add-ice-nilpc", DefaultConfig(), nil)
	defer m.Close()

	// Simulate "remote already set" without going through SetRemoteDescription.
	m.pendingICEMu.Lock()
	m.remoteSet = true
	m.pendingICEMu.Unlock()

	// pc is still nil because CreateOffer was never called.
	candidate := pionwebrtc.ICECandidateInit{
		Candidate: "candidate:1 1 UDP 2130706431 192.168.1.1 54321 typ host",
	}
	err := m.AddICECandidate(context.Background(), candidate)
	if err == nil {
		t.Fatal("expected error when pc is nil and remoteSet=true")
	}
	if err.Error() != "peer connection not initialized" {
		t.Errorf("expected \"peer connection not initialized\", got %q", err.Error())
	}
}

// TestManager_SetRemoteDescription_NilPC verifies that SetRemoteDescription
// returns an error when pc has not been created.
func TestManager_SetRemoteDescription_NilPC(t *testing.T) {
	m := NewManager("edge-set-remote-nilpc", DefaultConfig(), nil)
	defer m.Close()

	sd := pionwebrtc.SessionDescription{
		Type: pionwebrtc.SDPTypeAnswer,
		SDP:  "v=0\r\n",
	}
	err := m.SetRemoteDescription(context.Background(), sd)
	if err == nil {
		t.Fatal("expected error when pc is nil")
	}
	if err.Error() != "peer connection not initialized" {
		t.Errorf("expected \"peer connection not initialized\", got %q", err.Error())
	}

	// Verify state was NOT mutated by the failed call.
	m.pendingICEMu.Lock()
	remoteSet := m.remoteSet
	m.pendingICEMu.Unlock()
	if remoteSet {
		t.Error("remoteSet should remain false after nil-pc SetRemoteDescription")
	}
}
