package sqlite

import (
	"os"
	"testing"

	"github.com/amaydixit11/vaultd/internal/core"
	"github.com/amaydixit11/vaultd/internal/storage"
	"github.com/google/uuid"
)

func TestNew(t *testing.T) {
	store, err := New(":memory:")
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	defer store.Close()
}

func TestNewWithFile(t *testing.T) {
	tmpFile := "/tmp/vaultd_test_" + uuid.New().String() + ".db"
	defer os.Remove(tmpFile)

	store, err := New(tmpFile)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	store.Close()

	// Verify file was created
	if _, err := os.Stat(tmpFile); os.IsNotExist(err) {
		t.Error("database file was not created")
	}
}

func TestPutAndGet(t *testing.T) {
	store, _ := New(":memory:")
	defer store.Close()

	entry := core.NewEntry(core.Note, []byte("test content"), []string{"tag1", "tag2"}, 1)

	err := store.Put(entry)
	if err != nil {
		t.Fatalf("failed to put entry: %v", err)
	}

	retrieved, err := store.Get(entry.ID)
	if err != nil {
		t.Fatalf("failed to get entry: %v", err)
	}

	if retrieved.ID != entry.ID {
		t.Errorf("ID mismatch: got %s, want %s", retrieved.ID, entry.ID)
	}
	if retrieved.Type != entry.Type {
		t.Errorf("Type mismatch: got %s, want %s", retrieved.Type, entry.Type)
	}
	if string(retrieved.Content) != string(entry.Content) {
		t.Errorf("Content mismatch")
	}
	if len(retrieved.Tags) != 2 {
		t.Errorf("Tags count mismatch: got %d, want 2", len(retrieved.Tags))
	}
}

func TestGetNotFound(t *testing.T) {
	store, _ := New(":memory:")
	defer store.Close()

	_, err := store.Get(uuid.New())
	if _, ok := err.(storage.ErrNotFound); !ok {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestPutIdempotent(t *testing.T) {
	store, _ := New(":memory:")
	defer store.Close()

	entry := core.NewEntry(core.Note, []byte("original"), []string{"tag1"}, 1)
	store.Put(entry)

	// Update the entry
	entry.Content = []byte("updated")
	entry.Tags = []string{"tag2", "tag3"}
	entry.UpdatedAt = 2
	store.Put(entry)

	retrieved, _ := store.Get(entry.ID)
	if string(retrieved.Content) != "updated" {
		t.Error("content was not updated")
	}
	if len(retrieved.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(retrieved.Tags))
	}
	if retrieved.UpdatedAt != 2 {
		t.Error("UpdatedAt was not updated")
	}
}

func TestList(t *testing.T) {
	store, _ := New(":memory:")
	defer store.Close()

	// Add multiple entries
	e1 := core.NewEntry(core.Note, []byte("note 1"), []string{"work"}, 1)
	e2 := core.NewEntry(core.Log, []byte("log 1"), []string{"personal"}, 2)
	e3 := core.NewEntry(core.Note, []byte("note 2"), []string{"work"}, 3)

	store.Put(e1)
	store.Put(e2)
	store.Put(e3)

	// List all
	entries, err := store.List(storage.ListFilter{})
	if err != nil {
		t.Fatalf("failed to list: %v", err)
	}
	if len(entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(entries))
	}

	// Filter by type
	noteType := core.Note
	entries, _ = store.List(storage.ListFilter{Type: &noteType})
	if len(entries) != 2 {
		t.Errorf("expected 2 notes, got %d", len(entries))
	}

	// Filter by tag
	workTag := "work"
	entries, _ = store.List(storage.ListFilter{Tag: &workTag})
	if len(entries) != 2 {
		t.Errorf("expected 2 entries with 'work' tag, got %d", len(entries))
	}
}

func TestListWithTimeFilters(t *testing.T) {
	store, _ := New(":memory:")
	defer store.Close()

	e1 := core.NewEntry(core.Note, []byte("1"), nil, 10)
	e2 := core.NewEntry(core.Note, []byte("2"), nil, 20)
	e3 := core.NewEntry(core.Note, []byte("3"), nil, 30)

	store.Put(e1)
	store.Put(e2)
	store.Put(e3)

	// Since filter
	since := uint64(15)
	entries, _ := store.List(storage.ListFilter{Since: &since})
	if len(entries) != 2 {
		t.Errorf("expected 2 entries since 15, got %d", len(entries))
	}

	// Until filter
	until := uint64(25)
	entries, _ = store.List(storage.ListFilter{Until: &until})
	if len(entries) != 2 {
		t.Errorf("expected 2 entries until 25, got %d", len(entries))
	}

	// Combined
	entries, _ = store.List(storage.ListFilter{Since: &since, Until: &until})
	if len(entries) != 1 {
		t.Errorf("expected 1 entry between 15-25, got %d", len(entries))
	}
}

func TestListWithPagination(t *testing.T) {
	store, _ := New(":memory:")
	defer store.Close()

	for i := 0; i < 10; i++ {
		e := core.NewEntry(core.Note, []byte("content"), nil, uint64(i))
		store.Put(e)
	}

	// Limit
	entries, _ := store.List(storage.ListFilter{Limit: 5})
	if len(entries) != 5 {
		t.Errorf("expected 5 entries with limit, got %d", len(entries))
	}

	// Offset
	entries, _ = store.List(storage.ListFilter{Limit: 5, Offset: 5})
	if len(entries) != 5 {
		t.Errorf("expected 5 entries with offset, got %d", len(entries))
	}
}

func TestDelete(t *testing.T) {
	store, _ := New(":memory:")
	defer store.Close()

	entry := core.NewEntry(core.Note, []byte("to delete"), nil, 1)
	store.Put(entry)

	err := store.Delete(entry.ID)
	if err != nil {
		t.Fatalf("failed to delete: %v", err)
	}

	// Entry should still exist but be marked deleted
	retrieved, _ := store.Get(entry.ID)
	if !retrieved.Deleted {
		t.Error("entry should be marked as deleted")
	}

	// Should not appear in list by default
	entries, _ := store.List(storage.ListFilter{})
	if len(entries) != 0 {
		t.Errorf("deleted entry should not appear in list, got %d", len(entries))
	}

	// Should appear when Deleted filter is true
	entries, _ = store.List(storage.ListFilter{Deleted: true})
	if len(entries) != 1 {
		t.Errorf("expected 1 entry with Deleted=true, got %d", len(entries))
	}
}

func TestDeleteNotFound(t *testing.T) {
	store, _ := New(":memory:")
	defer store.Close()

	err := store.Delete(uuid.New())
	if _, ok := err.(storage.ErrNotFound); !ok {
		t.Errorf("expected ErrNotFound, got %v", err)
	}
}

func TestApplyBatch(t *testing.T) {
	store, _ := New(":memory:")
	defer store.Close()

	e1 := core.NewEntry(core.Note, []byte("entry 1"), nil, 1)
	e2 := core.NewEntry(core.Log, []byte("entry 2"), nil, 2)

	// Put both in a batch
	err := store.ApplyBatch([]storage.Operation{
		{Type: storage.OpPut, Entry: e1},
		{Type: storage.OpPut, Entry: e2},
	})
	if err != nil {
		t.Fatalf("failed to apply batch: %v", err)
	}

	entries, _ := store.List(storage.ListFilter{})
	if len(entries) != 2 {
		t.Errorf("expected 2 entries, got %d", len(entries))
	}

	// Delete one in a batch
	err = store.ApplyBatch([]storage.Operation{
		{Type: storage.OpDelete, Entry: e1},
	})
	if err != nil {
		t.Fatalf("failed to apply delete batch: %v", err)
	}

	entries, _ = store.List(storage.ListFilter{})
	if len(entries) != 1 {
		t.Errorf("expected 1 entry after delete, got %d", len(entries))
	}
}

func TestGetMaxTimestamp(t *testing.T) {
	store, _ := New(":memory:")
	defer store.Close()

	// Empty database
	maxTime, err := store.GetMaxTimestamp()
	if err != nil {
		t.Fatalf("failed to get max timestamp: %v", err)
	}
	if maxTime != 0 {
		t.Errorf("expected 0 for empty db, got %d", maxTime)
	}

	// Add entries
	e1 := core.NewEntry(core.Note, []byte("1"), nil, 100)
	e2 := core.NewEntry(core.Note, []byte("2"), nil, 50)
	store.Put(e1)
	store.Put(e2)

	maxTime, _ = store.GetMaxTimestamp()
	if maxTime != 100 {
		t.Errorf("expected max timestamp 100, got %d", maxTime)
	}
}

func TestPersistence(t *testing.T) {
	tmpFile := "/tmp/vaultd_persist_test_" + uuid.New().String() + ".db"
	defer os.Remove(tmpFile)

	// Create and populate
	store1, _ := New(tmpFile)
	entry := core.NewEntry(core.Note, []byte("persistent"), []string{"tag"}, 42)
	store1.Put(entry)
	store1.Close()

	// Reopen and verify
	store2, _ := New(tmpFile)
	defer store2.Close()

	retrieved, err := store2.Get(entry.ID)
	if err != nil {
		t.Fatalf("failed to get entry after reopen: %v", err)
	}
	if string(retrieved.Content) != "persistent" {
		t.Error("content was not persisted correctly")
	}
	if len(retrieved.Tags) != 1 || retrieved.Tags[0] != "tag" {
		t.Error("tags were not persisted correctly")
	}
}
