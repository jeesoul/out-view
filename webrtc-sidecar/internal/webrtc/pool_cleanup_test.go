package webrtc

import (
	"fmt"
	"net"
	"testing"
	"time"

	"github.com/outview/webrtc-sidecar/internal/ipc"
)

// ---------------------------------------------------------------------------
// Pool.CloseAll on disconnect
// ---------------------------------------------------------------------------

// TestPool_CloseAll_OnDisconnect verifies that CloseAll removes all managers
// and sets the count to zero — simulating what happens when the IPC connection
// from Java drops.
func TestPool_CloseAll_OnDisconnect(t *testing.T) {
	registry := ipc.NewConnRegistry()
	pool := NewPool(registry, nil)

	// Create several managers.
	for _, id := range []string{"conn-a", "conn-b", "conn-c"} {
		if _, err := pool.Create(id); err != nil {
			t.Fatalf("Create(%q): %v", id, err)
		}
	}

	if pool.Count() != 3 {
		t.Fatalf("precondition: expected 3 managers, got %d", pool.Count())
	}

	// Simulate IPC disconnect: call CloseAll.
	pool.CloseAll()

	if pool.Count() != 0 {
		t.Errorf("expected 0 managers after CloseAll, got %d", pool.Count())
	}
}

// TestPool_CloseAll_ManagersAreClosed verifies that each manager transitions to
// a closed state after CloseAll is called.
func TestPool_CloseAll_ManagersAreClosed(t *testing.T) {
	registry := ipc.NewConnRegistry()
	pool := NewPool(registry, nil)

	ids := []string{"d1", "d2"}
	managers := make([]*Manager, 0, len(ids))
	for _, id := range ids {
		m, err := pool.Create(id)
		if err != nil {
			t.Fatalf("Create(%q): %v", id, err)
		}
		managers = append(managers, m)
	}

	pool.CloseAll()

	for i, m := range managers {
		deadline := time.Now().Add(200 * time.Millisecond)
		for time.Now().Before(deadline) {
			state := m.State()
			// Accept either stateClosed or stateFailed — the PeerConnection's
			// OnConnectionStateChange callback may fire after Close() and set
			// the state to stateFailed.
			if state == stateClosed || state == stateFailed {
				break
			}
			time.Sleep(5 * time.Millisecond)
		}
		state := m.State()
		if state != stateClosed && state != stateFailed {
			t.Errorf("manager[%d] expected stateClosed or stateFailed, got %d", i, state)
		}
	}
}

// TestPool_CloseAll_OnConnectionClosedCallback verifies that the
// OnConnectionClosed callback is invoked for each manager during CloseAll.
func TestPool_CloseAll_OnConnectionClosedCallback(t *testing.T) {
	registry := ipc.NewConnRegistry()
	pool := NewPool(registry, nil)

	ids := []string{"cb-1", "cb-2", "cb-3"}
	for _, id := range ids {
		if _, err := pool.Create(id); err != nil {
			t.Fatalf("Create(%q): %v", id, err)
		}
	}

	closed := make(map[string]bool)
	pool.OnConnectionClosed(func(connectionID string) {
		closed[connectionID] = true
	})

	pool.CloseAll()

	for _, id := range ids {
		if !closed[id] {
			t.Errorf("expected OnConnectionClosed to be called for %q", id)
		}
	}
}

// TestPool_CloseAll_Idempotent verifies that calling CloseAll twice does not panic.
func TestPool_CloseAll_Idempotent(t *testing.T) {
	registry := ipc.NewConnRegistry()
	pool := NewPool(registry, nil)

	if _, err := pool.Create("idem-1"); err != nil {
		t.Fatal(err)
	}

	pool.CloseAll()
	pool.CloseAll() // should not panic
}

// TestPool_CloseAll_StatsUpdated verifies that TotalClosed is updated correctly.
func TestPool_CloseAll_StatsUpdated(t *testing.T) {
	registry := ipc.NewConnRegistry()
	pool := NewPool(registry, nil)

	const n = 4
	for i := 0; i < n; i++ {
		id := fmt.Sprintf("stats-%d", i)
		if _, err := pool.Create(id); err != nil {
			t.Fatalf("Create: %v", err)
		}
	}

	pool.CloseAll()

	stats := pool.Stats()
	if stats.TotalClosed != n {
		t.Errorf("expected TotalClosed=%d, got %d", n, stats.TotalClosed)
	}
	if stats.ActiveConnections != 0 {
		t.Errorf("expected ActiveConnections=0, got %d", stats.ActiveConnections)
	}
}

// ---------------------------------------------------------------------------
// IPC Server OnDisconnect callback
// ---------------------------------------------------------------------------

// TestServer_OnDisconnect_CalledOnConnClose verifies that the OnDisconnect
// callback fires when a client connection closes.
func TestServer_OnDisconnect_CalledOnConnClose(t *testing.T) {
	srv := ipc.NewServer(ipc.DefaultHandler)

	disconnected := make(chan string, 1)
	srv.SetOnDisconnect(func(remoteAddr string) {
		disconnected <- remoteAddr
	})

	// Start on a random TCP port for testability.
	if err := srv.ListenTCP("127.0.0.1:0"); err != nil {
		t.Fatalf("ListenTCP: %v", err)
	}
	defer srv.Close()

	addr := srv.Addr()
	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}

	// Close the client connection — this should trigger OnDisconnect.
	conn.Close()

	select {
	case addr := <-disconnected:
		if addr == "" {
			t.Error("expected non-empty remote address in OnDisconnect callback")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("OnDisconnect callback was not called within 2s")
	}
}

// TestServer_OnDisconnect_PoolCleanup verifies the end-to-end scenario:
// when the IPC connection drops, the pool is cleaned up via OnDisconnect.
func TestServer_OnDisconnect_PoolCleanup(t *testing.T) {
	registry := ipc.NewConnRegistry()
	pool := NewPool(registry, nil)

	// Pre-populate the pool.
	for _, id := range []string{"p1", "p2"} {
		if _, err := pool.Create(id); err != nil {
			t.Fatalf("Create(%q): %v", id, err)
		}
	}

	srv := ipc.NewServer(ipc.DefaultHandler)
	srv.SetOnDisconnect(func(_ string) {
		pool.CloseAll()
	})

	if err := srv.ListenTCP("127.0.0.1:0"); err != nil {
		t.Fatalf("ListenTCP: %v", err)
	}
	defer srv.Close()

	conn, err := net.Dial("tcp", srv.Addr())
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	conn.Close()

	// Wait for the pool to be cleaned up.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if pool.Count() == 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	if pool.Count() != 0 {
		t.Errorf("expected pool to be empty after IPC disconnect, got %d", pool.Count())
	}
}
