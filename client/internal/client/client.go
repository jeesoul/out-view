package client

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/outview/client/internal/protocol"
)

// State represents the client state
type State int32

const (
	StateDisconnected State = iota
	StateConnecting
	StateConnected
	StateRegistered
)

// String returns the string representation of the state
func (s State) String() string {
	switch s {
	case StateDisconnected:
		return "Disconnected"
	case StateConnecting:
		return "Connecting"
	case StateConnected:
		return "Connected"
	case StateRegistered:
		return "Registered"
	default:
		return "Unknown"
	}
}

// Client is the outView client
type Client struct {
	config *Config

	conn   net.Conn
	reader *bufio.Reader
	writer *bufio.Writer

	state     atomic.Int32
	externalPort int

	proxyManager *ProxyManager

	// Connection ID -> local RDP connection
	localConnections map[string]*connectionConn
	connMu           sync.Mutex

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu       sync.Mutex // Protects writer
	writeMu  sync.Mutex // Dedicated mutex for write operations

	// Callbacks
	OnStateChange    func(old, new State)
	OnRegisterResult func(success bool, externalPort int, err error)
	OnDataReceived   func(data []byte)
	OnError          func(err error)
}

// NewClient creates a new client
func NewClient(config *Config) *Client {
	ctx, cancel := context.WithCancel(context.Background())
	return &Client{
		config:           config,
		proxyManager:     NewProxyManager(),
		localConnections: make(map[string]*connectionConn),
		ctx:              ctx,
		cancel:           cancel,
	}
}

// Connect connects to the server
func (c *Client) Connect() error {
	c.setState(StateConnecting)

	dialer := &net.Dialer{
		Timeout: 10 * time.Second,
	}

	conn, err := dialer.DialContext(c.ctx, "tcp", c.config.ServerAddr())
	if err != nil {
		c.setState(StateDisconnected)
		return fmt.Errorf("failed to connect to server: %w", err)
	}

	c.mu.Lock()
	c.conn = conn
	c.reader = bufio.NewReader(conn)
	c.writer = bufio.NewWriter(conn)
	c.mu.Unlock()

	c.setState(StateConnected)

	// Start read goroutine
	c.wg.Add(1)
	go c.readLoop()

	return nil
}

// Register sends a registration request
func (c *Client) Register() error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	c.mu.Lock()
	writer := c.writer
	c.mu.Unlock()

	if writer == nil {
		return fmt.Errorf("not connected")
	}

	msg, err := protocol.NewRegisterMessage(c.config.DeviceID, c.config.Token, c.config.LocalPort)
	if err != nil {
		return fmt.Errorf("failed to create register message: %w", err)
	}

	encoder := protocol.NewEncoder(writer)
	if err := encoder.Encode(msg); err != nil {
		return fmt.Errorf("failed to send register message: %w", err)
	}

	if err := writer.Flush(); err != nil {
		return fmt.Errorf("failed to flush: %w", err)
	}

	return nil
}

// SendHeartbeat sends a heartbeat message
func (c *Client) SendHeartbeat() error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	c.mu.Lock()
	writer := c.writer
	c.mu.Unlock()

	if writer == nil {
		return fmt.Errorf("not connected")
	}

	msg, err := protocol.NewHeartbeatMessage()
	if err != nil {
		return fmt.Errorf("failed to create heartbeat message: %w", err)
	}

	encoder := protocol.NewEncoder(writer)
	if err := encoder.Encode(msg); err != nil {
		return fmt.Errorf("failed to send heartbeat message: %w", err)
	}

	if err := writer.Flush(); err != nil {
		return fmt.Errorf("failed to flush: %w", err)
	}

	return nil
}

// SendData sends a data message
func (c *Client) SendData(data []byte) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	c.mu.Lock()
	writer := c.writer
	c.mu.Unlock()

	if writer == nil {
		return fmt.Errorf("not connected")
	}

	msg := protocol.NewDataMessage(data)
	encoder := protocol.NewEncoder(writer)
	if err := encoder.Encode(msg); err != nil {
		return fmt.Errorf("failed to send data message: %w", err)
	}

	if err := writer.Flush(); err != nil {
		return fmt.Errorf("failed to flush: %w", err)
	}

	return nil
}

// Start starts the client with auto heartbeat
func (c *Client) Start() error {
	if err := c.Connect(); err != nil {
		return err
	}

	// Send register
	if err := c.Register(); err != nil {
		return err
	}

	// Start heartbeat goroutine
	c.wg.Add(1)
	go c.heartbeatLoop()

	return nil
}

// Stop stops the client
func (c *Client) Stop() {
	c.cancel()

	c.mu.Lock()
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
	c.mu.Unlock()

	// Close all local connections
	c.connMu.Lock()
	for _, connObj := range c.localConnections {
		connObj.once.Do(func() {
			close(connObj.closeCh)
			connObj.conn.Close()
		})
	}
	c.localConnections = make(map[string]*connectionConn)
	c.connMu.Unlock()

	c.proxyManager.CloseAll()
	c.wg.Wait()
	c.setState(StateDisconnected)
}

// GetState returns the current state
func (c *Client) GetState() State {
	return State(c.state.Load())
}

// GetExternalPort returns the assigned external port
func (c *Client) GetExternalPort() int {
	return c.externalPort
}

func (c *Client) setState(state State) {
	old := State(c.state.Swap(int32(state)))
	if old != state && c.OnStateChange != nil {
		c.OnStateChange(old, state)
	}
}

func (c *Client) readLoop() {
	defer c.wg.Done()

	decoder := protocol.NewDecoder(c.reader)

	for {
		select {
		case <-c.ctx.Done():
			return
		default:
		}

		msg, err := decoder.Decode()
		if err != nil {
			select {
			case <-c.ctx.Done():
				return
			default:
			}

			c.setState(StateDisconnected)
			if c.OnError != nil {
				c.OnError(fmt.Errorf("read error: %w", err))
			}
			return
		}

		c.handleMessage(msg)
	}
}

func (c *Client) handleMessage(msg *protocol.Message) {
	switch msg.Header.Type {
	case protocol.TypeRegisterAck:
		c.handleRegisterAck(msg)
	case protocol.TypeHeartbeatAck:
		c.handleHeartbeatAck(msg)
	case protocol.TypeData:
		c.handleData(msg)
	case protocol.TypeError:
		c.handleError(msg)
	default:
		if c.OnError != nil {
			c.OnError(fmt.Errorf("unknown message type: %d", msg.Header.Type))
		}
	}
}

func (c *Client) handleRegisterAck(msg *protocol.Message) {
	resp, err := protocol.ParseRegisterResponse(msg.Body)
	if err != nil {
		if c.OnRegisterResult != nil {
			c.OnRegisterResult(false, 0, err)
		}
		return
	}

	if resp.Success {
		c.externalPort = resp.ExternalPort
		c.setState(StateRegistered)
	} else {
		c.setState(StateConnected)
	}

	if c.OnRegisterResult != nil {
		var regErr error
		if !resp.Success {
			regErr = fmt.Errorf("registration failed: %s", resp.Message)
		}
		c.OnRegisterResult(resp.Success, resp.ExternalPort, regErr)
	}
}

func (c *Client) handleHeartbeatAck(msg *protocol.Message) {
	// Heartbeat acknowledged, nothing special to do
}

func (c *Client) handleData(msg *protocol.Message) {
	// Parse the data packet to get connection ID
	packet, err := protocol.ParseDataPacket(msg.Body)
	if err != nil {
		if c.OnError != nil {
			c.OnError(fmt.Errorf("failed to parse data packet: %w", err))
		}
		return
	}

	fmt.Printf("[Debug] Received data: connectionId=%s, len=%d\n", packet.ConnectionID, len(packet.Data))

	if c.OnDataReceived != nil {
		c.OnDataReceived(packet.Data)
	}

	// Forward to local service using connection ID
	c.forwardToLocal(packet.ConnectionID, packet.Data)
}

// connectionConn tracks a connection to local service
type connectionConn struct {
	conn     net.Conn
	closeCh  chan struct{}
	once     sync.Once
}

// forwardToLocal forwards data to local service, maintaining connection per connectionID
func (c *Client) forwardToLocal(connectionID string, data []byte) {
	// Get or create connection for this connection ID
	connObj := c.getOrCreateConnection(connectionID)
	if connObj == nil {
		fmt.Printf("[Error] Failed to create connection for connectionId=%s\n", connectionID)
		return
	}

	fmt.Printf("[Debug] Writing %d bytes to local RDP for connectionId=%s\n", len(data), connectionID)

	// Write data to local service
	if _, err := connObj.conn.Write(data); err != nil {
		if c.OnError != nil {
			c.OnError(fmt.Errorf("failed to write to local service: %w", err))
		}
		c.closeConnection(connectionID)
	}
}

// getOrCreateConnection gets existing or creates new connection to local RDP
func (c *Client) getOrCreateConnection(connectionID string) *connectionConn {
	c.connMu.Lock()
	defer c.connMu.Unlock()

	// Check if connection already exists
	if connObj, ok := c.localConnections[connectionID]; ok {
		return connObj
	}

	fmt.Printf("[Debug] Creating new connection to %s for connectionId=%s\n", c.config.LocalAddr(), connectionID)

	// Create new connection to local RDP
	conn, err := net.DialTimeout("tcp", c.config.LocalAddr(), 5*time.Second)
	if err != nil {
		if c.OnError != nil {
			c.OnError(fmt.Errorf("failed to connect to local service at %s: %w", c.config.LocalAddr(), err))
		}
		return nil
	}

	connObj := &connectionConn{
		conn:    conn,
		closeCh: make(chan struct{}),
	}
	c.localConnections[connectionID] = connObj

	fmt.Printf("[Debug] Connected to local RDP for connectionId=%s\n", connectionID)

	// Start goroutine to read from local service and send back to server
	c.wg.Add(1)
	go c.readFromLocal(connectionID, connObj)

	return connObj
}

// readFromLocal continuously reads from local service and sends to server
func (c *Client) readFromLocal(connectionID string, connObj *connectionConn) {
	defer c.wg.Done()
	defer c.closeConnection(connectionID)

	fmt.Printf("[Debug] Started reading from local RDP for connectionId=%s\n", connectionID)

	buf := make([]byte, 32*1024)
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-connObj.closeCh:
			return
		default:
		}

		connObj.conn.SetReadDeadline(time.Now().Add(time.Second * 5))
		n, err := connObj.conn.Read(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			// Connection closed or error
			fmt.Printf("[Debug] Local RDP connection closed for connectionId=%s: %v\n", connectionID, err)
			return
		}

		fmt.Printf("[Debug] Received %d bytes from local RDP for connectionId=%s\n", n, connectionID)

		// Send data back to server with connection ID
		msg := protocol.NewDataMessageWithConnectionID(connectionID, buf[:n])
		if err := c.sendRaw(msg); err != nil {
			if c.OnError != nil {
				c.OnError(fmt.Errorf("failed to send response: %w", err))
			}
			return
		}
		fmt.Printf("[Debug] Sent %d bytes to server for connectionId=%s\n", n, connectionID)
	}
}

// closeConnection closes a local connection
func (c *Client) closeConnection(connectionID string) {
	c.connMu.Lock()
	connObj, ok := c.localConnections[connectionID]
	if ok {
		delete(c.localConnections, connectionID)
	}
	c.connMu.Unlock()

	if connObj != nil {
		connObj.once.Do(func() {
			close(connObj.closeCh)
			connObj.conn.Close()
		})
	}
}

// sendRaw sends a raw protocol message
func (c *Client) sendRaw(msg *protocol.Message) error {
	c.writeMu.Lock()
	defer c.writeMu.Unlock()

	c.mu.Lock()
	writer := c.writer
	c.mu.Unlock()

	if writer == nil {
		return fmt.Errorf("not connected")
	}

	encoder := protocol.NewEncoder(writer)
	if err := encoder.Encode(msg); err != nil {
		return fmt.Errorf("failed to encode message: %w", err)
	}

	return writer.Flush()
}

func (c *Client) handleError(msg *protocol.Message) {
	resp, err := protocol.ParseErrorResponse(msg.Body)
	if err != nil {
		if c.OnError != nil {
			c.OnError(fmt.Errorf("parse error response failed: %w", err))
		}
		return
	}

	if c.OnError != nil {
		c.OnError(fmt.Errorf("server error: %s", resp.Message))
	}
}

func (c *Client) heartbeatLoop() {
	defer c.wg.Done()

	ticker := time.NewTicker(time.Duration(c.config.HeartbeatInterval) * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-ticker.C:
			if err := c.SendHeartbeat(); err != nil {
				if c.OnError != nil {
					c.OnError(fmt.Errorf("heartbeat failed: %w", err))
				}
			}
		}
	}
}