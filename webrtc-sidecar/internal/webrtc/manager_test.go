package webrtc

import (
	"context"
	"testing"

	pionwebrtc "github.com/pion/webrtc/v4"

	"github.com/outview/webrtc-sidecar/internal/ipc"
)

func TestManager_NewManager(t *testing.T) {
	registry := ipc.NewConnRegistry()
	m := NewManager("test-conn-1", registry, nil)
	defer m.Close()

	if m.State() != stateIdle {
		t.Errorf("expected stateIdle, got %d", m.State())
	}
}

func TestManager_CreatePeerConnection(t *testing.T) {
	registry := ipc.NewConnRegistry()
	m := NewManager("test-conn-2", registry, nil)
	defer m.Close()

	if err := m.CreatePeerConnection(); err != nil {
		t.Fatalf("CreatePeerConnection: %v", err)
	}

	if m.State() != stateConnecting {
		t.Errorf("expected stateConnecting after CreatePeerConnection, got %d", m.State())
	}
}

func TestManager_Close_Idempotent(t *testing.T) {
	registry := ipc.NewConnRegistry()
	m := NewManager("test-conn-close", registry, nil)
	m.Close()
	m.Close() // should not panic
}

func TestManager_AddICECandidate_BeforeRemoteSet(t *testing.T) {
	registry := ipc.NewConnRegistry()
	m := NewManager("test-conn-ice", registry, nil)
	defer m.Close()

	if err := m.CreatePeerConnection(); err != nil {
		t.Fatal(err)
	}

	candidate := pionwebrtc.ICECandidateInit{
		Candidate: "candidate:1 1 UDP 2130706431 192.168.1.1 54321 typ host",
	}
	if err := m.AddICECandidate(context.Background(), candidate); err != nil {
		t.Fatalf("AddICECandidate: %v", err)
	}

	m.pendingICEMu.Lock()
	count := len(m.pendingICE)
	m.pendingICEMu.Unlock()

	if count != 1 {
		t.Errorf("expected 1 buffered candidate, got %d", count)
	}
}

func TestPool_CreateAndGet(t *testing.T) {
	registry := ipc.NewConnRegistry()
	pool := NewPool(registry, nil)
	defer pool.CloseAll()

	m, err := pool.Create("conn-1")
	if err != nil {
		t.Fatalf("Create: %v", err)
	}
	if m == nil {
		t.Fatal("expected non-nil manager")
	}

	got, ok := pool.Get("conn-1")
	if !ok {
		t.Fatal("expected to find conn-1")
	}
	if got != m {
		t.Error("Get returned different manager than Create")
	}
}

func TestPool_Create_Duplicate(t *testing.T) {
	registry := ipc.NewConnRegistry()
	pool := NewPool(registry, nil)
	defer pool.CloseAll()

	if _, err := pool.Create("conn-dup"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Create("conn-dup"); err == nil {
		t.Error("expected error for duplicate connectionID")
	}
}

func TestPool_Remove(t *testing.T) {
	registry := ipc.NewConnRegistry()
	pool := NewPool(registry, nil)

	if _, err := pool.Create("conn-remove"); err != nil {
		t.Fatal(err)
	}
	pool.Remove("conn-remove")

	if _, ok := pool.Get("conn-remove"); ok {
		t.Error("expected conn-remove to be removed")
	}
}

func TestPool_Count(t *testing.T) {
	registry := ipc.NewConnRegistry()
	pool := NewPool(registry, nil)
	defer pool.CloseAll()

	if _, err := pool.Create("c1"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Create("c2"); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Create("c3"); err != nil {
		t.Fatal(err)
	}

	if pool.Count() != 3 {
		t.Errorf("expected count=3, got %d", pool.Count())
	}
}
