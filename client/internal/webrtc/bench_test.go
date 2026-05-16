package webrtc

import (
	"testing"
)

// BenchmarkStateLoad measures the cost of an atomic state read.
func BenchmarkStateLoad(b *testing.B) {
	m := NewManager("bench-state", DefaultConfig(), nil)
	defer m.Close()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = m.State()
	}
}

// BenchmarkStateTransition measures the cost of requesting a state transition.
// Drives a valid Idle -> GatheringICE cycle by re-creating the manager when
// terminal states are reached. The hot path being measured is the channel
// send in requestStateTransition.
func BenchmarkStateTransition(b *testing.B) {
	m := NewManager("bench-state-trans", DefaultConfig(), nil)
	defer m.Close()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		m.requestStateTransition(StateGatheringICE, "bench")
	}
}

// BenchmarkManagerStats measures the cost of a Stats() snapshot.
func BenchmarkManagerStats(b *testing.B) {
	m := NewManager("bench-stats", DefaultConfig(), nil)
	defer m.Close()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = m.Stats()
	}
}

// BenchmarkIsConnected measures the cost of an IsConnected() check.
func BenchmarkIsConnected(b *testing.B) {
	m := NewManager("bench-isconn", DefaultConfig(), nil)
	defer m.Close()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = m.IsConnected()
	}
}

// BenchmarkFallbackCount measures the cost of FallbackCount() reads.
func BenchmarkFallbackCount(b *testing.B) {
	m := NewManager("bench-fbcount", DefaultConfig(), nil)
	defer m.Close()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = m.FallbackCount()
	}
}

// BenchmarkMetricsRecord measures the cost of recording a connection lifecycle.
func BenchmarkMetricsRecord(b *testing.B) {
	mt := NewMetrics()
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		mt.RecordConnectionAttempt()
		mt.RecordConnectionSuccess(1)
		mt.RecordConnectionClosed()
	}
}

// BenchmarkMetricsSnapshot measures the cost of computing percentiles.
func BenchmarkMetricsSnapshot(b *testing.B) {
	mt := NewMetrics()
	for i := 0; i < 100; i++ {
		mt.RecordConnectionSuccess(1)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		_ = mt.Snapshot()
	}
}
