package engine

import (
	"encoding/json"
	"testing"

	"github.com/amaydixit11/acorde/internal/core"
)

// TestEngineSyncPayload tests that sync payload can be generated and applied
func TestEngineSyncPayload(t *testing.T) {
	e1 := newTestEngine(t).(*engineImpl)
	e2 := newTestEngine(t).(*engineImpl)
	defer e1.Close()
	defer e2.Close()

	// e1 adds entry
	entry1, _ := e1.AddEntry(AddEntryInput{
		Type:    "note",
		Content: []byte("from e1"),
		Tags:    []string{"source:e1"},
	})

	// e2 adds different entry
	entry2, _ := e2.AddEntry(AddEntryInput{
		Type:    "note",
		Content: []byte("from e2"),
		Tags:    []string{"source:e2"},
	})

	// Get sync payload from e1
	payload, err := e1.GetSyncPayload()
	if err != nil {
		t.Fatalf("failed to get sync payload: %v", err)
	}

	// Apply to e2
	if err := e2.ApplyRemotePayload(payload); err != nil {
		t.Fatalf("failed to apply payload: %v", err)
	}

	// e2 should now have both entries in its replica
	entries := e2.replica.ListEntries()
	if len(entries) != 2 {
		t.Errorf("expected 2 entries after merge, got %d", len(entries))
	}

	// Verify both entries exist
	_, err1 := e2.replica.GetEntry(entry1.ID)
	_, err2 := e2.replica.GetEntry(entry2.ID)
	if err1 != nil {
		t.Error("entry1 should exist in e2 after merge")
	}
	if err2 != nil {
		t.Error("entry2 should exist in e2 after merge")
	}
}

// TestEngineSyncMergeConflict tests that concurrent updates merge correctly
func TestEngineSyncMergeConflict(t *testing.T) {
	e1 := newTestEngine(t).(*engineImpl)
	e2 := newTestEngine(t).(*engineImpl)
	defer e1.Close()
	defer e2.Close()

	// e1 creates entry
	entry, _ := e1.AddEntry(AddEntryInput{
		Type:    "note",
		Content: []byte("original"),
	})

	// Sync to e2
	payload1, _ := e1.GetSyncPayload()
	e2.ApplyRemotePayload(payload1)

	// Allow e2 to update the entry so this test exercises LWW conflict resolution,
	// not ACL rejection.
	sharedACL := core.ACL{
		EntryID:   entry.ID,
		Owner:     e1.localID,
		Writers:   []string{e2.localID},
		Timestamp: entry.UpdatedAt + 1,
	}
	if err := e1.acls.SetACL(sharedACL); err != nil {
		t.Fatalf("failed to persist ACL on e1: %v", err)
	}
	e1.replica.SetACL(sharedACL)
	if err := e2.acls.SetACL(sharedACL); err != nil {
		t.Fatalf("failed to persist ACL on e2: %v", err)
	}
	e2.replica.SetACL(sharedACL)

	// Both update the same entry (simulating offline edits)
	content1 := []byte("update from e1")
	e1.UpdateEntry(entry.ID, UpdateEntryInput{Content: &content1})

	// e2 updates with higher clock (simulating later update)
	// Force e2's clock forward
	for i := 0; i < 10; i++ {
		e2.replica.AddEntry("note", []byte(""), nil)
	}
	content2 := []byte("update from e2 - should win")
	e2.UpdateEntry(entry.ID, UpdateEntryInput{Content: &content2})

	// Now merge e2 into e1
	payload2, _ := e2.GetSyncPayload()
	e1.ApplyRemotePayload(payload2)

	// e1 should have e2's version (higher timestamp wins)
	result, _ := e1.replica.GetEntry(entry.ID)
	if string(result.Content) != "update from e2 - should win" {
		t.Errorf("expected e2's update to win, got: %s", string(result.Content))
	}
}

// TestEngineChaos_DuplicateSync verifies idempotency at the engine level
func TestEngineChaos_DuplicateSync(t *testing.T) {
	e1 := newTestEngine(t).(*engineImpl)
	e2 := newTestEngine(t).(*engineImpl)
	defer e1.Close()
	defer e2.Close()

	e1.AddEntry(AddEntryInput{Type: "note", Content: []byte("chaos")})
	payload, _ := e1.GetSyncPayload()

	// Apply twice
	e2.ApplyRemotePayload(payload)
	e2.ApplyRemotePayload(payload)

	entries := e2.replica.ListEntries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry after duplicate sync, got %d", len(entries))
	}
}

// TestEngineChaos_OutOfOrderSync verifies convergence with misordered payloads
func TestEngineChaos_OutOfOrderSync(t *testing.T) {
	e1 := newTestEngine(t).(*engineImpl)
	e2 := newTestEngine(t).(*engineImpl)
	defer e1.Close()
	defer e2.Close()

	// Step 1: Create entry and get payload
	entry, _ := e1.AddEntry(AddEntryInput{Type: "note", Content: []byte("v1")})
	payload1, _ := e1.GetSyncPayload()

	// Step 2: Update entry and get new payload
	v2 := []byte("v2")
	e1.UpdateEntry(entry.ID, UpdateEntryInput{Content: &v2})
	payload2, _ := e1.GetSyncPayload()

	// Step 3: Apply v2 THEN v1 to e2
	if err := e2.ApplyRemotePayload(payload2); err != nil {
		t.Fatalf("failed to apply p2: %v", err)
	}
	if err := e2.ApplyRemotePayload(payload1); err != nil {
		t.Fatalf("failed to apply p1: %v", err)
	}

	// Step 4: e2 should have v2 (LWW logic)
	result, err := e2.replica.GetEntry(entry.ID)
	if err != nil {
		t.Fatalf("expected synced entry in replica, got error: %v", err)
	}
	if string(result.Content) != "v2" {
		t.Errorf("expected v2 after out-of-order sync, got: %s", string(result.Content))
	}
}

func TestApplySyncStateRejectsUnauthorizedRemoteUpdate(t *testing.T) {
	e1 := newTestEngine(t).(*engineImpl)
	e2 := newTestEngine(t).(*engineImpl)
	defer e1.Close()
	defer e2.Close()

	entry, err := e1.AddEntry(AddEntryInput{
		Type:    core.Note,
		Content: []byte("owner data"),
	})
	if err != nil {
		t.Fatalf("failed to add entry: %v", err)
	}

	payload, err := e1.GetSyncPayload()
	if err != nil {
		t.Fatalf("failed to get sync payload: %v", err)
	}
	if err := e2.ApplyRemotePayloadFromPeer(payload, e1.localID); err != nil {
		t.Fatalf("failed to apply initial sync: %v", err)
	}

	remoteState := e2.replica.State()
	for i, elem := range remoteState.Entries {
		if elem.Entry.ID == entry.ID {
			elem.Entry.Content = []byte("forged")
			elem.Entry.UpdatedAt += 100
			elem.Timestamp = elem.Entry.UpdatedAt
			remoteState.Entries[i] = elem
		}
	}
	remoteState.ClockTime += 100

	stateBytes, err := json.Marshal(remoteState)
	if err != nil {
		t.Fatalf("failed to marshal forged state: %v", err)
	}

	if err := e1.ApplyRemotePayloadFromPeer(stateBytes, e2.localID); err != nil {
		t.Fatalf("apply remote payload returned error: %v", err)
	}

	result, err := e1.GetEntry(entry.ID)
	if err != nil {
		t.Fatalf("failed to get entry after forged sync: %v", err)
	}
	if string(result.Content) != "owner data" {
		t.Fatalf("expected unauthorized sync update to be rejected, got %q", string(result.Content))
	}
}
