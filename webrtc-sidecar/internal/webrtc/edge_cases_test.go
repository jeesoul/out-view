package webrtc

import (
	"context"
	"fmt"
	"sync"
	"testing"

	pionwebrtc "github.com/pion/webrtc/v4"

	"github.com/outview/webrtc-sidecar/internal/ipc"
)

// TestPool_Stats verifies that Stats correctly reports active connections,
// total created, and total closed across the pool's lifecycle.
func TestPool_Stats(t *testing.T) {
	registry := ipc.NewConnRegistry()
	pool := NewPool(registry, nil)
	defer pool.CloseAll()

	// Initial: all zero.
	got := pool.Stats()
	if got.ActiveConnections != 0 || got.TotalCreated != 0 || got.TotalClosed != 0 {
		t.Errorf("expected zeroed stats, got %+v", got)
	}

	const created = 4
	ids := make([]string, 0, created)
	for i := 0; i < created; i++ {
		id := fmt.Sprintf("edge-stats-%d", i)
		ids = append(ids, id)
		if _, err := pool.Create(id); err != nil {
			t.Fatalf("Create(%q): %v", id, err)
		}
	}
	got = pool.Stats()
	if got.ActiveConnections != created {
		t.Errorf("ActiveConnections after creates: got %d, want %d", got.ActiveConnections, created)
	}
	if got.TotalCreated != created {
		t.Errorf("TotalCreated after creates: got %d, want %d", got.TotalCreated, created)
	}
	if got.TotalClosed != 0 {
		t.Errorf("TotalClosed after creates: got %d, want 0", got.TotalClosed)
	}

	// Remove two — TotalCreated must stay constant, TotalClosed must increment.
	pool.Remove(ids[0])
	pool.Remove(ids[1])

	got = pool.Stats()
	if got.ActiveConnections != created-2 {
		t.Errorf("ActiveConnections after 2 removes: got %d, want %d", got.ActiveConnections, created-2)
	}
	if got.TotalCreated != created {
		t.Errorf("TotalCreated must remain %d after removes, got %d", created, got.TotalCreated)
	}
	if got.TotalClosed != 2 {
		t.Errorf("TotalClosed: got %d, want 2", got.TotalClosed)
	}

	// CloseAll the rest — TotalClosed should equal TotalCreated when pool is empty.
	pool.CloseAll()

	got = pool.Stats()
	if got.ActiveConnections != 0 {
		t.Errorf("ActiveConnections after CloseAll: got %d, want 0", got.ActiveConnections)
	}
	if got.TotalCreated != created {
		t.Errorf("TotalCreated after CloseAll: got %d, want %d", got.TotalCreated, created)
	}
	if got.TotalClosed != created {
		t.Errorf("TotalClosed after CloseAll: got %d, want %d", got.TotalClosed, created)
	}
}

// TestPool_ConcurrentCreateAndRemove verifies the pool is safe under
// concurrent Create/Remove pairs from many goroutines, with no data races,
// no panics, and a clean final empty state.
func TestPool_ConcurrentCreateAndRemove(t *testing.T) {
	registry := ipc.NewConnRegistry()
	pool := NewPool(registry, nil)
	defer pool.CloseAll()

	const goroutines = 20
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func(idx int) {
			defer wg.Done()
			id := fmt.Sprintf("edge-concurrent-%d", idx)
			if _, err := pool.Create(id); err != nil {
				t.Errorf("Create(%q): %v", id, err)
				return
			}
			pool.Remove(id)
		}(i)
	}
	wg.Wait()

	if pool.Count() != 0 {
		t.Errorf("expected pool empty after concurrent create/remove pairs, got %d", pool.Count())
	}
	stats := pool.Stats()
	if stats.TotalCreated != goroutines || stats.TotalClosed != goroutines {
		t.Errorf("expected created=%d closed=%d, got created=%d closed=%d",
			goroutines, goroutines, stats.TotalCreated, stats.TotalClosed)
	}
}

// TestManager_AddICECandidate_NilPC verifies that AddICECandidate returns an
// error when remoteSet is true but pc is nil. This case can occur if the
// PeerConnection was torn down between SetRemoteOffer and the candidate
// being delivered.
func TestManager_AddICECandidate_NilPC(t *testing.T) {
	registry := ipc.NewConnRegistry()
	m := NewManager("edge-add-ice-nilpc", registry, nil)
	defer m.Close()

	// Simulate "remote already set" without going through SetRemoteOffer.
	m.pendingICEMu.Lock()
	m.remoteSet = true
	m.pendingICEMu.Unlock()

	// pc is still nil because CreatePeerConnection was never called.
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

// TestManager_SetRemoteOffer_NilPC verifies that SetRemoteOffer returns an
// error when pc has not been created.
func TestManager_SetRemoteOffer_NilPC(t *testing.T) {
	registry := ipc.NewConnRegistry()
	m := NewManager("edge-set-remote-nilpc", registry, nil)
	defer m.Close()

	_, err := m.SetRemoteOffer(context.Background(), "v=0\r\n")
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
		t.Error("remoteSet should remain false after nil-pc SetRemoteOffer")
	}
}

// TestManager_ConcurrentClose verifies that 100 goroutines calling Close()
// concurrently does not panic and the final state is stateClosed.
func TestManager_ConcurrentClose(t *testing.T) {
	registry := ipc.NewConnRegistry()
	m := NewManager("edge-concurrent-close", registry, nil)

	const goroutines = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			m.Close()
		}()
	}
	wg.Wait()

	if m.State() != stateClosed {
		t.Errorf("expected stateClosed, got %d", m.State())
	}
}
