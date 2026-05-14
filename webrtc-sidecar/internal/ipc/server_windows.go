//go:build windows

package ipc

import (
	"net"

	winio "github.com/Microsoft/go-winio"
)

// listenIPC creates a platform-specific IPC listener.
// On Windows, uses Named Pipe for efficient local IPC.
func listenIPC(address string) (net.Listener, error) {
	return winio.ListenPipe(address, nil)
}
