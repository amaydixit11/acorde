// Package crdt provides conflict-free replicated data types for vaultd.
//
// This package implements state-based CRDTs:
// - LWWSet: Last-Writer-Wins Element Set for entries
// - ORSet: Observed-Remove Set for tags
// - Replica: State container for a vaultd replica
package crdt

import (
	"github.com/amaydixit11/vaultd/internal/core"
	"github.com/google/uuid"
)

// LWWElement represents a single element in the LWW-Set.
// It stores the entry along with metadata for conflict resolution.
type LWWElement struct {
	Entry     core.Entry
	Timestamp uint64 // Logical time for LWW comparison
	Deleted   bool   // Tombstone marker
}

// LWWSet is a Last-Writer-Wins Element Set for entries.
// When merging, the element with the highest timestamp wins.
// Deleted entries are kept as tombstones for proper CRDT semantics.
type LWWSet struct {
	elements map[uuid.UUID]LWWElement
}

// NewLWWSet creates a new empty LWW-Set.
func NewLWWSet() *LWWSet {
	return &LWWSet{
		elements: make(map[uuid.UUID]LWWElement),
	}
}

// Add adds or updates an entry in the set.
// If an entry with the same ID exists with a higher timestamp, this is a no-op.
func (s *LWWSet) Add(entry core.Entry) {
	existing, exists := s.elements[entry.ID]

	// Only update if new timestamp is higher, or equal timestamp with higher ID (tie-breaker)
	if !exists || entry.UpdatedAt > existing.Timestamp ||
		(entry.UpdatedAt == existing.Timestamp && entry.ID.String() > existing.Entry.ID.String()) {
		s.elements[entry.ID] = LWWElement{
			Entry:     entry.Clone(),
			Timestamp: entry.UpdatedAt,
			Deleted:   entry.Deleted,
		}
	}
}

// Remove marks an entry as deleted (tombstone) with the given timestamp.
// If the entry doesn't exist or has a higher timestamp, this is a no-op.
func (s *LWWSet) Remove(id uuid.UUID, timestamp uint64) {
	existing, exists := s.elements[id]

	// Only mark deleted if timestamp is higher
	if !exists {
		// Create tombstone for unknown entry
		s.elements[id] = LWWElement{
			Entry:     core.Entry{ID: id, Deleted: true, UpdatedAt: timestamp},
			Timestamp: timestamp,
			Deleted:   true,
		}
		return
	}

	if timestamp > existing.Timestamp ||
		(timestamp == existing.Timestamp && !existing.Deleted) {
		existing.Entry.Deleted = true
		existing.Entry.UpdatedAt = timestamp
		existing.Timestamp = timestamp
		existing.Deleted = true
		s.elements[id] = existing
	}
}

// Lookup returns an entry by ID if it exists and is not deleted.
func (s *LWWSet) Lookup(id uuid.UUID) (core.Entry, bool) {
	elem, exists := s.elements[id]
	if !exists || elem.Deleted {
		return core.Entry{}, false
	}
	return elem.Entry.Clone(), true
}

// LookupWithDeleted returns an entry by ID including deleted entries.
func (s *LWWSet) LookupWithDeleted(id uuid.UUID) (core.Entry, bool) {
	elem, exists := s.elements[id]
	if !exists {
		return core.Entry{}, false
	}
	return elem.Entry.Clone(), true
}

// Elements returns all non-deleted entries.
func (s *LWWSet) Elements() []core.Entry {
	result := make([]core.Entry, 0, len(s.elements))
	for _, elem := range s.elements {
		if !elem.Deleted {
			result = append(result, elem.Entry.Clone())
		}
	}
	return result
}

// AllElements returns all entries including deleted (tombstones).
func (s *LWWSet) AllElements() []LWWElement {
	result := make([]LWWElement, 0, len(s.elements))
	for _, elem := range s.elements {
		result = append(result, elem)
	}
	return result
}

// Merge merges another LWW-Set into this one.
// For each element, the one with the highest timestamp wins.
// This operation is:
// - Commutative: A.Merge(B) = B.Merge(A)
// - Associative: (A.Merge(B)).Merge(C) = A.Merge(B.Merge(C))
// - Idempotent: A.Merge(A) = A
func (s *LWWSet) Merge(other *LWWSet) {
	for id, otherElem := range other.elements {
		existing, exists := s.elements[id]

		if !exists {
			// New element, just add it
			s.elements[id] = LWWElement{
				Entry:     otherElem.Entry.Clone(),
				Timestamp: otherElem.Timestamp,
				Deleted:   otherElem.Deleted,
			}
			continue
		}

		// Compare timestamps - highest wins
		if otherElem.Timestamp > existing.Timestamp {
			s.elements[id] = LWWElement{
				Entry:     otherElem.Entry.Clone(),
				Timestamp: otherElem.Timestamp,
				Deleted:   otherElem.Deleted,
			}
		} else if otherElem.Timestamp == existing.Timestamp {
			// Tie-breaker: deleted wins, then higher ID wins
			if otherElem.Deleted && !existing.Deleted {
				s.elements[id] = LWWElement{
					Entry:     otherElem.Entry.Clone(),
					Timestamp: otherElem.Timestamp,
					Deleted:   otherElem.Deleted,
				}
			} else if !otherElem.Deleted && !existing.Deleted {
				// Both not deleted, use ID as tie-breaker
				if otherElem.Entry.ID.String() > existing.Entry.ID.String() {
					s.elements[id] = LWWElement{
						Entry:     otherElem.Entry.Clone(),
						Timestamp: otherElem.Timestamp,
						Deleted:   otherElem.Deleted,
					}
				}
			}
		}
		// If existing.Timestamp > otherElem.Timestamp, keep existing (no-op)
	}
}

// Clone creates a deep copy of the LWW-Set.
func (s *LWWSet) Clone() *LWWSet {
	clone := NewLWWSet()
	for id, elem := range s.elements {
		clone.elements[id] = LWWElement{
			Entry:     elem.Entry.Clone(),
			Timestamp: elem.Timestamp,
			Deleted:   elem.Deleted,
		}
	}
	return clone
}

// Size returns the total number of elements (including tombstones).
func (s *LWWSet) Size() int {
	return len(s.elements)
}

// ActiveSize returns the number of non-deleted elements.
func (s *LWWSet) ActiveSize() int {
	count := 0
	for _, elem := range s.elements {
		if !elem.Deleted {
			count++
		}
	}
	return count
}
