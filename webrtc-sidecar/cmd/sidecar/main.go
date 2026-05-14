// cmd/sidecar/main.go - WebRTC Sidecar main entry point.
// Uses platform-specific IPC: Unix Domain Socket on Linux/macOS, Named Pipe on Windows.
//
// Usage:
//
//	go run cmd/sidecar/main.go
//	go run cmd/sidecar/main.go -ipc /tmp/outview-webrtc.sock        (Linux/macOS)
//	go run cmd/sidecar/main.go -ipc \\.\pipe\outview-webrtc         (Windows)
package main

import (
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"runtime"
	"syscall"

	"github.com/outview/webrtc-sidecar/internal/ipc"
)

func defaultIPCAddress() string {
	if runtime.GOOS == "windows" {
		return `\\.\pipe\outview-webrtc`
	}
	return "/tmp/outview-webrtc.sock"
}

func main() {
	ipcAddr := flag.String("ipc", defaultIPCAddress(), "IPC address (Unix socket path or Windows Named Pipe)")
	flag.Parse()

	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.Printf("[Sidecar] Starting on %s (platform: %s/%s)", *ipcAddr, runtime.GOOS, runtime.GOARCH)

	// Remove stale Unix socket if it exists
	if runtime.GOOS != "windows" {
		os.Remove(*ipcAddr)
	}

	srv := ipc.NewServer(ipc.DefaultHandler)
	if err := srv.ListenIPC(*ipcAddr); err != nil {
		log.Fatalf("[Sidecar] Failed to start IPC server: %v", err)
	}

	fmt.Printf("IPC_ADDR=%s\n", *ipcAddr)
	log.Println("[Sidecar] Ready. Waiting for connections...")

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	log.Printf("[Sidecar] Received signal %v, shutting down...", sig)
	srv.Close()
	log.Println("[Sidecar] Stopped.")
}
