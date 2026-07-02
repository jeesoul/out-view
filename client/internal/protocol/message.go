package protocol

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"time"
)

// MessageHeader represents the protocol message header (12 bytes)
type MessageHeader struct {
	Magic    int32 // 4 bytes
	Version  byte  // 1 byte
	Type     byte  // 1 byte
	Length   int32 // 4 bytes
	Reserved int16 // 2 bytes
}

// Message represents a complete protocol message
type Message struct {
	Header *MessageHeader
	Body   []byte
}

// RegisterRequest represents a registration request body
type RegisterRequest struct {
	DeviceID  string `json:"deviceId"`
	Token     string `json:"token"`
	LocalPort int    `json:"localPort"`
}

// RegisterResponse represents a registration response body
type RegisterResponse struct {
	Success      bool   `json:"success"`
	DeviceID     string `json:"deviceId"`
	ExternalPort int    `json:"externalPort"`
	Message      string `json:"message,omitempty"`
}

// HeartbeatRequest represents a heartbeat request body
type HeartbeatRequest struct {
	Timestamp int64 `json:"timestamp"`
}

// ErrorResponse represents an error response body
type ErrorResponse struct {
	Message string `json:"message"`
}

// NewRegisterMessage creates a new registration message
func NewRegisterMessage(deviceID, token string, localPort int) (*Message, error) {
	req := RegisterRequest{
		DeviceID:  deviceID,
		Token:     token,
		LocalPort: localPort,
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal register request: %w", err)
	}

	return &Message{
		Header: &MessageHeader{
			Magic:   MagicNumber,
			Version: Version,
			Type:    TypeRegister,
			Length:  int32(len(body)),
		},
		Body: body,
	}, nil
}

// NewHeartbeatMessage creates a new heartbeat message
func NewHeartbeatMessage() (*Message, error) {
	req := HeartbeatRequest{Timestamp: time.Now().UnixMilli()}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal heartbeat request: %w", err)
	}

	return &Message{
		Header: &MessageHeader{
			Magic:   MagicNumber,
			Version: Version,
			Type:    TypeHeartbeat,
			Length:  int32(len(body)),
		},
		Body: body,
	}, nil
}

// NewDataMessage creates a plain data message (no connection ID).
func NewDataMessage(data []byte) *Message {
	return &Message{
		Header: &MessageHeader{
			Magic:   MagicNumber,
			Version: Version,
			Type:    TypeData,
			Length:  int32(len(data)),
		},
		Body: data,
	}
}

// ParseRegisterResponse parses a register response from body
func ParseRegisterResponse(body []byte) (*RegisterResponse, error) {
	var resp RegisterResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse register response: %w", err)
	}
	return &resp, nil
}

// ParseErrorResponse parses an error response from body
func ParseErrorResponse(body []byte) (*ErrorResponse, error) {
	var resp ErrorResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse error response: %w", err)
	}
	return &resp, nil
}

// DataPacket represents a data packet with connection ID
type DataPacket struct {
	ConnectionID string
	Data         []byte
}

// NewDataMessageWithConnectionID creates a data message with connection ID.
//
// Binary body format:
//
//	[2B big-endian: connectionId length][connectionId bytes][payload bytes]
func NewDataMessageWithConnectionID(connectionID string, data []byte) *Message {
	idBytes := []byte(connectionID)
	body := make([]byte, 2+len(idBytes)+len(data))
	binary.BigEndian.PutUint16(body[0:2], uint16(len(idBytes)))
	copy(body[2:], idBytes)
	copy(body[2+len(idBytes):], data)

	return &Message{
		Header: &MessageHeader{
			Magic:   MagicNumber,
			Version: Version,
			Type:    TypeData,
			Length:  int32(len(body)),
		},
		Body: body,
	}
}

// ParseDataPacket parses a data message body with connection ID.
//
// Binary body format:
//
//	[2B big-endian: connectionId length][connectionId bytes][payload bytes]
func ParseDataPacket(body []byte) (*DataPacket, error) {
	if len(body) < 2 {
		return nil, fmt.Errorf("data packet too short: %d bytes", len(body))
	}
	idLen := int(binary.BigEndian.Uint16(body[0:2]))
	if 2+idLen > len(body) {
		return nil, fmt.Errorf("data packet connectionId length %d exceeds body", idLen)
	}
	connectionID := string(body[2 : 2+idLen])
	data := body[2+idLen:]
	return &DataPacket{ConnectionID: connectionID, Data: data}, nil
}

// WebRTCOfferBody is the body for TypeWebRTCOffer and TypeWebRTCAnswer messages.
type WebRTCOfferBody struct {
	ConnectionID string `json:"connectionId"`
	SDP          string `json:"sdp"`
	SDPType      string `json:"sdpType"` // "offer" or "answer"
}

// WebRTCICECandidateBody is the body for TypeWebRTCICECandidate messages.
type WebRTCICECandidateBody struct {
	ConnectionID  string  `json:"connectionId"`
	Candidate     string  `json:"candidate"`
	SDPMid        string  `json:"sdpMid,omitempty"`
	SDPMLineIndex *uint16 `json:"sdpMLineIndex,omitempty"`
}

// WebRTCConnectionBody is the body for TypeWebRTCICEComplete, TypeWebRTCEstablished, TypeWebRTCFailed.
type WebRTCConnectionBody struct {
	ConnectionID string `json:"connectionId"`
	Reason       string `json:"reason,omitempty"` // only for Failed
}

// NewWebRTCOfferMessage creates a TypeWebRTCOffer message.
func NewWebRTCOfferMessage(connectionID, sdp string) (*Message, error) {
	body, err := json.Marshal(WebRTCOfferBody{ConnectionID: connectionID, SDP: sdp, SDPType: "offer"})
	if err != nil {
		return nil, fmt.Errorf("marshal webrtc offer: %w", err)
	}
	return &Message{
		Header: &MessageHeader{
			Magic:   MagicNumber,
			Version: Version,
			Type:    TypeWebRTCOffer,
			Length:  int32(len(body)),
		},
		Body: body,
	}, nil
}

// NewWebRTCAnswerMessage creates a TypeWebRTCAnswer message.
func NewWebRTCAnswerMessage(connectionID, sdp string) (*Message, error) {
	body, err := json.Marshal(WebRTCOfferBody{ConnectionID: connectionID, SDP: sdp, SDPType: "answer"})
	if err != nil {
		return nil, fmt.Errorf("marshal webrtc answer: %w", err)
	}
	return &Message{
		Header: &MessageHeader{
			Magic:   MagicNumber,
			Version: Version,
			Type:    TypeWebRTCAnswer,
			Length:  int32(len(body)),
		},
		Body: body,
	}, nil
}

// NewWebRTCICECandidateMessage creates a TypeWebRTCICECandidate message.
func NewWebRTCICECandidateMessage(connectionID, candidate, sdpMid string, sdpMLineIndex *uint16) (*Message, error) {
	body, err := json.Marshal(WebRTCICECandidateBody{
		ConnectionID:  connectionID,
		Candidate:     candidate,
		SDPMid:        sdpMid,
		SDPMLineIndex: sdpMLineIndex,
	})
	if err != nil {
		return nil, fmt.Errorf("marshal webrtc ice candidate: %w", err)
	}
	return &Message{
		Header: &MessageHeader{
			Magic:   MagicNumber,
			Version: Version,
			Type:    TypeWebRTCICECandidate,
			Length:  int32(len(body)),
		},
		Body: body,
	}, nil
}

// NewWebRTCICECompleteMessage creates a TypeWebRTCICEComplete message.
func NewWebRTCICECompleteMessage(connectionID string) (*Message, error) {
	body, err := json.Marshal(WebRTCConnectionBody{ConnectionID: connectionID})
	if err != nil {
		return nil, fmt.Errorf("marshal webrtc ice complete: %w", err)
	}
	return &Message{
		Header: &MessageHeader{
			Magic:   MagicNumber,
			Version: Version,
			Type:    TypeWebRTCICEComplete,
			Length:  int32(len(body)),
		},
		Body: body,
	}, nil
}

// NewWebRTCEstablishedMessage creates a TypeWebRTCEstablished message.
func NewWebRTCEstablishedMessage(connectionID string) (*Message, error) {
	body, err := json.Marshal(WebRTCConnectionBody{ConnectionID: connectionID})
	if err != nil {
		return nil, fmt.Errorf("marshal webrtc established: %w", err)
	}
	return &Message{
		Header: &MessageHeader{
			Magic:   MagicNumber,
			Version: Version,
			Type:    TypeWebRTCEstablished,
			Length:  int32(len(body)),
		},
		Body: body,
	}, nil
}

// NewWebRTCFailedMessage creates a TypeWebRTCFailed message.
func NewWebRTCFailedMessage(connectionID, reason string) (*Message, error) {
	body, err := json.Marshal(WebRTCConnectionBody{ConnectionID: connectionID, Reason: reason})
	if err != nil {
		return nil, fmt.Errorf("marshal webrtc failed: %w", err)
	}
	return &Message{
		Header: &MessageHeader{
			Magic:   MagicNumber,
			Version: Version,
			Type:    TypeWebRTCFailed,
			Length:  int32(len(body)),
		},
		Body: body,
	}, nil
}

// ParseWebRTCOfferBody parses the body of a TypeWebRTCOffer or TypeWebRTCAnswer message.
func ParseWebRTCOfferBody(body []byte) (*WebRTCOfferBody, error) {
	var b WebRTCOfferBody
	if err := json.Unmarshal(body, &b); err != nil {
		return nil, fmt.Errorf("parse webrtc offer body: %w", err)
	}
	return &b, nil
}

// ParseWebRTCICECandidateBody parses the body of a TypeWebRTCICECandidate message.
func ParseWebRTCICECandidateBody(body []byte) (*WebRTCICECandidateBody, error) {
	var b WebRTCICECandidateBody
	if err := json.Unmarshal(body, &b); err != nil {
		return nil, fmt.Errorf("parse webrtc ice candidate body: %w", err)
	}
	return &b, nil
}

// ParseWebRTCConnectionBody parses the body of ICEComplete, Established, or Failed messages.
func ParseWebRTCConnectionBody(body []byte) (*WebRTCConnectionBody, error) {
	var b WebRTCConnectionBody
	if err := json.Unmarshal(body, &b); err != nil {
		return nil, fmt.Errorf("parse webrtc connection body: %w", err)
	}
	return &b, nil
}

// DeviceQueryRequest is the body for TypeDeviceQuery.
type DeviceQueryRequest struct {
	DeviceCode string `json:"deviceCode"`
}

// DeviceQueryResponse is the body for TypeDeviceQueryAck.
type DeviceQueryResponse struct {
	Found        bool   `json:"found"`
	DeviceCode   string `json:"deviceCode,omitempty"`
	ExternalPort int    `json:"externalPort,omitempty"`
	Message      string `json:"message,omitempty"`
}

// NewDeviceQueryMessage creates a device query message.
func NewDeviceQueryMessage(deviceCode string) (*Message, error) {
	body, err := json.Marshal(DeviceQueryRequest{DeviceCode: deviceCode})
	if err != nil {
		return nil, fmt.Errorf("marshal device query: %w", err)
	}
	return &Message{
		Header: &MessageHeader{
			Magic:   MagicNumber,
			Version: Version,
			Type:    TypeDeviceQuery,
			Length:  int32(len(body)),
		},
		Body: body,
	}, nil
}

// ParseDeviceQueryResponse parses a TypeDeviceQueryAck body.
func ParseDeviceQueryResponse(body []byte) (*DeviceQueryResponse, error) {
	var resp DeviceQueryResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("parse device query response: %w", err)
	}
	return &resp, nil
}

// NewCloseConnectionMessage creates a close-connection notification.
// Body format: same binary framing — connectionId with zero-length payload.
func NewCloseConnectionMessage(connectionID string) *Message {
	idBytes := []byte(connectionID)
	body := make([]byte, 2+len(idBytes))
	binary.BigEndian.PutUint16(body[0:2], uint16(len(idBytes)))
	copy(body[2:], idBytes)

	return &Message{
		Header: &MessageHeader{
			Magic:   MagicNumber,
			Version: Version,
			Type:    TypeCloseConnection,
			Length:  int32(len(body)),
		},
		Body: body,
	}
}
