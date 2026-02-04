package crdt

import (
	"sort"
	"testing"

	"github.com/amaydixit11/acorde/internal/core"
	"github.com/google/uuid"
)

func TestReplicaAddEntry(t *testing.T) {
	r := NewReplica(core.NewClock())

	entry := r.AddEntry(core.Note, []byte("test"), []string{"work", "important"})

	if entry.ID == uuid.Nil {
		t.Error("entry should have valid ID")
	}
	if entry.Type != core.Note {
		t.Error("type mismatch")
	}
	if string(entry.Content) != "test" {
		t.Error("content mismatch")
	}
	if len(entry.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(entry.Tags))
	}
}

func TestReplicaGetEntry(t *testing.T) {
	r := NewReplica(core.NewClock())

	added := r.AddEntry(core.Note, []byte("test"), []string{"tag1"})

	retrieved, err := r.GetEntry(added.ID)
	if err != nil {
		t.Fatalf("failed to get entry: %v", err)
	}
	if retrieved.ID != added.ID {
		t.Error("ID mismatch")
	}
}

func TestReplicaUpdateEntry(t *testing.T) {
	r := NewReplica(core.NewClock())

	entry := r.AddEntry(core.Note, []byte("original"), []string{"tag1"})

	newContent := []byte("updated")
	err := r.UpdateEntry(entry.ID, &newContent, nil)
	if err != nil {
		t.Fatalf("failed to update: %v", err)
	}

	updated, _ := r.GetEntry(entry.ID)
	if string(updated.Content) != "updated" {
		t.Error("content not updated")
	}
	if len(updated.Tags) != 1 {
		t.Error("tags should be unchanged")
	}
}

func TestReplicaUpdateTags(t *testing.T) {
	r := NewReplica(core.NewClock())

	entry := r.AddEntry(core.Note, []byte("test"), []string{"old"})

	newTags := []string{"new1", "new2"}
	r.UpdateEntry(entry.ID, nil, &newTags)

	updated, _ := r.GetEntry(entry.ID)
	if len(updated.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(updated.Tags))
	}
}

func TestReplicaDeleteEntry(t *testing.T) {
	r := NewReplica(core.NewClock())

	entry := r.AddEntry(core.Note, []byte("test"), nil)

	err := r.DeleteEntry(entry.ID)
	if err != nil {
		t.Fatalf("failed to delete: %v", err)
	}

	_, err = r.GetEntry(entry.ID)
	if _, ok := err.(*ErrEntryDeleted); !ok {
		t.Error("should get ErrEntryDeleted for deleted entry")
	}
}

func TestReplicaListEntries(t *testing.T) {
	r := NewReplica(core.NewClock())

	r.AddEntry(core.Note, []byte("1"), nil)
	r.AddEntry(core.Log, []byte("2"), nil)
	r.AddEntry(core.Event, []byte("3"), nil)

	entries := r.ListEntries()
	if len(entries) != 3 {
		t.Errorf("expected 3 entries, got %d", len(entries))
	}
}

func TestReplicaMergeBasic(t *testing.T) {
	r1 := NewReplica(core.NewClock())
	r2 := NewReplica(core.NewClock())

	e1 := r1.AddEntry(core.Note, []byte("from r1"), nil)
	e2 := r2.AddEntry(core.Note, []byte("from r2"), nil)

	r1.Merge(r2)

	entries := r1.ListEntries()
	if len(entries) != 2 {
		t.Errorf("expected 2 entries after merge, got %d", len(entries))
	}

	_, err := r1.GetEntry(e1.ID)
	if err != nil {
		t.Error("e1 should exist after merge")
	}

	_, err = r1.GetEntry(e2.ID)
	if err != nil {
		t.Error("e2 should exist after merge")
	}
}

func TestReplicaMergeConflict(t *testing.T) {
	// Both replicas modify the same entry
	r1 := NewReplica(core.NewClockWithTime(0))
	r2 := NewReplica(core.NewClockWithTime(0))

	// Create entry in r1
	entry := r1.AddEntry(core.Note, []byte("original"), nil)

	// Sync to r2
	r2.Merge(r1)

	// Concurrent updates
	content1 := []byte("r1 update")
	r1.UpdateEntry(entry.ID, &content1, nil)

	// Simulate r2 having higher clock
	r2.clock = core.NewClockWithTime(100)
	content2 := []byte("r2 update")
	r2.UpdateEntry(entry.ID, &content2, nil)

	// Merge - r2 should win (higher timestamp)
	r1.Merge(r2)

	result, _ := r1.GetEntry(entry.ID)
	if string(result.Content) != "r2 update" {
		t.Errorf("expected 'r2 update', got '%s'", string(result.Content))
	}
}

func TestReplicaMergeDeleteVsUpdate(t *testing.T) {
	r1 := NewReplica(core.NewClockWithTime(0))
	r2 := NewReplica(core.NewClockWithTime(0))

	entry := r1.AddEntry(core.Note, []byte("test"), nil)
	r2.Merge(r1)

	// r1 updates, r2 deletes (with higher timestamp)
	content := []byte("updated")
	r1.UpdateEntry(entry.ID, &content, nil)

	r2.clock = core.NewClockWithTime(100)
	r2.DeleteEntry(entry.ID)

	// Merge
	r1.Merge(r2)

	_, err := r1.GetEntry(entry.ID)
	if _, ok := err.(*ErrEntryDeleted); !ok {
		t.Error("entry should be deleted after merge (delete had higher timestamp)")
	}
}

func TestReplicaMergeTags(t *testing.T) {
	r1 := NewReplica(core.NewClock())
	r2 := NewReplica(core.NewClock())

	entry := r1.AddEntry(core.Note, []byte("test"), []string{"tag1"})
	r2.Merge(r1)

	// Both add different tags
	tags1 := []string{"tag1", "from-r1"}
	r1.UpdateEntry(entry.ID, nil, &tags1)

	tags2 := []string{"tag1", "from-r2"}
	r2.UpdateEntry(entry.ID, nil, &tags2)

	// Merge
	r1.Merge(r2)

	result, _ := r1.GetEntry(entry.ID)
	sort.Strings(result.Tags)

	// Should have tags from both (OR-Set semantics)
	// Note: actual result depends on OR-Set merge, may have tag1, from-r1, from-r2
	if len(result.Tags) < 2 {
		t.Errorf("expected at least 2 tags, got %d: %v", len(result.Tags), result.Tags)
	}
}

func TestReplicaMergeCommutative(t *testing.T) {
	a := NewReplica(core.NewClock())
	b := NewReplica(core.NewClock())

	a.AddEntry(core.Note, []byte("from a"), []string{"a"})
	b.AddEntry(core.Log, []byte("from b"), []string{"b"})

	a1 := a.Clone()
	b1 := b.Clone()
	a2 := a.Clone()
	b2 := b.Clone()

	a1.Merge(b1)
	b2.Merge(a2)

	entries1 := a1.ListEntries()
	entries2 := b2.ListEntries()

	if len(entries1) != len(entries2) {
		t.Errorf("commutative: sizes differ %d vs %d", len(entries1), len(entries2))
	}
}

func TestReplicaMergeAssociative(t *testing.T) {
	a := NewReplica(core.NewClock())
	b := NewReplica(core.NewClock())
	c := NewReplica(core.NewClock())

	a.AddEntry(core.Note, []byte("a"), nil)
	b.AddEntry(core.Note, []byte("b"), nil)
	c.AddEntry(core.Note, []byte("c"), nil)

	// (A ⊔ B) ⊔ C
	left := a.Clone()
	left.Merge(b.Clone())
	left.Merge(c.Clone())

	// A ⊔ (B ⊔ C)
	bc := b.Clone()
	bc.Merge(c.Clone())
	right := a.Clone()
	right.Merge(bc)

	if len(left.ListEntries()) != len(right.ListEntries()) {
		t.Error("associative: sizes differ")
	}
}

func TestReplicaMergeIdempotent(t *testing.T) {
	a := NewReplica(core.NewClock())

	a.AddEntry(core.Note, []byte("1"), []string{"tag"})
	a.AddEntry(core.Log, []byte("2"), nil)

	before := len(a.ListEntries())
	a.Merge(a.Clone())
	after := len(a.ListEntries())

	if before != after {
		t.Error("idempotent: size changed after self-merge")
	}
}

func TestReplicaState(t *testing.T) {
	r := NewReplica(core.NewClock())

	r.AddEntry(core.Note, []byte("test"), []string{"tag1", "tag2"})

	state := r.State()

	if len(state.Entries) != 1 {
		t.Errorf("expected 1 entry in state, got %d", len(state.Entries))
	}
	if len(state.Tags) != 1 {
		t.Errorf("expected 1 tag set in state, got %d", len(state.Tags))
	}
}
