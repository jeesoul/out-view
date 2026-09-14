package client

import (
	"context"
	"github.com/outview/client/internal/logger"
	"github.com/outview/client/internal/protocol"
	clientwebrtc "github.com/outview/client/internal/webrtc"
	pionwebrtc "github.com/pion/webrtc/v4"
	"net"
	"time"
)

// -------------------------------------------------------------------------
// WebRTC signaling helpers
// -------------------------------------------------------------------------

// initiateWebRTCOffer creates a PeerConnection, wires ICE callbacks, and sends
// the SDP offer to the server. Called in a goroutine after successful registration.
// mgr must be the Manager snapshot captured by the caller under webrtcMu, so this
// goroutine holds a stable reference without racing on c.webrtcManager.
func (c *Client) initiateWebRTCOffer(mgr *clientwebrtc.Manager) {
	c.initiateWebRTCOfferFor(mgr, c.currentConn())
}

func (c *Client) activeWebRTC(mgr *clientwebrtc.Manager, control net.Conn) bool {
	if mgr == nil || control == nil || c.ctx.Err() != nil || c.currentConn() != control {
		return false
	}
	c.webrtcMu.RLock()
	defer c.webrtcMu.RUnlock()
	return c.webrtcManager == mgr
}

func (c *Client) closeWebRTCManager(mgr *clientwebrtc.Manager) {
	c.signalingMu.Lock()
	defer c.signalingMu.Unlock()
	if mgr != nil {
		mgr.Close()
	}
}

// 控制连接和 manager 双重快照贯穿 offer、ICE 和超时回调。
func (c *Client) initiateWebRTCOfferFor(mgr *clientwebrtc.Manager, control net.Conn) {
	if !c.activeWebRTC(mgr, control) {
		return
	}

	// Wire ICE candidate callback: send each candidate to the server.
	mgr.SetOnICECandidate(func(init pionwebrtc.ICECandidateInit) {
		if !c.activeWebRTC(mgr, control) {
			return
		}
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
		if err := c.sendRawFor(control, iceMsg); err != nil {
			logger.Error("Failed to send ICE candidate: %v", err)
		}
	})

	// Wire ICE complete callback.
	mgr.SetOnICEComplete(func() {
		if !c.activeWebRTC(mgr, control) {
			return
		}
		connID := mgr.ConnectionID()
		logger.Info("ICE gathering complete, notifying server: connectionId=%s", connID)
		completeMsg, err := protocol.NewWebRTCICECompleteMessage(connID)
		if err != nil {
			logger.Error("Failed to create ICE complete message: %v", err)
			return
		}
		if err := c.sendRawFor(control, completeMsg); err != nil {
			logger.Error("Failed to send ICE complete: %v", err)
		}
	})

	// Wire WebRTC state change callback for GUI progress display.
	mgr.SetOnStateChange(func(to clientwebrtc.ConnectionState) {
		if !c.activeWebRTC(mgr, control) {
			return
		}
		if cb := c.OnWebRTCStateChange; cb != nil {
			cb(to.String())
		}
	})

	// Wire fallback callback: stop routing via WebRTC and notify the server.
	mgr.SetOnFallback(func(reason string) {
		if !c.activeWebRTC(mgr, control) {
			return
		}
		connID := mgr.ConnectionID()
		logger.Warn("WebRTC failed, falling back to TCP relay: connectionId=%s, reason=%s", connID, reason)

		// Mark that we are no longer using WebRTC for data routing.
		c.webrtcMu.Lock()
		if c.webrtcManager != mgr {
			c.webrtcMu.Unlock()
			return
		}
		c.usingWebRTC = false
		c.webrtcMu.Unlock()

		failedMsg, err := protocol.NewWebRTCFailedMessage(connID, reason)
		if err != nil {
			logger.Error("Failed to create WebRTC failed message: %v", err)
			return
		}
		if err := c.sendRawFor(control, failedMsg); err != nil {
			logger.Error("Failed to send WebRTC failed: %v", err)
		}
	})

	// Determine the WebRTC establishment timeout.
	webrtcTimeout := 8 * time.Second
	if c.webrtcCfg != nil && c.webrtcCfg.WebRTCTimeout > 0 {
		webrtcTimeout = c.webrtcCfg.WebRTCTimeout
	}

	// Use a CreateOffer timeout derived from the configured WebRTC timeout so it
	// scales with the deployment environment rather than being a fixed constant.
	ctx, cancel := context.WithTimeout(c.ctx, webrtcTimeout*4)
	defer cancel()

	c.signalingMu.Lock()
	if !c.activeWebRTC(mgr, control) {
		c.signalingMu.Unlock()
		return
	}
	offer, err := mgr.CreateOffer(ctx)
	c.signalingMu.Unlock()
	if err != nil {
		logger.Error("Failed to create WebRTC offer: %v", err)
		// Close the manager so it transitions to a failed state and triggers
		// the fallback callback (if set), allowing the client to continue via TCP relay.
		c.closeWebRTCManager(mgr)
		return
	}

	connID := mgr.ConnectionID()
	logger.Info("Sending WebRTC offer to server: connectionId=%s", connID)

	offerMsg, err := protocol.NewWebRTCOfferMessage(connID, offer.SDP)
	if err != nil {
		logger.Error("Failed to create offer message: %v", err)
		return
	}
	if err := c.sendRawFor(control, offerMsg); err != nil {
		logger.Error("Failed to send WebRTC offer: %v", err)
		return
	}

	// Mark WebRTC as enabled only after the offer has been successfully sent.
	c.webrtcMu.Lock()
	if c.webrtcManager == mgr {
		c.webrtcEnabled = true
	}
	c.webrtcMu.Unlock()

	// Start a timeout watchdog: if WebRTC is not connected within webrtcTimeout,
	// trigger fallback to TCP relay.
	c.launch(func() {
		timer := time.NewTimer(webrtcTimeout)
		defer timer.Stop()
		select {
		case <-c.ctx.Done():
			return
		case <-timer.C:
			if c.activeWebRTC(mgr, control) && !mgr.IsConnected() {
				logger.Warn("WebRTC connection timed out after %v, triggering fallback: connectionId=%s",
					webrtcTimeout, connID)
				c.closeWebRTCManager(mgr)
			}
		}
	})
}

// handleWebRTCAnswer processes a TypeWebRTCAnswer message from the server.
func (c *Client) handleWebRTCAnswer(msg *protocol.Message) {
	c.webrtcMu.RLock()
	mgr := c.webrtcManager
	c.webrtcMu.RUnlock()

	if mgr == nil {
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

	if err := mgr.SetRemoteDescription(ctx, sd); err != nil {
		logger.Error("Failed to set remote description: %v", err)
	}
}

// handleWebRTCICECandidate processes a TypeWebRTCICECandidate message from the server.
func (c *Client) handleWebRTCICECandidate(msg *protocol.Message) {
	c.webrtcMu.RLock()
	mgr := c.webrtcManager
	c.webrtcMu.RUnlock()

	if mgr == nil {
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

	if err := mgr.AddICECandidate(ctx, init); err != nil {
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

	// Switch data routing to WebRTC path.
	c.webrtcMu.Lock()
	c.usingWebRTC = true
	c.webrtcMu.Unlock()
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

	// Ensure data routing falls back to TCP.
	c.webrtcMu.Lock()
	c.usingWebRTC = false
	mgr := c.webrtcManager
	c.webrtcMu.Unlock()

	// Close the local WebRTC manager so it transitions to failed/closed state.
	if mgr != nil {
		c.closeWebRTCManager(mgr)
	}
}
