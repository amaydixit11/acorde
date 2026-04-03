package sync

import (
	"context"
	"os"
	"testing"
	"time"

	"github.com/amaydixit11/acorde/internal/core"
	"github.com/amaydixit11/acorde/internal/crdt"
)

// mockStateProvider implements StateProvider for testing
type mockStateProvider struct {
	replica *crdt.Replica
}

func newMockProvider() *mockStateProvider {
	return &mockStateProvider{
		replica: crdt.NewReplica(core.NewClock()),
	}
}

func (p *mockStateProvider) GetState() crdt.ReplicaState {
	return p.replica.State()
}

func (p *mockStateProvider) ApplyState(state crdt.ReplicaState, senderPeerID string) error {
	tempClock := core.NewClockWithTime(state.ClockTime)
	tempReplica := crdt.NewReplica(tempClock)
	tempReplica.LoadState(state)
	p.replica.Merge(tempReplica)
	return nil
}

func (p *mockStateProvider) StateHash() []byte {
	return ComputeStateHash(p.replica.State())
}

func TestP2PServiceLifecycle(t *testing.T) {
	provider := newMockProvider()
	cfg := DefaultConfig()
	cfg.EnableMDNS = false // Disable for unit test

	svc, err := NewP2PService(provider, cfg)
	if err != nil {
		t.Skipf("skipping lifecycle test in restricted environment: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	if err := svc.Start(ctx); err != nil {
		t.Skipf("skipping lifecycle test in restricted environment: %v", err)
	}

	// Should have no peers initially
	peers := svc.Peers()
	if len(peers) != 0 {
		t.Errorf("expected 0 peers, got %d", len(peers))
	}

	if err := svc.Stop(); err != nil {
		t.Fatalf("failed to stop: %v", err)
	}
}

func TestP2PSyncBetweenPeers(t *testing.T) {
	// Create two providers
	provider1 := newMockProvider()
	provider2 := newMockProvider()

	cfg := DefaultConfig()
	cfg.EnableMDNS = false

	// Create two services
	svc1, err := NewP2PService(provider1, cfg)
	if err != nil {
		t.Skipf("skipping sync test in restricted environment: %v", err)
	}

	svc2, err := NewP2PService(provider2, cfg)
	if err != nil {
		t.Skipf("skipping sync test in restricted environment: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	if err := svc1.Start(ctx); err != nil {
		t.Skipf("skipping sync test in restricted environment: %v", err)
	}
	defer svc1.Stop()

	if err := svc2.Start(ctx); err != nil {
		t.Skipf("skipping sync test in restricted environment: %v", err)
	}
	defer svc2.Stop()

	// Add entry to provider1's replica
	provider1.replica.AddEntry(core.Note, []byte("from peer 1"), []string{"test"})

	// Get svc1's peer info
	p2p1 := svc1.(*p2pService)
	p2p2 := svc2.(*p2pService)

	// Connect svc2 to svc1
	peerInfo1 := p2p1.host.Peerstore().PeerInfo(p2p1.host.ID())
	if err := p2p2.host.Connect(ctx, peerInfo1); err != nil {
		t.Fatalf("failed to connect: %v", err)
	}

	// Trigger sync from svc2 to svc1
	if err := svc2.SyncWith(ctx, p2p1.host.ID()); err != nil {
		t.Fatalf("sync failed: %v", err)
	}

	// Provider2 should now have the entry from provider1
	entries := provider2.replica.ListEntries()
	if len(entries) != 1 {
		t.Errorf("expected 1 entry after sync, got %d", len(entries))
	}

	if len(entries) > 0 && string(entries[0].Content) != "from peer 1" {
		t.Errorf("expected 'from peer 1', got '%s'", string(entries[0].Content))
	}

	// Add a second entry after the first sync and initiate sync from the side that
	// owns the new change. The peer should still converge.
	provider1.replica.AddEntry(core.Note, []byte("from peer 1 again"), []string{"test2"})

	if err := svc1.SyncWith(ctx, p2p2.host.ID()); err != nil {
		t.Fatalf("second sync failed: %v", err)
	}

	entries = provider2.replica.ListEntries()
	if len(entries) != 2 {
		t.Fatalf("expected 2 entries after second sync, got %d", len(entries))
	}
}

func TestP2PConnectsToAllowlistedPeerWithoutMDNS(t *testing.T) {
	provider1 := newMockProvider()
	provider2 := newMockProvider()
	provider1.replica.AddEntry(core.Note, []byte("allowlisted"), []string{"pair"})

	cfg1 := DefaultConfig()
	cfg1.EnableMDNS = false
	svc1, err := NewP2PService(provider1, cfg1)
	if err != nil {
		t.Skipf("skipping allowlist sync test in restricted environment: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := svc1.Start(ctx); err != nil {
		t.Skipf("skipping allowlist sync test in restricted environment: %v", err)
	}
	defer svc1.Stop()

	dir, err := os.MkdirTemp("", "acorde-allowlist-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(dir)

	p2p1 := svc1.(*p2pService)
	addrs := make([]string, 0, len(p2p1.host.Addrs()))
	for _, addr := range p2p1.host.Addrs() {
		addrs = append(addrs, addr.String())
	}

	al, err := NewAllowlist(dir, false)
	if err != nil {
		t.Fatalf("failed to create allowlist: %v", err)
	}
	if err := al.Add(p2p1.host.ID(), "", addrs); err != nil {
		t.Fatalf("failed to add allowlisted peer: %v", err)
	}

	cfg2 := DefaultConfig()
	cfg2.EnableMDNS = false
	cfg2.AllowlistPath = dir
	svc2, err := NewP2PService(provider2, cfg2)
	if err != nil {
		t.Skipf("skipping allowlist sync test in restricted environment: %v", err)
	}
	if err := svc2.Start(ctx); err != nil {
		t.Skipf("skipping allowlist sync test in restricted environment: %v", err)
	}
	defer svc2.Stop()

	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if len(provider2.replica.ListEntries()) == 1 {
			return
		}
		time.Sleep(100 * time.Millisecond)
	}

	t.Fatalf("expected allowlisted peer sync without mDNS")
}
