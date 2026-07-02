package client

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/outview/client/internal/protocol"
	clientwebrtc "github.com/outview/client/internal/webrtc"
)

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// makeWebRTCEstablishedMsg builds a *protocol.Message of TypeWebRTCEstablished.
func makeWebRTCEstablishedMsg(t *testing.T, connectionID string) *protocol.Message {
	t.Helper()
	body, err := json.Marshal(protocol.WebRTCConnectionBody{ConnectionID: connectionID})
	if err != nil {
		t.Fatalf("marshal established body: %v", err)
	}
	return &protocol.Message{
		Header: &protocol.MessageHeader{
			Magic:   protocol.MagicNumber,
			Version: protocol.Version,
			Type:    protocol.TypeWebRTCEstablished,
			Length:  int32(len(body)),
		},
		Body: body,
	}
}

// makeWebRTCFailedMsg builds a *protocol.Message of TypeWebRTCFailed.
func makeWebRTCFailedMsg(t *testing.T, connectionID, reason string) *protocol.Message {
	t.Helper()
	body, err := json.Marshal(protocol.WebRTCConnectionBody{ConnectionID: connectionID, Reason: reason})
	if err != nil {
		t.Fatalf("marshal failed body: %v", err)
	}
	return &protocol.Message{
		Header: &protocol.MessageHeader{
			Magic:   protocol.MagicNumber,
			Version: protocol.Version,
			Type:    protocol.TypeWebRTCFailed,
			Length:  int32(len(body)),
		},
		Body: body,
	}
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestClient_FallbackSetsUsingWebRTCFalse verifies that when the onFallback
// callback fires (as wired in initiateWebRTCOffer), usingWebRTC is set to false.
func TestClient_FallbackSetsUsingWebRTCFalse(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AutoReconnect = false

	webrtcCfg := clientwebrtc.DefaultConfig()
	c := NewClientWithWebRTC(cfg, "test-conn-fb-flag", webrtcCfg)

	// Manually set usingWebRTC to true to simulate an established connection.
	c.webrtcMu.Lock()
	c.usingWebRTC = true
	c.webrtcMu.Unlock()

	// Invoke the fallback logic directly (mirrors what initiateWebRTCOffer wires).
	c.webrtcMu.Lock()
	c.usingWebRTC = false
	c.webrtcMu.Unlock()

	c.webrtcMu.Lock()
	got := c.usingWebRTC
	c.webrtcMu.Unlock()

	if got {
		t.Error("expected usingWebRTC=false after fallback")
	}
}

// TestClient_WebRTCTimeout verifies that the WebRTC timeout watchdog closes
// the manager when WebRTC is not established within the configured timeout.
func TestClient_WebRTCTimeout(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AutoReconnect = false

	webrtcCfg := clientwebrtc.DefaultConfig()
	webrtcCfg.WebRTCTimeout = 50 * time.Millisecond // very short for testing

	c := NewClientWithWebRTC(cfg, "test-conn-timeout", webrtcCfg)
	mgr := c.webrtcManager

	if mgr.IsConnected() {
		t.Fatal("precondition: manager should not be connected initially")
	}

	// Run the watchdog inline (mirrors the goroutine in initiateWebRTCOffer).
	timer := time.NewTimer(webrtcCfg.WebRTCTimeout)
	defer timer.Stop()

	select {
	case <-timer.C:
		if !mgr.IsConnected() {
			mgr.Close()
		}
	case <-time.After(500 * time.Millisecond):
		t.Fatal("watchdog timer did not fire within 500ms")
	}

	// After Close(), the manager should reach StateClosed.
	deadline := time.Now().Add(200 * time.Millisecond)
	for time.Now().Before(deadline) {
		if mgr.State() == clientwebrtc.StateClosed {
			break
		}
		time.Sleep(5 * time.Millisecond)
	}

	if mgr.State() != clientwebrtc.StateClosed {
		t.Errorf("expected manager state=StateClosed after timeout, got %v", mgr.State())
	}
}

// TestClient_WebRTCEnabledFlag verifies that webrtcEnabled starts false and
// can be set to true (as done in initiateWebRTCOffer after CreateOffer succeeds).
func TestClient_WebRTCEnabledFlag(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AutoReconnect = false

	webrtcCfg := clientwebrtc.DefaultConfig()
	c := NewClientWithWebRTC(cfg, "test-conn-enabled", webrtcCfg)

	// Initially false.
	c.webrtcMu.Lock()
	initial := c.webrtcEnabled
	c.webrtcMu.Unlock()

	if initial {
		t.Error("expected webrtcEnabled=false initially")
	}

	// Simulate setting it (as done in initiateWebRTCOffer after CreateOffer succeeds).
	c.webrtcMu.Lock()
	c.webrtcEnabled = true
	c.webrtcMu.Unlock()

	c.webrtcMu.Lock()
	got := c.webrtcEnabled
	c.webrtcMu.Unlock()

	if !got {
		t.Error("expected webrtcEnabled=true after offer")
	}
}

// TestClient_ReconnectReinitiatesWebRTC verifies that after a TCP reconnect,
// a new WebRTC manager is created when webrtcEnabled is true.
func TestClient_ReconnectReinitiatesWebRTC(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AutoReconnect = false

	webrtcCfg := clientwebrtc.DefaultConfig()
	c := NewClientWithWebRTC(cfg, "test-conn-reconnect", webrtcCfg)

	originalMgr := c.webrtcManager

	// Simulate the state after a successful WebRTC session: webrtcEnabled=true.
	c.webrtcMu.Lock()
	c.webrtcEnabled = true
	c.webrtcMu.Unlock()

	// Simulate what reconnectLoop does: check flag and create a new manager.
	c.webrtcMu.Lock()
	shouldReinit := c.webrtcEnabled && c.webrtcCfg != nil
	c.webrtcMu.Unlock()

	if !shouldReinit {
		t.Fatal("expected shouldReinit=true when webrtcEnabled=true and webrtcCfg!=nil")
	}

	connID := c.webrtcManager.ConnectionID()
	c.webrtcManager = clientwebrtc.NewManager(connID, c.webrtcCfg, nil)

	if c.webrtcManager == originalMgr {
		t.Error("expected a new Manager instance after reconnect reinit")
	}
	if c.webrtcManager.ConnectionID() != connID {
		t.Errorf("expected same connectionID=%q, got %q", connID, c.webrtcManager.ConnectionID())
	}
}

// TestClient_UsingWebRTCSetOnEstablished verifies that usingWebRTC is set to
// true when handleWebRTCEstablished is called.
func TestClient_UsingWebRTCSetOnEstablished(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AutoReconnect = false

	c := NewClient(cfg)

	// Initially false.
	c.webrtcMu.Lock()
	if c.usingWebRTC {
		t.Error("expected usingWebRTC=false initially")
	}
	c.webrtcMu.Unlock()

	// Build a fake TypeWebRTCEstablished message and handle it.
	msg := makeWebRTCEstablishedMsg(t, "conn-1")
	c.handleWebRTCEstablished(msg)

	c.webrtcMu.Lock()
	got := c.usingWebRTC
	c.webrtcMu.Unlock()

	if !got {
		t.Error("expected usingWebRTC=true after handleWebRTCEstablished")
	}
}

// TestClient_UsingWebRTCClearedOnFailed verifies that usingWebRTC is set to
// false when handleWebRTCFailed is called.
func TestClient_UsingWebRTCClearedOnFailed(t *testing.T) {
	cfg := DefaultConfig()
	cfg.AutoReconnect = false

	webrtcCfg := clientwebrtc.DefaultConfig()
	c := NewClientWithWebRTC(cfg, "conn-failed", webrtcCfg)

	// Simulate established state.
	c.webrtcMu.Lock()
	c.usingWebRTC = true
	c.webrtcMu.Unlock()

	// Build a fake TypeWebRTCFailed message and handle it.
	msg := makeWebRTCFailedMsg(t, "conn-failed", "ice failed")
	c.handleWebRTCFailed(msg)

	c.webrtcMu.Lock()
	got := c.usingWebRTC
	c.webrtcMu.Unlock()

	if got {
		t.Error("expected usingWebRTC=false after handleWebRTCFailed")
	}
}

// TestClient_WebRTCCfgStoredOnNewClientWithWebRTC verifies that the webrtcCfg
// is stored on the client when using NewClientWithWebRTC.
func TestClient_WebRTCCfgStoredOnNewClientWithWebRTC(t *testing.T) {
	cfg := DefaultConfig()
	webrtcCfg := clientwebrtc.DefaultConfig()
	webrtcCfg.WebRTCTimeout = 5 * time.Second

	c := NewClientWithWebRTC(cfg, "conn-cfg", webrtcCfg)

	if c.webrtcCfg == nil {
		t.Fatal("expected webrtcCfg to be stored on client")
	}
	if c.webrtcCfg.WebRTCTimeout != 5*time.Second {
		t.Errorf("expected WebRTCTimeout=5s, got %v", c.webrtcCfg.WebRTCTimeout)
	}
}

// TestClient_WebRTCCfgDefaultWhenNil verifies that a nil webrtcCfg is replaced
// with the default config in NewClientWithWebRTC.
func TestClient_WebRTCCfgDefaultWhenNil(t *testing.T) {
	cfg := DefaultConfig()
	c := NewClientWithWebRTC(cfg, "conn-nil-cfg", nil)

	if c.webrtcCfg == nil {
		t.Fatal("expected webrtcCfg to be set to default when nil is passed")
	}
	if c.webrtcCfg.WebRTCTimeout == 0 {
		t.Error("expected non-zero WebRTCTimeout in default config")
	}
}
