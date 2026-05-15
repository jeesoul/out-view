package webrtc

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	pionwebrtc "github.com/pion/webrtc/v4"
)

// Manager manages a single WebRTC PeerConnection for the Go client.
type Manager struct {
	mu sync.RWMutex
	pc *pionwebrtc.PeerConnection
	dc *pionwebrtc.DataChannel

	state atomic.Int32 // ConnectionState

	ctx    context.Context
	cancel context.CancelFunc

	onDataReceived func([]byte)
	onStateChange  func(ConnectionState)
	onFallback     func(reason string) // called when WebRTC fails, trigger TCP fallback

	config       *Config
	connectionID string
	logger       *slog.Logger

	stateCh      chan stateTransition
	sendResumeCh chan struct{}

	// ICE candidates buffered before SetRemoteDescription
	pendingICEMu sync.Mutex
	pendingICE   []pionwebrtc.ICECandidateInit
	remoteSet    bool // true after SetRemoteDescription called

	// ICE candidate sender (set by caller)
	onICECandidate func(pionwebrtc.ICECandidateInit)
	onICEComplete  func()

	closeOnce sync.Once

	// connectedAt is set when the state transitions to StateWebRTCConnected.
	connectedAt atomic.Pointer[time.Time]

	// fallbackCount tracks the total number of times fallback was triggered.
	fallbackCount atomic.Int64

	// idleTimer fires when no data has been sent/received for IdleTimeout.
	idleTimer   *time.Timer
	idleTimerMu sync.Mutex
}

type stateTransition struct {
	to     ConnectionState
	reason string
}

// NewManager creates a new WebRTC Manager.
func NewManager(connectionID string, cfg *Config, logger *slog.Logger) *Manager {
	if cfg == nil {
		cfg = DefaultConfig()
	}
	if logger == nil {
		logger = slog.Default()
	}
	ctx, cancel := context.WithCancel(context.Background())
	m := &Manager{
		config:       cfg,
		connectionID: connectionID,
		logger:       logger.With("connectionId", connectionID),
		ctx:          ctx,
		cancel:       cancel,
		stateCh:      make(chan stateTransition, 16),
		sendResumeCh: make(chan struct{}, 1),
	}
	go m.stateActor()
	if cfg.IdleTimeout > 0 {
		go m.idleTimeoutWatcher()
	}
	return m
}

// SetOnDataReceived sets the callback for incoming DataChannel messages.
func (m *Manager) SetOnDataReceived(fn func([]byte)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onDataReceived = fn
}

// SetDataHandler is an alias for SetOnDataReceived. It sets the callback
// invoked when data arrives from the DataChannel (server → client direction).
func (m *Manager) SetDataHandler(fn func(data []byte)) {
	m.SetOnDataReceived(fn)
}

// SetOnStateChange sets the callback for state transitions.
func (m *Manager) SetOnStateChange(fn func(ConnectionState)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onStateChange = fn
}

// SetOnFallback sets the callback triggered when WebRTC fails and TCP fallback should start.
// The callback is invoked outside any Manager lock. Callers must not re-enter the Manager
// from within the callback while holding any external lock that the Manager might also acquire,
// to prevent deadlock.
func (m *Manager) SetOnFallback(fn func(reason string)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onFallback = fn
}

// SetOnICECandidate sets the callback to send ICE candidates to the remote peer.
func (m *Manager) SetOnICECandidate(fn func(pionwebrtc.ICECandidateInit)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onICECandidate = fn
}

// SetOnICEComplete sets the callback for when ICE gathering is complete.
func (m *Manager) SetOnICEComplete(fn func()) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onICEComplete = fn
}

// CreateOffer creates a PeerConnection + DataChannel and returns an SDP offer.
func (m *Manager) CreateOffer(ctx context.Context) (pionwebrtc.SessionDescription, error) {
	se := pionwebrtc.SettingEngine{}
	se.SetDTLSInsecureSkipHelloVerify(false)
	// Wire DTLSTimeout into the DTLS handshake context
	dtlsTimeout := m.config.DTLSTimeout
	se.SetDTLSConnectContextMaker(func() (context.Context, func()) {
		return context.WithTimeout(context.Background(), dtlsTimeout)
	})
	// Set DTLS replay protection window
	se.SetDTLSReplayProtectionWindow(64)

	api := pionwebrtc.NewAPI(pionwebrtc.WithSettingEngine(se))

	icePolicy := pionwebrtc.ICETransportPolicyAll
	if m.config.ICETransportPolicy == "relay" {
		icePolicy = pionwebrtc.ICETransportPolicyRelay
	}

	pc, err := api.NewPeerConnection(pionwebrtc.Configuration{
		ICEServers:         m.config.ICEServers,
		ICETransportPolicy: icePolicy,
	})
	if err != nil {
		return pionwebrtc.SessionDescription{}, fmt.Errorf("create peer connection: %w", err)
	}

	// Create DataChannel before offer (so it's included in SDP)
	dc, err := pc.CreateDataChannel("rdp-data", &pionwebrtc.DataChannelInit{
		Ordered: boolPtr(true),
	})
	if err != nil {
		pc.Close()
		return pionwebrtc.SessionDescription{}, fmt.Errorf("create data channel: %w", err)
	}

	m.setupDataChannel(dc)
	m.setupICEHandlers(pc)
	m.setupConnectionHandlers(pc)

	m.mu.Lock()
	m.pc = pc
	m.dc = dc
	m.mu.Unlock()

	offer, err := pc.CreateOffer(nil)
	if err != nil {
		return pionwebrtc.SessionDescription{}, fmt.Errorf("create offer: %w", err)
	}

	if err := pc.SetLocalDescription(offer); err != nil {
		return pionwebrtc.SessionDescription{}, fmt.Errorf("set local description: %w", err)
	}

	m.requestStateTransition(StateGatheringICE, "offer created")
	return offer, nil
}

// SetRemoteDescription sets the remote SDP answer and flushes buffered ICE candidates.
func (m *Manager) SetRemoteDescription(ctx context.Context, sd pionwebrtc.SessionDescription) error {
	m.mu.RLock()
	pc := m.pc
	m.mu.RUnlock()

	if pc == nil {
		return errors.New("peer connection not initialized")
	}

	// Mark remote as set BEFORE calling pion, so concurrent AddICECandidate
	// calls during the pion call go directly to pc.AddICECandidate.
	m.pendingICEMu.Lock()
	m.remoteSet = true
	pending := m.pendingICE
	m.pendingICE = nil
	m.pendingICEMu.Unlock()

	if err := pc.SetRemoteDescription(sd); err != nil {
		// Roll back remoteSet on failure so future candidates still buffer.
		m.pendingICEMu.Lock()
		m.remoteSet = false
		m.pendingICE = append(pending, m.pendingICE...)
		m.pendingICEMu.Unlock()
		return fmt.Errorf("set remote description: %w", err)
	}

	m.requestStateTransition(StateConnecting, "remote description set")

	// Flush candidates that were buffered before this call.
	for _, c := range pending {
		if err := pc.AddICECandidate(c); err != nil {
			m.logger.Warn("Failed to add buffered ICE candidate", "err", err)
		}
	}

	return nil
}

// AddICECandidate adds a remote ICE candidate. Buffers if called before SetRemoteDescription.
func (m *Manager) AddICECandidate(ctx context.Context, c pionwebrtc.ICECandidateInit) error {
	m.pendingICEMu.Lock()
	if !m.remoteSet {
		m.pendingICE = append(m.pendingICE, c)
		m.pendingICEMu.Unlock()
		return nil
	}
	m.pendingICEMu.Unlock()

	m.mu.RLock()
	pc := m.pc
	m.mu.RUnlock()

	if pc == nil {
		return errors.New("peer connection not initialized")
	}

	return pc.AddICECandidate(c)
}

// SendData sends data over the DataChannel with backpressure.
func (m *Manager) SendData(ctx context.Context, data []byte) error {
	m.mu.RLock()
	dc := m.dc
	m.mu.RUnlock()

	if dc == nil {
		return errors.New("data channel not ready")
	}

	if dc.BufferedAmount() < BufferHighWaterMark {
		m.resetIdleTimer()
		return dc.Send(data)
	}

	// Buffer is full — wait with a single timer to avoid per-iteration allocations.
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()

	for dc.BufferedAmount() >= BufferHighWaterMark {
		// Drain any pending resume signal before blocking to avoid a TOCTOU stall:
		// OnBufferedAmountLow may have fired between the loop condition check and
		// the select below, consuming the signal. Draining here ensures we don't
		// block unnecessarily when the buffer has already drained.
		select {
		case <-m.sendResumeCh:
		default:
		}
		// Re-check after draining — buffer may have already fallen below the mark.
		if dc.BufferedAmount() < BufferHighWaterMark {
			break
		}
		m.logger.Debug("Buffer full, waiting", "buffered", dc.BufferedAmount())
		select {
		case <-m.sendResumeCh:
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return errors.New("send timeout: buffer full")
		case <-m.ctx.Done():
			return fmt.Errorf("webrtc: manager closed")
		}
	}

	m.resetIdleTimer()
	return dc.Send(data)
}

// closeWithReason initiates cleanup with a named trigger reason.
// It is safe to call from any goroutine and is idempotent.
func (m *Manager) closeWithReason(reason string) {
	m.logger.Info("Closing manager", "trigger", reason)
	m.closeOnce.Do(func() {
		m.doClose(reason)
	})
}

// Close shuts down the Manager and releases all resources.
// Trigger 5: Application shutdown — graceful shutdown signal.
func (m *Manager) Close() error {
	m.closeOnce.Do(func() {
		m.doClose("application shutdown")
	})
	return nil
}

// doClose performs the actual cleanup in the correct order.
// Must only be called once, guarded by closeOnce.
func (m *Manager) doClose(reason string) {
	m.logger.Info("Manager cleanup started", "trigger", reason)

	// Step 1: Cancel context — stops all goroutines (including idleTimeoutWatcher).
	m.requestStateTransition(StateClosing, reason)
	m.cancel()

	// Step 2: Stop idle timer.
	m.idleTimerMu.Lock()
	if m.idleTimer != nil {
		m.idleTimer.Stop()
	}
	m.idleTimerMu.Unlock()

	// Step 3: Close DataChannel first, then PeerConnection.
	m.mu.Lock()
	dc := m.dc
	pc := m.pc
	m.dc = nil
	m.pc = nil
	m.mu.Unlock()

	if dc != nil {
		if err := dc.Close(); err != nil {
			m.logger.Warn("DataChannel close error", "err", err)
		}
	}
	if pc != nil {
		if err := pc.Close(); err != nil {
			m.logger.Warn("PeerConnection close error", "err", err)
		}
	}

	// Step 4: Mark closed and log completion.
	m.state.Store(int32(StateClosed))
	m.logger.Info("Manager closed", "trigger", reason)
}

// resetIdleTimer resets the idle timeout. Called on every send/receive.
func (m *Manager) resetIdleTimer() {
	if m.config.IdleTimeout <= 0 {
		return
	}
	m.idleTimerMu.Lock()
	defer m.idleTimerMu.Unlock()
	if m.idleTimer == nil {
		m.idleTimer = time.AfterFunc(m.config.IdleTimeout, func() {
			m.closeWithReason("idle timeout")
		})
	} else {
		m.idleTimer.Reset(m.config.IdleTimeout)
	}
}

// idleTimeoutWatcher starts the initial idle timer and stops it when ctx is cancelled.
// The timer itself fires closeWithReason; this goroutine just ensures cleanup on exit.
func (m *Manager) idleTimeoutWatcher() {
	// Start the initial idle timer.
	m.idleTimerMu.Lock()
	m.idleTimer = time.AfterFunc(m.config.IdleTimeout, func() {
		m.closeWithReason("idle timeout")
	})
	m.idleTimerMu.Unlock()

	// Wait for context cancellation, then stop the timer.
	<-m.ctx.Done()
	m.idleTimerMu.Lock()
	if m.idleTimer != nil {
		m.idleTimer.Stop()
	}
	m.idleTimerMu.Unlock()
}

// State returns the current connection state.
func (m *Manager) State() ConnectionState {
	return ConnectionState(m.state.Load())
}

// ConnectionID returns the connection identifier for this Manager.
func (m *Manager) ConnectionID() string {
	return m.connectionID
}

// IsConnected returns true if the WebRTC DataChannel is currently open
// (i.e. the state is StateWebRTCConnected).
func (m *Manager) IsConnected() bool {
	return ConnectionState(m.state.Load()) == StateWebRTCConnected
}

// ManagerStats holds a snapshot of Manager statistics.
type ManagerStats struct {
	ConnectionID string
	State        ConnectionState
	// Uptime is the duration since the connection reached StateWebRTCConnected.
	// Zero if the connection has never been established.
	Uptime time.Duration
	// FallbackCount is the total number of times fallback was triggered.
	FallbackCount int64
}

// Stats returns a snapshot of the Manager's current state and uptime.
func (m *Manager) Stats() ManagerStats {
	state := ConnectionState(m.state.Load())
	var uptime time.Duration
	if state == StateWebRTCConnected {
		if t := m.connectedAt.Load(); t != nil {
			uptime = time.Since(*t)
		}
	}
	return ManagerStats{
		ConnectionID:  m.connectionID,
		State:         state,
		Uptime:        uptime,
		FallbackCount: m.fallbackCount.Load(),
	}
}

func (m *Manager) setupDataChannel(dc *pionwebrtc.DataChannel) {
	dc.SetBufferedAmountLowThreshold(BufferLowWaterMark)

	dc.OnBufferedAmountLow(func() {
		select {
		case m.sendResumeCh <- struct{}{}:
		default:
		}
	})

	dc.OnOpen(func() {
		m.logger.Info("DataChannel opened")
		m.resetIdleTimer()
		m.requestStateTransition(StateWebRTCConnected, "data channel open")
	})

	// Trigger 3: DataChannel onClose — DataChannel closes unexpectedly.
	dc.OnClose(func() {
		m.logger.Info("DataChannel closed unexpectedly")
		m.closeWithReason("data channel closed")
	})

	dc.OnMessage(func(msg pionwebrtc.DataChannelMessage) {
		m.resetIdleTimer()
		m.mu.RLock()
		fn := m.onDataReceived
		m.mu.RUnlock()
		if fn != nil {
			fn(msg.Data)
		}
	})
}

func (m *Manager) setupICEHandlers(pc *pionwebrtc.PeerConnection) {
	pc.OnICECandidate(func(c *pionwebrtc.ICECandidate) {
		if c == nil {
			m.logger.Info("ICE gathering complete")
			m.mu.RLock()
			fn := m.onICEComplete
			m.mu.RUnlock()
			if fn != nil {
				fn()
			}
			return
		}
		m.logger.Debug("ICE candidate", "type", c.Typ, "address", c.Address)
		m.mu.RLock()
		fn := m.onICECandidate
		m.mu.RUnlock()
		if fn != nil {
			fn(c.ToJSON())
		}
	})

	pc.OnICEConnectionStateChange(func(state pionwebrtc.ICEConnectionState) {
		m.logger.Info("ICE connection state", "state", state)
		switch state {
		// Trigger 2: ICE failed — pion reports ICE connection failed.
		case pionwebrtc.ICEConnectionStateFailed:
			m.closeWithReason("ICE connection failed")
		case pionwebrtc.ICEConnectionStateDisconnected:
			m.requestStateTransition(StateWebRTCReconnecting, "ICE disconnected")
		}
	})
}

func (m *Manager) setupConnectionHandlers(pc *pionwebrtc.PeerConnection) {
	pc.OnConnectionStateChange(func(state pionwebrtc.PeerConnectionState) {
		m.logger.Info("PeerConnection state", "state", state)
		switch state {
		case pionwebrtc.PeerConnectionStateFailed:
			m.closeWithReason("peer connection failed")
		case pionwebrtc.PeerConnectionStateClosed:
			m.requestStateTransition(StateClosing, "peer connection closed")
		}
	})
}

func (m *Manager) triggerFallback(reason string) {
	m.fallbackCount.Add(1)
	m.requestStateTransition(StateWebRTCFailed, reason)
	m.mu.RLock()
	fn := m.onFallback
	m.mu.RUnlock()
	if fn != nil {
		fn(reason)
	}
}

// FallbackCount returns the total number of times fallback was triggered.
func (m *Manager) FallbackCount() int64 {
	return m.fallbackCount.Load()
}

func (m *Manager) requestStateTransition(to ConnectionState, reason string) {
	select {
	case m.stateCh <- stateTransition{to: to, reason: reason}:
	case <-m.ctx.Done():
	}
}

func (m *Manager) stateActor() {
	for {
		select {
		case <-m.ctx.Done():
			return
		case trans := <-m.stateCh:
			current := ConnectionState(m.state.Load())
			if !isValidTransition(current, trans.to) {
				m.logger.Debug("Ignoring invalid state transition",
					"from", current, "to", trans.to, "reason", trans.reason)
				continue
			}
			m.state.Store(int32(trans.to))
			m.logger.Info("State transition",
				"from", current, "to", trans.to, "reason", trans.reason)
			if trans.to == StateWebRTCConnected {
				now := time.Now()
				m.connectedAt.Store(&now)
			}
			m.mu.RLock()
			fn := m.onStateChange
			m.mu.RUnlock()
			if fn != nil {
				fn(trans.to)
			}
		}
	}
}

func boolPtr(b bool) *bool { return &b }
