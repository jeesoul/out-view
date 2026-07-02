package webrtc

import (
	"context"
	"fmt"
	"math/rand"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	pionwebrtc "github.com/pion/webrtc/v4"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func makeCandidate(id int) pionwebrtc.ICECandidateInit {
	return pionwebrtc.ICECandidateInit{
		Candidate: fmt.Sprintf("candidate:%d 1 UDP 2130706431 10.0.%d.%d 5000%d typ host",
			id, (id/256)&0xff, id&0xff, id%10),
	}
}

// pendingCount returns the number of candidates currently buffered.
func pendingCount(m *Manager) int {
	m.pendingICEMu.Lock()
	defer m.pendingICEMu.Unlock()
	return len(m.pendingICE)
}

// ---------------------------------------------------------------------------
// Scenario 1: ICE candidate 30% loss
// ---------------------------------------------------------------------------

// TestSignaling_ICECandidate30PercentLoss verifies that when 30% of ICE
// candidates are dropped before reaching AddICECandidate, the manager still
// buffers exactly the delivered subset with no error and no spurious
// duplicates. This simulates a noisy signaling channel where a fraction of
// candidate messages are lost in transit.
func TestSignaling_ICECandidate30PercentLoss(t *testing.T) {
	const (
		total     = 100
		lossRate  = 0.30
		seed      = int64(20260517)
		tolerance = 0.05 // ±5% slack on the observed loss fraction
	)

	m := NewManager("sig-loss-30", DefaultConfig(), nil)
	defer m.Close()

	rng := rand.New(rand.NewSource(seed))
	delivered := 0
	for i := 0; i < total; i++ {
		if rng.Float64() < lossRate {
			continue // simulated loss
		}
		delivered++
		if err := m.AddICECandidate(context.Background(), makeCandidate(i)); err != nil {
			t.Fatalf("AddICECandidate(%d): %v", i, err)
		}
	}

	got := pendingCount(m)
	if got != delivered {
		t.Errorf("expected %d buffered, got %d", delivered, got)
	}

	// Sanity-check the loss rate fell into the expected band.
	observed := 1 - float64(delivered)/float64(total)
	if observed < lossRate-tolerance || observed > lossRate+tolerance {
		t.Logf("observed loss rate %.3f outside ±%.0f%% of expected %.2f (informational)",
			observed, tolerance*100, lossRate)
	}
}

// ---------------------------------------------------------------------------
// Scenario 2: ANSWER delay 10s
// ---------------------------------------------------------------------------

// TestSignaling_AnswerDelay10s verifies that ICE candidates arriving while
// the SDP answer is delayed are buffered and not lost. The original spec
// names a 10s ceiling, but the test only needs to demonstrate the buffering
// invariant holds across a meaningful delay; we use 200ms in the unit-test
// loop to keep CI fast while still exercising the wait path.
func TestSignaling_AnswerDelay10s(t *testing.T) {
	const (
		preAnswer  = 8
		postAnswer = 4
		delay      = 200 * time.Millisecond
	)

	m := NewManager("sig-answer-delay", DefaultConfig(), nil)
	defer m.Close()

	// Phase 1: candidates arrive before SDP answer is processed.
	for i := 0; i < preAnswer; i++ {
		if err := m.AddICECandidate(context.Background(), makeCandidate(i)); err != nil {
			t.Fatalf("pre-answer AddICECandidate(%d): %v", i, err)
		}
	}

	// Simulate the answerer taking a long time to respond.
	time.Sleep(delay)

	// Phase 2: more candidates trickle in during the delay.
	for i := preAnswer; i < preAnswer+postAnswer; i++ {
		if err := m.AddICECandidate(context.Background(), makeCandidate(i)); err != nil {
			t.Fatalf("post-answer AddICECandidate(%d): %v", i, err)
		}
	}

	// Without a real PC the SDP can never be applied — but the contract under
	// test is "candidates remain buffered until SetRemoteDescription succeeds".
	if got := pendingCount(m); got != preAnswer+postAnswer {
		t.Errorf("expected %d buffered after answer delay, got %d", preAnswer+postAnswer, got)
	}

	// Verify remoteSet is still false: nothing has flushed.
	m.pendingICEMu.Lock()
	rs := m.remoteSet
	m.pendingICEMu.Unlock()
	if rs {
		t.Error("remoteSet should remain false while answer is still pending")
	}
}

// ---------------------------------------------------------------------------
// Scenario 3: Message reorder
// ---------------------------------------------------------------------------

// TestSignaling_MessageReorder verifies that candidates delivered out of
// their original numbering order are stored in arrival order, not original
// order. A reordering signaling channel must not corrupt the buffer; the
// downstream pion stack tolerates any ordering, so the only invariant is
// "no candidate is dropped or duplicated".
func TestSignaling_MessageReorder(t *testing.T) {
	const n = 20
	m := NewManager("sig-reorder", DefaultConfig(), nil)
	defer m.Close()

	// Build a deterministic reordering of [0..n).
	order := rand.New(rand.NewSource(42)).Perm(n)

	for _, idx := range order {
		if err := m.AddICECandidate(context.Background(), makeCandidate(idx)); err != nil {
			t.Fatalf("AddICECandidate(%d): %v", idx, err)
		}
	}

	m.pendingICEMu.Lock()
	defer m.pendingICEMu.Unlock()

	if len(m.pendingICE) != n {
		t.Fatalf("expected %d buffered, got %d", n, len(m.pendingICE))
	}

	// Each delivered candidate must be present exactly once. Build a histogram
	// from the buffer and verify every original ID is accounted for.
	seen := make(map[string]int, n)
	for _, c := range m.pendingICE {
		seen[c.Candidate]++
	}
	for i := 0; i < n; i++ {
		want := makeCandidate(i).Candidate
		if seen[want] != 1 {
			t.Errorf("candidate %d: expected exactly 1 occurrence, got %d", i, seen[want])
		}
	}

	// And confirm arrival-order preservation: the i-th buffered slot matches
	// the i-th element of the permutation.
	for i, idx := range order {
		want := makeCandidate(idx).Candidate
		if m.pendingICE[i].Candidate != want {
			t.Errorf("position %d: got %q, want %q", i, m.pendingICE[i].Candidate, want)
		}
	}
}

// ---------------------------------------------------------------------------
// Scenario 4: Signaling delay 5s
// ---------------------------------------------------------------------------

// TestSignaling_Delay5s verifies that a sustained per-candidate signaling
// delay does not cause data races, lost candidates, or buffer corruption.
// The 5s figure in the spec is the worst-case ceiling on a slow channel; we
// use a short cumulative budget here (~250ms) to validate the same property
// in CI: every candidate eventually lands in the buffer.
func TestSignaling_Delay5s(t *testing.T) {
	const (
		count   = 10
		perStep = 25 * time.Millisecond
	)

	m := NewManager("sig-delay-5s", DefaultConfig(), nil)
	defer m.Close()

	for i := 0; i < count; i++ {
		time.Sleep(perStep)
		if err := m.AddICECandidate(context.Background(), makeCandidate(i)); err != nil {
			t.Fatalf("AddICECandidate(%d) after delay: %v", i, err)
		}
	}

	if got := pendingCount(m); got != count {
		t.Errorf("expected %d buffered after delayed delivery, got %d", count, got)
	}
}

// ---------------------------------------------------------------------------
// Scenario 5: Combined fault injection
// ---------------------------------------------------------------------------

// TestSignaling_CombinedFaults stresses the manager with all four signaling
// fault types running concurrently: random loss, reorder, sustained delay,
// and goroutine-level concurrency. The invariant is "delivered count equals
// buffered count" with no panics or races.
func TestSignaling_CombinedFaults(t *testing.T) {
	const (
		producers     = 4
		perProducer   = 25
		lossRate      = 0.30
		seed          = int64(20260517)
		maxDelayJitt  = 5 * time.Millisecond
	)

	m := NewManager("sig-combined", DefaultConfig(), nil)
	defer m.Close()

	var deliveredTotal atomic.Int64
	var wg sync.WaitGroup
	wg.Add(producers)
	for p := 0; p < producers; p++ {
		go func(producerID int) {
			defer wg.Done()
			rng := rand.New(rand.NewSource(seed + int64(producerID)))
			// Each producer reorders its own slice.
			indices := rng.Perm(perProducer)
			for _, j := range indices {
				if rng.Float64() < lossRate {
					continue
				}
				// Tiny jitter to interleave goroutines.
				time.Sleep(time.Duration(rng.Int63n(int64(maxDelayJitt))))
				id := producerID*1000 + j
				if err := m.AddICECandidate(context.Background(), makeCandidate(id)); err != nil {
					t.Errorf("producer %d AddICECandidate(%d): %v", producerID, j, err)
					return
				}
				deliveredTotal.Add(1)
			}
		}(p)
	}
	wg.Wait()

	got := pendingCount(m)
	want := int(deliveredTotal.Load())
	if got != want {
		t.Errorf("buffered count diverged from delivered count: buffered=%d, delivered=%d", got, want)
	}
}
