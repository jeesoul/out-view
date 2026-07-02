package logger

import (
	"fmt"
	"log"
	"strings"
	"sync/atomic"
)

// Level represents a log severity. Higher number = more severe.
// Levels are ordered so that a configured minimum level filters out
// anything below it: SetLevel(LevelWarn) silences Debug and Info.
type Level int32

const (
	LevelDebug Level = iota
	LevelInfo
	LevelWarn
	LevelError
	LevelFatal
	// LevelOff disables all logging (useful in tests).
	LevelOff
)

// String returns the canonical short name of the level.
func (l Level) String() string {
	switch l {
	case LevelDebug:
		return "DEBUG"
	case LevelInfo:
		return "INFO"
	case LevelWarn:
		return "WARN"
	case LevelError:
		return "ERROR"
	case LevelFatal:
		return "FATAL"
	case LevelOff:
		return "OFF"
	default:
		return fmt.Sprintf("Level(%d)", int(l))
	}
}

// ParseLevel converts a case-insensitive level name to a Level.
// Unknown values fall back to LevelInfo.
func ParseLevel(s string) Level {
	switch strings.ToUpper(strings.TrimSpace(s)) {
	case "DEBUG":
		return LevelDebug
	case "INFO":
		return LevelInfo
	case "WARN", "WARNING":
		return LevelWarn
	case "ERROR":
		return LevelError
	case "FATAL":
		return LevelFatal
	case "OFF", "NONE", "SILENT":
		return LevelOff
	default:
		return LevelInfo
	}
}

// currentLevel is the runtime log filter, default LevelDebug for
// backwards compatibility (the previous logger always emitted everything).
var currentLevel atomic.Int32

func init() { currentLevel.Store(int32(LevelDebug)) }

// SetLevel updates the runtime log level. Subsequent log calls below
// this level are dropped before formatting.
func SetLevel(l Level) { currentLevel.Store(int32(l)) }

// GetLevel returns the current runtime log level.
func GetLevel() Level { return Level(currentLevel.Load()) }

// IsEnabled reports whether a log call at the given level would be emitted.
// Useful to gate expensive log argument construction.
func IsEnabled(l Level) bool { return l >= GetLevel() }

func emit(l Level, format string, args ...interface{}) {
	if !IsEnabled(l) {
		return
	}
	prefix := levelPrefix(l)
	if l == LevelFatal {
		log.Fatalf(prefix+format, args...)
		return
	}
	log.Printf(prefix+format, args...)
}

func levelPrefix(l Level) string {
	switch l {
	case LevelDebug:
		return "[DEBUG] "
	case LevelInfo:
		return "[INFO]  "
	case LevelWarn:
		return "[WARN]  "
	case LevelError:
		return "[ERROR] "
	case LevelFatal:
		return "[FATAL] "
	default:
		return "[" + l.String() + "] "
	}
}

// Info logs at LevelInfo.
func Info(format string, args ...interface{}) { emit(LevelInfo, format, args...) }

// Debug logs at LevelDebug.
func Debug(format string, args ...interface{}) { emit(LevelDebug, format, args...) }

// Error logs at LevelError.
func Error(format string, args ...interface{}) { emit(LevelError, format, args...) }

// Warn logs at LevelWarn.
func Warn(format string, args ...interface{}) { emit(LevelWarn, format, args...) }

// Fatal logs at LevelFatal and calls os.Exit(1).
// Bypasses level filtering since fatal events must always be visible.
func Fatal(format string, args ...interface{}) {
	log.Fatalf("[FATAL] "+format, args...)
}

// Sprintf returns a formatted string (helper exposed for callers that
// prefer to compose log lines elsewhere).
func Sprintf(format string, args ...interface{}) string {
	return fmt.Sprintf(format, args...)
}

// ----------------------------------------------------------------------
// Structured logging for WebRTC events.
//
// The WebRTC manager emits well-known events (connection_established,
// connection_failed, fallback, ice_candidate, state_transition) that need
// machine-greppable fields — connectionId, state, reason — for log
// aggregation and post-incident analysis. The Field/Event API below
// renders those events as logfmt-style key=value pairs prefixed with
// the level marker, so existing line-oriented log shippers can ingest
// them without any structural change.

// Field is a single structured key/value pair.
type Field struct {
	Key   string
	Value interface{}
}

// String creates a string-valued Field.
func String(k, v string) Field { return Field{Key: k, Value: v} }

// Int creates an int-valued Field.
func Int(k string, v int) Field { return Field{Key: k, Value: v} }

// Int64 creates an int64-valued Field.
func Int64(k string, v int64) Field { return Field{Key: k, Value: v} }

// Any wraps an arbitrary value.
func Any(k string, v interface{}) Field { return Field{Key: k, Value: v} }

// Err creates a Field for an error value, or returns a no-op if err is nil.
func Err(err error) Field {
	if err == nil {
		return Field{Key: "err", Value: nil}
	}
	return Field{Key: "err", Value: err.Error()}
}

func formatFields(fields []Field) string {
	if len(fields) == 0 {
		return ""
	}
	var sb strings.Builder
	for i, f := range fields {
		if i == 0 {
			sb.WriteByte(' ')
		} else {
			sb.WriteByte(' ')
		}
		sb.WriteString(f.Key)
		sb.WriteByte('=')
		writeValue(&sb, f.Value)
	}
	return sb.String()
}

func writeValue(sb *strings.Builder, v interface{}) {
	switch x := v.(type) {
	case nil:
		sb.WriteString("<nil>")
	case string:
		if needsQuote(x) {
			fmt.Fprintf(sb, "%q", x)
		} else {
			sb.WriteString(x)
		}
	default:
		s := fmt.Sprint(v)
		if needsQuote(s) {
			fmt.Fprintf(sb, "%q", s)
		} else {
			sb.WriteString(s)
		}
	}
}

func needsQuote(s string) bool {
	if s == "" {
		return true
	}
	for _, r := range s {
		if r == ' ' || r == '"' || r == '=' || r < 0x20 {
			return true
		}
	}
	return false
}

// Logw emits a structured log line at the given level. Fields are
// rendered as `key=value` pairs after the message.
//
//	logger.Logw(logger.LevelInfo, "state_transition",
//	    logger.String("connectionId", id),
//	    logger.String("from", "Idle"), logger.String("to", "Connecting"))
//
// → [INFO]  state_transition connectionId=abc-123 from=Idle to=Connecting
func Logw(l Level, msg string, fields ...Field) {
	if !IsEnabled(l) {
		return
	}
	prefix := levelPrefix(l)
	log.Print(prefix + msg + formatFields(fields))
}

// Infow / Debugw / Warnw / Errorw mirror the unstructured helpers but
// accept Field arguments for structured output.
func Debugw(msg string, fields ...Field) { Logw(LevelDebug, msg, fields...) }
func Infow(msg string, fields ...Field)  { Logw(LevelInfo, msg, fields...) }
func Warnw(msg string, fields ...Field)  { Logw(LevelWarn, msg, fields...) }
func Errorw(msg string, fields ...Field) { Logw(LevelError, msg, fields...) }

// ----------------------------------------------------------------------
// WebRTC critical-event helpers.
//
// These wrap the structured API with the canonical field names used by
// the rest of the client (connectionId / state / reason) so callers don't
// drift on naming. Aggregators can grep for `event=...` to find each.

// WebRTCConnectionEstablished records that a WebRTC PeerConnection's
// DataChannel reached the open state.
func WebRTCConnectionEstablished(connectionID string, establishMs int64) {
	Infow("webrtc_event",
		String("event", "connection_established"),
		String("connectionId", connectionID),
		Int64("establishMs", establishMs),
	)
}

// WebRTCConnectionFailed records that a WebRTC connection failed before
// or after data was flowing.
func WebRTCConnectionFailed(connectionID, reason string) {
	Errorw("webrtc_event",
		String("event", "connection_failed"),
		String("connectionId", connectionID),
		String("reason", reason),
	)
}

// WebRTCFallback records that the client is falling back to TCP relay.
func WebRTCFallback(connectionID, reason string) {
	Warnw("webrtc_event",
		String("event", "fallback"),
		String("connectionId", connectionID),
		String("reason", reason),
	)
}

// WebRTCICECandidate records that an ICE candidate was observed locally
// or received from the peer.
func WebRTCICECandidate(connectionID, candidateType, address string) {
	Debugw("webrtc_event",
		String("event", "ice_candidate"),
		String("connectionId", connectionID),
		String("candidateType", candidateType),
		String("address", address),
	)
}

// WebRTCStateTransition records a state-machine transition. `from` and
// `to` are accepted as any so the caller can pass either ConnectionState
// values or string labels without coupling this package to internal/webrtc.
func WebRTCStateTransition(connectionID string, from, to interface{}, reason string) {
	Infow("webrtc_event",
		String("event", "state_transition"),
		String("connectionId", connectionID),
		Any("from", from),
		Any("to", to),
		String("reason", reason),
	)
}
