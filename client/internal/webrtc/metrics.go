package webrtc

import (
	"sort"
	"sync/atomic"
	"time"
)

const establishSampleSize = 100

// Metrics tracks WebRTC connection statistics.
//
// All counters are lock-free via sync/atomic. The establish-duration ring
// buffer is also lock-free; readers compute percentiles from a snapshot of
// the buffer, so values may be slightly stale during heavy concurrent
// updates, but they are never torn.
type Metrics struct {
	connectionsTotal  atomic.Int64
	connectionsActive atomic.Int64
	successCount      atomic.Int64
	fallbackCount     atomic.Int64
	errorsTotal       atomic.Int64

	// establishSamples holds the most recent connection-establishment
	// durations (in nanoseconds). Indexed by sampleIdx % establishSampleSize.
	establishSamples [establishSampleSize]atomic.Int64
	sampleIdx        atomic.Int64

	startTime time.Time
}

// NewMetrics creates a new Metrics instance.
func NewMetrics() *Metrics {
	return &Metrics{startTime: time.Now()}
}

// RecordConnectionAttempt increments total and active connection counts.
func (m *Metrics) RecordConnectionAttempt() {
	m.connectionsTotal.Add(1)
	m.connectionsActive.Add(1)
}

// RecordConnectionSuccess records a successful WebRTC connection.
// The establish duration is recorded into the ring buffer for percentile reporting.
func (m *Metrics) RecordConnectionSuccess(establishDuration time.Duration) {
	m.successCount.Add(1)
	idx := m.sampleIdx.Add(1) - 1
	m.establishSamples[idx%establishSampleSize].Store(int64(establishDuration))
}

// RecordConnectionClosed decrements active count.
func (m *Metrics) RecordConnectionClosed() {
	m.connectionsActive.Add(-1)
}

// RecordFallback records a TCP fallback event.
func (m *Metrics) RecordFallback() {
	m.fallbackCount.Add(1)
}

// RecordError records an error.
func (m *Metrics) RecordError() {
	m.errorsTotal.Add(1)
}

// MetricsSnapshot is a point-in-time snapshot of all metrics.
type MetricsSnapshot struct {
	ConnectionsTotal  int64
	ConnectionsActive int64
	SuccessCount      int64
	FallbackCount     int64
	ErrorsTotal       int64
	SuccessRate       float64
	FallbackRate      float64
	EstablishP50Ms    float64
	EstablishP95Ms    float64
	EstablishP99Ms    float64
	UptimeSeconds     float64
}

// Snapshot returns a point-in-time snapshot.
func (m *Metrics) Snapshot() MetricsSnapshot {
	total := m.connectionsTotal.Load()
	success := m.successCount.Load()
	fallback := m.fallbackCount.Load()

	snap := MetricsSnapshot{
		ConnectionsTotal:  total,
		ConnectionsActive: m.connectionsActive.Load(),
		SuccessCount:      success,
		FallbackCount:     fallback,
		ErrorsTotal:       m.errorsTotal.Load(),
		UptimeSeconds:     time.Since(m.startTime).Seconds(),
	}
	if total > 0 {
		snap.SuccessRate = float64(success) / float64(total)
		snap.FallbackRate = float64(fallback) / float64(total)
	}

	p50, p95, p99 := m.percentilesMs()
	snap.EstablishP50Ms = p50
	snap.EstablishP95Ms = p95
	snap.EstablishP99Ms = p99
	return snap
}

// percentilesMs computes the p50/p95/p99 establish durations (in ms) over
// the populated portion of the ring buffer.
func (m *Metrics) percentilesMs() (p50, p95, p99 float64) {
	idx := m.sampleIdx.Load()
	if idx <= 0 {
		return 0, 0, 0
	}
	n := idx
	if n > establishSampleSize {
		n = establishSampleSize
	}
	samples := make([]int64, 0, n)
	for i := int64(0); i < n; i++ {
		v := m.establishSamples[i].Load()
		if v > 0 {
			samples = append(samples, v)
		}
	}
	if len(samples) == 0 {
		return 0, 0, 0
	}
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })
	return percentileMs(samples, 0.50),
		percentileMs(samples, 0.95),
		percentileMs(samples, 0.99)
}

// percentileMs returns the p-th percentile of the sorted nanosecond samples,
// converted to milliseconds. p must be in [0, 1].
func percentileMs(sortedNs []int64, p float64) float64 {
	if len(sortedNs) == 0 {
		return 0
	}
	idx := int(float64(len(sortedNs)-1) * p)
	if idx < 0 {
		idx = 0
	}
	if idx >= len(sortedNs) {
		idx = len(sortedNs) - 1
	}
	return float64(sortedNs[idx]) / float64(time.Millisecond)
}
