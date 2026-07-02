package ipc

import (
	"encoding/json"
	"net"
	"sync"
)

// ConnRegistry tracks active IPC client connections.
type ConnRegistry struct {
	mu    sync.RWMutex
	conns map[string]net.Conn // connectionID → IPC conn (for sending events back)
}

// NewConnRegistry creates a new ConnRegistry.
func NewConnRegistry() *ConnRegistry {
	return &ConnRegistry{conns: make(map[string]net.Conn)}
}

// Register associates a connectionID with an IPC connection.
func (r *ConnRegistry) Register(connectionID string, conn net.Conn) {
	r.mu.Lock()
	r.conns[connectionID] = conn
	r.mu.Unlock()
}

// Unregister removes a connectionID from the registry.
func (r *ConnRegistry) Unregister(connectionID string) {
	r.mu.Lock()
	delete(r.conns, connectionID)
	r.mu.Unlock()
}

// Get retrieves the IPC connection for a given connectionID.
func (r *ConnRegistry) Get(connectionID string) (net.Conn, bool) {
	r.mu.RLock()
	conn, ok := r.conns[connectionID]
	r.mu.RUnlock()
	return conn, ok
}

// SendEvent sends an event message back to the Java client for a given connectionID.
func (r *ConnRegistry) SendEvent(connectionID string, evt *EventPayload) error {
	conn, ok := r.Get(connectionID)
	if !ok {
		return nil // connection already gone, ignore
	}
	payload, err := json.Marshal(evt)
	if err != nil {
		return err
	}
	return WriteMessage(conn, &Message{Type: MsgEvent, Payload: payload})
}
