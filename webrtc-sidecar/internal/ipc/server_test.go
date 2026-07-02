package ipc

import (
	"encoding/json"
	"fmt"
	"net"
	"sync"
	"sync/atomic"
	"testing"
	"time"
)

// TestPingPong validates basic ping-pong communication.
func TestPingPong(t *testing.T) {
	srv := NewServer(DefaultHandler)
	if err := srv.ListenTCP("127.0.0.1:0"); err != nil {
		t.Fatalf("ListenTCP: %v", err)
	}
	addr := srv.listener.Addr().String()
	defer srv.Close()

	conn, err := net.Dial("tcp", addr)
	if err != nil {
		t.Fatalf("Dial: %v", err)
	}
	defer conn.Close()

	ping := PingPayload{
		Timestamp: time.Now().UnixMilli(),
		Message:   "hello from test",
	}
	payload, _ := json.Marshal(ping)
	req := &Message{Type: "ping", Payload: payload}

	if err := WriteMessage(conn, req); err != nil {
		t.Fatalf("WriteMessage: %v", err)
	}

	resp, err := ReadMessage(conn)
	if err != nil {
		t.Fatalf("ReadMessage: %v", err)
	}

	if resp.Type != "pong" {
		t.Errorf("expected pong, got %s", resp.Type)
	}

	var pong PongPayload
	if err := json.Unmarshal(resp.Payload, &pong); err != nil {
		t.Fatalf("unmarshal pong: %v", err)
	}
	if pong.EchoMessage != ping.Message {
		t.Errorf("echo mismatch: got %q, want %q", pong.EchoMessage, ping.Message)
	}
	if pong.Timestamp != ping.Timestamp {
		t.Errorf("timestamp mismatch: got %d, want %d", pong.Timestamp, ping.Timestamp)
	}
	t.Logf("Ping-pong OK: latency ~%dms", pong.ServerTime-ping.Timestamp)
}

// TestConcurrent100 validates 100 concurrent connections.
func TestConcurrent100(t *testing.T) {
	const numConns = 100

	srv := NewServer(DefaultHandler)
	if err := srv.ListenTCP("127.0.0.1:0"); err != nil {
		t.Fatalf("ListenTCP: %v", err)
	}
	addr := srv.listener.Addr().String()
	defer srv.Close()

	var (
		wg       sync.WaitGroup
		errCount int64
		okCount  int64
	)

	start := time.Now()
	for i := 0; i < numConns; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()

			conn, err := net.Dial("tcp", addr)
			if err != nil {
				t.Logf("goroutine %d: Dial error: %v", id, err)
				atomic.AddInt64(&errCount, 1)
				return
			}
			defer conn.Close()

			ping := PingPayload{
				Timestamp: time.Now().UnixMilli(),
				Message:   fmt.Sprintf("ping from goroutine %d", id),
			}
			payload, _ := json.Marshal(ping)
			req := &Message{Type: "ping", Payload: payload}

			if err := WriteMessage(conn, req); err != nil {
				t.Logf("goroutine %d: WriteMessage error: %v", id, err)
				atomic.AddInt64(&errCount, 1)
				return
			}

			resp, err := ReadMessage(conn)
			if err != nil {
				t.Logf("goroutine %d: ReadMessage error: %v", id, err)
				atomic.AddInt64(&errCount, 1)
				return
			}

			if resp.Type != "pong" {
				t.Logf("goroutine %d: expected pong, got %s", id, resp.Type)
				atomic.AddInt64(&errCount, 1)
				return
			}
			atomic.AddInt64(&okCount, 1)
		}(i)
	}

	wg.Wait()
	elapsed := time.Since(start)

	t.Logf("100 concurrent connections: %d OK, %d errors, elapsed %v", okCount, errCount, elapsed)

	if errCount > 0 {
		t.Errorf("got %d errors out of %d connections", errCount, numConns)
	}
	if okCount != numConns {
		t.Errorf("expected %d successful connections, got %d", numConns, okCount)
	}
}
