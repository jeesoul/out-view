package webrtc

import (
	"sync"
	"testing"
	"time"
)

func TestMetrics_InitialState(t *testing.T) {
	m := NewMetrics()
	snap := m.Snapshot()
	if snap.ConnectionsTotal != 0 {
		t.Errorf("expected 0 total, got %d", snap.ConnectionsTotal)
	}
	if snap.ConnectionsActive != 0 {
		t.Errorf("expected 0 active, got %d", snap.ConnectionsActive)
	}
	if snap.SuccessRate != 0 {
		t.Errorf("expected 0 SuccessRate, got %v", snap.SuccessRate)
	}
	if snap.FallbackRate != 0 {
		t.Errorf("expected 0 FallbackRate, got %v", snap.FallbackRate)
	}
	if snap.UptimeSeconds < 0 {
		t.Errorf("UptimeSeconds must be >= 0, got %v", snap.UptimeSeconds)
	}
}

func TestMetrics_RecordAttemptAndClose(t *testing.T) {
	m := NewMetrics()
	m.RecordConnectionAttempt()
	m.RecordConnectionAttempt()
	snap := m.Snapshot()
	if snap.ConnectionsTotal != 2 {
		t.Errorf("expected 2 total, got %d", snap.ConnectionsTotal)
	}
	if snap.ConnectionsActive != 2 {
		t.Errorf("expected 2 active, got %d", snap.ConnectionsActive)
	}

	m.RecordConnectionClosed()
	snap = m.Snapshot()
	if snap.ConnectionsActive != 1 {
		t.Errorf("expected 1 active after close, got %d", snap.ConnectionsActive)
	}
}

func TestMetrics_SuccessRate(t *testing.T) {
	m := NewMetrics()
	for i := 0; i < 4; i++ {
		m.RecordConnectionAttempt()
	}
	m.RecordConnectionSuccess(10 * time.Millisecond)
	m.RecordConnectionSuccess(20 * time.Millisecond)
	m.RecordConnectionSuccess(30 * time.Millisecond)

	snap := m.Snapshot()
	if snap.SuccessCount != 3 {
		t.Errorf("expected 3 success, got %d", snap.SuccessCount)
	}
	if snap.SuccessRate != 0.75 {
		t.Errorf("expected SuccessRate 0.75, got %v", snap.SuccessRate)
	}
}

func TestMetrics_FallbackRate(t *testing.T) {
	m := NewMetrics()
	for i := 0; i < 10; i++ {
		m.RecordConnectionAttempt()
	}
	for i := 0; i < 2; i++ {
		m.RecordFallback()
	}
	snap := m.Snapshot()
	if snap.FallbackCount != 2 {
		t.Errorf("expected 2 fallback, got %d", snap.FallbackCount)
	}
	if snap.FallbackRate != 0.2 {
		t.Errorf("expected FallbackRate 0.2, got %v", snap.FallbackRate)
	}
}

func TestMetrics_Errors(t *testing.T) {
	m := NewMetrics()
	m.RecordError()
	m.RecordError()
	if got := m.Snapshot().ErrorsTotal; got != 2 {
		t.Errorf("expected 2 errors, got %d", got)
	}
}

func TestMetrics_PercentilesEmpty(t *testing.T) {
	m := NewMetrics()
	snap := m.Snapshot()
	if snap.EstablishP50Ms != 0 || snap.EstablishP95Ms != 0 || snap.EstablishP99Ms != 0 {
		t.Errorf("expected zero percentiles when no samples, got %+v", snap)
	}
}

func TestMetrics_PercentilesRanked(t *testing.T) {
	m := NewMetrics()
	// Insert durations 1ms..100ms in ascending order.
	for i := 1; i <= 100; i++ {
		m.RecordConnectionSuccess(time.Duration(i) * time.Millisecond)
	}
	snap := m.Snapshot()
	// p50 of [1..100] -> index ~49 -> 50ms
	if snap.EstablishP50Ms < 49 || snap.EstablishP50Ms > 52 {
		t.Errorf("p50 expected ~50ms, got %v", snap.EstablishP50Ms)
	}
	// p95 -> index 94 -> 95ms
	if snap.EstablishP95Ms < 93 || snap.EstablishP95Ms > 97 {
		t.Errorf("p95 expected ~95ms, got %v", snap.EstablishP95Ms)
	}
	// p99 -> index 98 -> 99ms
	if snap.EstablishP99Ms < 97 || snap.EstablishP99Ms > 100 {
		t.Errorf("p99 expected ~99ms, got %v", snap.EstablishP99Ms)
	}
}

func TestMetrics_PercentilesRingOverwrite(t *testing.T) {
	m := NewMetrics()
	// Fill 200 samples; only the last 100 should remain.
	for i := 1; i <= 200; i++ {
		m.RecordConnectionSuccess(time.Duration(i) * time.Millisecond)
	}
	snap := m.Snapshot()
	// Min retained ~101ms, max ~200ms; p50 ~150ms.
	if snap.EstablishP50Ms < 145 || snap.EstablishP50Ms > 155 {
		t.Errorf("p50 expected ~150ms after ring overwrite, got %v", snap.EstablishP50Ms)
	}
}

func TestMetrics_ConcurrentRecording(t *testing.T) {
	m := NewMetrics()
	const goroutines = 20
	const perG = 100
	var wg sync.WaitGroup
	wg.Add(goroutines)
	for g := 0; g < goroutines; g++ {
		go func() {
			defer wg.Done()
			for i := 0; i < perG; i++ {
				m.RecordConnectionAttempt()
				m.RecordConnectionSuccess(time.Millisecond)
				m.RecordConnectionClosed()
			}
		}()
	}
	wg.Wait()
	snap := m.Snapshot()
	want := int64(goroutines * perG)
	if snap.ConnectionsTotal != want {
		t.Errorf("expected %d total, got %d", want, snap.ConnectionsTotal)
	}
	if snap.SuccessCount != want {
		t.Errorf("expected %d success, got %d", want, snap.SuccessCount)
	}
	if snap.ConnectionsActive != 0 {
		t.Errorf("expected 0 active after equal close, got %d", snap.ConnectionsActive)
	}
}
