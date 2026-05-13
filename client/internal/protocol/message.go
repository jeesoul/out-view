package protocol

import (
	"encoding/binary"
	"encoding/json"
	"fmt"
	"time"
)

// MessageHeader represents the protocol message header (12 bytes)
type MessageHeader struct {
	Magic    int   // 4 bytes
	Version  byte  // 1 byte
	Type     byte  // 1 byte
	Length   int   // 4 bytes
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
			Length:  len(body),
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
			Length:  len(body),
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
			Length:  len(data),
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
			Length:  len(body),
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
			Length:  len(body),
		},
		Body: body,
	}
}
