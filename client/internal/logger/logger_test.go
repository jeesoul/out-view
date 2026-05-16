package logger

import (
	"bytes"
	"errors"
	"log"
	"strings"
	"testing"
)

// captureLog redirects the standard log package output to a buffer for
// the duration of the test, then restores the previous output.
func captureLog(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	prev := log.Writer()
	prevFlags := log.Flags()
	log.SetOutput(&buf)
	log.SetFlags(0) // strip date/time so assertions stay deterministic
	t.Cleanup(func() {
		log.SetOutput(prev)
		log.SetFlags(prevFlags)
	})
	return &buf
}

func resetLevel(t *testing.T) {
	t.Helper()
	prev := GetLevel()
	t.Cleanup(func() { SetLevel(prev) })
}

func TestParseLevel(t *testing.T) {
	cases := map[string]Level{
		"DEBUG":   LevelDebug,
		"debug":   LevelDebug,
		"info":    LevelInfo,
		" Warn ":  LevelWarn,
		"WARNING": LevelWarn,
		"error":   LevelError,
		"FATAL":   LevelFatal,
		"off":     LevelOff,
		"silent":  LevelOff,
		"none":    LevelOff,
		"":        LevelInfo,
		"random":  LevelInfo,
	}
	for input, want := range cases {
		if got := ParseLevel(input); got != want {
			t.Errorf("ParseLevel(%q) = %v, want %v", input, got, want)
		}
	}
}

func TestLevelString(t *testing.T) {
	if LevelDebug.String() != "DEBUG" {
		t.Errorf("LevelDebug.String() = %q", LevelDebug.String())
	}
	if LevelOff.String() != "OFF" {
		t.Errorf("LevelOff.String() = %q", LevelOff.String())
	}
	if Level(99).String() == "" {
		t.Error("unknown level should produce non-empty string")
	}
}

func TestSetGetLevel(t *testing.T) {
	resetLevel(t)
	SetLevel(LevelWarn)
	if GetLevel() != LevelWarn {
		t.Errorf("GetLevel() = %v, want LevelWarn", GetLevel())
	}
	if IsEnabled(LevelInfo) {
		t.Error("LevelInfo should be filtered out at LevelWarn")
	}
	if !IsEnabled(LevelWarn) {
		t.Error("LevelWarn should be enabled at LevelWarn")
	}
	if !IsEnabled(LevelError) {
		t.Error("LevelError should be enabled at LevelWarn")
	}
}

func TestLevelFiltering(t *testing.T) {
	resetLevel(t)
	buf := captureLog(t)

	SetLevel(LevelWarn)
	Debug("debug-message")
	Info("info-message")
	Warn("warn-message")
	Error("error-message")

	out := buf.String()
	if strings.Contains(out, "debug-message") {
		t.Errorf("debug should be filtered: %q", out)
	}
	if strings.Contains(out, "info-message") {
		t.Errorf("info should be filtered: %q", out)
	}
	if !strings.Contains(out, "warn-message") {
		t.Errorf("warn should be present: %q", out)
	}
	if !strings.Contains(out, "error-message") {
		t.Errorf("error should be present: %q", out)
	}
}

func TestLevelOffSilencesEverything(t *testing.T) {
	resetLevel(t)
	buf := captureLog(t)

	SetLevel(LevelOff)
	Debug("d")
	Info("i")
	Warn("w")
	Error("e")
	if buf.Len() != 0 {
		t.Errorf("LevelOff should silence all output, got %q", buf.String())
	}
}

func TestStructuredFields(t *testing.T) {
	resetLevel(t)
	buf := captureLog(t)
	SetLevel(LevelDebug)

	Infow("test_event",
		String("connectionId", "abc-123"),
		String("state", "Connecting"),
		Int("retries", 2),
	)
	out := buf.String()
	if !strings.Contains(out, "[INFO]") {
		t.Errorf("missing INFO prefix: %q", out)
	}
	if !strings.Contains(out, "test_event") {
		t.Errorf("missing message: %q", out)
	}
	if !strings.Contains(out, "connectionId=abc-123") {
		t.Errorf("missing connectionId field: %q", out)
	}
	if !strings.Contains(out, "state=Connecting") {
		t.Errorf("missing state field: %q", out)
	}
	if !strings.Contains(out, "retries=2") {
		t.Errorf("missing retries field: %q", out)
	}
}

func TestStructuredQuoting(t *testing.T) {
	resetLevel(t)
	buf := captureLog(t)
	SetLevel(LevelDebug)

	Errorw("event_with_spaces",
		String("reason", "DTLS handshake failed"),
		String("addr", "192.168.1.1:3478"),
	)
	out := buf.String()
	if !strings.Contains(out, `reason="DTLS handshake failed"`) {
		t.Errorf("value with spaces should be quoted: %q", out)
	}
	if !strings.Contains(out, "addr=192.168.1.1:3478") {
		t.Errorf("value without whitespace should not be quoted: %q", out)
	}
}

func TestErrField(t *testing.T) {
	f := Err(errors.New("boom"))
	if f.Key != "err" || f.Value != "boom" {
		t.Errorf("Err: got %+v", f)
	}

	nilF := Err(nil)
	if nilF.Key != "err" || nilF.Value != nil {
		t.Errorf("Err(nil): got %+v", nilF)
	}
}

func TestWebRTCConnectionEstablished(t *testing.T) {
	resetLevel(t)
	buf := captureLog(t)
	SetLevel(LevelDebug)

	WebRTCConnectionEstablished("conn-1", 250)
	out := buf.String()
	if !strings.Contains(out, "event=connection_established") {
		t.Errorf("missing event field: %q", out)
	}
	if !strings.Contains(out, "connectionId=conn-1") {
		t.Errorf("missing connectionId: %q", out)
	}
	if !strings.Contains(out, "establishMs=250") {
		t.Errorf("missing establishMs: %q", out)
	}
	if !strings.Contains(out, "[INFO]") {
		t.Errorf("expected INFO level: %q", out)
	}
}

func TestWebRTCConnectionFailed(t *testing.T) {
	resetLevel(t)
	buf := captureLog(t)
	SetLevel(LevelDebug)

	WebRTCConnectionFailed("conn-2", "ice timeout")
	out := buf.String()
	if !strings.Contains(out, "event=connection_failed") {
		t.Errorf("missing event field: %q", out)
	}
	if !strings.Contains(out, "connectionId=conn-2") {
		t.Errorf("missing connectionId: %q", out)
	}
	if !strings.Contains(out, `reason="ice timeout"`) {
		t.Errorf("missing/unquoted reason: %q", out)
	}
	if !strings.Contains(out, "[ERROR]") {
		t.Errorf("expected ERROR level: %q", out)
	}
}

func TestWebRTCFallback(t *testing.T) {
	resetLevel(t)
	buf := captureLog(t)
	SetLevel(LevelDebug)

	WebRTCFallback("conn-3", "DTLS handshake failed")
	out := buf.String()
	if !strings.Contains(out, "event=fallback") {
		t.Errorf("missing event field: %q", out)
	}
	if !strings.Contains(out, "[WARN]") {
		t.Errorf("expected WARN level: %q", out)
	}
}

func TestWebRTCICECandidate(t *testing.T) {
	resetLevel(t)
	buf := captureLog(t)
	SetLevel(LevelDebug)

	WebRTCICECandidate("conn-4", "host", "192.168.1.10")
	out := buf.String()
	if !strings.Contains(out, "event=ice_candidate") {
		t.Errorf("missing event field: %q", out)
	}
	if !strings.Contains(out, "candidateType=host") {
		t.Errorf("missing candidateType: %q", out)
	}
	if !strings.Contains(out, "address=192.168.1.10") {
		t.Errorf("missing address: %q", out)
	}
	if !strings.Contains(out, "[DEBUG]") {
		t.Errorf("expected DEBUG level: %q", out)
	}
}

func TestWebRTCICECandidateFilteredAtInfo(t *testing.T) {
	resetLevel(t)
	buf := captureLog(t)
	SetLevel(LevelInfo)

	WebRTCICECandidate("conn-5", "host", "10.0.0.1")
	if buf.Len() != 0 {
		t.Errorf("ICE candidate (DEBUG) should be filtered at INFO level, got %q", buf.String())
	}
}

func TestWebRTCStateTransition(t *testing.T) {
	resetLevel(t)
	buf := captureLog(t)
	SetLevel(LevelDebug)

	WebRTCStateTransition("conn-6", "Idle", "GatheringICE", "offer created")
	out := buf.String()
	if !strings.Contains(out, "event=state_transition") {
		t.Errorf("missing event field: %q", out)
	}
	if !strings.Contains(out, "from=Idle") {
		t.Errorf("missing from field: %q", out)
	}
	if !strings.Contains(out, "to=GatheringICE") {
		t.Errorf("missing to field: %q", out)
	}
	if !strings.Contains(out, `reason="offer created"`) {
		t.Errorf("missing reason: %q", out)
	}
}

func TestSprintf(t *testing.T) {
	got := Sprintf("hello %s %d", "world", 42)
	if got != "hello world 42" {
		t.Errorf("Sprintf = %q", got)
	}
}

func TestUnstructuredAPIPreserved(t *testing.T) {
	resetLevel(t)
	buf := captureLog(t)
	SetLevel(LevelDebug)

	Info("connection from %s", "client-A")
	out := buf.String()
	if !strings.Contains(out, "[INFO]") {
		t.Errorf("missing INFO prefix: %q", out)
	}
	if !strings.Contains(out, "connection from client-A") {
		t.Errorf("missing formatted message: %q", out)
	}
}
