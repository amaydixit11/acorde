package crdt

import (
	"sort"
	"testing"
)

func TestORSetAdd(t *testing.T) {
	s := NewORSet()

	s.Add("work")
	s.Add("personal")

	if !s.Contains("work") {
		t.Error("should contain 'work'")
	}
	if !s.Contains("personal") {
		t.Error("should contain 'personal'")
	}
	if s.Contains("unknown") {
		t.Error("should not contain 'unknown'")
	}
}

func TestORSetRemove(t *testing.T) {
	s := NewORSet()

	s.Add("work")
	s.Add("personal")

	s.Remove("work")

	if s.Contains("work") {
		t.Error("'work' should be removed")
	}
	if !s.Contains("personal") {
		t.Error("'personal' should still exist")
	}
}

func TestORSetAddAfterRemove(t *testing.T) {
	s := NewORSet()

	s.Add("work")
	s.Remove("work")
	s.Add("work") // Re-add with new token

	if !s.Contains("work") {
		t.Error("'work' should exist after re-add")
	}
}

func TestORSetConcurrentAddRemove(t *testing.T) {
	// Simulate: Device A adds "tag", Device B doesn't see it and also adds "tag"
	// Then Device A removes "tag" (only sees its own token)
	// After merge, B's add should still be visible

	a := NewORSet()
	b := NewORSet()

	a.Add("shared")
	b.Add("shared")

	// A removes (only sees its own token)
	a.Remove("shared")

	// Merge
	a.Merge(b)
	b.Merge(a)

	// B's token survived because A only removed A's tokens
	if !a.Contains("shared") {
		t.Error("'shared' should survive concurrent add/remove")
	}
	if !b.Contains("shared") {
		t.Error("'shared' should survive concurrent add/remove in b")
	}
}

func TestORSetMergeBasic(t *testing.T) {
	a := NewORSet()
	b := NewORSet()

	a.Add("tag1")
	b.Add("tag2")

	a.Merge(b)

	if !a.Contains("tag1") || !a.Contains("tag2") {
		t.Error("a should contain both tags after merge")
	}
}

func TestORSetMergeWithRemoves(t *testing.T) {
	a := NewORSet()
	b := NewORSet()

	// A adds and removes
	a.Add("tag")
	a.Remove("tag")

	// B adds the same tag
	b.Add("tag")

	// Merge: B's add should survive because A's remove only observed A's token
	a.Merge(b)

	if !a.Contains("tag") {
		t.Error("tag should survive merge because B's token wasn't removed")
	}
}

func TestORSetMergeCommutative(t *testing.T) {
	a := NewORSet()
	b := NewORSet()

	a.Add("tag1")
	a.Add("tag2")
	b.Add("tag2")
	b.Add("tag3")

	a1 := a.Clone()
	b1 := b.Clone()
	a2 := a.Clone()
	b2 := b.Clone()

	a1.Merge(b1)
	b2.Merge(a2)

	elems1 := a1.Elements()
	elems2 := b2.Elements()
	sort.Strings(elems1)
	sort.Strings(elems2)

	if len(elems1) != len(elems2) {
		t.Errorf("commutative: sizes differ %d vs %d", len(elems1), len(elems2))
	}

	for i := range elems1 {
		if elems1[i] != elems2[i] {
			t.Errorf("commutative: elements differ at %d: %s vs %s", i, elems1[i], elems2[i])
		}
	}
}

func TestORSetMergeAssociative(t *testing.T) {
	a := NewORSet()
	b := NewORSet()
	c := NewORSet()

	a.Add("tag1")
	b.Add("tag2")
	c.Add("tag3")

	// (A ⊔ B) ⊔ C
	left := a.Clone()
	left.Merge(b.Clone())
	left.Merge(c.Clone())

	// A ⊔ (B ⊔ C)
	bc := b.Clone()
	bc.Merge(c.Clone())
	right := a.Clone()
	right.Merge(bc)

	leftElems := left.Elements()
	rightElems := right.Elements()
	sort.Strings(leftElems)
	sort.Strings(rightElems)

	if len(leftElems) != len(rightElems) {
		t.Errorf("associative: sizes differ")
	}

	for i := range leftElems {
		if leftElems[i] != rightElems[i] {
			t.Error("associative: elements differ")
		}
	}
}

func TestORSetMergeIdempotent(t *testing.T) {
	a := NewORSet()

	a.Add("tag1")
	a.Add("tag2")
	a.Remove("tag1")

	before := a.Elements()
	a.Merge(a.Clone())
	after := a.Elements()

	sort.Strings(before)
	sort.Strings(after)

	if len(before) != len(after) {
		t.Error("idempotent: size changed")
	}

	for i := range before {
		if before[i] != after[i] {
			t.Error("idempotent: elements changed")
		}
	}
}

func TestORSetElements(t *testing.T) {
	s := NewORSet()

	s.Add("a")
	s.Add("b")
	s.Add("c")
	s.Remove("b")

	elems := s.Elements()
	if len(elems) != 2 {
		t.Errorf("expected 2 elements, got %d", len(elems))
	}

	sort.Strings(elems)
	if elems[0] != "a" || elems[1] != "c" {
		t.Errorf("expected [a, c], got %v", elems)
	}
}

func TestORSetSize(t *testing.T) {
	s := NewORSet()

	if s.Size() != 0 {
		t.Error("empty set should have size 0")
	}

	s.Add("tag1")
	s.Add("tag1") // Duplicate add
	s.Add("tag2")

	if s.Size() != 2 {
		t.Errorf("expected size 2, got %d", s.Size())
	}

	s.Remove("tag1")
	if s.Size() != 1 {
		t.Errorf("expected size 1 after remove, got %d", s.Size())
	}
}
