package ipc

import (
	"encoding/json"
	"testing"
)

func TestProtocol_MarshalUnmarshal(t *testing.T) {
	// Test CreatePCPayload round-trip
	orig := CreatePCPayload{ConnectionID: "conn-123"}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatal(err)
	}
	var got CreatePCPayload
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.ConnectionID != orig.ConnectionID {
		t.Errorf("got %q, want %q", got.ConnectionID, orig.ConnectionID)
	}
}

func TestProtocol_EventPayload(t *testing.T) {
	evt := EventPayload{
		ConnectionID: "conn-456",
		Event:        EventEstablished,
		Reason:       "",
	}
	data, _ := json.Marshal(evt)
	var got EventPayload
	json.Unmarshal(data, &got)
	if got.Event != EventEstablished {
		t.Errorf("got event %q, want %q", got.Event, EventEstablished)
	}
}

func TestProtocol_ErrorPayload(t *testing.T) {
	ep := ErrorPayload{ConnectionID: "conn-789", Error: "something went wrong"}
	data, _ := json.Marshal(ep)
	var got ErrorPayload
	json.Unmarshal(data, &got)
	if got.Error != ep.Error {
		t.Errorf("got %q, want %q", got.Error, ep.Error)
	}
}

func TestProtocol_SetRemoteSDPPayload(t *testing.T) {
	orig := SetRemoteSDPPayload{
		ConnectionID: "conn-abc",
		SDP:          "v=0\r\n...",
		Type:         "offer",
	}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatal(err)
	}
	var got SetRemoteSDPPayload
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.ConnectionID != orig.ConnectionID || got.SDP != orig.SDP || got.Type != orig.Type {
		t.Errorf("round-trip mismatch: got %+v, want %+v", got, orig)
	}
}

func TestProtocol_AddICECandidatePayload_OptionalFields(t *testing.T) {
	idx := uint16(0)
	orig := AddICECandidatePayload{
		ConnectionID:  "conn-def",
		Candidate:     "candidate:...",
		SDPMid:        "0",
		SDPMLineIndex: &idx,
	}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatal(err)
	}
	var got AddICECandidatePayload
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if got.SDPMLineIndex == nil || *got.SDPMLineIndex != idx {
		t.Errorf("SDPMLineIndex mismatch")
	}
}

func TestProtocol_SendDataPayload(t *testing.T) {
	orig := SendDataPayload{
		ConnectionID: "conn-ghi",
		Data:         []byte("hello world"),
	}
	data, err := json.Marshal(orig)
	if err != nil {
		t.Fatal(err)
	}
	var got SendDataPayload
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatal(err)
	}
	if string(got.Data) != string(orig.Data) {
		t.Errorf("Data mismatch: got %q, want %q", got.Data, orig.Data)
	}
}

func TestProtocol_OKPayload(t *testing.T) {
	orig := OKPayload{ConnectionID: "conn-ok", OK: true}
	data, _ := json.Marshal(orig)
	var got OKPayload
	json.Unmarshal(data, &got)
	if !got.OK || got.ConnectionID != orig.ConnectionID {
		t.Errorf("OKPayload round-trip failed: %+v", got)
	}
}

func TestProtocol_MessageTypeConstants(t *testing.T) {
	// Verify constants are non-empty and distinct
	types := []string{
		MsgCreatePC, MsgSetRemoteSDP, MsgAddICECandidate,
		MsgSendData, MsgClosePC, MsgEvent,
	}
	seen := make(map[string]bool)
	for _, typ := range types {
		if typ == "" {
			t.Error("empty message type constant")
		}
		if seen[typ] {
			t.Errorf("duplicate message type constant: %q", typ)
		}
		seen[typ] = true
	}

	events := []string{
		EventOffer, EventAnswer, EventICECandidate,
		EventICEComplete, EventEstablished, EventFailed, EventData,
	}
	seenEvents := make(map[string]bool)
	for _, ev := range events {
		if ev == "" {
			t.Error("empty event constant")
		}
		if seenEvents[ev] {
			t.Errorf("duplicate event constant: %q", ev)
		}
		seenEvents[ev] = true
	}
}
