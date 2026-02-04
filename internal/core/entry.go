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
