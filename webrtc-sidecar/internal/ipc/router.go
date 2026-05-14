package ipc

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
)

// HandlerFunc handles a specific IPC message type.
// conn is the IPC connection the message arrived on.
// Returns a response message (nil = no response).
type HandlerFunc func(conn net.Conn, msg *Message) *Message

// Router dispatches IPC messages to registered handlers.
type Router struct {
	handlers map[string]HandlerFunc
	registry *ConnRegistry
}

// NewRouter creates a new Router with the given connection registry.
func NewRouter(registry *ConnRegistry) *Router {
	return &Router{
		handlers: make(map[string]HandlerFunc),
		registry: registry,
	}
}

// Handle registers a handler for a message type.
func (r *Router) Handle(msgType string, fn HandlerFunc) {
	r.handlers[msgType] = fn
}

// Dispatch routes a message to the appropriate handler.
// Implements the connHandler signature expected by Server.
func (r *Router) Dispatch(conn net.Conn, msg *Message) *Message {
	fn, ok := r.handlers[msg.Type]
	if !ok {
		log.Printf("[Router] Unknown message type: %s", msg.Type)
		errPayload, _ := json.Marshal(ErrorPayload{Error: "unknown type: " + msg.Type})
		return &Message{Type: "error", Payload: errPayload}
	}
	return fn(conn, msg)
}

// AsHandler returns a Handler-compatible function for use with Server.
// Note: this loses the conn reference; use ServeWithRouter for full routing.
func (r *Router) AsHandler() Handler {
	return func(msg *Message) *Message {
		return r.Dispatch(nil, msg)
	}
}

// MustUnmarshal is a helper to unmarshal payload with error response on failure.
func MustUnmarshal(payload json.RawMessage, v interface{}) error {
	if payload == nil {
		return fmt.Errorf("missing payload")
	}
	return json.Unmarshal(payload, v)
}
