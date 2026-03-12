// Package protocol defines the outView binary protocol
package protocol

// Magic Number: "OVWS" (0x4F565753)
const MagicNumber = 0x4F565753

// Protocol version
const Version = 1

// Header length: 12 bytes
const HeaderLength = 12

// Maximum body length: 10MB
const MaxBodyLength = 10 * 1024 * 1024

// Message types
const (
	TypeRegister     byte = 1 // Registration request
	TypeHeartbeat    byte = 2 // Heartbeat request
	TypeData         byte = 3 // Data forward
	TypeError        byte = 4 // Error message
	TypeRegisterAck  byte = 5 // Registration response
	TypeHeartbeatAck byte = 6 // Heartbeat response
)

// Default heartbeat interval (30 seconds)
const DefaultHeartbeatInterval = 30

// Default read timeout (60 seconds)
const DefaultReadTimeout = 60