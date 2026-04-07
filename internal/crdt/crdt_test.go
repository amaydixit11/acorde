package crdt

import (
	"reflect"
	"testing"

	"github.com/amaydixit11/acorde/internal/core"
	"github.com/google/uuid"
)

func TestORSet_AddRemove(t *testing.T) {
	s := NewORSet()

	s.Add("a")
	s.Add("a") // Duplicate add should be idempotent
	s.Remove("a")

	if s.Contains("a") {
		t.Fatal("expected tag to be removed")
	}
}

func TestORSet_Concurrent(t *testing.T) {
	a := NewORSet()
	b := NewORSet()

	// Peer A adds "x"
	token := a.Add("x")

	// Same token is synced to Peer B, which then removes it
	b.AddWithToken("x", token)
	b.Remove("x")

	// Merge B back to A
	a.Merge(b)

	if a.Contains("x") {
		t.Fatal("should be removed after merge (remove wins over that specific token)")
	}
}

func TestReplica_Convergence(t *testing.T) {
	c1 := core.NewClock()
	c2 := core.NewClock()

	r1 := NewReplica(c1)
	r2 := NewReplica(c2)

	// Peer 1 adds entry A
	id := uuid.New()
	r1.AddEntryWithID(id, "note", []byte("A"), []string{"t1"})

	// Peer 2 adds entry B with SAME ID (conflict!)
	// Manually sync clock or rely on merge logic
	c2.Update(c1.Now())
	r2.AddEntryWithID(id, "note", []byte("B"), []string{"t2"})

	// Bi-directional merge
	r1.Merge(r2)
	r2.Merge(r1)

	e1 := r1.ListEntries()
	e2 := r2.ListEntries()

	if len(e1) != len(e2) || len(e1) != 1 {
		t.Fatalf("expected 1 entry, got r1:%d r2:%d", len(e1), len(e2))
	}

	if !reflect.DeepEqual(e1[0], e2[0]) {
		t.Errorf("replicas did not converge to identical entry state\nNode 1: %+v\nNode 2: %+v", e1[0], e2[0])
	}
}

func TestReplica_Idempotency(t *testing.T) {
	c1 := core.NewClock()
	r1 := NewReplica(c1)

	r1.AddEntry("note", []byte("hello"), []string{"sync"})
	state1 := r1.State()

	// Merge own state multiple times
	r1.Merge(r1)
	r1.Merge(r1)

	state2 := r1.State()
	if !reflect.DeepEqual(state1.Entries, state2.Entries) {
		t.Fatal("Merge is not idempotent for entries")
	}
}
