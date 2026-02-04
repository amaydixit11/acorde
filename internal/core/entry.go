package core

import (
	"github.com/google/uuid"
)

// EntryType represents the category of an entry
type EntryType string

const (
	Note  EntryType = "note"
	Log   EntryType = "log"
	File  EntryType = "file"
	Event EntryType = "event"
)

// ValidEntryTypes contains all valid entry types for validation
var ValidEntryTypes = map[EntryType]bool{
	Note:  true,
	Log:   true,
	File:  true,
	Event: true,
}

// IsValid checks if the entry type is valid
func (t EntryType) IsValid() bool {
	return ValidEntryTypes[t]
}

// Entry is the canonical state unit in acorde
// Content is opaque to acorde - it doesn't parse or interpret it
type Entry struct {
	ID        uuid.UUID `json:"id"`
	Type      EntryType `json:"type"`
	Content   []byte    `json:"content"` // Opaque to acorde
	Tags      []string  `json:"tags"`
	CreatedAt uint64    `json:"created_at"` // Logical time (Lamport)
	UpdatedAt uint64    `json:"updated_at"` // Logical time (Lamport)
	Deleted   bool      `json:"deleted"`    // Tombstone for CRDT
}

// NewEntry creates a new entry with the given parameters
// clockTime should be obtained from the Lamport clock
func NewEntry(entryType EntryType, content []byte, tags []string, clockTime uint64) Entry {
	if tags == nil {
		tags = []string{}
	}
	
	return Entry{
		ID:        uuid.New(),
		Type:      entryType,
		Content:   content,
		Tags:      tags,
		CreatedAt: clockTime,
		UpdatedAt: clockTime,
		Deleted:   false,
	}
}

// Clone creates a deep copy of the entry
func (e Entry) Clone() Entry {
	contentCopy := make([]byte, len(e.Content))
	copy(contentCopy, e.Content)
	
	tagsCopy := make([]string, len(e.Tags))
	copy(tagsCopy, e.Tags)
	
	return Entry{
		ID:        e.ID,
		Type:      e.Type,
		Content:   contentCopy,
		Tags:      tagsCopy,
		CreatedAt: e.CreatedAt,
		UpdatedAt: e.UpdatedAt,
		Deleted:   e.Deleted,
	}
}

// ACL represents access control for an entry
type ACL struct {
	EntryID   uuid.UUID `json:"entry_id"`
	Owner     string    `json:"owner"`              // PeerID of owner
	Readers   []string  `json:"readers,omitempty"`  // PeerIDs with read access
	Writers   []string  `json:"writers,omitempty"`  // PeerIDs with write access
	Public    bool      `json:"public"`              // Anyone can read
	Timestamp uint64    `json:"timestamp"`          // Logical time for LWW resolution
}

// Clone creates a deep copy of the ACL
func (a ACL) Clone() ACL {
	readers := make([]string, len(a.Readers))
	copy(readers, a.Readers)
	writers := make([]string, len(a.Writers))
	copy(writers, a.Writers)

	return ACL{
		EntryID:   a.EntryID,
		Owner:     a.Owner,
		Readers:   readers,
		Writers:   writers,
		Public:    a.Public,
		Timestamp: a.Timestamp,
	}
}
