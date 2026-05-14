package webrtc

import (
	"fmt"
	"log/slog"
	"sync"

	"github.com/outview/webrtc-sidecar/internal/ipc"
)

// Pool manages multiple WebRTC Managers, one per connectionID.
type Pool struct {
	mu       sync.RWMutex
	managers map[string]*Manager
	registry *ipc.ConnRegistry
	logger   *slog.Logger
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
func (p *Pool) Remove(connectionID string) {
	p.mu.Lock()
	m, ok := p.managers[connectionID]
	if ok {
		delete(p.managers, connectionID)
	}
	p.mu.Unlock()

	if m != nil {
		m.Close()
		p.logger.Info("Removed WebRTC manager", "connectionId", connectionID)
	}
}

// CloseAll closes all Managers.
func (p *Pool) CloseAll() {
	p.mu.Lock()
	managers := make([]*Manager, 0, len(p.managers))
	for _, m := range p.managers {
		managers = append(managers, m)
	}
	p.managers = make(map[string]*Manager)
	p.mu.Unlock()

	for _, m := range managers {
		m.Close()
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
