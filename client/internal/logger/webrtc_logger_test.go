package logger

import (
	"bytes"
	"encoding/json"
	"log/slog"
	"strings"
	"testing"
	"time"
)

func newTestLogger(t *testing.T, buf *bytes.Buffer) *WebRTCLogger {
	t.Helper()
	h := slog.NewJSONHandler(buf, &slog.HandlerOptions{Level: slog.LevelDebug})
	return NewWebRTCLoggerFromSlog(slog.New(h), "test-conn-1")
}

func parseLines(t *testing.T, buf *bytes.Buffer) []map[string]interface{} {
	t.Helper()
	out := []map[string]interface{}{}
	for _, line := range strings.Split(strings.TrimSpace(buf.String()), "\n") {
		if line == "" {
			continue
		}
		var m map[string]interface{}
		if err := json.Unmarshal([]byte(line), &m); err != nil {
			t.Fatalf("invalid json log line %q: %v", line, err)
		}
		out = append(out, m)
	}
	return out
}

func TestWebRTCLogger_ConnectionEstablished(t *testing.T) {
	var buf bytes.Buffer
	l := newTestLogger(t, &buf)
	l.ConnectionEstablished("c1", 250*time.Millisecond)
	lines := parseLines(t, &buf)
	if len(lines) != 1 {
		t.Fatalf("expected 1 log line, got %d", len(lines))
	}
	got := lines[0]
	if got["event"] != "connection_established" {
		t.Errorf("event = %v, want connection_established", got["event"])
	}
	if got["component"] != "webrtc" {
		t.Errorf("component = %v, want webrtc", got["component"])
	}
	if got["connectionId"] != "test-conn-1" {
		t.Errorf("connectionId = %v, want test-conn-1", got["connectionId"])
	}
	if got["targetConnectionId"] != "c1" {
		t.Errorf("targetConnectionId = %v, want c1", got["targetConnectionId"])
	}
	if got["establishMs"].(float64) != 250 {
		t.Errorf("establishMs = %v, want 250", got["establishMs"])
	}
}

func TestWebRTCLogger_ConnectionFailed(t *testing.T) {
	var buf bytes.Buffer
	l := newTestLogger(t, &buf)
	l.ConnectionFailed("c2", "ice timeout")
	lines := parseLines(t, &buf)
	if len(lines) != 1 {
		t.Fatalf("expected 1 line, got %d", len(lines))
	}
	got := lines[0]
	if got["event"] != "connection_failed" {
		t.Errorf("event = %v", got["event"])
	}
	if got["reason"] != "ice timeout" {
		t.Errorf("reason = %v", got["reason"])
	}
	if got["level"] != "ERROR" {
		t.Errorf("level = %v, want ERROR", got["level"])
	}
}

func TestWebRTCLogger_Fallback(t *testing.T) {
	var buf bytes.Buffer
	l := newTestLogger(t, &buf)
	l.Fallback("c3", "dtls handshake failed")
	got := parseLines(t, &buf)[0]
	if got["event"] != "fallback" {
		t.Errorf("event = %v", got["event"])
	}
	if got["level"] != "WARN" {
		t.Errorf("level = %v, want WARN", got["level"])
	}
}

func TestWebRTCLogger_ICECandidate(t *testing.T) {
	var buf bytes.Buffer
	l := newTestLogger(t, &buf)
	l.ICECandidate("c4", "host", "192.168.1.10")
	got := parseLines(t, &buf)[0]
	if got["candidateType"] != "host" {
		t.Errorf("candidateType = %v", got["candidateType"])
	}
	if got["address"] != "192.168.1.10" {
		t.Errorf("address = %v", got["address"])
	}
	if got["level"] != "DEBUG" {
		t.Errorf("level = %v, want DEBUG", got["level"])
	}
}

func TestWebRTCLogger_StateTransition(t *testing.T) {
	var buf bytes.Buffer
	l := newTestLogger(t, &buf)
	l.StateTransition("c5", "Idle", "GatheringICE")
	got := parseLines(t, &buf)[0]
	if got["event"] != "state_transition" {
		t.Errorf("event = %v", got["event"])
	}
	if got["from"] != "Idle" {
		t.Errorf("from = %v", got["from"])
	}
	if got["to"] != "GatheringICE" {
		t.Errorf("to = %v", got["to"])
	}
}

func TestWebRTCLogger_With(t *testing.T) {
	var buf bytes.Buffer
	l := newTestLogger(t, &buf).With(slog.String("scenario", "p2p"))
	l.ConnectionEstablished("cx", 5*time.Millisecond)
	got := parseLines(t, &buf)[0]
	if got["scenario"] != "p2p" {
		t.Errorf("scenario = %v, want p2p", got["scenario"])
	}
}
