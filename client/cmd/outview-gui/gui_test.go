//go:build headless_test

// Headless GUI tests for the outview-gui binary. These tests intentionally
// do not exercise any Fyne widgets directly so they can compile and run on
// CI hosts without OpenGL/X11. The widget-construction paths are covered
// by the integration tests in client/internal/gui/.
//
// Build / run:
//
//	go test -tags=headless_test ./cmd/outview-gui/
//
// Without the build tag, the normal GUI binary is built (main.go is
// included, this file is excluded).
package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
	"time"

	clientwebrtc "github.com/outview/client/internal/webrtc"
)

// guiConfig mirrors the on-disk JSON shape used by the GUI to persist the
// last-used connection settings. Kept here (and in internal/guitest) so the
// schema is owned by the GUI binary, not by the WebRTC core.
type guiConfig struct {
	ServerHost string             `json:"server_host"`
	ServerPort int                `json:"server_port"`
	DeviceID   string             `json:"device_id"`
	LocalPort  int                `json:"local_port"`
	WebRTC     webrtcConfigFields `json:"webrtc"`
}

type webrtcConfigFields struct {
	Enabled       bool     `json:"enabled"`
	STUNServers   []string `json:"stun_servers"`
	WebRTCTimeout string   `json:"webrtc_timeout"`
	DTLSTimeout   string   `json:"dtls_timeout"`
	IdleTimeout   string   `json:"idle_timeout"`
}

func saveGUIConfig(path string, cfg guiConfig) error {
	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0o600)
}

func loadGUIConfig(path string) (guiConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return guiConfig{}, err
	}
	var cfg guiConfig
	return cfg, json.Unmarshal(data, &cfg)
}

func TestGUI_ConfigSaveLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	original := guiConfig{
		ServerHost: "192.168.1.100",
		ServerPort: 7000,
		DeviceID:   "test-device-001",
		LocalPort:  3389,
		WebRTC: webrtcConfigFields{
			Enabled:       true,
			STUNServers:   []string{"stun:stun.l.google.com:19302", "stun:stun.qq.com:3478"},
			WebRTCTimeout: "8s",
			DTLSTimeout:   "10s",
			IdleTimeout:   "60s",
		},
	}

	if err := saveGUIConfig(path, original); err != nil {
		t.Fatalf("saveGUIConfig: %v", err)
	}
	loaded, err := loadGUIConfig(path)
	if err != nil {
		t.Fatalf("loadGUIConfig: %v", err)
	}
	if loaded.ServerHost != original.ServerHost {
		t.Errorf("ServerHost: got %q, want %q", loaded.ServerHost, original.ServerHost)
	}
	if loaded.ServerPort != original.ServerPort {
		t.Errorf("ServerPort: got %d, want %d", loaded.ServerPort, original.ServerPort)
	}
	if loaded.DeviceID != original.DeviceID {
		t.Errorf("DeviceID: got %q, want %q", loaded.DeviceID, original.DeviceID)
	}
	if loaded.WebRTC.Enabled != original.WebRTC.Enabled {
		t.Errorf("WebRTC.Enabled: got %v, want %v", loaded.WebRTC.Enabled, original.WebRTC.Enabled)
	}
	if len(loaded.WebRTC.STUNServers) != len(original.WebRTC.STUNServers) {
		t.Errorf("STUNServers count: got %d, want %d",
			len(loaded.WebRTC.STUNServers), len(original.WebRTC.STUNServers))
	}
	for i, s := range original.WebRTC.STUNServers {
		if i >= len(loaded.WebRTC.STUNServers) || loaded.WebRTC.STUNServers[i] != s {
			t.Errorf("STUNServers[%d]: got %v, want %q", i, loaded.WebRTC.STUNServers, s)
		}
	}
	if loaded.WebRTC.WebRTCTimeout != original.WebRTC.WebRTCTimeout {
		t.Errorf("WebRTCTimeout: got %q, want %q",
			loaded.WebRTC.WebRTCTimeout, original.WebRTC.WebRTCTimeout)
	}
}

func TestGUI_ConfigSaveLoad_Defaults(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")

	minimal := guiConfig{ServerHost: "localhost"}
	if err := saveGUIConfig(path, minimal); err != nil {
		t.Fatalf("saveGUIConfig: %v", err)
	}
	loaded, err := loadGUIConfig(path)
	if err != nil {
		t.Fatalf("loadGUIConfig: %v", err)
	}
	if loaded.ServerHost != "localhost" {
		t.Errorf("ServerHost: got %q, want %q", loaded.ServerHost, "localhost")
	}
	if loaded.WebRTC.Enabled {
		t.Error("WebRTC.Enabled should default to false")
	}
}

func TestGUI_ConfigSaveLoad_InvalidFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte("not valid json"), 0o600); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	if _, err := loadGUIConfig(path); err == nil {
		t.Error("expected error loading invalid JSON config")
	}
}

func TestGUI_ConfigSaveLoad_MissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "does-not-exist.json")
	if _, err := loadGUIConfig(path); err == nil {
		t.Error("expected error loading missing config file")
	}
}

// connectionStateDisplay is the headless representation of the visible
// WebRTC indicator: a localized label and a semantic color name. The real
// widget (client/internal/gui/connection_status.go) maps these to canvas
// colors; here we just verify the mapping itself.
type connectionStateDisplay struct {
	text  string
	color string
}

func stateToDisplay(state clientwebrtc.ConnectionState) connectionStateDisplay {
	switch state {
	case clientwebrtc.StateWebRTCConnected:
		return connectionStateDisplay{"WebRTC ✓", "green"}
	case clientwebrtc.StateWebRTCReconnecting:
		return connectionStateDisplay{"WebRTC 重连中...", "yellow"}
	case clientwebrtc.StateWebRTCFailed, clientwebrtc.StateTCPRelay:
		return connectionStateDisplay{"TCP 降级", "orange"}
	case clientwebrtc.StateClosing, clientwebrtc.StateClosed:
		return connectionStateDisplay{"已断开", "red"}
	default:
		return connectionStateDisplay{"连接中...", "gray"}
	}
}

func TestGUI_ConnectionStateDisplay(t *testing.T) {
	tests := []struct {
		state     clientwebrtc.ConnectionState
		wantText  string
		wantColor string
	}{
		{clientwebrtc.StateWebRTCConnected, "WebRTC ✓", "green"},
		{clientwebrtc.StateWebRTCReconnecting, "WebRTC 重连中...", "yellow"},
		{clientwebrtc.StateWebRTCFailed, "TCP 降级", "orange"},
		{clientwebrtc.StateTCPRelay, "TCP 降级", "orange"},
		{clientwebrtc.StateClosing, "已断开", "red"},
		{clientwebrtc.StateClosed, "已断开", "red"},
		{clientwebrtc.StateIdle, "连接中...", "gray"},
		{clientwebrtc.StateGatheringICE, "连接中...", "gray"},
		{clientwebrtc.StateConnecting, "连接中...", "gray"},
	}
	for _, tt := range tests {
		t.Run(tt.state.String(), func(t *testing.T) {
			got := stateToDisplay(tt.state)
			if got.text != tt.wantText {
				t.Errorf("text: got %q, want %q", got.text, tt.wantText)
			}
			if got.color != tt.wantColor {
				t.Errorf("color: got %q, want %q", got.color, tt.wantColor)
			}
		})
	}
}

// trayState is the headless representation of the system tray state.
// The real tray (client/internal/gui/tray.go) maps these to icons and
// menu tooltips; here we verify the state-machine portion only.
type trayState struct {
	webrtcConnected bool
	tooltip         string
	iconName        string
}

func updateTrayState(connected bool) trayState {
	if connected {
		return trayState{true, "outView - WebRTC 已连接", "icon_webrtc"}
	}
	return trayState{false, "outView - TCP 连接", "icon_tcp"}
}

func TestGUI_TrayIcon(t *testing.T) {
	state := updateTrayState(false)
	if state.webrtcConnected {
		t.Error("initial state should not be WebRTC connected")
	}
	if state.iconName != "icon_tcp" {
		t.Errorf("icon: got %q, want %q", state.iconName, "icon_tcp")
	}

	state = updateTrayState(true)
	if !state.webrtcConnected {
		t.Error("state should be WebRTC connected")
	}
	if state.iconName != "icon_webrtc" {
		t.Errorf("icon: got %q, want %q", state.iconName, "icon_webrtc")
	}
	if state.tooltip == "" {
		t.Error("tooltip should be set when connected")
	}

	state = updateTrayState(false)
	if state.webrtcConnected {
		t.Error("state should not be WebRTC connected after disconnect")
	}
}

func TestGUI_WebRTCConfigToManagerConfig(t *testing.T) {
	guiCfg := webrtcConfigFields{
		Enabled:       true,
		STUNServers:   []string{"stun:stun.l.google.com:19302"},
		WebRTCTimeout: "8s",
		DTLSTimeout:   "10s",
		IdleTimeout:   "60s",
	}

	webrtcTimeout, err := time.ParseDuration(guiCfg.WebRTCTimeout)
	if err != nil {
		t.Fatalf("parse WebRTCTimeout: %v", err)
	}
	dtlsTimeout, err := time.ParseDuration(guiCfg.DTLSTimeout)
	if err != nil {
		t.Fatalf("parse DTLSTimeout: %v", err)
	}
	idleTimeout, err := time.ParseDuration(guiCfg.IdleTimeout)
	if err != nil {
		t.Fatalf("parse IdleTimeout: %v", err)
	}

	if webrtcTimeout != 8*time.Second {
		t.Errorf("WebRTCTimeout: got %v, want %v", webrtcTimeout, 8*time.Second)
	}
	if dtlsTimeout != 10*time.Second {
		t.Errorf("DTLSTimeout: got %v, want %v", dtlsTimeout, 10*time.Second)
	}
	if idleTimeout != 60*time.Second {
		t.Errorf("IdleTimeout: got %v, want %v", idleTimeout, 60*time.Second)
	}

	cfg := clientwebrtc.DefaultConfig()
	cfg.EnableWebRTC = guiCfg.Enabled
	cfg.WebRTCTimeout = webrtcTimeout
	cfg.DTLSTimeout = dtlsTimeout
	cfg.IdleTimeout = idleTimeout

	if !cfg.EnableWebRTC {
		t.Error("EnableWebRTC should be true")
	}
	if cfg.WebRTCTimeout != 8*time.Second {
		t.Errorf("cfg.WebRTCTimeout: got %v, want %v", cfg.WebRTCTimeout, 8*time.Second)
	}
}
