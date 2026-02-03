package core

import (
	"testing"

	"github.com/google/uuid"
)

func TestEntryTypeIsValid(t *testing.T) {
	tests := []struct {
		entryType EntryType
		valid     bool
	}{
		{Note, true},
		{Log, true},
		{File, true},
		{Event, true},
		{EntryType("invalid"), false},
		{EntryType(""), false},
	}
	
	for _, tt := range tests {
		t.Run(string(tt.entryType), func(t *testing.T) {
			if tt.entryType.IsValid() != tt.valid {
				t.Errorf("expected %s.IsValid() to be %v", tt.entryType, tt.valid)
			}
		})
	}
}

func TestNewEntry(t *testing.T) {
	content := []byte("test content")
	tags := []string{"work", "important"}
	clockTime := uint64(42)
	
	entry := NewEntry(Note, content, tags, clockTime)
	
	if entry.ID == uuid.Nil {
		t.Error("expected entry to have a valid UUID")
	}
	if entry.Type != Note {
		t.Errorf("expected type Note, got %s", entry.Type)
	}
	if string(entry.Content) != string(content) {
		t.Errorf("expected content %s, got %s", content, entry.Content)
	}
	if len(entry.Tags) != 2 {
		t.Errorf("expected 2 tags, got %d", len(entry.Tags))
	}
	if entry.CreatedAt != clockTime {
		t.Errorf("expected CreatedAt %d, got %d", clockTime, entry.CreatedAt)
	}
	if entry.UpdatedAt != clockTime {
		t.Errorf("expected UpdatedAt %d, got %d", clockTime, entry.UpdatedAt)
	}
	if entry.Deleted {
		t.Error("expected entry to not be deleted")
	}
}

func TestNewEntryNilTags(t *testing.T) {
	entry := NewEntry(Log, []byte("test"), nil, 1)
	
	if entry.Tags == nil {
		t.Error("expected tags to be initialized to empty slice, got nil")
	}
	if len(entry.Tags) != 0 {
		t.Errorf("expected 0 tags, got %d", len(entry.Tags))
	}
}

func TestEntryClone(t *testing.T) {
	original := NewEntry(Note, []byte("original"), []string{"tag1"}, 1)
	cloned := original.Clone()
	
	// Check values are equal
	if original.ID != cloned.ID {
		t.Error("cloned ID should match original")
	}
	if string(original.Content) != string(cloned.Content) {
		t.Error("cloned content should match original")
	}
	
	// Modify clone and verify original is unchanged
	cloned.Content[0] = 'X'
	cloned.Tags[0] = "modified"
	
	if original.Content[0] == 'X' {
		t.Error("modifying clone should not affect original content")
	}
	if original.Tags[0] == "modified" {
		t.Error("modifying clone should not affect original tags")
	}
}
