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

// BenchmarkSendData measures the cost of pushing a small payload through the
// BatchSender / sync.Pool fast path with a no-op flush sink. This isolates the
// pool + batching overhead from real DataChannel send latency, which depends
// on a connected peer and is not reproducible in CI.
func BenchmarkSendData(b *testing.B) {
	payload := make([]byte, 256)
	for i := range payload {
		payload[i] = byte(i)
	}
	bs := NewBatchSender(BatchSenderConfig{
		MaxBatchBytes: 32 * 1024,
		MaxBatchCount: 32,
		FlushInterval: 0,
	}, func(p []byte) error { return nil })

	b.ResetTimer()
	b.ReportAllocs()
	b.SetBytes(int64(len(payload)))
	for i := 0; i < b.N; i++ {
		if err := bs.Send(payload); err != nil {
			b.Fatalf("send: %v", err)
		}
	}
	b.StopTimer()
	_ = bs.Flush()
}

// BenchmarkBufferPool measures the steady-state cost of borrowing and
// returning a buffer from the sync.Pool. The Get/Put pair should report 0
// allocations once the pool warms up.
func BenchmarkBufferPool(b *testing.B) {
	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		buf := GetBuffer(1024)
		PutBuffer(buf)
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
