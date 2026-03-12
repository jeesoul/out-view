package protocol

import (
	"encoding/binary"
	"fmt"
	"io"
)

// Encoder encodes Message to binary format
type Encoder struct {
	writer io.Writer
}

// NewEncoder creates a new encoder
func NewEncoder(writer io.Writer) *Encoder {
	return &Encoder{writer: writer}
}

// Encode encodes a message to binary format and writes to the underlying writer
// Binary format:
// - Magic: 4 bytes (big-endian)
// - Version: 1 byte
// - Type: 1 byte
// - Length: 4 bytes (big-endian)
// - Reserved: 2 bytes (big-endian)
// - Body: Length bytes
func (e *Encoder) Encode(msg *Message) error {
	if msg == nil || msg.Header == nil {
		return fmt.Errorf("message or header is nil")
	}

	// Validate message
	if msg.Header.Magic != MagicNumber {
		return fmt.Errorf("invalid magic number: 0x%08X, expected: 0x%08X", msg.Header.Magic, MagicNumber)
	}

	if msg.Header.Length < 0 || msg.Header.Length > MaxBodyLength {
		return fmt.Errorf("invalid body length: %d", msg.Header.Length)
	}

	// Write header (12 bytes)
	header := make([]byte, HeaderLength)

	// Magic (4 bytes, big-endian)
	binary.BigEndian.PutUint32(header[0:4], uint32(msg.Header.Magic))

	// Version (1 byte)
	header[4] = msg.Header.Version

	// Type (1 byte)
	header[5] = msg.Header.Type

	// Length (4 bytes, big-endian)
	binary.BigEndian.PutUint32(header[6:10], uint32(msg.Header.Length))

	// Reserved (2 bytes, big-endian)
	binary.BigEndian.PutUint16(header[10:12], uint16(msg.Header.Reserved))

	// Write header
	if _, err := e.writer.Write(header); err != nil {
		return fmt.Errorf("failed to write header: %w", err)
	}

	// Write body
	if msg.Body != nil && len(msg.Body) > 0 {
		if _, err := e.writer.Write(msg.Body); err != nil {
			return fmt.Errorf("failed to write body: %w", err)
		}
	}

	return nil
}

// EncodeToBytes encodes a message to a byte slice
func EncodeToBytes(msg *Message) ([]byte, error) {
	if msg == nil || msg.Header == nil {
		return nil, fmt.Errorf("message or header is nil")
	}

	totalLen := HeaderLength + msg.Header.Length
	buf := make([]byte, totalLen)

	// Write header
	binary.BigEndian.PutUint32(buf[0:4], uint32(msg.Header.Magic))
	buf[4] = msg.Header.Version
	buf[5] = msg.Header.Type
	binary.BigEndian.PutUint32(buf[6:10], uint32(msg.Header.Length))
	binary.BigEndian.PutUint16(buf[10:12], uint16(msg.Header.Reserved))

	// Write body
	if msg.Body != nil && len(msg.Body) > 0 {
		copy(buf[HeaderLength:], msg.Body)
	}

	return buf, nil
}