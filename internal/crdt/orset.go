package crdt

import (
	"github.com/google/uuid"
)

// TagToken represents a unique identifier for a tag addition.
// Each time a tag is added, it gets a unique token.
type TagToken struct {
	Tag   string    `json:"tag"`
	Token uuid.UUID `json:"token"` // Unique identifier for this add operation
}

// ORSet is an Observed-Remove Set for tags.
// It correctly handles concurrent add/remove operations.
// Each add creates a unique token; remove observes and removes all known tokens.
type ORSet struct {
	adds    map[TagToken]struct{} // Set of (tag, token) pairs that have been added
	removes map[TagToken]struct{} // Set of (tag, token) pairs that have been removed
}

// NewORSet creates a new empty OR-Set.
func NewORSet() *ORSet {
	return &ORSet{
		adds:    make(map[TagToken]struct{}),
		removes: make(map[TagToken]struct{}),
	}
}

// Add adds a tag to the set with a new unique token.
// Returns the token for reference.
func (s *ORSet) Add(tag string) uuid.UUID {
	token := uuid.New()
	s.AddWithToken(tag, token)
	return token
}

// AddWithToken adds a tag with a specific token (used during merge).
func (s *ORSet) AddWithToken(tag string, token uuid.UUID) {
	tt := TagToken{Tag: tag, Token: token}
	s.adds[tt] = struct{}{}
}

// Remove removes a tag by marking all observed tokens as removed.
// This only affects tokens that are currently in the adds set.
func (s *ORSet) Remove(tag string) {
	for tt := range s.adds {
		if tt.Tag == tag {
			s.removes[tt] = struct{}{}
		}
	}
}

// Contains checks if a tag is currently in the set.
// A tag is present if it has at least one add that hasn't been removed.
func (s *ORSet) Contains(tag string) bool {
	for tt := range s.adds {
		if tt.Tag == tag {
			if _, removed := s.removes[tt]; !removed {
				return true
			}
		}
	}
	return false
}

// Elements returns all tags currently in the set.
func (s *ORSet) Elements() []string {
	seen := make(map[string]struct{})
	for tt := range s.adds {
		if _, removed := s.removes[tt]; !removed {
			seen[tt.Tag] = struct{}{}
		}
	}

	result := make([]string, 0, len(seen))
	for tag := range seen {
		result = append(result, tag)
	}
	return result
}

// Merge merges another OR-Set into this one.
// - Adds are unioned
// - Removes are unioned
// This is commutative, associative, and idempotent.
func (s *ORSet) Merge(other *ORSet) {
	// Union adds
	for tt := range other.adds {
		s.adds[tt] = struct{}{}
	}
	// Union removes
	for tt := range other.removes {
		s.removes[tt] = struct{}{}
	}
}

// Clone creates a deep copy of the OR-Set.
func (s *ORSet) Clone() *ORSet {
	clone := NewORSet()
	for tt := range s.adds {
		clone.adds[tt] = struct{}{}
	}
	for tt := range s.removes {
		clone.removes[tt] = struct{}{}
	}
	return clone
}

// Size returns the number of unique tags currently in the set.
func (s *ORSet) Size() int {
	return len(s.Elements())
}

// AllAdds returns all add tokens (for serialization).
func (s *ORSet) AllAdds() []TagToken {
	result := make([]TagToken, 0, len(s.adds))
	for tt := range s.adds {
		result = append(result, tt)
	}
	return result
}

// AllRemoves returns all remove tokens (for serialization).
func (s *ORSet) AllRemoves() []TagToken {
	result := make([]TagToken, 0, len(s.removes))
	for tt := range s.removes {
		result = append(result, tt)
	}
	return result
}

// RemoveToken adds a token to the remove set (for delta apply).
func (s *ORSet) RemoveToken(token uuid.UUID) {
	for tt := range s.adds {
		if tt.Token == token {
			s.removes[tt] = struct{}{}
		}
	}
}

