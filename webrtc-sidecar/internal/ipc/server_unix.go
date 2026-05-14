//go:build !windows

package ipc

import "net"

// listenIPC creates a platform-specific IPC listener.
// On Unix/Linux/macOS, uses Unix Domain Socket.
func listenIPC(address string) (net.Listener, error) {
	return net.Listen("unix", address)
}
