package webrtc

import (
	"fmt"
	"sync"
	"time"

	"github.com/pion/webrtc/v4"
)

// POCManager demonstrates pion/webrtc v4 functionality
// This is a simplified POC that creates two PeerConnections in the same process
type POCManager struct {
	offerer  *webrtc.PeerConnection
	answerer *webrtc.PeerConnection

	offererDC  *webrtc.DataChannel
	answererDC *webrtc.DataChannel

	// Synchronization
	iceCandidatesMux   sync.Mutex
	offererCandidates  []webrtc.ICECandidateInit
	answererCandidates []webrtc.ICECandidateInit

	// State tracking
	dataChannelOpen bool
	dataReceived    []byte
	mu              sync.Mutex
}

// NewPOCManager creates a new POC manager
func NewPOCManager() (*POCManager, error) {
	return &POCManager{
		offererCandidates:  make([]webrtc.ICECandidateInit, 0),
		answererCandidates: make([]webrtc.ICECandidateInit, 0),
	}, nil
}

// createPeerConnection creates a PeerConnection with STUN server
func (m *POCManager) createPeerConnection() (*webrtc.PeerConnection, error) {
	config := webrtc.Configuration{
		ICEServers: []webrtc.ICEServer{
			{
				URLs: []string{"stun:stun.l.google.com:19302"},
			},
		},
	}

	return webrtc.NewPeerConnection(config)
}

// SetupOfferer creates and configures the offerer peer
func (m *POCManager) SetupOfferer() error {
	pc, err := m.createPeerConnection()
	if err != nil {
		return fmt.Errorf("failed to create offerer peer connection: %w", err)
	}
	m.offerer = pc

	// Create DataChannel (ordered, reliable - for RDP traffic)
	dcConfig := &webrtc.DataChannelInit{
		Ordered: func() *bool { b := true; return &b }(),
		// maxRetransmits nil = reliable
	}
	dc, err := pc.CreateDataChannel("data", dcConfig)
	if err != nil {
		return fmt.Errorf("failed to create data channel: %w", err)
	}
	m.offererDC = dc

	// Set up DataChannel callbacks
	dc.OnOpen(func() {
		fmt.Println("[Offerer] DataChannel opened")
		m.mu.Lock()
		m.dataChannelOpen = true
		m.mu.Unlock()
	})

	dc.OnMessage(func(msg webrtc.DataChannelMessage) {
		fmt.Printf("[Offerer] Received message: %s\n", string(msg.Data))
		m.mu.Lock()
		m.dataReceived = msg.Data
		m.mu.Unlock()
	})

	// Set up ICE candidate callback
	pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate == nil {
			return
		}
		m.iceCandidatesMux.Lock()
		m.offererCandidates = append(m.offererCandidates, candidate.ToJSON())
		m.iceCandidatesMux.Unlock()
		fmt.Printf("[Offerer] ICE candidate: %s\n", candidate.String())
	})

	return nil
}

// SetupAnswerer creates and configures the answerer peer
func (m *POCManager) SetupAnswerer() error {
	pc, err := m.createPeerConnection()
	if err != nil {
		return fmt.Errorf("failed to create answerer peer connection: %w", err)
	}
	m.answerer = pc

	// Set up DataChannel callback (answerer receives the DC)
	pc.OnDataChannel(func(dc *webrtc.DataChannel) {
		fmt.Println("[Answerer] DataChannel received")
		m.answererDC = dc

		dc.OnOpen(func() {
			fmt.Println("[Answerer] DataChannel opened")
		})

		dc.OnMessage(func(msg webrtc.DataChannelMessage) {
			fmt.Printf("[Answerer] Received message: %s\n", string(msg.Data))
			// Echo back
			if err := dc.Send([]byte("Echo: " + string(msg.Data))); err != nil {
				fmt.Printf("[Answerer] Failed to send echo: %v\n", err)
			}
		})
	})

	// Set up ICE candidate callback
	pc.OnICECandidate(func(candidate *webrtc.ICECandidate) {
		if candidate == nil {
			return
		}
		m.iceCandidatesMux.Lock()
		m.answererCandidates = append(m.answererCandidates, candidate.ToJSON())
		m.iceCandidatesMux.Unlock()
		fmt.Printf("[Answerer] ICE candidate: %s\n", candidate.String())
	})

	return nil
}

// CreateOffer creates an SDP offer from the offerer
func (m *POCManager) CreateOffer() (string, error) {
	offer, err := m.offerer.CreateOffer(nil)
	if err != nil {
		return "", fmt.Errorf("failed to create offer: %w", err)
	}

	if err := m.offerer.SetLocalDescription(offer); err != nil {
		return "", fmt.Errorf("failed to set local description: %w", err)
	}

	fmt.Println("[Offerer] Created offer and set local description")
	return offer.SDP, nil
}

// SetRemoteOffer sets the remote offer on the answerer
func (m *POCManager) SetRemoteOffer(sdp string) error {
	offer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeOffer,
		SDP:  sdp,
	}

	if err := m.answerer.SetRemoteDescription(offer); err != nil {
		return fmt.Errorf("failed to set remote description: %w", err)
	}

	fmt.Println("[Answerer] Set remote offer")
	return nil
}

// CreateAnswer creates an SDP answer from the answerer
func (m *POCManager) CreateAnswer() (string, error) {
	answer, err := m.answerer.CreateAnswer(nil)
	if err != nil {
		return "", fmt.Errorf("failed to create answer: %w", err)
	}

	if err := m.answerer.SetLocalDescription(answer); err != nil {
		return "", fmt.Errorf("failed to set local description: %w", err)
	}

	fmt.Println("[Answerer] Created answer and set local description")
	return answer.SDP, nil
}

// SetRemoteAnswer sets the remote answer on the offerer
func (m *POCManager) SetRemoteAnswer(sdp string) error {
	answer := webrtc.SessionDescription{
		Type: webrtc.SDPTypeAnswer,
		SDP:  sdp,
	}

	if err := m.offerer.SetRemoteDescription(answer); err != nil {
		return fmt.Errorf("failed to set remote description: %w", err)
	}

	fmt.Println("[Offerer] Set remote answer")
	return nil
}

// ExchangeICECandidates exchanges ICE candidates between peers
func (m *POCManager) ExchangeICECandidates() error {
	// Wait a bit for ICE gathering
	time.Sleep(500 * time.Millisecond)

	m.iceCandidatesMux.Lock()
	offererCands := make([]webrtc.ICECandidateInit, len(m.offererCandidates))
	answererCands := make([]webrtc.ICECandidateInit, len(m.answererCandidates))
	copy(offererCands, m.offererCandidates)
	copy(answererCands, m.answererCandidates)
	m.iceCandidatesMux.Unlock()

	// Add offerer's candidates to answerer
	for _, cand := range offererCands {
		if err := m.answerer.AddICECandidate(cand); err != nil {
			return fmt.Errorf("failed to add ICE candidate to answerer: %w", err)
		}
	}

	// Add answerer's candidates to offerer
	for _, cand := range answererCands {
		if err := m.offerer.AddICECandidate(cand); err != nil {
			return fmt.Errorf("failed to add ICE candidate to offerer: %w", err)
		}
	}

	fmt.Printf("[ICE] Exchanged %d offerer candidates and %d answerer candidates\n",
		len(offererCands), len(answererCands))
	return nil
}

// SendData sends data through the offerer's DataChannel
func (m *POCManager) SendData(data []byte) error {
	if m.offererDC == nil {
		return fmt.Errorf("offerer data channel not initialized")
	}

	if err := m.offererDC.Send(data); err != nil {
		return fmt.Errorf("failed to send data: %w", err)
	}

	fmt.Printf("[Offerer] Sent data: %s\n", string(data))
	return nil
}

// WaitForDataChannel waits for the DataChannel to open
func (m *POCManager) WaitForDataChannel(timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		m.mu.Lock()
		if m.dataChannelOpen {
			m.mu.Unlock()
			return nil
		}
		m.mu.Unlock()
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for data channel to open")
}

// GetReceivedData returns the last received data
func (m *POCManager) GetReceivedData() []byte {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.dataReceived
}

// Close closes both peer connections
func (m *POCManager) Close() error {
	var errs []error

	if m.offerer != nil {
		if err := m.offerer.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close offerer: %w", err))
		}
	}

	if m.answerer != nil {
		if err := m.answerer.Close(); err != nil {
			errs = append(errs, fmt.Errorf("failed to close answerer: %w", err))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("errors closing connections: %v", errs)
	}

	return nil
}
