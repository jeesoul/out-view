package ipc

import (
	"encoding/json"
	"net"
	"sync"
	"testing"
)

func TestRouter_Dispatch_KnownType(t *testing.T) {
	registry := NewConnRegistry()
	router := NewRouter(registry)

	called := false
	router.Handle(MsgCreatePC, func(conn net.Conn, msg *Message) *Message {
		called = true
		payload, _ := json.Marshal(OKPayload{ConnectionID: "test", OK: true})
		return &Message{Type: "ok", Payload: payload}
	})

	payload, _ := json.Marshal(CreatePCPayload{ConnectionID: "test"})
	msg := &Message{Type: MsgCreatePC, Payload: payload}
	resp := router.Dispatch(nil, msg)

	if !called {
		t.Error("handler was not called")
	}
	if resp == nil {
		t.Fatal("expected response, got nil")
	}
	if resp.Type != "ok" {
		t.Errorf("expected type 'ok', got %q", resp.Type)
	}
}

func TestRouter_Dispatch_UnknownType(t *testing.T) {
	registry := NewConnRegistry()
	router := NewRouter(registry)

	msg := &Message{Type: "unknown_type"}
	resp := router.Dispatch(nil, msg)

	if resp == nil {
		t.Fatal("expected error response, got nil")
	}
	if resp.Type != "error" {
		t.Errorf("expected type 'error', got %q", resp.Type)
	}

	var errPayload ErrorPayload
	if err := json.Unmarshal(resp.Payload, &errPayload); err != nil {
		t.Fatalf("failed to unmarshal error payload: %v", err)
	}
	if errPayload.Error == "" {
		t.Error("expected non-empty error message")
	}
}

func TestRouter_Dispatch_NilResponse(t *testing.T) {
	registry := NewConnRegistry()
	router := NewRouter(registry)

	router.Handle(MsgClosePC, func(conn net.Conn, msg *Message) *Message {
		return nil // handler returns nil = no response
	})

	payload, _ := json.Marshal(ClosePCPayload{ConnectionID: "test"})
	msg := &Message{Type: MsgClosePC, Payload: payload}
	resp := router.Dispatch(nil, msg)

	if resp != nil {
		t.Errorf("expected nil response, got %+v", resp)
	}
}

func TestRouter_Handle_OverwritesHandler(t *testing.T) {
	registry := NewConnRegistry()
	router := NewRouter(registry)

	callCount := 0
	router.Handle(MsgCreatePC, func(conn net.Conn, msg *Message) *Message {
		callCount++
		return nil
	})
	// Overwrite with a new handler
	router.Handle(MsgCreatePC, func(conn net.Conn, msg *Message) *Message {
		callCount += 10
		return nil
	})

	payload, _ := json.Marshal(CreatePCPayload{ConnectionID: "test"})
	router.Dispatch(nil, &Message{Type: MsgCreatePC, Payload: payload})

	if callCount != 10 {
		t.Errorf("expected callCount=10 (second handler), got %d", callCount)
	}
}

func TestRouter_AsHandler(t *testing.T) {
	registry := NewConnRegistry()
	router := NewRouter(registry)

	called := false
	router.Handle(MsgSendData, func(conn net.Conn, msg *Message) *Message {
		called = true
		return nil
	})

	h := router.AsHandler()
	payload, _ := json.Marshal(SendDataPayload{ConnectionID: "test", Data: []byte("hi")})
	h(&Message{Type: MsgSendData, Payload: payload})

	if !called {
		t.Error("AsHandler did not invoke the registered handler")
	}
}

func TestConnRegistry_RegisterUnregister(t *testing.T) {
	registry := NewConnRegistry()

	// Register a nil conn (for testing purposes)
	registry.Register("conn-1", nil)

	_, ok := registry.Get("conn-1")
	if !ok {
		t.Error("expected to find conn-1")
	}

	registry.Unregister("conn-1")
	_, ok = registry.Get("conn-1")
	if ok {
		t.Error("expected conn-1 to be unregistered")
	}
}

func TestConnRegistry_GetMissing(t *testing.T) {
	registry := NewConnRegistry()
	_, ok := registry.Get("nonexistent")
	if ok {
		t.Error("expected Get to return false for nonexistent key")
	}
}

func TestConnRegistry_SendEvent_MissingConn(t *testing.T) {
	registry := NewConnRegistry()
	// SendEvent on a connectionID that was never registered should return nil (not error)
	err := registry.SendEvent("ghost-conn", &EventPayload{
		ConnectionID: "ghost-conn",
		Event:        EventFailed,
		Reason:       "test",
	})
	if err != nil {
		t.Errorf("expected nil error for missing conn, got %v", err)
	}
}

func TestMustUnmarshal_NilPayload(t *testing.T) {
	var v CreatePCPayload
	err := MustUnmarshal(nil, &v)
	if err == nil {
		t.Error("expected error for nil payload")
	}
}

func TestMustUnmarshal_ValidPayload(t *testing.T) {
	payload, _ := json.Marshal(CreatePCPayload{ConnectionID: "abc"})
	var v CreatePCPayload
	err := MustUnmarshal(payload, &v)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if v.ConnectionID != "abc" {
		t.Errorf("got %q, want %q", v.ConnectionID, "abc")
	}
}

func TestRouter_ConcurrentDispatch(t *testing.T) {
	registry := NewConnRegistry()
	router := NewRouter(registry)
	router.Handle(MsgCreatePC, func(conn net.Conn, msg *Message) *Message {
		return nil
	})

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			payload, _ := json.Marshal(CreatePCPayload{ConnectionID: "test"})
			router.Dispatch(nil, &Message{Type: MsgCreatePC, Payload: payload})
		}()
	}
	wg.Wait()
}
