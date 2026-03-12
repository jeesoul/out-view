package protocol

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"time"
)

// MessageHeader represents the protocol message header (12 bytes)
type MessageHeader struct {
	Magic    int    // 4 bytes
	Version  byte   // 1 byte
	Type     byte   // 1 byte
	Length   int    // 4 bytes
	Reserved int16  // 2 bytes
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

// HeartbeatResponse represents a heartbeat response body
type HeartbeatResponse struct {
	Success   bool  `json:"success"`
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
			Magic:    MagicNumber,
			Version:  Version,
			Type:     TypeRegister,
			Length:   len(body),
			Reserved: 0,
		},
		Body: body,
	}, nil
}

// NewHeartbeatMessage creates a new heartbeat message
func NewHeartbeatMessage() (*Message, error) {
	req := HeartbeatRequest{
		Timestamp: time.Now().UnixMilli(),
	}
	body, err := json.Marshal(req)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal heartbeat request: %w", err)
	}

	return &Message{
		Header: &MessageHeader{
			Magic:    MagicNumber,
			Version:  Version,
			Type:     TypeHeartbeat,
			Length:   len(body),
			Reserved: 0,
		},
		Body: body,
	}, nil
}

// NewDataMessage creates a new data message
func NewDataMessage(data []byte) *Message {
	return &Message{
		Header: &MessageHeader{
			Magic:    MagicNumber,
			Version:  Version,
			Type:     TypeData,
			Length:   len(data),
			Reserved: 0,
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

// ParseHeartbeatResponse parses a heartbeat response from body
func ParseHeartbeatResponse(body []byte) (*HeartbeatResponse, error) {
	var resp HeartbeatResponse
	if err := json.Unmarshal(body, &resp); err != nil {
		return nil, fmt.Errorf("failed to parse heartbeat response: %w", err)
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
	ConnectionID string `json:"connectionId"`
	Data         []byte `json:"-"`
}

// DataPacketJSON is used for JSON unmarshaling
type DataPacketJSON struct {
	ConnectionID string `json:"connectionId"`
	Data         string `json:"data"`
}

// ParseDataPacket parses a data message body with connection ID
// Format: {"connectionId":"xxx","data":"base64_encoded_data"}
func ParseDataPacket(body []byte) (*DataPacket, error) {
	var raw DataPacketJSON
	if err := json.Unmarshal(body, &raw); err != nil {
		return nil, fmt.Errorf("failed to parse data packet: %w", err)
	}

	data, err := base64.StdEncoding.DecodeString(raw.Data)
	if err != nil {
		return nil, fmt.Errorf("failed to decode base64 data: %w", err)
	}

	return &DataPacket{
		ConnectionID: raw.ConnectionID,
		Data:         data,
	}, nil
}

// NewDataMessageWithConnectionID creates a data message with connection ID
func NewDataMessageWithConnectionID(connectionID string, data []byte) *Message {
	packet := map[string]interface{}{
		"connectionId": connectionID,
		"data":         base64.StdEncoding.EncodeToString(data),
	}
	body, _ := json.Marshal(packet)

	return &Message{
		Header: &MessageHeader{
			Magic:    MagicNumber,
			Version:  Version,
			Type:     TypeData,
			Length:   len(body),
			Reserved: 0,
		},
		Body: body,
	}
}