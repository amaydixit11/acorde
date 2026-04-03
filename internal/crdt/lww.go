// Package crdt provides conflict-free replicated data types for acorde.
//
// This package implements state-based CRDTs:
// - LWWSet: Last-Writer-Wins Element Set for entries
// - ORSet: Observed-Remove Set for tags
// - Replica: State container for a acorde replica
package crdt

import (
	"sort"

	"github.com/amaydixit11/acorde/internal/core"
	"github.com/google/uuid"
)

// LWWElement represents a single element in the LWW-Set.
// It stores the entry along with metadata for conflict resolution.
type LWWElement struct {
	Entry     core.Entry `json:"entry"`
	Timestamp uint64     `json:"timestamp"` // Logical time for LWW comparison
	Deleted   bool       `json:"deleted"`   // Tombstone marker
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

// shouldUpdateEntry determines if new should replace existing (deterministic tie-breaker)
func shouldUpdateEntry(existing LWWElement, new core.Entry) bool {
	if new.UpdatedAt > existing.Timestamp {
		return true
	}
	if new.UpdatedAt < existing.Timestamp {
		return false
	}

	// Timestamps equal, use deterministic tie-breakers:
	// 1. Deleted wins over not deleted
	if new.Deleted && !existing.Deleted {
		return true
	}
	if !new.Deleted && existing.Deleted {
		return false
	}

	// 2. Higher CreatedAt wins
	if new.CreatedAt > existing.Entry.CreatedAt {
		return true
	}
	if new.CreatedAt < existing.Entry.CreatedAt {
		return false
	}

	// 3. Higher Type (string) wins
	if string(new.Type) > string(existing.Entry.Type) {
		return true
	}
	if string(new.Type) < string(existing.Entry.Type) {
		return false
	}

	// 4. Higher Content wins
	cmp := compareBytes(new.Content, existing.Entry.Content)
	if cmp > 0 {
		return true
	}
	if cmp < 0 {
		return false
	}

	// 5. Higher Tags win
	return compareTags(new.Tags, existing.Entry.Tags) > 0
}

// Add adds or updates an entry in the set.
func (s *LWWSet) Add(entry core.Entry) {
	existing, exists := s.elements[entry.ID]

	if !exists || shouldUpdateEntry(existing, entry) {
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

	// Sort for deterministic serialization
	sort.Slice(result, func(i, j int) bool {
		return result[i].Entry.ID.String() < result[j].Entry.ID.String()
	})
	return result
}

// Merge merges another LWW-Set into this one.
// This operation is commutative, associative, and idempotent.
func (s *LWWSet) Merge(other *LWWSet) {
	for id, otherElem := range other.elements {
		existing, exists := s.elements[id]

		if !exists || shouldUpdateEntry(existing, otherElem.Entry) {
			s.elements[id] = LWWElement{
				Entry:     otherElem.Entry.Clone(),
				Timestamp: otherElem.Timestamp,
				Deleted:   otherElem.Deleted,
			}
		}
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

// compareBytes returns 1 if a > b, -1 if a < b, 0 if equal
func compareBytes(a, b []byte) int {
	if len(a) != len(b) {
		if len(a) > len(b) {
			return 1
		}
		return -1
	}
	for i := 0; i < len(a); i++ {
		if a[i] > b[i] {
			return 1
		}
		if a[i] < b[i] {
			return -1
		}
	}
	return 0
}

// compareTags returns 1 if a > b, -1 if a < b, 0 if equal
// Simple string representation comparison
func compareTags(a, b []string) int {
	if len(a) != len(b) {
		if len(a) > len(b) {
			return 1
		}
		return -1
	}
	for i := 0; i < len(a); i++ {
		if a[i] > b[i] {
			return 1
		}
		if a[i] < b[i] {
			return -1
		}
	}
	return 0
}
