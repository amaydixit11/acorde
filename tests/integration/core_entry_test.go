package integration

import (
	"fmt"
	"os"
	"testing"

	"github.com/amaydixit11/acorde/internal/core"
	"github.com/amaydixit11/acorde/internal/engine"
	"github.com/google/uuid"
)

// setupEngine creates a new engine instance with a temporary directory
func setupEngine(t *testing.T) (engine.Engine, string) {
	tempDir, err := os.MkdirTemp("", "acorde-integration-test-*")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}

	cfg := engine.Config{
		DataDir:  tempDir,
		InMemory: false,
	}

	e, err := engine.New(cfg)
	if err != nil {
		os.RemoveAll(tempDir)
		t.Fatalf("failed to create engine: %v", err)
	}

	return e, tempDir
}

func cleanupEngine(e engine.Engine, dir string) {
	e.Close()
	os.RemoveAll(dir)
}

func TestCoreEntryManagement(t *testing.T) {
	e, dir := setupEngine(t)
	defer cleanupEngine(e, dir)

	var createdEntries []engine.Entry

	t.Run("Create Entries", func(t *testing.T) {
		types := []core.EntryType{core.Note, core.Log, core.File, core.Event}
		
		for _, entryType := range types {
			content := []byte(fmt.Sprintf("content for %s", entryType))
			tags := []string{"test", string(entryType)}

			entry, err := e.AddEntry(engine.AddEntryInput{
				Type:    entryType,
				Content: content,
				Tags:    tags,
			})
			if err != nil {
				t.Fatalf("failed to add entry of type %s: %v", entryType, err)
			}

			if entry.ID == uuid.Nil {
				t.Error("expected valid UUID")
			}
			if entry.Type != entryType {
				t.Errorf("expected type %s, got %s", entryType, entry.Type)
			}
			if string(entry.Content) != string(content) {
				t.Errorf("expected content %s, got %s", content, entry.Content)
			}
			if len(entry.Tags) != 2 {
				t.Errorf("expected 2 tags, got %d", len(entry.Tags))
			}
			if entry.CreatedAt == 0 {
				t.Error("expected CreatedAt to be set")
			}

			createdEntries = append(createdEntries, entry)
		}
	})

	t.Run("Read Entries", func(t *testing.T) {
		// Test GetEntry
		for _, created := range createdEntries {
			retrieved, err := e.GetEntry(created.ID)
			if err != nil {
				t.Fatalf("failed to get entry %s: %v", created.ID, err)
			}
			if retrieved.ID != created.ID {
				t.Error("ID mismatch")
			}
		}

		// Test ListEntries with filters
		
		// Filter by Type
		filterType := core.Note
		entries, err := e.ListEntries(engine.ListFilter{Type: &filterType})
		if err != nil {
			t.Fatalf("failed to list entries by type: %v", err)
		}
		if len(entries) != 1 {
			t.Errorf("expected 1 note, got %d", len(entries))
		}
		if entries[0].Type != core.Note {
			t.Errorf("expected note type, got %s", entries[0].Type)
		}

		// Filter by Tag
		filterTag := "test"
		entries, err = e.ListEntries(engine.ListFilter{Tag: &filterTag})
		if err != nil {
			t.Fatalf("failed to list entries by tag: %v", err)
		}
		// All 4 entries added have "test" tag
		if len(entries) != 4 {
			t.Errorf("expected 4 entries with tag 'test', got %d", len(entries))
		}

		// Pagination
		entries, err = e.ListEntries(engine.ListFilter{Limit: 2})
		if err != nil {
			t.Fatalf("failed to list entries with limit: %v", err)
		}
		if len(entries) != 2 {
			t.Errorf("expected 2 entries with limit, got %d", len(entries))
		}

		entries, err = e.ListEntries(engine.ListFilter{Offset: 2})
		if err != nil {
			t.Fatalf("failed to list entries with offset: %v", err)
		}
		if len(entries) != 2 { // Total 4, offset 2 -> 2 remaining
			t.Errorf("expected 2 entries with offset, got %d", len(entries))
		}
	})

	t.Run("Update Entries", func(t *testing.T) {
		target := createdEntries[0] // Note
		
		newContent := []byte("updated content")
		newTags := []string{"updated", "tag"}

		err := e.UpdateEntry(target.ID, engine.UpdateEntryInput{
			Content: &newContent,
			Tags:    &newTags,
		})
		if err != nil {
			t.Fatalf("failed to update entry: %v", err)
		}

		updated, err := e.GetEntry(target.ID)
		if err != nil {
			t.Fatalf("failed to get updated entry: %v", err)
		}

		if string(updated.Content) != "updated content" {
			t.Errorf("expected updated content, got %s", updated.Content)
		}
		if len(updated.Tags) != 2 {
			t.Errorf("expected 2 tags, got %d", len(updated.Tags))
		}
		hasUpdated := false
		hasTag := false
		for _, tag := range updated.Tags {
			if tag == "updated" { hasUpdated = true }
			if tag == "tag" { hasTag = true }
		}
		if !hasUpdated || !hasTag {
			t.Errorf("tags not updated correctly: %v", updated.Tags)
		}
		if updated.UpdatedAt <= target.UpdatedAt {
			t.Error("UpdatedAt should be incremented")
		}
	})

	t.Run("Delete Entries", func(t *testing.T) {
		target := createdEntries[1] // Log

		err := e.DeleteEntry(target.ID)
		if err != nil {
			t.Fatalf("failed to delete entry: %v", err)
		}

		// Verify GetEntry returns error or marked deleted (depending on implementation choice exposed)
		// Engine impl says GetEntry returns error if deleted
		_, err = e.GetEntry(target.ID)
		if err == nil {
			t.Error("expected error getting deleted entry")
		}

		// Verify ListEntries excludes it
		entries, err := e.ListEntries(engine.ListFilter{})
		if err != nil {
			t.Fatalf("failed to list entries: %v", err)
		}
		for _, entry := range entries {
			if entry.ID == target.ID {
				t.Error("deleted entry found in default list")
			}
		}

		// Verify ListEntries with Deleted=true includes it
		entries, err = e.ListEntries(engine.ListFilter{Deleted: true})
		if err != nil {
			t.Fatalf("failed to list deleted entries: %v", err)
		}
		found := false
		for _, entry := range entries {
			if entry.ID == target.ID {
				found = true
				if !entry.Deleted {
					t.Error("entry should be marked as deleted")
				}
				break
			}
		}
		if !found {
			t.Error("deleted entry not found when including deleted")
		}
	})

	t.Run("Persistence", func(t *testing.T) {
		// Close and reopen engine to test persistence
		e.Close()

		// Reopen on same dir
		cfg := engine.Config{
			DataDir:  dir,
			InMemory: false,
		}
		newE, err := engine.New(cfg)
		if err != nil {
			t.Fatalf("failed to reopen engine: %v", err)
		}
		defer newE.Close() // In case subsequent tests were added, though this is the last one in this function scope for now, the defer cleanupEngine handles cleanup of dir. We just need to close this new instance.
		
		// Actually cleanupEngine above will try to close e which is already closed, but that's usually fine or we should handle it.
		// Let's rely on cleanupEngine closing the *initial* e, but since we closed it, we should be careful. 
		// Ideally we update 'e' variable but it's local to setup.
		// We'll just close newE manually here.
		
		// Check if data persists
		// Note (idx 0) was updated
		noteID := createdEntries[0].ID
		note, err := newE.GetEntry(noteID)
		if err != nil {
			t.Fatalf("failed to get persisted note: %v", err)
		}
		if string(note.Content) != "updated content" {
			t.Error("persistence failed for content update")
		}

		// Log (idx 1) was deleted
		logID := createdEntries[1].ID
		_, err = newE.GetEntry(logID)
		if err == nil {
			t.Error("persistence failed for deletion (should return error)")
		}
	})
}
