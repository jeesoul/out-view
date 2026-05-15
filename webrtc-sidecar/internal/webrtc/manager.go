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

	"github.com/outview/webrtc-sidecar/internal/ipc"
)

// Manager manages a single server-side WebRTC PeerConnection.
type Manager struct {
	mu sync.RWMutex
	pc *pionwebrtc.PeerConnection
	dc *pionwebrtc.DataChannel

	state atomic.Int32 // 0=idle, 1=connecting, 2=connected, 3=failed, 4=closed

	connectionID string
	registry     *ipc.ConnRegistry
	logger       *slog.Logger

	ctx    context.Context
	cancel context.CancelFunc

	closeOnce sync.Once

	// ICE candidates buffered before SetRemoteDescription
	pendingICEMu sync.Mutex
	pendingICE   []pionwebrtc.ICECandidateInit
	remoteSet    bool

	// onDataForward is called when data arrives from the DataChannel and needs
	// to be forwarded to the Java server. If nil, data is forwarded via IPC
	// registry (the default path).
	onDataForward func(connectionID string, data []byte)

	// sendResumeCh is signalled when the DataChannel buffer drains below the
	// low-water mark, allowing blocked SendData calls to resume.
	sendResumeCh chan struct{}
}

const (
	stateIdle       = 0
	stateConnecting = 1
	stateConnected  = 2
	stateFailed     = 3
	stateClosed     = 4

	// BufferHighWaterMark is the DataChannel buffer threshold above which SendData
	// will block waiting for the buffer to drain.
	BufferHighWaterMark uint64 = 1 * 1024 * 1024 // 1 MB

	// BufferLowWaterMark is the threshold at which the blocked SendData is resumed.
	BufferLowWaterMark uint64 = 512 * 1024 // 512 KB
)

// NewManager creates a new server-side WebRTC Manager.
func NewManager(connectionID string, registry *ipc.ConnRegistry, logger *slog.Logger) *Manager {
	if logger == nil {
		logger = slog.Default()
	}
	ctx, cancel := context.WithCancel(context.Background())
	return &Manager{
		connectionID: connectionID,
		registry:     registry,
		logger:       logger.With("connectionId", connectionID),
		ctx:          ctx,
		cancel:       cancel,
		sendResumeCh: make(chan struct{}, 1),
	}
}

// CreatePeerConnection initializes the PeerConnection (answerer role).
func (m *Manager) CreatePeerConnection() error {
	config := pionwebrtc.Configuration{
		ICEServers: []pionwebrtc.ICEServer{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
			{URLs: []string{"stun:stun1.l.google.com:19302"}},
		},
	}

	pc, err := pionwebrtc.NewPeerConnection(config)
	if err != nil {
		return fmt.Errorf("create peer connection: %w", err)
	}

	pc.OnICECandidate(func(c *pionwebrtc.ICECandidate) {
		if c == nil {
			m.logger.Info("ICE gathering complete")
			_ = m.registry.SendEvent(m.connectionID, &ipc.EventPayload{
				ConnectionID: m.connectionID,
				Event:        ipc.EventICEComplete,
			})
			return
		}
		m.logger.Debug("ICE candidate", "type", c.Typ, "address", c.Address)
		init := c.ToJSON()
		sdpMid := ""
		if init.SDPMid != nil {
			sdpMid = *init.SDPMid
		}
		_ = m.registry.SendEvent(m.connectionID, &ipc.EventPayload{
			ConnectionID: m.connectionID,
			Event:        ipc.EventICECandidate,
			Candidate:    init.Candidate,
			SDPMid:       sdpMid,
		})
	})

	pc.OnDataChannel(func(dc *pionwebrtc.DataChannel) {
		m.logger.Info("DataChannel received", "label", dc.Label())
		m.mu.Lock()
		m.dc = dc
		m.mu.Unlock()

		// Configure backpressure thresholds.
		dc.SetBufferedAmountLowThreshold(BufferLowWaterMark)
		dc.OnBufferedAmountLow(func() {
			select {
			case m.sendResumeCh <- struct{}{}:
			default:
			}
		})

		dc.OnOpen(func() {
			m.logger.Info("DataChannel opened")
			m.state.Store(stateConnected)
			_ = m.registry.SendEvent(m.connectionID, &ipc.EventPayload{
				ConnectionID: m.connectionID,
				Event:        ipc.EventEstablished,
			})
		})

		dc.OnClose(func() {
			m.logger.Info("DataChannel closed")
			m.state.Store(stateFailed)
			_ = m.registry.SendEvent(m.connectionID, &ipc.EventPayload{
				ConnectionID: m.connectionID,
				Event:        ipc.EventFailed,
				Reason:       "data channel closed",
			})
		})

		dc.OnMessage(func(msg pionwebrtc.DataChannelMessage) {
			m.mu.RLock()
			fwd := m.onDataForward
			m.mu.RUnlock()

			if fwd != nil {
				// Custom forward path (e.g. direct in-process routing).
				fwd(m.connectionID, msg.Data)
				return
			}
			// Default: forward to Java server via IPC event.
			_ = m.registry.SendEvent(m.connectionID, &ipc.EventPayload{
				ConnectionID: m.connectionID,
				Event:        ipc.EventData,
				Data:         msg.Data,
			})
		})
	})

	pc.OnConnectionStateChange(func(state pionwebrtc.PeerConnectionState) {
		m.logger.Info("PeerConnection state", "state", state)
		if state == pionwebrtc.PeerConnectionStateFailed || state == pionwebrtc.PeerConnectionStateClosed {
			m.state.Store(stateFailed)
			_ = m.registry.SendEvent(m.connectionID, &ipc.EventPayload{
				ConnectionID: m.connectionID,
				Event:        ipc.EventFailed,
				Reason:       state.String(),
			})
		}
	})

	m.mu.Lock()
	m.pc = pc
	m.mu.Unlock()

	m.state.Store(stateConnecting)
	return nil
}

// SetRemoteOffer sets the remote SDP offer and creates an answer.
// ctx is reserved for future deadline/cancellation support.
// Returns the answer SDP to be sent back to the client.
func (m *Manager) SetRemoteOffer(ctx context.Context, sdp string) (string, error) {
	m.mu.RLock()
	pc := m.pc
	m.mu.RUnlock()

	if pc == nil {
		return "", errors.New("peer connection not initialized")
	}

	offer := pionwebrtc.SessionDescription{
		Type: pionwebrtc.SDPTypeOffer,
		SDP:  sdp,
	}

	// Mark remote as set BEFORE calling pion. Any concurrent AddICECandidate
	// calls during the pion call will route to pc.AddICECandidate directly.
	// If pion rejects them (remote not yet set), they are silently dropped —
	// this is an acceptable trade-off since ICE candidates arriving this early
	// are rare and the connection will still succeed via other candidates.
	m.pendingICEMu.Lock()
	m.remoteSet = true
	pending := m.pendingICE
	m.pendingICE = nil
	m.pendingICEMu.Unlock()

	if err := pc.SetRemoteDescription(offer); err != nil {
		// Roll back on failure
		m.pendingICEMu.Lock()
		m.remoteSet = false
		m.pendingICE = append(pending, m.pendingICE...)
		m.pendingICEMu.Unlock()
		return "", fmt.Errorf("set remote description: %w", err)
	}

	// Flush buffered ICE candidates
	for _, c := range pending {
		if err := pc.AddICECandidate(c); err != nil {
			m.logger.Warn("Failed to add buffered ICE candidate", "err", err)
		}
	}

	answer, err := pc.CreateAnswer(nil)
	if err != nil {
		return "", fmt.Errorf("create answer: %w", err)
	}

	if err := pc.SetLocalDescription(answer); err != nil {
		return "", fmt.Errorf("set local description: %w", err)
	}

	return answer.SDP, nil
}

// AddICECandidate adds a remote ICE candidate. Buffers if called before SetRemoteOffer.
// ctx is reserved for future deadline/cancellation support.
func (m *Manager) AddICECandidate(ctx context.Context, candidate pionwebrtc.ICECandidateInit) error {
	m.pendingICEMu.Lock()
	if !m.remoteSet {
		m.pendingICE = append(m.pendingICE, candidate)
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
	return pc.AddICECandidate(candidate)
}

// SetOnDataForward sets a callback that is invoked when data arrives from the
// DataChannel and needs to be forwarded to the Java server. When set, the
// default IPC-based forwarding path is bypassed.
func (m *Manager) SetOnDataForward(fn func(connectionID string, data []byte)) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.onDataForward = fn
}

// SendData sends data over the DataChannel with backpressure.
// It blocks until the buffer drains below BufferHighWaterMark or ctx is cancelled.
// ctx is reserved for future deadline/cancellation support.
func (m *Manager) SendData(ctx context.Context, data []byte) error {
	m.mu.RLock()
	dc := m.dc
	m.mu.RUnlock()

	if dc == nil {
		return errors.New("data channel not ready")
	}

	if dc.BufferedAmount() < BufferHighWaterMark {
		return dc.Send(data)
	}

	// Buffer is full — wait for it to drain.
	timer := time.NewTimer(5 * time.Second)
	defer timer.Stop()

	for dc.BufferedAmount() >= BufferHighWaterMark {
		m.logger.Debug("Buffer full, waiting", "buffered", dc.BufferedAmount())
		select {
		case <-m.sendResumeCh:
		case <-ctx.Done():
			return ctx.Err()
		case <-timer.C:
			return errors.New("send timeout: buffer full")
		case <-m.ctx.Done():
			return errors.New("manager closed")
		}
	}

	return dc.Send(data)
}

// Close shuts down the Manager.
func (m *Manager) Close() {
	m.closeOnce.Do(func() {
		m.cancel()
		m.state.Store(stateClosed)

		m.mu.Lock()
		dc := m.dc
		pc := m.pc
		m.dc = nil
		m.pc = nil
		m.mu.Unlock()

		if dc != nil {
			dc.Close()
		}
		if pc != nil {
			pc.Close()
		}
		m.logger.Info("Manager closed")
	})
}

// State returns the current state (0=idle, 1=connecting, 2=connected, 3=failed, 4=closed).
func (m *Manager) State() int32 {
	return m.state.Load()
}
