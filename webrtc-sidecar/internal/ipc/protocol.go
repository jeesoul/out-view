package ipc

// Message type constants for WebRTC IPC protocol.
const (
	// Java → Sidecar commands
	MsgCreatePC        = "create_pc"
	MsgSetRemoteSDP    = "set_remote_sdp"
	MsgAddICECandidate = "add_ice_candidate"
	MsgSendData        = "send_data"
	MsgClosePC         = "close_pc"

	// Sidecar → Java events
	MsgEvent = "event"

	// Event subtypes (in EventPayload.Event field)
	EventOffer        = "offer"
	EventAnswer       = "answer"
	EventICECandidate = "ice_candidate"
	EventICEComplete  = "ice_complete"
	EventEstablished  = "established"
	EventFailed       = "failed"
	EventData         = "data"
)

// CreatePCPayload is the payload for create_pc messages.
type CreatePCPayload struct {
	ConnectionID string `json:"connection_id"`
}

// SetRemoteSDPPayload is the payload for set_remote_sdp messages.
type SetRemoteSDPPayload struct {
	ConnectionID string `json:"connection_id"`
	SDP          string `json:"sdp"`
	Type         string `json:"type"` // "offer" or "answer"
}

// AddICECandidatePayload is the payload for add_ice_candidate messages.
type AddICECandidatePayload struct {
	ConnectionID  string  `json:"connection_id"`
	Candidate     string  `json:"candidate"`
	SDPMid        string  `json:"sdp_mid,omitempty"`
	SDPMLineIndex *uint16 `json:"sdp_mline_index,omitempty"`
}

// SendDataPayload is the payload for send_data messages.
type SendDataPayload struct {
	ConnectionID string `json:"connection_id"`
	Data         []byte `json:"data"` // base64-encoded by JSON marshaling
}

// ClosePCPayload is the payload for close_pc messages.
type ClosePCPayload struct {
	ConnectionID string `json:"connection_id"`
}

// EventPayload is the payload for event messages (Sidecar → Java).
type EventPayload struct {
	ConnectionID string `json:"connection_id"`
	Event        string `json:"event"` // one of the Event* constants
	SDP          string `json:"sdp,omitempty"`
	SDPType      string `json:"sdp_type,omitempty"`
	Candidate    string `json:"candidate,omitempty"`
	SDPMid       string `json:"sdp_mid,omitempty"`
	Data         []byte `json:"data,omitempty"`
	Reason       string `json:"reason,omitempty"`
}

// OKPayload is a generic success response.
type OKPayload struct {
	ConnectionID string `json:"connection_id"`
	OK           bool   `json:"ok"`
}

// ErrorPayload is a generic error response.
type ErrorPayload struct {
	ConnectionID string `json:"connection_id"`
	Error        string `json:"error"`
}
