package engine

import (
	"testing"

	"github.com/amaydixit11/vaultd/internal/core"
	"github.com/google/uuid"
)

func newTestEngine(t *testing.T) Engine {
	e, err := New(Config{InMemory: true})
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	return e
}

func TestAddEntry(t *testing.T) {
	e := newTestEngine(t)
	defer e.Close()

	entry, err := e.AddEntry(AddEntryInput{
		Type:    core.Note,
		Content: []byte("test content"),
		Tags:    []string{"work"},
	})
	if err != nil {
		t.Fatalf("failed to add entry: %v", err)
	}

	if entry.ID == uuid.Nil {
		t.Error("expected valid UUID")
	}
	if entry.Type != core.Note {
		t.Errorf("expected Note type, got %s", entry.Type)
	}
	if string(entry.Content) != "test content" {
		t.Error("content mismatch")
	}
	if len(entry.Tags) != 1 || entry.Tags[0] != "work" {
		t.Error("tags mismatch")
	}
	if entry.CreatedAt == 0 || entry.UpdatedAt == 0 {
		t.Error("timestamps should be set")
	}
}

func TestAddEntryInvalidType(t *testing.T) {
	e := newTestEngine(t)
	defer e.Close()

	_, err := e.AddEntry(AddEntryInput{
		Type:    core.EntryType("invalid"),
		Content: []byte("test"),
	})
	if err == nil {
		t.Error("expected error for invalid type")
	}
}

func TestGetEntry(t *testing.T) {
	e := newTestEngine(t)
	defer e.Close()

	added, _ := e.AddEntry(AddEntryInput{
		Type:    core.Note,
		Content: []byte("test"),
	})

	retrieved, err := e.GetEntry(added.ID)
	if err != nil {
		t.Fatalf("failed to get entry: %v", err)
	}

	if retrieved.ID != added.ID {
		t.Error("ID mismatch")
	}
}

func TestGetEntryNotFound(t *testing.T) {
	e := newTestEngine(t)
	defer e.Close()

	_, err := e.GetEntry(uuid.New())
	if err == nil {
		t.Error("expected error for non-existent entry")
	}
}

func TestUpdateEntry(t *testing.T) {
	e := newTestEngine(t)
	defer e.Close()

	entry, _ := e.AddEntry(AddEntryInput{
		Type:    core.Note,
		Content: []byte("original"),
		Tags:    []string{"tag1"},
	})

	newContent := []byte("updated")
	newTags := []string{"tag2", "tag3"}
	err := e.UpdateEntry(entry.ID, UpdateEntryInput{
		Content: &newContent,
		Tags:    &newTags,
	})
	if err != nil {
		t.Fatalf("failed to update entry: %v", err)
	}

	updated, _ := e.GetEntry(entry.ID)
	if string(updated.Content) != "updated" {
		t.Error("content was not updated")
	}
	if len(updated.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(updated.Tags))
	}
	if updated.UpdatedAt <= entry.UpdatedAt {
		t.Error("UpdatedAt should be incremented")
	}
}

func TestUpdateEntryPartial(t *testing.T) {
	e := newTestEngine(t)
	defer e.Close()

	entry, _ := e.AddEntry(AddEntryInput{
		Type:    core.Note,
		Content: []byte("original"),
		Tags:    []string{"tag1"},
	})

	// Update only content
	newContent := []byte("updated")
	e.UpdateEntry(entry.ID, UpdateEntryInput{
		Content: &newContent,
	})

	updated, _ := e.GetEntry(entry.ID)
	if string(updated.Content) != "updated" {
		t.Error("content was not updated")
	}
	if len(updated.Tags) != 1 || updated.Tags[0] != "tag1" {
		t.Error("tags should remain unchanged")
	}
}

func TestDeleteEntry(t *testing.T) {
	e := newTestEngine(t)
	defer e.Close()

	entry, _ := e.AddEntry(AddEntryInput{
		Type:    core.Note,
		Content: []byte("to delete"),
	})

	err := e.DeleteEntry(entry.ID)
	if err != nil {
		t.Fatalf("failed to delete entry: %v", err)
	}

	// Deleted entry returns error (CRDT tombstone semantics)
	_, err = e.GetEntry(entry.ID)
	if err == nil {
		t.Error("GetEntry should return error for deleted entry")
	}

	// Should not appear in default list
	entries, _ := e.ListEntries(ListFilter{})
	if len(entries) != 0 {
		t.Error("deleted entry should not appear in list")
	}
}

func TestUpdateDeletedEntry(t *testing.T) {
	e := newTestEngine(t)
	defer e.Close()

	entry, _ := e.AddEntry(AddEntryInput{
		Type:    core.Note,
		Content: []byte("test"),
	})

	e.DeleteEntry(entry.ID)

	newContent := []byte("updated")
	err := e.UpdateEntry(entry.ID, UpdateEntryInput{
		Content: &newContent,
	})
	if err == nil {
		t.Error("expected error when updating deleted entry")
	}
}

func TestListEntries(t *testing.T) {
	e := newTestEngine(t)
	defer e.Close()

	e.AddEntry(AddEntryInput{Type: core.Note, Content: []byte("1"), Tags: []string{"work"}})
	e.AddEntry(AddEntryInput{Type: core.Log, Content: []byte("2"), Tags: []string{"personal"}})
	e.AddEntry(AddEntryInput{Type: core.Note, Content: []byte("3"), Tags: []string{"work"}})

	// All entries
	entries, _ := e.ListEntries(ListFilter{})
	if len(entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(entries))
	}

	// Filter by type
	noteType := core.Note
	entries, _ = e.ListEntries(ListFilter{Type: &noteType})
	if len(entries) != 2 {
		t.Errorf("expected 2 notes, got %d", len(entries))
	}

	// Filter by tag
	workTag := "work"
	entries, _ = e.ListEntries(ListFilter{Tag: &workTag})
	if len(entries) != 2 {
		t.Errorf("expected 2 entries with 'work' tag, got %d", len(entries))
	}
}

func TestClockIncrementsMonotonically(t *testing.T) {
	e := newTestEngine(t)
	defer e.Close()

	var lastTime uint64 = 0
	for i := 0; i < 10; i++ {
		entry, _ := e.AddEntry(AddEntryInput{
			Type:    core.Note,
			Content: []byte("test"),
		})
		if entry.CreatedAt <= lastTime {
			t.Errorf("clock should be monotonically increasing: %d <= %d", entry.CreatedAt, lastTime)
		}
		lastTime = entry.CreatedAt
	}
}
