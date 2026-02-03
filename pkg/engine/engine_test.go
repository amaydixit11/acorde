package engine_test

import (
	"testing"

	"github.com/amaydixit11/vaultd/pkg/engine"
	"github.com/google/uuid"
)

// TestPublicAPI verifies the public API works correctly
func TestPublicAPI(t *testing.T) {
	e, err := engine.New(engine.Config{InMemory: true})
	if err != nil {
		t.Fatalf("failed to create engine: %v", err)
	}
	defer e.Close()

	// Test AddEntry
	entry, err := e.AddEntry(engine.AddEntryInput{
		Type:    engine.Note,
		Content: []byte("test content"),
		Tags:    []string{"work", "important"},
	})
	if err != nil {
		t.Fatalf("failed to add entry: %v", err)
	}

	if entry.ID == uuid.Nil {
		t.Error("expected valid UUID")
	}
	if entry.Type != engine.Note {
		t.Errorf("expected Note type, got %s", entry.Type)
	}
	if string(entry.Content) != "test content" {
		t.Error("content mismatch")
	}
	if len(entry.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(entry.Tags))
	}

	// Test GetEntry
	retrieved, err := e.GetEntry(entry.ID)
	if err != nil {
		t.Fatalf("failed to get entry: %v", err)
	}
	if retrieved.ID != entry.ID {
		t.Error("ID mismatch")
	}

	// Test ListEntries
	entries, err := e.ListEntries(engine.ListFilter{})
	if err != nil {
		t.Fatalf("failed to list: %v", err)
	}
	if len(entries) != 1 {
		t.Errorf("expected 1 entry, got %d", len(entries))
	}

	// Test UpdateEntry
	newContent := []byte("updated content")
	err = e.UpdateEntry(entry.ID, engine.UpdateEntryInput{Content: &newContent})
	if err != nil {
		t.Fatalf("failed to update: %v", err)
	}

	updated, _ := e.GetEntry(entry.ID)
	if string(updated.Content) != "updated content" {
		t.Error("content not updated")
	}

	// Test DeleteEntry
	err = e.DeleteEntry(entry.ID)
	if err != nil {
		t.Fatalf("failed to delete: %v", err)
	}

	deleted, _ := e.GetEntry(entry.ID)
	if !deleted.Deleted {
		t.Error("entry should be deleted")
	}
}

func TestAllEntryTypes(t *testing.T) {
	e, _ := engine.New(engine.Config{InMemory: true})
	defer e.Close()

	types := []engine.EntryType{engine.Note, engine.Log, engine.File, engine.Event}

	for _, entryType := range types {
		entry, err := e.AddEntry(engine.AddEntryInput{
			Type:    entryType,
			Content: []byte("test"),
		})
		if err != nil {
			t.Errorf("failed to add %s: %v", entryType, err)
		}
		if entry.Type != entryType {
			t.Errorf("type mismatch: expected %s, got %s", entryType, entry.Type)
		}
	}

	entries, _ := e.ListEntries(engine.ListFilter{})
	if len(entries) != 4 {
		t.Errorf("expected 4 entries (one per type), got %d", len(entries))
	}
}

func TestInvalidEntryType(t *testing.T) {
	e, _ := engine.New(engine.Config{InMemory: true})
	defer e.Close()

	_, err := e.AddEntry(engine.AddEntryInput{
		Type:    engine.EntryType("invalid"),
		Content: []byte("test"),
	})
	if err == nil {
		t.Error("expected error for invalid type")
	}
}

func TestErrorTypes(t *testing.T) {
	e, _ := engine.New(engine.Config{InMemory: true})
	defer e.Close()

	// Test ErrNotFound
	_, err := e.GetEntry(uuid.New())
	if _, ok := err.(engine.ErrNotFound); !ok {
		t.Errorf("expected ErrNotFound, got %T", err)
	}
}

func TestEntryTypeValidation(t *testing.T) {
	validTypes := []engine.EntryType{engine.Note, engine.Log, engine.File, engine.Event}
	for _, t := range validTypes {
		if !t.IsValid() {
			panic("expected valid type: " + string(t))
		}
	}

	invalidTypes := []engine.EntryType{"invalid", "", "task"}
	for _, t := range invalidTypes {
		if t.IsValid() {
			panic("expected invalid type: " + string(t))
		}
	}
}
