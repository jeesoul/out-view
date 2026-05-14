// Package ipc implements the IPC server for Java ↔ Go communication.
// Protocol: [4 bytes big-endian length][JSON payload]
// Message structure: {"type": "...", "payload": {...}}
package ipc

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net"
	"sync"
	"sync/atomic"
	"time"
)

// Message is the standard IPC message format.
type Message struct {
	Type    string          `json:"type"`
	Payload json.RawMessage `json:"payload,omitempty"`
}

// Handler is a function that processes an incoming message and returns a response.
// Return nil to send no response.
type Handler func(msg *Message) *Message

// Server is the IPC server that listens for connections.
type Server struct {
	listener    net.Listener
	handler     Handler
	connCount   int64
	mu          sync.Mutex
	done        chan struct{}
	wg          sync.WaitGroup
}

// NewServer creates a new IPC server with the given message handler.
func NewServer(handler Handler) *Server {
	return &Server{
		handler: handler,
		done:    make(chan struct{}),
	}
}

// ListenTCP starts the server on a TCP address (e.g. "127.0.0.1:9999").
// Used as fallback on Windows where Unix sockets may not be available.
func (s *Server) ListenTCP(addr string) error {
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		return fmt.Errorf("ipc: listen tcp %s: %w", addr, err)
	}
	s.listener = ln
	log.Printf("[IPC] Listening on TCP %s", addr)
	go s.acceptLoop()
	return nil
}

// ListenUnix starts the server on a Unix domain socket path.
func (s *Server) ListenUnix(socketPath string) error {
	ln, err := net.Listen("unix", socketPath)
	if err != nil {
		return fmt.Errorf("ipc: listen unix %s: %w", socketPath, err)
	}
	s.listener = ln
	log.Printf("[IPC] Listening on Unix socket %s", socketPath)
	go s.acceptLoop()
	return nil
}

// Close shuts down the server and waits for all connections to finish.
func (s *Server) Close() {
	close(s.done)
	if s.listener != nil {
		s.listener.Close()
	}
	s.wg.Wait()
}

// ConnCount returns the current number of active connections.
func (s *Server) ConnCount() int64 {
	return atomic.LoadInt64(&s.connCount)
}

func (s *Server) acceptLoop() {
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.done:
				return
			default:
				log.Printf("[IPC] Accept error: %v", err)
				continue
			}
		}
		s.wg.Add(1)
		atomic.AddInt64(&s.connCount, 1)
		go s.handleConn(conn)
	}
}

func (s *Server) handleConn(conn net.Conn) {
	defer func() {
		conn.Close()
		atomic.AddInt64(&s.connCount, -1)
		s.wg.Done()
	}()

	for {
		// Read 4-byte big-endian length prefix
		var length uint32
		if err := binary.Read(conn, binary.BigEndian, &length); err != nil {
			if err != io.EOF {
				log.Printf("[IPC] Read length error: %v", err)
			}
			return
		}

		if length == 0 || length > 4*1024*1024 { // max 4MB
			log.Printf("[IPC] Invalid message length: %d", length)
			return
		}

		// Read JSON payload
		buf := make([]byte, length)
		if _, err := io.ReadFull(conn, buf); err != nil {
			log.Printf("[IPC] Read payload error: %v", err)
			return
		}

		var msg Message
		if err := json.Unmarshal(buf, &msg); err != nil {
			log.Printf("[IPC] JSON unmarshal error: %v", err)
			return
		}

		// Dispatch to handler
		resp := s.handler(&msg)
		if resp == nil {
			continue
		}

		// Write response
		if err := WriteMessage(conn, resp); err != nil {
			log.Printf("[IPC] Write response error: %v", err)
			return
		}
	}
}

// WriteMessage writes a message to a connection using the length-prefixed protocol.
func WriteMessage(w io.Writer, msg *Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("ipc: marshal message: %w", err)
	}

	length := uint32(len(data))
	if err := binary.Write(w, binary.BigEndian, length); err != nil {
		return fmt.Errorf("ipc: write length: %w", err)
	}
	if _, err := w.Write(data); err != nil {
		return fmt.Errorf("ipc: write payload: %w", err)
	}
	return nil
}

// ReadMessage reads a single message from a connection.
func ReadMessage(r io.Reader) (*Message, error) {
	var length uint32
	if err := binary.Read(r, binary.BigEndian, &length); err != nil {
		return nil, fmt.Errorf("ipc: read length: %w", err)
	}

	if length == 0 || length > 4*1024*1024 {
		return nil, fmt.Errorf("ipc: invalid message length: %d", length)
	}

	buf := make([]byte, length)
	if _, err := io.ReadFull(r, buf); err != nil {
		return nil, fmt.Errorf("ipc: read payload: %w", err)
	}

	var msg Message
	if err := json.Unmarshal(buf, &msg); err != nil {
		return nil, fmt.Errorf("ipc: unmarshal: %w", err)
	}
	return &msg, nil
}

// PingPayload is the payload for ping messages.
type PingPayload struct {
	Timestamp int64  `json:"timestamp"`
	Message   string `json:"message"`
}

// PongPayload is the payload for pong responses.
type PongPayload struct {
	Timestamp    int64  `json:"timestamp"`
	EchoMessage  string `json:"echo_message"`
	ServerTime   int64  `json:"server_time"`
}

// DefaultHandler is a simple ping-pong handler for POC validation.
func DefaultHandler(msg *Message) *Message {
	switch msg.Type {
	case "ping":
		var ping PingPayload
		if msg.Payload != nil {
			_ = json.Unmarshal(msg.Payload, &ping)
		}
		log.Printf("[IPC] Received ping: %s (ts=%d)", ping.Message, ping.Timestamp)

		pong := PongPayload{
			Timestamp:   ping.Timestamp,
			EchoMessage: ping.Message,
			ServerTime:  time.Now().UnixMilli(),
		}
		payload, _ := json.Marshal(pong)
		return &Message{
			Type:    "pong",
			Payload: payload,
		}

	default:
		log.Printf("[IPC] Unknown message type: %s", msg.Type)
		errPayload, _ := json.Marshal(map[string]string{"error": "unknown type: " + msg.Type})
		return &Message{
			Type:    "error",
			Payload: errPayload,
		}
	}
}
