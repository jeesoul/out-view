package protocol

import (
	"testing"
)

func TestNewWebRTCOfferMessage(t *testing.T) {
	msg, err := NewWebRTCOfferMessage("conn-1", "v=0\r\n...")
	if err != nil {
		t.Fatal(err)
	}
	if msg.Header.Type != TypeWebRTCOffer {
		t.Errorf("expected TypeWebRTCOffer, got %d", msg.Header.Type)
	}
	body, err := ParseWebRTCOfferBody(msg.Body)
	if err != nil {
		t.Fatal(err)
	}
	if body.ConnectionID != "conn-1" {
		t.Errorf("expected connectionId=conn-1, got %s", body.ConnectionID)
	}
	if body.SDPType != "offer" {
		t.Errorf("expected sdpType=offer, got %s", body.SDPType)
	}
}

func TestNewWebRTCAnswerMessage(t *testing.T) {
	msg, err := NewWebRTCAnswerMessage("conn-2", "v=0\r\n...")
	if err != nil {
		t.Fatal(err)
	}
	if msg.Header.Type != TypeWebRTCAnswer {
		t.Errorf("expected TypeWebRTCAnswer, got %d", msg.Header.Type)
	}
	body, err := ParseWebRTCOfferBody(msg.Body)
	if err != nil {
		t.Fatal(err)
	}
	if body.SDPType != "answer" {
		t.Errorf("expected sdpType=answer, got %s", body.SDPType)
	}
}

func TestNewWebRTCICECandidateMessage(t *testing.T) {
	idx := uint16(0)
	msg, err := NewWebRTCICECandidateMessage("conn-3", "candidate:1 1 UDP ...", "0", &idx)
	if err != nil {
		t.Fatal(err)
	}
	if msg.Header.Type != TypeWebRTCICECandidate {
		t.Errorf("expected TypeWebRTCICECandidate, got %d", msg.Header.Type)
	}
	body, err := ParseWebRTCICECandidateBody(msg.Body)
	if err != nil {
		t.Fatal(err)
	}
	if body.Candidate != "candidate:1 1 UDP ..." {
		t.Errorf("unexpected candidate: %s", body.Candidate)
	}
	if body.SDPMLineIndex == nil || *body.SDPMLineIndex != 0 {
		t.Error("expected sdpMLineIndex=0")
	}
}

func TestNewWebRTCICECompleteMessage(t *testing.T) {
	msg, err := NewWebRTCICECompleteMessage("conn-4")
	if err != nil {
		t.Fatal(err)
	}
	if msg.Header.Type != TypeWebRTCICEComplete {
		t.Errorf("expected TypeWebRTCICEComplete, got %d", msg.Header.Type)
	}
	body, err := ParseWebRTCConnectionBody(msg.Body)
	if err != nil {
		t.Fatal(err)
	}
	if body.ConnectionID != "conn-4" {
		t.Errorf("expected connectionId=conn-4, got %s", body.ConnectionID)
	}
}

func TestNewWebRTCEstablishedMessage(t *testing.T) {
	msg, err := NewWebRTCEstablishedMessage("conn-5")
	if err != nil {
		t.Fatal(err)
	}
	if msg.Header.Type != TypeWebRTCEstablished {
		t.Errorf("expected TypeWebRTCEstablished, got %d", msg.Header.Type)
	}
}

func TestNewWebRTCFailedMessage(t *testing.T) {
	msg, err := NewWebRTCFailedMessage("conn-6", "ICE failed")
	if err != nil {
		t.Fatal(err)
	}
	if msg.Header.Type != TypeWebRTCFailed {
		t.Errorf("expected TypeWebRTCFailed, got %d", msg.Header.Type)
	}
	body, err := ParseWebRTCConnectionBody(msg.Body)
	if err != nil {
		t.Fatal(err)
	}
	if body.Reason != "ICE failed" {
		t.Errorf("expected reason='ICE failed', got %s", body.Reason)
	}
}

func TestWebRTCTypeConstants_Unique(t *testing.T) {
	types := []byte{
		TypeRegister, TypeHeartbeat, TypeData, TypeError,
		TypeRegisterAck, TypeHeartbeatAck, TypeCloseConnection,
		TypeWebRTCOffer, TypeWebRTCAnswer, TypeWebRTCICECandidate,
		TypeWebRTCICEComplete, TypeWebRTCEstablished, TypeWebRTCFailed,
	}
	seen := make(map[byte]bool)
	for _, t2 := range types {
		if seen[t2] {
			t.Errorf("duplicate type value: %d", t2)
		}
		seen[t2] = true
	}
}
