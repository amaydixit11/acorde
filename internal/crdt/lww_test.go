package crdt

import (
	"testing"

	"github.com/amaydixit11/acorde/internal/core"
	"github.com/google/uuid"
)

func TestLWWSetAdd(t *testing.T) {
	s := NewLWWSet()

	entry := core.NewEntry(core.Note, []byte("test"), nil, 1)
	s.Add(entry)

	retrieved, exists := s.Lookup(entry.ID)
	if !exists {
		t.Fatal("entry should exist")
	}
	if string(retrieved.Content) != "test" {
		t.Error("content mismatch")
	}
}

func TestLWWSetAddHigherTimestampWins(t *testing.T) {
	s := NewLWWSet()

	id := uuid.New()
	entry1 := core.Entry{ID: id, Content: []byte("old"), UpdatedAt: 1}
	entry2 := core.Entry{ID: id, Content: []byte("new"), UpdatedAt: 2}

	s.Add(entry1)
	s.Add(entry2)

	retrieved, _ := s.Lookup(id)
	if string(retrieved.Content) != "new" {
		t.Error("higher timestamp should win")
	}

	// Adding older entry should not overwrite
	entry3 := core.Entry{ID: id, Content: []byte("older"), UpdatedAt: 1}
	s.Add(entry3)

	retrieved, _ = s.Lookup(id)
	if string(retrieved.Content) != "new" {
		t.Error("older timestamp should not overwrite")
	}
}

func TestLWWSetRemove(t *testing.T) {
	s := NewLWWSet()

	entry := core.NewEntry(core.Note, []byte("test"), nil, 1)
	s.Add(entry)

	s.Remove(entry.ID, 2)

	_, exists := s.Lookup(entry.ID)
	if exists {
		t.Error("entry should be deleted")
	}

	// Should still be retrievable with deleted flag
	deleted, exists := s.LookupWithDeleted(entry.ID)
	if !exists {
		t.Error("tombstone should exist")
	}
	if !deleted.Deleted {
		t.Error("entry should be marked deleted")
	}
}

func TestLWWSetRemoveOlderTimestamp(t *testing.T) {
	s := NewLWWSet()

	entry := core.Entry{ID: uuid.New(), Content: []byte("test"), UpdatedAt: 5}
	s.Add(entry)

	// Try to delete with older timestamp
	s.Remove(entry.ID, 3)

	_, exists := s.Lookup(entry.ID)
	if !exists {
		t.Error("entry should not be deleted with older timestamp")
	}
}

func TestLWWSetMergeBasic(t *testing.T) {
	s1 := NewLWWSet()
	s2 := NewLWWSet()

	entry1 := core.NewEntry(core.Note, []byte("entry1"), nil, 1)
	entry2 := core.NewEntry(core.Note, []byte("entry2"), nil, 2)

	s1.Add(entry1)
	s2.Add(entry2)

	s1.Merge(s2)

	if s1.ActiveSize() != 2 {
		t.Errorf("expected 2 entries after merge, got %d", s1.ActiveSize())
	}

	_, exists := s1.Lookup(entry2.ID)
	if !exists {
		t.Error("entry2 should exist after merge")
	}
}

func TestLWWSetMergeConflict(t *testing.T) {
	s1 := NewLWWSet()
	s2 := NewLWWSet()

	id := uuid.New()
	entry1 := core.Entry{ID: id, Content: []byte("from s1"), UpdatedAt: 100}
	entry2 := core.Entry{ID: id, Content: []byte("from s2"), UpdatedAt: 105}

	s1.Add(entry1)
	s2.Add(entry2)

	s1.Merge(s2)

	retrieved, _ := s1.Lookup(id)
	if string(retrieved.Content) != "from s2" {
		t.Error("higher timestamp (s2) should win")
	}
}

func TestLWWSetMergeDeleteWins(t *testing.T) {
	s1 := NewLWWSet()
	s2 := NewLWWSet()

	id := uuid.New()
	entry := core.Entry{ID: id, Content: []byte("test"), UpdatedAt: 100}

	s1.Add(entry)
	s2.Add(entry)
	s2.Remove(id, 105)

	s1.Merge(s2)

	_, exists := s1.Lookup(id)
	if exists {
		t.Error("deleted entry should not be visible after merge")
	}
}

func TestLWWSetMergeCommutative(t *testing.T) {
	// A.Merge(B) should equal B.Merge(A)
	a := NewLWWSet()
	b := NewLWWSet()

	entry1 := core.NewEntry(core.Note, []byte("1"), nil, 1)
	entry2 := core.NewEntry(core.Note, []byte("2"), nil, 2)
	entry3 := core.Entry{ID: entry1.ID, Content: []byte("1-updated"), UpdatedAt: 3}

	a.Add(entry1)
	a.Add(entry2)
	b.Add(entry3)

	// Clone before merge
	a1 := a.Clone()
	b1 := b.Clone()
	a2 := a.Clone()
	b2 := b.Clone()

	a1.Merge(b1)
	b2.Merge(a2)

	// Both should have same elements
	if a1.ActiveSize() != b2.ActiveSize() {
		t.Errorf("commutative: sizes differ %d vs %d", a1.ActiveSize(), b2.ActiveSize())
	}

	for _, elem := range a1.AllElements() {
		other, exists := b2.LookupWithDeleted(elem.Entry.ID)
		if !exists {
			t.Error("commutative: element missing in b2")
			continue
		}
		if string(elem.Entry.Content) != string(other.Content) {
			t.Errorf("commutative: content differs for %s", elem.Entry.ID)
		}
	}
}

func TestLWWSetMergeAssociative(t *testing.T) {
	// (A.Merge(B)).Merge(C) should equal A.Merge(B.Merge(C))
	a := NewLWWSet()
	b := NewLWWSet()
	c := NewLWWSet()

	entry1 := core.NewEntry(core.Note, []byte("1"), nil, 1)
	entry2 := core.NewEntry(core.Note, []byte("2"), nil, 2)
	entry3 := core.NewEntry(core.Note, []byte("3"), nil, 3)

	a.Add(entry1)
	b.Add(entry2)
	c.Add(entry3)

	// (A ⊔ B) ⊔ C
	left := a.Clone()
	left.Merge(b.Clone())
	left.Merge(c.Clone())

	// A ⊔ (B ⊔ C)
	bc := b.Clone()
	bc.Merge(c.Clone())
	right := a.Clone()
	right.Merge(bc)

	if left.ActiveSize() != right.ActiveSize() {
		t.Errorf("associative: sizes differ %d vs %d", left.ActiveSize(), right.ActiveSize())
	}
}

func TestLWWSetMergeIdempotent(t *testing.T) {
	// A.Merge(A) should equal A
	a := NewLWWSet()

	entry1 := core.NewEntry(core.Note, []byte("1"), nil, 1)
	entry2 := core.NewEntry(core.Note, []byte("2"), nil, 2)

	a.Add(entry1)
	a.Add(entry2)

	before := a.Clone()
	a.Merge(before)

	if a.ActiveSize() != before.ActiveSize() {
		t.Error("idempotent: size changed after self-merge")
	}

	for _, elem := range before.AllElements() {
		after, _ := a.LookupWithDeleted(elem.Entry.ID)
		if string(elem.Entry.Content) != string(after.Content) {
			t.Error("idempotent: content changed after self-merge")
		}
	}
}

func TestLWWSetElements(t *testing.T) {
	s := NewLWWSet()

	e1 := core.NewEntry(core.Note, []byte("1"), nil, 1)
	e2 := core.NewEntry(core.Note, []byte("2"), nil, 2)
	e3 := core.NewEntry(core.Note, []byte("3"), nil, 3)

	s.Add(e1)
	s.Add(e2)
	s.Add(e3)
	s.Remove(e2.ID, 4)

	elements := s.Elements()
	if len(elements) != 2 {
		t.Errorf("expected 2 active elements, got %d", len(elements))
	}
}
