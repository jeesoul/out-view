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

	mu        sync.Mutex // protects conn/reader/writer
	writeMu   sync.Mutex // serializes writes
	sessionMu sync.Mutex // 串行安装和清理控制连接，防止旧清理删除新资源。

	// lifeMu 保证 Stop 开始后不再向 WaitGroup 添加任务。
	lifeMu             sync.Mutex
	started            bool
	stopping           bool
	stopOnce           sync.Once
	stopped            chan struct{}
	closeNotifications chan closeNotification
	controlOverload    chan net.Conn // 有界故障信号；在锁外关闭发生通知过载的那一代控制连接。
	// 心跳和注册状态由 mu 保护，绑定当前控制连接。
	registered    bool
	heartbeatSent time.Time
	latency       time.Duration
	latencyAt     time.Time
	closedLocal   map[string]bool
	closedOrder   []string

	// WebRTC manager (nil when WebRTC is disabled)
	webrtcManager *clientwebrtc.Manager
	// webrtcCfg holds the WebRTC configuration (timeout, ICE servers, etc.)
	webrtcCfg *clientwebrtc.Config

	// usingWebRTC is true when data is actively being routed through WebRTC.
	// Protected by webrtcMu.
	usingWebRTC bool
	// webrtcEnabled is true when WebRTC was successfully initiated at least once.
	// Used to decide whether to re-initiate WebRTC after a TCP reconnect.
	webrtcEnabled bool
	webrtcMu      sync.RWMutex
	signalingMu   sync.Mutex // 防止 manager 在 CreateOffer 过程中被关闭后又生成未托管资源。

	// reconnectCount is the total number of reconnect attempts (atomic).
	reconnectCount atomic.Int64
	// webrtcRecoveryCount counts how many times WebRTC was re-initiated after a
	// TCP reconnect (does not include the initial connection). Atomic.
	webrtcRecoveryCount atomic.Int64

	// Traffic counters (atomic, bytes).
	bytesSent atomic.Int64
	bytesRecv atomic.Int64

	// 回调应在 Start 前设置并及时返回；若需停止客户端，请在另一个 goroutine 调用 Stop。
	// 回调运行于客户端任务内，同步调用 Stop 会等待自身。
	OnStateChange       func(old, new State)
	OnRegisterResult    func(success bool, externalPort int, err error)
	OnDataReceived      func(data []byte)
	OnError             func(err error)
	OnDeviceQueryResult func(resp *protocol.DeviceQueryResponse)
	OnWebRTCStateChange func(state string)
}

// NewClient creates a new client
func NewClient(config *Config) *Client {
	if config == nil {
		config = DefaultConfig()
	}
	cloned := *config
	config = &cloned
	config.applyTimeoutDefaults()
	ctx, cancel := context.WithCancel(context.Background())
	c := &Client{
		config:             config,
		proxyManager:       NewProxyManager(),
		localConnections:   make(map[string]*connectionConn),
		ctx:                ctx,
		cancel:             cancel,
		stopped:            make(chan struct{}),
		closeNotifications: make(chan closeNotification, 128),
		controlOverload:    make(chan net.Conn, 1),
		closedLocal:        make(map[string]bool),
	}
	return c
}

// NewClientWithWebRTC creates a new client with WebRTC enabled.
// connectionID is the identifier used for the WebRTC PeerConnection.
func NewClientWithWebRTC(config *Config, connectionID string, webrtcCfg *clientwebrtc.Config) *Client {
	c := NewClient(config)
	if webrtcCfg == nil {
		webrtcCfg = clientwebrtc.DefaultConfig()
	}
	c.webrtcCfg = webrtcCfg
	c.webrtcManager = clientwebrtc.NewManager(connectionID, webrtcCfg, nil)
	return c
}

// GetState returns the current state
func (c *Client) GetState() State {
	return State(c.state.Load())
}

// GetExternalPort returns the assigned external port
func (c *Client) GetExternalPort() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.externalPort
}

// ReconnectCount returns the total number of reconnect attempts made so far.
func (c *Client) ReconnectCount() int64 {
	return c.reconnectCount.Load()
}

// WebRTCRecoveryCount returns the number of times WebRTC was re-initiated after
// a TCP reconnect (does not include the initial WebRTC connection).
func (c *Client) WebRTCRecoveryCount() int64 {
	return c.webrtcRecoveryCount.Load()
}

func (c *Client) setState(state State) {
	c.mu.Lock()
	// 心跳故障和读循环可以同时结束；失效连接的迟到状态不能覆盖当前连接事实。
	if (state == StateConnected || state == StateRegistered) && c.conn == nil ||
		state == StateDisconnected && c.conn != nil ||
		state != StateDisconnected && c.ctx.Err() != nil {
		c.mu.Unlock()
		return
	}
	old := State(c.state.Swap(int32(state)))
	c.mu.Unlock()
	if old != state && c.OnStateChange != nil {
		c.OnStateChange(old, state)
	}
}

func (c *Client) handleMessage(msg *protocol.Message) {
	switch msg.Header.Type {
	case protocol.TypeRegisterAck:
		c.handleRegisterAck(msg)
	case protocol.TypeHeartbeatAck:
		c.handleHeartbeatAck()
	case protocol.TypeCloseConnection:
		packet, err := protocol.ParseDataPacket(msg.Body)
		if err == nil {
			c.closeConnection(packet.ConnectionID)
		}
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
	case protocol.TypeDeviceQueryAck:
		c.handleDeviceQueryAck(msg)
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

	c.mu.Lock()
	if c.conn == nil || c.ctx.Err() != nil {
		c.mu.Unlock()
		return
	}
	firstRegistration := resp.Success && !c.registered
	if resp.Success {
		c.externalPort = resp.ExternalPort
		c.registered = true
	}
	c.mu.Unlock()
	if resp.Success {
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
	if firstRegistration {
		c.webrtcMu.RLock()
		mgr := c.webrtcManager
		c.webrtcMu.RUnlock()
		if mgr != nil {
			control := c.currentConn()
			c.launch(func() { c.initiateWebRTCOfferFor(mgr, control) })
		}
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

// SendMessage sends an arbitrary protocol message (used for device queries etc.)
func (c *Client) SendMessage(msg *protocol.Message) error {
	return c.sendRaw(msg)
}

// TrafficStats holds cumulative traffic counters.
type TrafficStats struct {
	BytesSent int64
	BytesRecv int64
}

// GetTrafficStats returns cumulative bytes sent and received since the client started.
func (c *Client) GetTrafficStats() TrafficStats {
	return TrafficStats{
		BytesSent: c.bytesSent.Load(),
		BytesRecv: c.bytesRecv.Load(),
	}
}

// IsUsingWebRTC returns true when data is actively routed through WebRTC P2P.
func (c *Client) IsUsingWebRTC() bool {
	c.webrtcMu.RLock()
	defer c.webrtcMu.RUnlock()
	return c.usingWebRTC
}

func (c *Client) handleDeviceQueryAck(msg *protocol.Message) {
	resp, err := protocol.ParseDeviceQueryResponse(msg.Body)
	if err != nil {
		logger.Error("Failed to parse device query response: %v", err)
		return
	}
	if c.OnDeviceQueryResult != nil {
		c.OnDeviceQueryResult(resp)
	}
}
