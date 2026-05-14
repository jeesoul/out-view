// cmd/poc/main.go - WebRTC Sidecar IPC POC server
// Validates Java ↔ Go IPC communication via TCP (Windows-compatible fallback).
//
// Usage:
//
//	go run cmd/poc/main.go
//	go run cmd/poc/main.go -addr 127.0.0.1:9999
package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/outview/webrtc-sidecar/internal/ipc"
)

var (
	addr       = flag.String("addr", "127.0.0.1:9999", "TCP address to listen on")
	socketPath = flag.String("socket", "", "Unix socket path (Linux/macOS only; overrides -addr)")
)

func main() {
	flag.Parse()

	log.SetFlags(log.LstdFlags | log.Lmicroseconds)
	log.Println("[POC] WebRTC Sidecar IPC Server starting...")

	srv := ipc.NewServer(ipc.DefaultHandler)

	var listenErr error
	if *socketPath != "" {
		// Remove stale socket file if it exists
		os.Remove(*socketPath)
		listenErr = srv.ListenUnix(*socketPath)
	} else {
		listenErr = srv.ListenTCP(*addr)
	}

	if listenErr != nil {
		log.Fatalf("[POC] Failed to start server: %v", listenErr)
	}

	// Print connection info for Java client
	if *socketPath != "" {
		fmt.Printf("IPC_SOCKET=%s\n", *socketPath)
	} else {
		fmt.Printf("IPC_ADDR=%s\n", *addr)
	}

	// Run a self-test to verify the server works
	go func() {
		time.Sleep(200 * time.Millisecond)
		runSelfTest(*addr, *socketPath)
	}()

	// Wait for shutdown signal
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	sig := <-sigCh
	log.Printf("[POC] Received signal %v, shutting down...", sig)
	srv.Close()
	log.Println("[POC] Server stopped.")
}

// runSelfTest performs a quick self-test to verify the server is working.
func runSelfTest(tcpAddr, unixPath string) {
	log.Println("[POC] Running self-test...")

	var conn net.Conn
	var err error

	if unixPath != "" {
		conn, err = net.Dial("unix", unixPath)
	} else {
		conn, err = net.Dial("tcp", tcpAddr)
	}
	if err != nil {
		log.Printf("[POC] Self-test: dial error: %v", err)
		return
	}
	defer conn.Close()

	// Send ping
	ping := ipc.PingPayload{
		Timestamp: time.Now().UnixMilli(),
		Message:   "self-test ping",
	}
	payload, _ := json.Marshal(ping)
	req := &ipc.Message{Type: "ping", Payload: payload}

	if err := ipc.WriteMessage(conn, req); err != nil {
		log.Printf("[POC] Self-test: write error: %v", err)
		return
	}

	// Read pong
	resp, err := ipc.ReadMessage(conn)
	if err != nil {
		log.Printf("[POC] Self-test: read error: %v", err)
		return
	}

	if resp.Type != "pong" {
		log.Printf("[POC] Self-test: unexpected response type: %s", resp.Type)
		return
	}

	var pong ipc.PongPayload
	if err := json.Unmarshal(resp.Payload, &pong); err != nil {
		log.Printf("[POC] Self-test: unmarshal pong error: %v", err)
		return
	}

	latency := pong.ServerTime - ping.Timestamp
	log.Printf("[POC] Self-test PASSED: echo=%q, latency=%dms", pong.EchoMessage, latency)
	log.Println("[POC] Server is ready to accept Java client connections.")
}
