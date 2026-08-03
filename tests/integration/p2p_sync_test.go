package integration

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/amaydixit11/acorde/internal/crdt"
	"github.com/amaydixit11/acorde/internal/sync"
	"github.com/amaydixit11/acorde/pkg/engine"
	libp2pcrypto "github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/peer"
)

// syncableEngine wraps pkg/engine.Engine to implement sync.Syncable
type syncableEngine struct {
	engine.Engine
}

func (s *syncableEngine) GetSyncState() crdt.ReplicaState {
	payload, _ := s.GetSyncPayload()
	var state crdt.ReplicaState
	if err := json.Unmarshal(payload, &state); err != nil {
		panic(fmt.Sprintf("failed to unmarshal sync state: %v", err))
	}
	return state
}

func (s *syncableEngine) ApplySyncState(state crdt.ReplicaState, senderPeerID string) error {
	payload, _ := json.Marshal(state)
	return s.Engine.ApplyRemotePayloadFromPeer(payload, senderPeerID)
}

type testNode struct {
	engine  engine.Engine
	syncSvc sync.SyncService
	dataDir string
}

func setupTestNode(t *testing.T, name string) *testNode {
	dir, err := os.MkdirTemp("", "acorde-test-"+name+"-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	// 1. Generate identity like the daemon does to ensure Engine and Sync match
	priv, _, err := libp2pcrypto.GenerateKeyPair(libp2pcrypto.Ed25519, -1)
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}
	id, _ := peer.IDFromPrivateKey(priv)

	// Save to node_id so engine loads it
	nodeIDPath := filepath.Join(dir, "node_id")
	if err := os.WriteFile(nodeIDPath, []byte(id.String()), 0644); err != nil {
		t.Fatalf("failed to write node_id: %v", err)
	}

	cfg := engine.Config{DataDir: dir}
	e, err := engine.New(cfg)
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}

	syncCfg := sync.DefaultConfig()
	syncCfg.EnableMDNS = false
	syncCfg.ListenAddrs = []string{"/ip4/127.0.0.1/tcp/0"}
	syncCfg.PrivateKey = priv

	// Use adapter like the daemon does
	adapter := sync.NewEngineAdapter(&syncableEngine{e})
	svc, err := sync.NewP2PService(adapter, syncCfg)
	if err != nil {
		t.Fatalf("failed to create sync service: %v", err)
	}

	return &testNode{
		engine:  e,
		syncSvc: svc,
		dataDir: dir,
	}
}

func (n *testNode) cleanup() {
	n.syncSvc.Stop()
	n.engine.Close()
	os.RemoveAll(n.dataDir)
}

func ptr[T any](v T) *T {
	return &v
}

func TestP2PThreeNodeMeshSync(t *testing.T) {
	// Setup 3 nodes
	nodeA := setupTestNode(t, "nodeA")
	defer nodeA.cleanup()
	nodeB := setupTestNode(t, "nodeB")
	defer nodeB.cleanup()
	nodeC := setupTestNode(t, "nodeC")
	defer nodeC.cleanup()

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Start sync services
	if err := nodeA.syncSvc.Start(ctx); err != nil {
		t.Fatalf("failed to start A: %v", err)
	}
	if err := nodeB.syncSvc.Start(ctx); err != nil {
		t.Fatalf("failed to start B: %v", err)
	}
	if err := nodeC.syncSvc.Start(ctx); err != nil {
		t.Fatalf("failed to start C: %v", err)
	}

	// Connect A -> B
	inviteA, _ := sync.CreateInvite(nodeA.syncSvc.GetHost(), time.Hour)
	if err := nodeB.syncSvc.ConnectPeer(inviteA); err != nil {
		t.Fatalf("failed to connect B to A: %v", err)
	}

	// Connect B -> C
	inviteB, _ := sync.CreateInvite(nodeB.syncSvc.GetHost(), time.Hour)
	if err := nodeC.syncSvc.ConnectPeer(inviteB); err != nil {
		t.Fatalf("failed to connect C to B: %v", err)
	}

	// Wait for connections to stabilize
	time.Sleep(time.Second)

	// 1. Add entry on A (linear topology: A <-> B <-> C)
	content := "Shared via mesh"
	entryA, err := nodeA.engine.AddEntry(engine.AddEntryInput{
		Type:    engine.Note,
		Content: []byte(content),
		Public:  true,
	})
	if err != nil {
		t.Fatalf("failed to add entry on A: %v", err)
	}

	// Propagation across nodes
	t.Run("Multi-hop Convergence (A->B->C)", func(t *testing.T) {
		// Eventual consistency check
		deadline := time.Now().Add(10 * time.Second)
		var entryC *engine.Entry
		for time.Now().Before(deadline) {
			got, err := nodeC.engine.GetEntry(entryA.ID)
			if err == nil {
				entryC = &got
				break
			}
			// Trigger explicit sync if needed (depends on internal polling interval)
			nodeB.syncSvc.SyncWith(ctx, nodeA.syncSvc.GetHost().ID())
			nodeC.syncSvc.SyncWith(ctx, nodeB.syncSvc.GetHost().ID())
			time.Sleep(500 * time.Millisecond)
		}

		if entryC == nil {
			t.Fatalf("Node C did not receive entry from Node A via Node B")
		}
		if string(entryC.Content) != content {
			t.Errorf("Node C content mismatch: expected %q, got %q", content, entryC.Content)
		}
	})

	// 2. Network Partition Test
	t.Run("Partition Resilience (A-B OK, B-C Partitioned)", func(t *testing.T) {
		// Disconnect Node C from Node B
		peerIDB := nodeB.syncSvc.GetHost().ID()
		nodeC.syncSvc.GetHost().Network().ClosePeer(peerIDB)
		nodeC.syncSvc.GetHost().Peerstore().ClearAddrs(peerIDB)

		// A updates the entry
		updatedContent := "Updated under partition"
		err := nodeA.engine.UpdateEntry(entryA.ID, engine.UpdateEntryInput{
			Content: ptr([]byte(updatedContent)),
		})
		if err != nil {
			t.Fatalf("failed to update A: %v", err)
		}

		// Sync A -> B
		nodeB.syncSvc.SyncWith(ctx, nodeA.syncSvc.GetHost().ID())

		// Verify Node B has update
		gotB, _ := nodeB.engine.GetEntry(entryA.ID)
		if string(gotB.Content) != updatedContent {
			t.Fatal("Node B should have received update from A")
		}

		// Verify Node C does NOT have update (still at old content)
		gotC, _ := nodeC.engine.GetEntry(entryA.ID)
		if string(gotC.Content) != content {
			t.Fatalf("Node C should NOT have received update during partition. Got: %s", gotC.Content)
		}

		// Heal partition
		if err := nodeC.syncSvc.ConnectPeer(inviteB); err != nil {
			t.Fatalf("failed to re-connect C to B: %v", err)
		}

		// Trigger sync
		nodeC.syncSvc.SyncWith(ctx, nodeB.syncSvc.GetHost().ID())

		// Verify eventual convergence on C
		deadline := time.Now().Add(10 * time.Second)
		for time.Now().Before(deadline) {
			got, _ := nodeC.engine.GetEntry(entryA.ID)
			if string(got.Content) == updatedContent {
				return
			}
			time.Sleep(500 * time.Millisecond)
		}
		t.Fatal("Node C failed to converge after partition was healed")
	})
}
