package sync

import (
	"testing"
	"time"

	"github.com/amaydixit11/acorde/internal/core"
	"github.com/amaydixit11/acorde/internal/crdt"
	"github.com/libp2p/go-libp2p"
)

func TestCreateAndParseInvite(t *testing.T) {
	// Create a libp2p host
	h, err := libp2p.New()
	if err != nil {
		t.Fatalf("failed to create host: %v", err)
	}
	defer h.Close()

	// Create invite
	invite, err := CreateInvite(h, 24*time.Hour)
	if err != nil {
		t.Fatalf("failed to create invite: %v", err)
	}

	// Check fields
	if invite.PeerID != h.ID().String() {
		t.Error("peer ID mismatch")
	}
	if len(invite.Addresses) == 0 {
		t.Error("should have addresses")
	}
	if invite.IsExpired() {
		t.Error("invite should not be expired")
	}

	// Encode and parse
	code, err := invite.Encode()
	if err != nil {
		t.Fatalf("failed to encode: %v", err)
	}

	parsed, err := ParseInvite(code)
	if err != nil {
		t.Fatalf("failed to parse: %v", err)
	}

	if parsed.PeerID != invite.PeerID {
		t.Error("parsed peer ID mismatch")
	}
}

func TestExpiredInvite(t *testing.T) {
	h, _ := libp2p.New()
	defer h.Close()

	// Create invite that expires immediately
	invite, _ := CreateInvite(h, -1*time.Second)
	
	code, _ := invite.Encode()
	_, err := ParseInvite(code)
	if err == nil {
		t.Error("should reject expired invite")
	}
}

func TestInviteQRGeneration(t *testing.T) {
	h, _ := libp2p.New()
	defer h.Close()

	invite, _ := CreateInvite(h, 24*time.Hour)

	// Generate QR PNG
	png, err := invite.ToQR()
	if err != nil {
		t.Fatalf("failed to generate QR: %v", err)
	}
	if len(png) == 0 {
		t.Error("QR PNG should not be empty")
	}

	// Generate QR string
	qrStr, err := invite.ToQRString()
	if err != nil {
		t.Fatalf("failed to generate QR string: %v", err)
	}
	if len(qrStr) == 0 {
		t.Error("QR string should not be empty")
	}
}

// mockStateProvider from p2p_test.go
type inviteTestProvider struct {
	replica *crdt.Replica
}

func newInviteTestProvider() *inviteTestProvider {
	return &inviteTestProvider{
		replica: crdt.NewReplica(core.NewClock()),
	}
}

func (p *inviteTestProvider) GetState() crdt.ReplicaState {
	return p.replica.State()
}

func (p *inviteTestProvider) ApplyState(state crdt.ReplicaState) error {
	return nil
}

func (p *inviteTestProvider) StateHash() []byte {
	return ComputeStateHash(p.replica.State())
}
