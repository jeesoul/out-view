//go:build !headless_test

package main

import (
	"encoding/json"
	"os"
	"path/filepath"
)

// WebRTCSettings holds user-configurable WebRTC options persisted to disk.
type WebRTCSettings struct {
	Enabled         bool   `json:"enabled"`
	STUNServers     string `json:"stunServers"`
	TURNServer      string `json:"turnServer"`
	TURNUsername    string `json:"turnUsername"`
	TURNPassword    string `json:"turnPassword"`
	TransportPolicy string `json:"transportPolicy"`
}

func defaultWebRTCSettings() *WebRTCSettings {
	return &WebRTCSettings{
		Enabled: true,
		STUNServers: "stun:stun.l.google.com:19302\n" +
			"stun:stun1.l.google.com:19302\n" +
			"stun:stun.qq.com:3478",
		TransportPolicy: "all",
	}
}

func webrtcSettingsPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = "."
	}
	return filepath.Join(dir, "outview", "webrtc.json")
}

func loadWebRTCSettings() *WebRTCSettings {
	data, err := os.ReadFile(webrtcSettingsPath())
	if err != nil {
		return defaultWebRTCSettings()
	}
	s := defaultWebRTCSettings()
	if err := json.Unmarshal(data, s); err != nil {
		return defaultWebRTCSettings()
	}
	return s
}

func saveWebRTCSettings(s *WebRTCSettings) error {
	path := webrtcSettingsPath()
	if err := os.MkdirAll(filepath.Dir(path), 0700); err != nil {
		return err
	}
	data, err := json.MarshalIndent(s, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, data, 0600)
}
