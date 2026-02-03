package sync

import (
	"testing"
)

func TestMessageEncode(t *testing.T) {
	msg := &Message{
		Type:      MsgStateHash,
		StateHash: []byte("test-hash"),
	}

	data, err := msg.Encode()
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	decoded, err := DecodeMessage(data)
	if err != nil {
		t.Fatalf("failed to decode: %v", err)
	}

	if decoded.Type != msg.Type {
		t.Error("type mismatch")
	}
	if string(decoded.StateHash) != string(msg.StateHash) {
		t.Error("state hash mismatch")
	}
}

func TestMessageTypes(t *testing.T) {
	tests := []struct {
		name string
		msg  Message
	}{
		{
			name: "StateHash",
			msg:  Message{Type: MsgStateHash, StateHash: []byte("hash")},
		},
		{
			name: "StateRequest",
			msg:  Message{Type: MsgStateRequest},
		},
		{
			name: "State",
			msg:  Message{Type: MsgState, State: []byte(`{"entries":[]}`)},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			data, _ := tt.msg.Encode()
			decoded, err := DecodeMessage(data)
			if err != nil {
				t.Fatalf("decode failed: %v", err)
			}
			if decoded.Type != tt.msg.Type {
				t.Errorf("type mismatch: got %d, want %d", decoded.Type, tt.msg.Type)
			}
		})
	}
}

func TestDefaultConfig(t *testing.T) {
	cfg := DefaultConfig()

	if len(cfg.ListenAddrs) == 0 {
		t.Error("should have default listen address")
	}
	if cfg.SyncInterval == 0 {
		t.Error("should have default sync interval")
	}
	if !cfg.EnableMDNS {
		t.Error("mDNS should be enabled by default")
	}
}
