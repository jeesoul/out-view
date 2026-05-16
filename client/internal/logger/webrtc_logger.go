package logger

import (
	"context"
	"log/slog"
	"os"
	"time"
)

// WebRTCLogger wraps a structured slog.Logger for WebRTC-specific events.
// All methods emit a single log line with the connectionId field already
// bound, plus event-specific structured fields.
type WebRTCLogger struct {
	logger *slog.Logger
}

// NewWebRTCLogger creates a new WebRTCLogger backed by the default JSON
// handler. The connectionId is bound on every log line.
func NewWebRTCLogger(connectionID string) *WebRTCLogger {
	h := slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelDebug})
	return &WebRTCLogger{
		logger: slog.New(h).With(
			slog.String("component", "webrtc"),
			slog.String("connectionId", connectionID),
		),
	}
}

// NewWebRTCLoggerFromSlog wraps an existing slog.Logger and binds the
// connectionId field. Useful when callers want to share an underlying
// handler (test capture, lumberjack rotation, etc.).
func NewWebRTCLoggerFromSlog(base *slog.Logger, connectionID string) *WebRTCLogger {
	if base == nil {
		base = slog.Default()
	}
	return &WebRTCLogger{
		logger: base.With(
			slog.String("component", "webrtc"),
			slog.String("connectionId", connectionID),
		),
	}
}

// With returns a derived logger with the additional attrs bound.
func (w *WebRTCLogger) With(args ...any) *WebRTCLogger {
	return &WebRTCLogger{logger: w.logger.With(args...)}
}

// Slog returns the underlying slog.Logger so callers can pass it to APIs
// that take *slog.Logger directly.
func (w *WebRTCLogger) Slog() *slog.Logger { return w.logger }

// ConnectionEstablished logs a successful WebRTC connection establishment.
func (w *WebRTCLogger) ConnectionEstablished(connectionID string, duration time.Duration) {
	w.logger.LogAttrs(context.Background(), slog.LevelInfo, "webrtc connection established",
		slog.String("event", "connection_established"),
		slog.String("targetConnectionId", connectionID),
		slog.Duration("establishDuration", duration),
		slog.Float64("establishMs", float64(duration.Milliseconds())),
	)
}

// ConnectionFailed logs a failed WebRTC connection attempt.
func (w *WebRTCLogger) ConnectionFailed(connectionID, reason string) {
	w.logger.LogAttrs(context.Background(), slog.LevelError, "webrtc connection failed",
		slog.String("event", "connection_failed"),
		slog.String("targetConnectionId", connectionID),
		slog.String("reason", reason),
	)
}

// Fallback logs a fallback to TCP relay.
func (w *WebRTCLogger) Fallback(connectionID, reason string) {
	w.logger.LogAttrs(context.Background(), slog.LevelWarn, "webrtc fallback to tcp",
		slog.String("event", "fallback"),
		slog.String("targetConnectionId", connectionID),
		slog.String("reason", reason),
	)
}

// ICECandidate logs an ICE candidate observation.
func (w *WebRTCLogger) ICECandidate(connectionID, candidateType, address string) {
	w.logger.LogAttrs(context.Background(), slog.LevelDebug, "ice candidate",
		slog.String("event", "ice_candidate"),
		slog.String("targetConnectionId", connectionID),
		slog.String("candidateType", candidateType),
		slog.String("address", address),
	)
}

// StateTransition logs a state machine transition. from/to are accepted as
// any to keep the API friendly to both ConnectionState and string callers.
func (w *WebRTCLogger) StateTransition(connectionID string, from, to interface{}) {
	w.logger.LogAttrs(context.Background(), slog.LevelInfo, "webrtc state transition",
		slog.String("event", "state_transition"),
		slog.String("targetConnectionId", connectionID),
		slog.Any("from", from),
		slog.Any("to", to),
	)
}
