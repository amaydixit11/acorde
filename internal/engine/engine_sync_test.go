package engine

import (
	"testing"
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
