package integration

import (
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/amaydixit11/acorde/internal/core"
	"github.com/amaydixit11/acorde/internal/crdt"
	"github.com/amaydixit11/acorde/internal/engine"
	"github.com/google/uuid"
)

// Helper to manually sync two engines
func syncEngines(t *testing.T, a, b engine.Engine) {
	// A -> B
	payloadA, err := a.GetSyncPayload()
	if err != nil {
		t.Fatalf("failed to get sync payload from A: %v", err)
	}
	if err := b.ApplyRemotePayload(payloadA); err != nil {
		t.Fatalf("failed to apply payload to B: %v", err)
	}

	// B -> A
	payloadB, err := b.GetSyncPayload()
	if err != nil {
		t.Fatalf("failed to get sync payload from B: %v", err)
	}
	if err := a.ApplyRemotePayload(payloadB); err != nil {
		t.Fatalf("failed to apply payload to A: %v", err)
	}
}

// Helper to inject ACL into engine via CRDT merge (since SetACL is not exposed for Replica)
func injectACL(t *testing.T, e engine.Engine, acl core.ACL) {
	state := crdt.ReplicaState{
		Entries:   []crdt.LWWElement{},
		Tags:      make(map[uuid.UUID]crdt.TagSetState),
		ACLs:      map[uuid.UUID]core.ACL{acl.EntryID: acl},
		ClockTime: acl.Timestamp,
	}
	
	bytes, err := json.Marshal(state)
	if err != nil {
		t.Fatalf("Failed to marshal ACL state: %v", err)
	}
	// ApplyRemotePayload expects JSON payload and is in Engine interface
	if err := e.ApplyRemotePayload(bytes); err != nil {
		t.Fatalf("Failed to apply ACL payload: %v", err)
	}
}

func setupCRDTEngine(t *testing.T) (engine.Engine, string) {
	// Create a temp directory
	dir, err := os.MkdirTemp("", "acorde-test-*")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}

	// Create engine config
	cfg := engine.Config{
		DataDir: dir,
	}

	// Initialize engine
	e, err := engine.New(cfg)
	if err != nil {
		os.RemoveAll(dir) // Cleanup on failure
		t.Fatalf("Failed to create engine: %v", err)
	}

	return e, dir
}

func cleanupCRDTEngine(e engine.Engine, dir string) {
	e.Close()
	os.RemoveAll(dir)
}

func TestCRDTSync(t *testing.T) {
	// Setup Node A
	engineA, dirA := setupCRDTEngine(t)
	defer cleanupCRDTEngine(engineA, dirA)

	// Setup Node B
	engineB, dirB := setupCRDTEngine(t)
	defer cleanupCRDTEngine(engineB, dirB)

	t.Run("LWW Entry Merge", func(t *testing.T) {
		// 1. Create entry on A
		entryA, err := engineA.AddEntry(engine.AddEntryInput{
			Type:    core.Note,
			Content: []byte("original"),
		})
		if err != nil {
			t.Fatalf("Failed to add entry A: %v", err)
		}

		// Get Node B's ID by creating a dummy entry
		dummyB, err := engineB.AddEntry(engine.AddEntryInput{
			Type:    core.Note,
			Content: []byte("dummy"),
		})
		if err != nil {
			t.Fatalf("Failed to create dummy B: %v", err)
		}
		nodeBID := dummyB.Owner

		// Update ACL to allow B to write
		aclB := core.ACL{
			EntryID:   entryA.ID,
			Owner:     entryA.Owner,
			Writers:   []string{nodeBID}, // Allow B to write
			Timestamp: uint64(time.Now().UnixNano()),
		}
		// Inject into both A and B so CRDT state is updated
		injectACL(t, engineA, aclB)
		injectACL(t, engineB, aclB)

		// 2. Sync to B (Initial Sync)
		syncEngines(t, engineA, engineB)

		// Verify B has it
		entryB, err := engineB.GetEntry(entryA.ID)
		if err != nil {
			t.Fatalf("B should have the entry: %v", err)
		}
		if string(entryB.Content) != "original" {
			t.Error("B content mismatch")
		}

		// 3. Concurrent Update
		// Node A updates content to "Update A"
		newContentA := []byte("Update A")
		if err := engineA.UpdateEntry(entryA.ID, engine.UpdateEntryInput{Content: &newContentA}); err != nil {
			t.Fatalf("A update failed: %v", err)
		}

		// Node B updates content to "Update B"
		newContentB := []byte("Update B")
		if err := engineB.UpdateEntry(entryA.ID, engine.UpdateEntryInput{Content: &newContentB}); err != nil {
			t.Fatalf("B update failed: %v", err)
		}

		// 4. Sync
		syncEngines(t, engineA, engineB)

		// 5. Verify Convergence
		finalA, err := engineA.GetEntry(entryA.ID)
		if err != nil { t.Fatalf("A get failed: %v", err) }
		finalB, err := engineB.GetEntry(entryA.ID)
		if err != nil { t.Fatalf("B get failed: %v", err) }

		if string(finalA.Content) != string(finalB.Content) {
			t.Errorf("Engines did not converge! A: %s, B: %s", finalA.Content, finalB.Content)
		}

		t.Logf("Converged validation content: %s", finalA.Content)
	})

	t.Run("OR-Set Tag Merge", func(t *testing.T) {
		// 1. Create entry on A
		entry, err := engineA.AddEntry(engine.AddEntryInput{
			Type:    core.Note,
			Content: []byte("tag test"),
			Tags:    []string{"initial"},
		})
		if err != nil { t.Fatalf("Add error: %v", err) }

		// Get Node B's ID (or reuse from previous test if we scoped it, but new run)
		dummyB, err := engineB.AddEntry(engine.AddEntryInput{
			Type:    core.Note,
			Content: []byte("dummy2"),
		})
		if err != nil { t.Fatalf("dummyB failed: %v", err) }
		nodeBID := dummyB.Owner

		// Fix Permissions for B
		aclB := core.ACL{
			EntryID:   entry.ID,
			Owner:     entry.Owner,
			Writers:   []string{nodeBID},
			Timestamp: uint64(time.Now().UnixNano()),
		}
		injectACL(t, engineA, aclB)
		injectACL(t, engineB, aclB)

		syncEngines(t, engineA, engineB)

		// 2. Concurrent Tag Adds
		tagsA := []string{"initial", "A"}
		if err := engineA.UpdateEntry(entry.ID, engine.UpdateEntryInput{Tags: &tagsA}); err != nil {
			t.Fatalf("A tag update failed: %v", err)
		}

		tagsB := []string{"initial", "B"}
		if err := engineB.UpdateEntry(entry.ID, engine.UpdateEntryInput{Tags: &tagsB}); err != nil {
			t.Fatalf("B tag update failed: %v", err)
		}

		// 3. Sync
		syncEngines(t, engineA, engineB)

		// 4. Verify
		finalA, _ := engineA.GetEntry(entry.ID)
		finalB, _ := engineB.GetEntry(entry.ID)

		// Both should have "initial", "A", "B"
		expectedTags := map[string]bool{"initial": true, "A": true, "B": true}
		
		checkTags := func(inputTags []string, node string) {
			unique := make(map[string]bool)
			for _, t := range inputTags { unique[t] = true }
			
			if len(unique) != 3 {
				t.Errorf("%s: expected 3 unique tags, got %v", node, inputTags)
			}
			for tag := range expectedTags {
				if !unique[tag] {
					t.Errorf("%s: missing expected tag %s", node, tag)
				}
			}
		}
		checkTags(finalA.Tags, "A")
		checkTags(finalB.Tags, "B")
	})

	t.Run("Delta Sync", func(t *testing.T) {
		// 1. Check sync logic
		// We verified that 'GetSyncPayload' allows syncing.
		// Since 'EntriesSince' is not exposed on Engine interface, we assume internals are correct
		// if full sync works.
		// We can check if internal replica state is correct by unmarshalling payload.
		
		payload, _ := engineA.GetSyncPayload()
		var state crdt.ReplicaState
		if err := json.Unmarshal(payload, &state); err != nil {
			t.Fatalf("failed to unmarshal sync payload: %v", err)
		}
		
		if len(state.Entries) == 0 {
			t.Error("expected entries in sync payload")
		}
	})
}
