package webrtc

import (
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/outview/webrtc-sidecar/internal/ipc"
)

// PoolStats holds snapshot statistics for a Pool.
type PoolStats struct {
	ActiveConnections int
	TotalCreated      int64
	TotalClosed       int64
}

// Pool manages multiple WebRTC Managers, one per connectionID.
type Pool struct {
	mu       sync.RWMutex
	managers map[string]*Manager
	registry *ipc.ConnRegistry
	logger   *slog.Logger

	onConnectionClosed func(connectionID string)

	totalCreated atomic.Int64
	totalClosed  atomic.Int64
}

// NewPool creates a new Pool.
func NewPool(registry *ipc.ConnRegistry, logger *slog.Logger) *Pool {
	if logger == nil {
		logger = slog.Default()
	}
	return &Pool{
		managers: make(map[string]*Manager),
		registry: registry,
		logger:   logger,
	}
}

// OnConnectionClosed registers a callback that is invoked whenever a connection
// is removed from the pool. The callback receives the connectionID of the closed
// connection. Only one callback can be registered at a time; calling this method
// again replaces the previous callback.
func (p *Pool) OnConnectionClosed(fn func(connectionID string)) {
	p.mu.Lock()
	p.onConnectionClosed = fn
	p.mu.Unlock()
}

// Create creates a new Manager for the given connectionID.
// Returns an error if a Manager already exists for that ID.
func (p *Pool) Create(connectionID string) (*Manager, error) {
	p.mu.Lock()
	defer p.mu.Unlock()

	if _, exists := p.managers[connectionID]; exists {
		return nil, fmt.Errorf("connection %q already exists", connectionID)
	}

	m := NewManager(connectionID, p.registry, p.logger)
	if err := m.CreatePeerConnection(); err != nil {
		return nil, fmt.Errorf("create peer connection for %q: %w", connectionID, err)
	}

	p.managers[connectionID] = m
	p.totalCreated.Add(1)
	p.logger.Info("Created WebRTC manager", "connectionId", connectionID, "total", len(p.managers))
	return m, nil
}

// Get returns the Manager for the given connectionID.
func (p *Pool) Get(connectionID string) (*Manager, bool) {
	p.mu.RLock()
	m, ok := p.managers[connectionID]
	p.mu.RUnlock()
	return m, ok
}

// Remove closes and removes the Manager for the given connectionID.
// If an OnConnectionClosed callback is registered it is invoked after the
// Manager is closed.
func (p *Pool) Remove(connectionID string) {
	p.mu.Lock()
	m, ok := p.managers[connectionID]
	if ok {
		delete(p.managers, connectionID)
	}
	cb := p.onConnectionClosed
	p.mu.Unlock()

	if m != nil {
		m.Close()
		p.totalClosed.Add(1)
		p.logger.Info("Removed WebRTC manager", "connectionId", connectionID)
		if cb != nil {
			cb(connectionID)
		}
	}
}

// CloseAll closes all Managers.
func (p *Pool) CloseAll() {
	p.mu.Lock()
	managers := make([]*Manager, 0, len(p.managers))
	ids := make([]string, 0, len(p.managers))
	for id, m := range p.managers {
		managers = append(managers, m)
		ids = append(ids, id)
	}
	p.managers = make(map[string]*Manager)
	cb := p.onConnectionClosed
	p.mu.Unlock()

	for i, m := range managers {
		m.Close()
		p.totalClosed.Add(1)
		if cb != nil {
			cb(ids[i])
		}
	}
	p.logger.Info("Closed all WebRTC managers")
}

// Count returns the number of active managers.
func (p *Pool) Count() int {
	p.mu.RLock()
	n := len(p.managers)
	p.mu.RUnlock()
	return n
}

// Stats returns a snapshot of pool statistics.
func (p *Pool) Stats() PoolStats {
	p.mu.RLock()
	active := len(p.managers)
	p.mu.RUnlock()
	return PoolStats{
		ActiveConnections: active,
		TotalCreated:      p.totalCreated.Load(),
		TotalClosed:       p.totalClosed.Load(),
	}
}

