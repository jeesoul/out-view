package protocol

import (
	"encoding/binary"
	"fmt"
	"io"
)

// Decoder decodes binary data to Message
type Decoder struct {
	reader io.Reader
}

// NewDecoder creates a new decoder
func NewDecoder(reader io.Reader) *Decoder {
	return &Decoder{reader: reader}
}

// Decode reads and decodes a message from the underlying reader
func (d *Decoder) Decode() (*Message, error) {
	// Read header (12 bytes)
	headerBuf := make([]byte, HeaderLength)
	if _, err := io.ReadFull(d.reader, headerBuf); err != nil {
		return nil, fmt.Errorf("failed to read header: %w", err)
	}

	// Parse header
	header := &MessageHeader{
		Magic:    int(binary.BigEndian.Uint32(headerBuf[0:4])),
		Version:  headerBuf[4],
		Type:     headerBuf[5],
		Length:   int(binary.BigEndian.Uint32(headerBuf[6:10])),
		Reserved: int16(binary.BigEndian.Uint16(headerBuf[10:12])),
	}

	// Validate magic number
	if header.Magic != MagicNumber {
		return nil, fmt.Errorf("invalid magic number: 0x%08X, expected: 0x%08X", header.Magic, MagicNumber)
	}

	// Validate body length
	if header.Length < 0 || header.Length > MaxBodyLength {
		return nil, fmt.Errorf("invalid body length: %d", header.Length)
	}

	// Read body
	var body []byte
	if header.Length > 0 {
		body = make([]byte, header.Length)
		if _, err := io.ReadFull(d.reader, body); err != nil {
			return nil, fmt.Errorf("failed to read body: %w", err)
		}
	}

	return &Message{
		Header: header,
		Body:   body,
	}, nil
}

// DecodeFromBytes decodes a message from a byte slice
// Returns the message and the number of bytes consumed
func DecodeFromBytes(data []byte) (*Message, int, error) {
	if len(data) < HeaderLength {
		return nil, 0, fmt.Errorf("insufficient data for header: %d bytes", len(data))
	}

	// Parse header
	header := &MessageHeader{
		Magic:    int(binary.BigEndian.Uint32(data[0:4])),
		Version:  data[4],
		Type:     data[5],
		Length:   int(binary.BigEndian.Uint32(data[6:10])),
		Reserved: int16(binary.BigEndian.Uint16(data[10:12])),
	}

	// Validate magic number
	if header.Magic != MagicNumber {
		return nil, 0, fmt.Errorf("invalid magic number: 0x%08X, expected: 0x%08X", header.Magic, MagicNumber)
	}

	// Validate body length
	if header.Length < 0 || header.Length > MaxBodyLength {
		return nil, 0, fmt.Errorf("invalid body length: %d", header.Length)
	}

	totalLen := HeaderLength + header.Length
	if len(data) < totalLen {
		return nil, 0, fmt.Errorf("insufficient data for body: need %d, have %d", totalLen, len(data))
	}

	// Extract body
	var body []byte
	if header.Length > 0 {
		body = make([]byte, header.Length)
		copy(body, data[HeaderLength:totalLen])
	}

	return &Message{
		Header: header,
		Body:   body,
	}, totalLen, nil
}