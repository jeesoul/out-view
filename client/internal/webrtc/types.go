package webrtc

import "fmt"

// ConnectionState represents the WebRTC connection state.
type ConnectionState int32

const (
	StateIdle              ConnectionState = iota
	StateGatheringICE                      // ICE candidate collection in progress
	StateConnecting                        // DTLS handshake in progress
	StateWebRTCConnected                   // DataChannel open
	StateWebRTCFailed                      // WebRTC failed, may fallback to TCP
	StateWebRTCReconnecting                // Background reconnect attempt
	StateTCPRelay                          // Using TCP relay
	StateClosing                           // Shutting down
	StateClosed                            // Fully closed
)

func (s ConnectionState) String() string {
	switch s {
	case StateIdle:
		return "Idle"
	case StateGatheringICE:
		return "GatheringICE"
	case StateConnecting:
		return "Connecting"
	case StateWebRTCConnected:
		return "WebRTCConnected"
	case StateWebRTCFailed:
		return "WebRTCFailed"
	case StateWebRTCReconnecting:
		return "WebRTCReconnecting"
	case StateTCPRelay:
		return "TCPRelay"
	case StateClosing:
		return "Closing"
	case StateClosed:
		return "Closed"
	default:
		return fmt.Sprintf("Unknown(%d)", int(s))
	}
}

// stateTransitions defines valid state transitions.
var stateTransitions = map[ConnectionState][]ConnectionState{
	StateIdle:               {StateGatheringICE, StateTCPRelay, StateClosing},
	StateGatheringICE:       {StateConnecting, StateWebRTCFailed, StateClosing},
	StateConnecting:         {StateWebRTCConnected, StateWebRTCFailed, StateClosing},
	StateWebRTCConnected:    {StateWebRTCReconnecting, StateTCPRelay, StateClosing},
	StateWebRTCFailed:       {StateTCPRelay, StateWebRTCReconnecting, StateClosing},
	StateWebRTCReconnecting: {StateGatheringICE, StateTCPRelay, StateClosing},
	StateTCPRelay:           {StateGatheringICE, StateClosing},
	StateClosing:            {StateClosed},
	StateClosed:             {},
}

// isValidTransition returns true if transitioning from -> to is allowed.
func isValidTransition(from, to ConnectionState) bool {
	allowed, ok := stateTransitions[from]
	if !ok {
		return false
	}
	for _, s := range allowed {
		if s == to {
			return true
		}
	}
	return false
}
