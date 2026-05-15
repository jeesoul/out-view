package webrtc

import (
	"time"

	pionwebrtc "github.com/pion/webrtc/v4"
)

const (
	BufferHighWaterMark = 1 * 1024 * 1024 // 1MB
	BufferLowWaterMark  = 512 * 1024      // 512KB
)

// Config holds WebRTC configuration for the client Manager.
type Config struct {
	EnableWebRTC       bool
	ICEServers         []pionwebrtc.ICEServer
	WebRTCTimeout      time.Duration // total connection timeout (default 8s)
	DTLSTimeout        time.Duration // DTLS handshake timeout (default 10s)
	ICETransportPolicy string        // "all" or "relay"
	IdleTimeout        time.Duration // close after this long with no data (default 60s, 0=disabled)
}

// DefaultConfig returns a Config with sensible defaults.
func DefaultConfig() *Config {
	return &Config{
		EnableWebRTC:  true,
		WebRTCTimeout: 8 * time.Second,
		DTLSTimeout:   10 * time.Second,
		IdleTimeout:   60 * time.Second,
		ICEServers: []pionwebrtc.ICEServer{
			{URLs: []string{"stun:stun.l.google.com:19302"}},
			{URLs: []string{"stun:stun1.l.google.com:19302"}},
			{URLs: []string{"stun:stun.qq.com:3478"}},
		},
		ICETransportPolicy: "all",
	}
}
