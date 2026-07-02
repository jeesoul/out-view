package webrtc

import (
	"context"
	"fmt"
	"sync"
	"testing"

	pionwebrtc "github.com/pion/webrtc/v4"
)

// TestICECandidateBuffering_BeforeRemoteSet verifies that candidates added
// before SetRemoteDescription are buffered into pendingICE rather than sent
// to the PeerConnection.
func TestICECandidateBuffering_BeforeRemoteSet(t *testing.T) {
	m := NewManager("sig-before-remote", DefaultConfig(), nil)
	defer m.Close()

	const n = 5
	for i := 0; i < n; i++ {
		c := pionwebrtc.ICECandidateInit{
			Candidate: fmt.Sprintf("candidate:%d 1 UDP 2130706431 192.168.1.1 5400%d typ host", i, i),
		}
		if err := m.AddICECandidate(context.Background(), c); err != nil {
			t.Fatalf("AddICECandidate %d: %v", i, err)
		}
	}

	m.pendingICEMu.Lock()
	count := len(m.pendingICE)
	remoteSet := m.remoteSet
	m.pendingICEMu.Unlock()

	if remoteSet {
		t.Error("remoteSet should be false before SetRemoteDescription")
	}
	if count != n {
		t.Errorf("expected %d buffered candidates, got %d", n, count)
	}
}

// TestICECandidateBuffering_AfterRemoteSet verifies that once remoteSet=true,
// candidates skip the buffer and go directly to pc.AddICECandidate. With pc
// nil this surfaces as the "peer connection not initialized" error, proving
// the buffering branch was bypassed.
func TestICECandidateBuffering_AfterRemoteSet(t *testing.T) {
	m := NewManager("sig-after-remote", DefaultConfig(), nil)
	defer m.Close()

	// Force the post-SetRemoteDescription state without a real PC.
	m.pendingICEMu.Lock()
	m.remoteSet = true
	m.pendingICEMu.Unlock()

	c := pionwebrtc.ICECandidateInit{
		Candidate: "candidate:1 1 UDP 2130706431 192.168.1.1 54321 typ host",
	}
	err := m.AddICECandidate(context.Background(), c)
	if err == nil {
		t.Fatal("expected error when remoteSet=true and pc is nil")
	}
	if err.Error() != "peer connection not initialized" {
		t.Errorf("expected \"peer connection not initialized\", got %q", err.Error())
	}

	// Confirm the candidate was NOT appended to pendingICE.
	m.pendingICEMu.Lock()
	count := len(m.pendingICE)
	m.pendingICEMu.Unlock()
	if count != 0 {
		t.Errorf("expected no buffered candidates after remoteSet, got %d", count)
	}
}

// TestICECandidateBuffering_RollbackOnFailure verifies that if
// SetRemoteDescription returns an error early (pc nil), neither remoteSet nor
// the pendingICE buffer is mutated. Future buffered candidates remain queued
// for a later successful SetRemoteDescription.
func TestICECandidateBuffering_RollbackOnFailure(t *testing.T) {
	m := NewManager("sig-rollback", DefaultConfig(), nil)
	defer m.Close()

	// Pre-populate the buffer with two candidates.
	pre := []pionwebrtc.ICECandidateInit{
		{Candidate: "candidate:1 1 UDP 2130706431 10.0.0.1 50001 typ host"},
		{Candidate: "candidate:2 1 UDP 2130706431 10.0.0.2 50002 typ host"},
	}
	for i, c := range pre {
		if err := m.AddICECandidate(context.Background(), c); err != nil {
			t.Fatalf("AddICECandidate %d: %v", i, err)
		}
	}

	// Sanity check.
	m.pendingICEMu.Lock()
	beforeCount := len(m.pendingICE)
	m.pendingICEMu.Unlock()
	if beforeCount != len(pre) {
		t.Fatalf("precondition: expected %d buffered, got %d", len(pre), beforeCount)
	}

	// Trigger failure: pc is nil so SetRemoteDescription returns early.
	sd := pionwebrtc.SessionDescription{Type: pionwebrtc.SDPTypeAnswer, SDP: "v=0\r\n"}
	if err := m.SetRemoteDescription(context.Background(), sd); err == nil {
		t.Fatal("expected error from SetRemoteDescription with nil pc")
	}

	// Verify rollback semantics: remoteSet stays false, candidates preserved.
	m.pendingICEMu.Lock()
	remoteSet := m.remoteSet
	afterCount := len(m.pendingICE)
	m.pendingICEMu.Unlock()

	if remoteSet {
		t.Error("remoteSet should remain false after SetRemoteDescription failure")
	}
	if afterCount != len(pre) {
		t.Errorf("expected %d preserved candidates, got %d", len(pre), afterCount)
	}

	// A subsequent AddICECandidate should still be buffered (i.e. the
	// pre-remote-set behaviour is still in effect).
	extra := pionwebrtc.ICECandidateInit{
		Candidate: "candidate:3 1 UDP 2130706431 10.0.0.3 50003 typ host",
	}
	if err := m.AddICECandidate(context.Background(), extra); err != nil {
		t.Fatalf("AddICECandidate after rollback: %v", err)
	}
	m.pendingICEMu.Lock()
	finalCount := len(m.pendingICE)
	m.pendingICEMu.Unlock()
	if finalCount != len(pre)+1 {
		t.Errorf("expected %d buffered after rollback+add, got %d", len(pre)+1, finalCount)
	}
}

// TestICECandidateBuffering_ConcurrentAdd verifies that 10 goroutines adding
// candidates concurrently before SetRemoteDescription all end up in the
// pending buffer, with no data races and no lost candidates.
func TestICECandidateBuffering_ConcurrentAdd(t *testing.T) {
	m := NewManager("sig-concurrent-add", DefaultConfig(), nil)
	defer m.Close()

	const goroutines = 10
	const perGoroutine = 5

	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func(idx int) {
			defer wg.Done()
			for j := 0; j < perGoroutine; j++ {
				c := pionwebrtc.ICECandidateInit{
					Candidate: fmt.Sprintf("candidate:%d-%d 1 UDP 2130706431 10.0.%d.%d 50000 typ host", idx, j, idx, j),
				}
				if err := m.AddICECandidate(context.Background(), c); err != nil {
					t.Errorf("goroutine %d AddICECandidate %d: %v", idx, j, err)
				}
			}
		}(g)
	}
	wg.Wait()

	m.pendingICEMu.Lock()
	count := len(m.pendingICE)
	m.pendingICEMu.Unlock()

	want := goroutines * perGoroutine
	if count != want {
		t.Errorf("expected %d buffered candidates, got %d", want, count)
	}
}

// TestICECandidateBuffering_FlushOrder verifies that buffered candidates are
// preserved in insertion order. The flush itself happens inside
// SetRemoteDescription, but we can still assert the contract: pendingICE is
// FIFO before the flush, since it is implemented as an append-only slice.
func TestICECandidateBuffering_FlushOrder(t *testing.T) {
	m := NewManager("sig-flush-order", DefaultConfig(), nil)
	defer m.Close()

	cands := []pionwebrtc.ICECandidateInit{
		{Candidate: "candidate:A 1 UDP 2130706431 10.0.0.1 50001 typ host"},
		{Candidate: "candidate:B 1 UDP 2130706431 10.0.0.2 50002 typ host"},
		{Candidate: "candidate:C 1 UDP 2130706431 10.0.0.3 50003 typ host"},
		{Candidate: "candidate:D 1 UDP 2130706431 10.0.0.4 50004 typ host"},
	}

	for i, c := range cands {
		if err := m.AddICECandidate(context.Background(), c); err != nil {
			t.Fatalf("AddICECandidate %d: %v", i, err)
		}
	}

	m.pendingICEMu.Lock()
	defer m.pendingICEMu.Unlock()

	if len(m.pendingICE) != len(cands) {
		t.Fatalf("expected %d buffered, got %d", len(cands), len(m.pendingICE))
	}
	for i, want := range cands {
		if m.pendingICE[i].Candidate != want.Candidate {
			t.Errorf("position %d: expected %q, got %q", i, want.Candidate, m.pendingICE[i].Candidate)
		}
	}
}
