package client

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/outview/client/internal/logger"
	"github.com/outview/client/internal/protocol"
	clientwebrtc "github.com/outview/client/internal/webrtc"
	pionwebrtc "github.com/pion/webrtc/v4"
)

// State represents the client state
type State int32

const (
	StateDisconnected State = iota
	StateConnecting
	StateConnected
	StateRegistered
	StateReconnecting
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
	case StateReconnecting:
		return "Reconnecting"
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

	state        atomic.Int32
	externalPort int

	proxyManager *ProxyManager

	// Connection ID -> local RDP connection
	localConnections map[string]*connectionConn
	connMu           sync.Mutex

	ctx    context.Context
	cancel context.CancelFunc
	wg     sync.WaitGroup

	mu      sync.Mutex // protects conn/reader/writer
	writeMu sync.Mutex // serializes writes

	// reconnect signal: readLoop sends here when connection drops
	reconnectCh chan struct{}

	// WebRTC manager (nil when WebRTC is disabled)
	webrtcManager *clientwebrtc.Manager

	// Callbacks
	OnStateChange    func(old, new State)
	OnRegisterResult func(success bool, externalPort int, err error)
	OnDataReceived   func(data []byte)
	OnError          func(err error)
}

// NewClient creates a new client
func NewClient(config *Config) *Client {
	ctx, cancel := context.WithCancel(context.Background())
	c := &Client{
		config:           config,
		proxyManager:     NewProxyManager(),
		localConnections: make(map[string]*connectionConn),
		ctx:              ctx,
		cancel:           cancel,
		reconnectCh:      make(chan struct{}, 1),
	}
	return c
}

// NewClientWithWebRTC creates a new client with WebRTC enabled.
// connectionID is the identifier used for the WebRTC PeerConnection.
func NewClientWithWebRTC(config *Config, connectionID string, webrtcCfg *clientwebrtc.Config) *Client {
	c := NewClient(config)
	c.webrtcManager = clientwebrtc.NewManager(connectionID, webrtcCfg, nil)
	return c
}

// connect establishes a TCP connection to the server (no registration).
func (c *Client) connect() error {
	c.setState(StateConnecting)

	dialer := &net.Dialer{Timeout: 10 * time.Second}
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
	return nil
}

// Connect connects to the server (kept for backward compat).
func (c *Client) Connect() error {
	return c.connect()
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

	return writer.Flush()
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

	return writer.Flush()
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

	return writer.Flush()
}

// Start connects, registers, and starts background loops.
func (c *Client) Start() error {
	if err := c.connect(); err != nil {
		return err
	}
	if err := c.Register(); err != nil {
		return err
	}

	c.wg.Add(1)
	go c.readLoop()

	c.wg.Add(1)
	go c.heartbeatLoop()

	if c.config.AutoReconnect {
		c.wg.Add(1)
		go c.reconnectLoop()
	}

	return nil
}

// Stop shuts down the client.
func (c *Client) Stop() {
	c.cancel()

	c.mu.Lock()
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
	}
	c.mu.Unlock()

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

	if c.webrtcManager != nil {
		c.webrtcManager.Close()
	}

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

// closeConn closes the underlying TCP connection and clears reader/writer.
func (c *Client) closeConn() {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn != nil {
		c.conn.Close()
		c.conn = nil
		c.reader = nil
		c.writer = nil
	}
}

// closeAllLocalConns closes all per-connectionID local RDP connections.
func (c *Client) closeAllLocalConns() {
	c.connMu.Lock()
	defer c.connMu.Unlock()
	for id, connObj := range c.localConnections {
		connObj.once.Do(func() {
			close(connObj.closeCh)
			connObj.conn.Close()
		})
		delete(c.localConnections, id)
	}
}

func (c *Client) readLoop() {
	defer c.wg.Done()

	for {
		c.mu.Lock()
		reader := c.reader
		c.mu.Unlock()

		if reader == nil {
			return
		}

		decoder := protocol.NewDecoder(reader)
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

				logger.Error("Connection lost: %v", err)
				c.setState(StateDisconnected)
				c.closeConn()
				c.closeAllLocalConns()

				// signal reconnectLoop (non-blocking)
				select {
				case c.reconnectCh <- struct{}{}:
				default:
				}
				return
			}

			c.handleMessage(msg)
		}
	}
}

// reconnectLoop waits for a disconnect signal and re-establishes the connection
// using exponential backoff. It respects ctx cancellation.
func (c *Client) reconnectLoop() {
	defer c.wg.Done()

	for {
		select {
		case <-c.ctx.Done():
			return
		case <-c.reconnectCh:
		}

		if !c.config.AutoReconnect {
			return
		}

		delay := time.Duration(c.config.RetryDelay) * time.Second
		maxDelay := 60 * time.Second

		for attempt := 1; ; attempt++ {
			select {
			case <-c.ctx.Done():
				return
			case <-time.After(delay):
			}

			if c.config.MaxRetries > 0 && attempt > c.config.MaxRetries {
				logger.Error("Max reconnect attempts (%d) reached, giving up", c.config.MaxRetries)
				if c.OnError != nil {
					c.OnError(fmt.Errorf("max reconnect attempts reached"))
				}
				return
			}

			c.setState(StateReconnecting)
			logger.Info("Reconnecting (attempt %d)...", attempt)

			if err := c.connect(); err != nil {
				logger.Error("Reconnect failed: %v", err)
				// exponential backoff, cap at maxDelay
				delay *= 2
				if delay > maxDelay {
					delay = maxDelay
				}
				continue
			}

			if err := c.Register(); err != nil {
				logger.Error("Re-register failed: %v", err)
				c.closeConn()
				delay *= 2
				if delay > maxDelay {
					delay = maxDelay
				}
				continue
			}

			logger.Info("Reconnected successfully")

			// restart readLoop for the new connection
			c.wg.Add(1)
			go c.readLoop()
			break
		}
	}
}

func (c *Client) handleMessage(msg *protocol.Message) {
	switch msg.Header.Type {
	case protocol.TypeRegisterAck:
		c.handleRegisterAck(msg)
	case protocol.TypeHeartbeatAck:
		// nothing to do
	case protocol.TypeData:
		c.handleData(msg)
	case protocol.TypeError:
		c.handleError(msg)
	case protocol.TypeWebRTCAnswer:
		c.handleWebRTCAnswer(msg)
	case protocol.TypeWebRTCICECandidate:
		c.handleWebRTCICECandidate(msg)
	case protocol.TypeWebRTCICEComplete:
		c.handleWebRTCICEComplete(msg)
	case protocol.TypeWebRTCEstablished:
		c.handleWebRTCEstablished(msg)
	case protocol.TypeWebRTCFailed:
		c.handleWebRTCFailed(msg)
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

	// If WebRTC is enabled and registration succeeded, initiate the offer.
	if resp.Success && c.webrtcManager != nil {
		go c.initiateWebRTCOffer()
	}
}

func (c *Client) handleData(msg *protocol.Message) {
	packet, err := protocol.ParseDataPacket(msg.Body)
	if err != nil {
		if c.OnError != nil {
			c.OnError(fmt.Errorf("failed to parse data packet: %w", err))
		}
		return
	}

	logger.Debug("Received data: connectionId=%s, len=%d", packet.ConnectionID, len(packet.Data))

	if c.OnDataReceived != nil {
		c.OnDataReceived(packet.Data)
	}

	c.forwardToLocal(packet.ConnectionID, packet.Data)
}

// connectionConn tracks a connection to local service
type connectionConn struct {
	conn    net.Conn
	closeCh chan struct{}
	once    sync.Once
}

// forwardToLocal forwards data to local service, maintaining connection per connectionID
func (c *Client) forwardToLocal(connectionID string, data []byte) {
	connObj := c.getOrCreateConnection(connectionID)
	if connObj == nil {
		logger.Error("Failed to create connection for connectionId=%s", connectionID)
		return
	}

	logger.Debug("Writing %d bytes to local RDP for connectionId=%s", len(data), connectionID)

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

	if connObj, ok := c.localConnections[connectionID]; ok {
		return connObj
	}

	logger.Debug("Creating new connection to %s for connectionId=%s", c.config.LocalAddr(), connectionID)

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

	c.wg.Add(1)
	go c.readFromLocal(connectionID, connObj)

	return connObj
}

// readFromLocal continuously reads from local service and sends to server
func (c *Client) readFromLocal(connectionID string, connObj *connectionConn) {
	defer c.wg.Done()
	defer c.closeConnection(connectionID)

	buf := make([]byte, 32*1024)
	for {
		select {
		case <-c.ctx.Done():
			return
		case <-connObj.closeCh:
			return
		default:
		}

		connObj.conn.SetReadDeadline(time.Now().Add(5 * time.Second))
		n, err := connObj.conn.Read(buf)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
				continue
			}
			logger.Error("Local RDP connection closed for connectionId=%s: %v", connectionID, err)
			_ = c.sendRaw(protocol.NewCloseConnectionMessage(connectionID))
			return
		}

		msg := protocol.NewDataMessageWithConnectionID(connectionID, buf[:n])
		if err := c.sendRaw(msg); err != nil {
			if c.OnError != nil {
				c.OnError(fmt.Errorf("failed to send response: %w", err))
			}
			return
		}
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

// -------------------------------------------------------------------------
// WebRTC signaling helpers
// -------------------------------------------------------------------------

// initiateWebRTCOffer creates a PeerConnection, wires ICE callbacks, and sends
// the SDP offer to the server. Called in a goroutine after successful registration.
func (c *Client) initiateWebRTCOffer() {
	mgr := c.webrtcManager
	if mgr == nil {
		return
	}

	// Wire ICE candidate callback: send each candidate to the server.
	mgr.SetOnICECandidate(func(init pionwebrtc.ICECandidateInit) {
		candidate := init.Candidate
		sdpMid := ""
		if init.SDPMid != nil {
			sdpMid = *init.SDPMid
		}
		var sdpMLineIndex *uint16
		if init.SDPMLineIndex != nil {
			sdpMLineIndex = init.SDPMLineIndex
		}
		connID := mgr.ConnectionID()
		iceMsg, err := protocol.NewWebRTCICECandidateMessage(connID, candidate, sdpMid, sdpMLineIndex)
		if err != nil {
			logger.Error("Failed to create ICE candidate message: %v", err)
			return
		}
		if err := c.sendRaw(iceMsg); err != nil {
			logger.Error("Failed to send ICE candidate: %v", err)
		}
	})

	// Wire ICE complete callback.
	mgr.SetOnICEComplete(func() {
		connID := mgr.ConnectionID()
		logger.Info("ICE gathering complete, notifying server: connectionId=%s", connID)
		completeMsg, err := protocol.NewWebRTCICECompleteMessage(connID)
		if err != nil {
			logger.Error("Failed to create ICE complete message: %v", err)
			return
		}
		if err := c.sendRaw(completeMsg); err != nil {
			logger.Error("Failed to send ICE complete: %v", err)
		}
	})

	// Wire fallback callback.
	mgr.SetOnFallback(func(reason string) {
		connID := mgr.ConnectionID()
		logger.Warn("WebRTC failed, falling back to TCP relay: connectionId=%s, reason=%s", connID, reason)
		failedMsg, err := protocol.NewWebRTCFailedMessage(connID, reason)
		if err != nil {
			logger.Error("Failed to create WebRTC failed message: %v", err)
			return
		}
		if err := c.sendRaw(failedMsg); err != nil {
			logger.Error("Failed to send WebRTC failed: %v", err)
		}
	})

	ctx, cancel := context.WithTimeout(c.ctx, 30*time.Second)
	defer cancel()

	offer, err := mgr.CreateOffer(ctx)
	if err != nil {
		logger.Error("Failed to create WebRTC offer: %v", err)
		// Close the manager so it transitions to a failed state and triggers
		// the fallback callback (if set), allowing the client to continue via TCP relay.
		mgr.Close()
		return
	}

	connID := mgr.ConnectionID()
	logger.Info("Sending WebRTC offer to server: connectionId=%s", connID)

	offerMsg, err := protocol.NewWebRTCOfferMessage(connID, offer.SDP)
	if err != nil {
		logger.Error("Failed to create offer message: %v", err)
		return
	}
	if err := c.sendRaw(offerMsg); err != nil {
		logger.Error("Failed to send WebRTC offer: %v", err)
	}
}

// handleWebRTCAnswer processes a TypeWebRTCAnswer message from the server.
func (c *Client) handleWebRTCAnswer(msg *protocol.Message) {
	if c.webrtcManager == nil {
		logger.Warn("Received WebRTC answer but WebRTC is not enabled")
		return
	}

	body, err := protocol.ParseWebRTCOfferBody(msg.Body)
	if err != nil {
		logger.Error("Failed to parse WebRTC answer: %v", err)
		return
	}

	logger.Info("Received WebRTC answer: connectionId=%s", body.ConnectionID)

	sd := pionwebrtc.SessionDescription{
		Type: pionwebrtc.SDPTypeAnswer,
		SDP:  body.SDP,
	}

	ctx, cancel := context.WithTimeout(c.ctx, 10*time.Second)
	defer cancel()

	if err := c.webrtcManager.SetRemoteDescription(ctx, sd); err != nil {
		logger.Error("Failed to set remote description: %v", err)
	}
}

// handleWebRTCICECandidate processes a TypeWebRTCICECandidate message from the server.
func (c *Client) handleWebRTCICECandidate(msg *protocol.Message) {
	if c.webrtcManager == nil {
		return
	}

	body, err := protocol.ParseWebRTCICECandidateBody(msg.Body)
	if err != nil {
		logger.Error("Failed to parse ICE candidate: %v", err)
		return
	}

	logger.Debug("Received ICE candidate from server: connectionId=%s", body.ConnectionID)

	init := pionwebrtc.ICECandidateInit{
		Candidate: body.Candidate,
	}
	if body.SDPMid != "" {
		sdpMid := body.SDPMid
		init.SDPMid = &sdpMid
	}
	if body.SDPMLineIndex != nil {
		init.SDPMLineIndex = body.SDPMLineIndex
	}

	ctx, cancel := context.WithTimeout(c.ctx, 5*time.Second)
	defer cancel()

	if err := c.webrtcManager.AddICECandidate(ctx, init); err != nil {
		logger.Error("Failed to add ICE candidate: %v", err)
	}
}

// handleWebRTCICEComplete processes a TypeWebRTCICEComplete message from the server.
func (c *Client) handleWebRTCICEComplete(msg *protocol.Message) {
	body, err := protocol.ParseWebRTCConnectionBody(msg.Body)
	if err != nil {
		logger.Error("Failed to parse ICE complete message: %v", err)
		return
	}
	logger.Info("Server ICE gathering complete: connectionId=%s", body.ConnectionID)
}

// handleWebRTCEstablished processes a TypeWebRTCEstablished message from the server.
func (c *Client) handleWebRTCEstablished(msg *protocol.Message) {
	body, err := protocol.ParseWebRTCConnectionBody(msg.Body)
	if err != nil {
		logger.Error("Failed to parse WebRTC established message: %v", err)
		return
	}
	logger.Info("WebRTC connection established: connectionId=%s", body.ConnectionID)
}

// handleWebRTCFailed processes a TypeWebRTCFailed message from the server.
// Logs the failure and triggers fallback to TCP relay.
func (c *Client) handleWebRTCFailed(msg *protocol.Message) {
	body, err := protocol.ParseWebRTCConnectionBody(msg.Body)
	if err != nil {
		logger.Error("Failed to parse WebRTC failed message: %v", err)
		return
	}
	logger.Warn("WebRTC failed (server notification): connectionId=%s, reason=%s",
		body.ConnectionID, body.Reason)

	// Close the local WebRTC manager so it transitions to failed/closed state.
	if c.webrtcManager != nil {
		c.webrtcManager.Close()
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
			// only send heartbeat when registered or connected
			state := c.GetState()
			if state != StateRegistered && state != StateConnected {
				continue
			}
			if err := c.SendHeartbeat(); err != nil {
				// connection is gone; reconnectLoop will handle it
				logger.Debug("Heartbeat skipped (connection unavailable): %v", err)
			}
		}
	}
}
