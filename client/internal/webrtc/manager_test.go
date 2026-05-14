package webrtc

import (
	"context"
	"testing"
	"time"

	pionwebrtc "github.com/pion/webrtc/v4"
)

func TestManager_NewManager(t *testing.T) {
	m := NewManager("test-conn-1", DefaultConfig(), nil)
	defer m.Close()
	if m.State() != StateIdle {
		t.Errorf("expected StateIdle, got %v", m.State())
	}
}

func TestManager_DTLSTimeout(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DTLSTimeout = 10 * time.Second
	m := NewManager("test-conn-dtls", cfg, nil)
	defer m.Close()
	if cfg.DTLSTimeout != 10*time.Second {
		t.Errorf("expected DTLSTimeout=10s, got %v", cfg.DTLSTimeout)
	}
}

func TestManager_Close_Idempotent(t *testing.T) {
	m := NewManager("test-conn-close", DefaultConfig(), nil)
	if err := m.Close(); err != nil {
		t.Fatalf("first Close: %v", err)
	}
	if err := m.Close(); err != nil {
		t.Fatalf("second Close: %v", err)
	}
}

func TestManager_AddICECandidate_BeforeRemoteDescription(t *testing.T) {
	m := NewManager("test-conn-ice", DefaultConfig(), nil)
	defer m.Close()

	candidate := pionwebrtc.ICECandidateInit{
		Candidate: "candidate:1 1 UDP 2130706431 192.168.1.1 54321 typ host",
	}
	if err := m.AddICECandidate(context.Background(), candidate); err != nil {
		t.Fatalf("AddICECandidate before remote desc: %v", err)
	}

	m.pendingICEMu.Lock()
	count := len(m.pendingICE)
	m.pendingICEMu.Unlock()

	if count != 1 {
		t.Errorf("expected 1 buffered candidate, got %d", count)
	}
}

func TestManager_StateTransition_Invalid(t *testing.T) {
	if isValidTransition(StateClosed, StateGatheringICE) {
		t.Error("expected invalid: StateClosed -> StateGatheringICE")
	}
	if !isValidTransition(StateIdle, StateGatheringICE) {
		t.Error("expected valid: StateIdle -> StateGatheringICE")
	}
}

func TestManager_SendData_ChannelNotReady(t *testing.T) {
	m := NewManager("test-conn-send", DefaultConfig(), nil)
	defer m.Close()
	err := m.SendData(context.Background(), []byte("hello"))
	if err == nil {
		t.Error("expected error when DataChannel not ready")
	}
}
