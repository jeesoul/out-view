package webrtc

import (
	"sync"
	"sync/atomic"
	"time"
)

// bufferPoolSize is the default capacity for buffers borrowed from bufferPool.
// It matches the typical SCTP datagram size used by the DataChannel send path,
// avoiding most reallocations while still being small enough to keep pooled
// buffers cheap to retain in steady state.
const bufferPoolSize = 16 * 1024

// bufferPool is a process-wide sync.Pool of reusable byte slices.
// Use GetBuffer / PutBuffer rather than touching the pool directly so that
// length and capacity are normalized on every borrow.
var bufferPool = sync.Pool{
	New: func() any {
		buf := make([]byte, 0, bufferPoolSize)
		return &buf
	},
}

// GetBuffer borrows a byte slice from the pool with at least minCap capacity.
// The returned slice has length 0; the caller must append into it.
// Buffers larger than the requested size are reused as-is to avoid reallocation
// churn when callers oscillate between small payload sizes.
func GetBuffer(minCap int) []byte {
	bp := bufferPool.Get().(*[]byte)
	buf := *bp
	if cap(buf) < minCap {
		buf = make([]byte, 0, minCap)
	}
	return buf[:0]
}

// PutBuffer returns a buffer to the pool. Buffers larger than 1 MiB are dropped
// on the floor instead of pooled, to bound steady-state memory use.
func PutBuffer(buf []byte) {
	if cap(buf) == 0 || cap(buf) > 1024*1024 {
		return
	}
	buf = buf[:0]
	bufferPool.Put(&buf)
}

// BatchSenderConfig configures a BatchSender. Zero values fall back to defaults
// tuned for interactive RDP traffic (small payloads, low latency).
type BatchSenderConfig struct {
	// MaxBatchBytes flushes the batch once accumulated payload reaches this size.
	// Default: 32 KiB.
	MaxBatchBytes int
	// MaxBatchCount flushes the batch once it contains this many messages.
	// Default: 32.
	MaxBatchCount int
	// FlushInterval forces a flush this long after the first buffered message.
	// Default: 5 ms. Set to 0 to disable timed flushes.
	FlushInterval time.Duration
}

func (c BatchSenderConfig) withDefaults() BatchSenderConfig {
	if c.MaxBatchBytes <= 0 {
		c.MaxBatchBytes = 32 * 1024
	}
	if c.MaxBatchCount <= 0 {
		c.MaxBatchCount = 32
	}
	if c.FlushInterval == 0 {
		c.FlushInterval = 5 * time.Millisecond
	}
	return c
}

// BatchSender coalesces small writes into larger batches before invoking
// the supplied flush function. It is safe for concurrent use.
//
// Lock granularity: a single sync.Mutex guards the pending buffer; the flush
// callback is invoked outside the lock so callers may re-enter Send without
// risk of self-deadlock when their flush path is asynchronous.
type BatchSender struct {
	cfg   BatchSenderConfig
	flush func([]byte) error

	mu       sync.Mutex
	pending  []byte
	count    int
	deadline time.Time

	// stats kept under atomics so callers can observe throughput without
	// contending with senders.
	flushes  atomic.Uint64
	bytesOut atomic.Uint64
	msgsIn   atomic.Uint64
}

// NewBatchSender constructs a BatchSender that delegates batched writes to flushFn.
// flushFn is called synchronously inside Send / Flush; it must not block forever.
func NewBatchSender(cfg BatchSenderConfig, flushFn func([]byte) error) *BatchSender {
	return &BatchSender{
		cfg:     cfg.withDefaults(),
		flush:   flushFn,
		pending: GetBuffer(cfg.withDefaults().MaxBatchBytes),
	}
}

// Send appends data to the in-flight batch and flushes when any threshold trips.
// The data slice is copied; callers may reuse it as soon as Send returns.
func (b *BatchSender) Send(data []byte) error {
	if len(data) == 0 {
		return nil
	}
	b.msgsIn.Add(1)

	b.mu.Lock()
	if b.count == 0 && b.cfg.FlushInterval > 0 {
		b.deadline = time.Now().Add(b.cfg.FlushInterval)
	}
	b.pending = append(b.pending, data...)
	b.count++

	flushNow := len(b.pending) >= b.cfg.MaxBatchBytes ||
		b.count >= b.cfg.MaxBatchCount ||
		(b.cfg.FlushInterval > 0 && !b.deadline.IsZero() && time.Now().After(b.deadline))

	if !flushNow {
		b.mu.Unlock()
		return nil
	}
	payload := b.pending
	b.pending = GetBuffer(b.cfg.MaxBatchBytes)
	b.count = 0
	b.deadline = time.Time{}
	b.mu.Unlock()

	return b.dispatch(payload)
}

// Flush forces any pending batch to be written. Safe to call when idle.
func (b *BatchSender) Flush() error {
	b.mu.Lock()
	if b.count == 0 {
		b.mu.Unlock()
		return nil
	}
	payload := b.pending
	b.pending = GetBuffer(b.cfg.MaxBatchBytes)
	b.count = 0
	b.deadline = time.Time{}
	b.mu.Unlock()

	return b.dispatch(payload)
}

func (b *BatchSender) dispatch(payload []byte) error {
	b.flushes.Add(1)
	b.bytesOut.Add(uint64(len(payload)))
	err := b.flush(payload)
	PutBuffer(payload)
	return err
}

// BatchStats reports counters tracked by the BatchSender.
type BatchStats struct {
	Flushes  uint64
	BytesOut uint64
	MsgsIn   uint64
}

// Stats returns a snapshot of BatchSender counters.
func (b *BatchSender) Stats() BatchStats {
	return BatchStats{
		Flushes:  b.flushes.Load(),
		BytesOut: b.bytesOut.Load(),
		MsgsIn:   b.msgsIn.Load(),
	}
}
