package crdt

import (
	"github.com/amaydixit11/vaultd/internal/core"
	"github.com/google/uuid"
)

// Replica represents a vaultd replica's CRDT state.
// It contains the LWW-Set for entries and OR-Sets for tags (one per entry).
type Replica struct {
	entries *LWWSet                // LWW-Set of all entries
	tags    map[uuid.UUID]*ORSet   // Entry ID → OR-Set of tags
	clock   *core.Clock            // Lamport clock for this replica
}

// NewReplica creates a new empty replica with the given clock.
func NewReplica(clock *core.Clock) *Replica {
	return &Replica{
		entries: NewLWWSet(),
		tags:    make(map[uuid.UUID]*ORSet),
		clock:   clock,
	}
}

// HydrateEntry loads an existing entry from storage into the CRDT.
// Used during startup to populate the replica from durable storage.
func (r *Replica) HydrateEntry(entry core.Entry) {
	// Add entry to LWW-Set
	r.entries.Add(entry)

	// Add tags to OR-Set (each tag gets a deterministic token based on entry ID)
	if len(entry.Tags) > 0 {
		tagSet := r.getOrCreateTagSet(entry.ID)
		for _, tag := range entry.Tags {
			// Use namespace UUID to generate deterministic token
			token := uuid.NewSHA1(entry.ID, []byte(tag))
			tagSet.AddWithToken(tag, token)
		}
	}
}

// AddEntry adds a new entry to the replica.
func (r *Replica) AddEntry(entryType core.EntryType, content []byte, tags []string) core.Entry {
	timestamp := r.clock.Tick()

	entry := core.Entry{
		ID:        uuid.New(),
		Type:      entryType,
		Content:   content,
		Tags:      []string{}, // Tags managed separately via OR-Set
		CreatedAt: timestamp,
		UpdatedAt: timestamp,
		Deleted:   false,
	}

	r.entries.Add(entry)

	// Add tags via OR-Set
	if len(tags) > 0 {
		tagSet := NewORSet()
		for _, tag := range tags {
			tagSet.Add(tag)
		}
		r.tags[entry.ID] = tagSet
	}

	return r.getEntryWithTags(entry.ID)
}

// UpdateEntry updates an existing entry's content and/or tags.
func (r *Replica) UpdateEntry(id uuid.UUID, content *[]byte, updateTags *[]string) error {
	existing, exists := r.entries.LookupWithDeleted(id)
	if !exists {
		return &ErrEntryNotFound{ID: id}
	}
	if existing.Deleted {
		return &ErrEntryDeleted{ID: id}
	}

	timestamp := r.clock.Tick()

	updated := existing.Clone()
	if content != nil {
		updated.Content = *content
	}
	updated.UpdatedAt = timestamp

	r.entries.Add(updated)

	// Update tags if provided
	if updateTags != nil {
		tagSet := r.getOrCreateTagSet(id)
		// Remove all existing tags
		for _, tag := range tagSet.Elements() {
			tagSet.Remove(tag)
		}
		// Add new tags
		for _, tag := range *updateTags {
			tagSet.Add(tag)
		}
	}

	return nil
}

// DeleteEntry marks an entry as deleted (tombstone).
func (r *Replica) DeleteEntry(id uuid.UUID) error {
	_, exists := r.entries.LookupWithDeleted(id)
	if !exists {
		return &ErrEntryNotFound{ID: id}
	}

	timestamp := r.clock.Tick()
	r.entries.Remove(id, timestamp)

	return nil
}

// GetEntry retrieves an entry by ID with its current tags.
func (r *Replica) GetEntry(id uuid.UUID) (core.Entry, error) {
	_, exists := r.entries.Lookup(id)
	if !exists {
		// Check if it exists but is deleted
		_, existsDeleted := r.entries.LookupWithDeleted(id)
		if existsDeleted {
			return core.Entry{}, &ErrEntryDeleted{ID: id}
		}
		return core.Entry{}, &ErrEntryNotFound{ID: id}
	}

	return r.getEntryWithTags(id), nil
}

// ListEntries returns all non-deleted entries with their tags.
func (r *Replica) ListEntries() []core.Entry {
	elements := r.entries.Elements()
	result := make([]core.Entry, len(elements))

	for i, entry := range elements {
		result[i] = r.getEntryWithTags(entry.ID)
	}

	return result
}

// Merge merges another replica's state into this one.
// Both entries (LWW-Set) and tags (OR-Sets) are merged.
//
// Tag Update Semantics:
// - Concurrent tag updates from different replicas will be merged
// - Both sets of tags will be present after merge (OR-Set behavior)
func (r *Replica) Merge(other *Replica) {
	// Update clock FIRST (before merging state)
	// This ensures causal consistency: any new operations after merge
	// will have timestamps higher than all merged entries
	otherMaxTime := other.MaxTimestamp()
	r.clock.Update(otherMaxTime)

	// Merge entries (LWW-Set)
	r.entries.Merge(other.entries)

	// Merge tags for all entries (OR-Sets)
	for id, otherTagSet := range other.tags {
		if localTagSet, exists := r.tags[id]; exists {
			localTagSet.Merge(otherTagSet)
		} else {
			r.tags[id] = otherTagSet.Clone()
		}
	}
}

// MaxTimestamp returns the highest timestamp in this replica.
func (r *Replica) MaxTimestamp() uint64 {
	var max uint64 = 0
	for _, elem := range r.entries.AllElements() {
		if elem.Timestamp > max {
			max = elem.Timestamp
		}
	}
	return max
}

// Clone creates a deep copy of the replica.
func (r *Replica) Clone() *Replica {
	clone := &Replica{
		entries: r.entries.Clone(),
		tags:    make(map[uuid.UUID]*ORSet),
		clock:   core.NewClockWithTime(r.clock.Now()),
	}

	for id, tagSet := range r.tags {
		clone.tags[id] = tagSet.Clone()
	}

	return clone
}

// State returns the current state for serialization/sync.
func (r *Replica) State() ReplicaState {
	return ReplicaState{
		Entries:      r.entries.AllElements(),
		Tags:         r.exportTags(),
		ClockTime:    r.clock.Now(),
	}
}

// LoadState loads state from a ReplicaState (for deserialization).
func (r *Replica) LoadState(state ReplicaState) {
	for _, elem := range state.Entries {
		r.entries.Add(elem.Entry)
		if elem.Deleted {
			r.entries.Remove(elem.Entry.ID, elem.Timestamp)
		}
	}

	for id, tagState := range state.Tags {
		tagSet := NewORSet()
		for _, tt := range tagState.Adds {
			tagSet.AddWithToken(tt.Tag, tt.Token)
		}
		for _, tt := range tagState.Removes {
			// Mark as removed (need to add first so it's in adds set)
			tagSet.adds[tt] = struct{}{}
			tagSet.removes[tt] = struct{}{}
		}
		r.tags[id] = tagSet
	}
}

// Helper methods

func (r *Replica) getOrCreateTagSet(id uuid.UUID) *ORSet {
	if tagSet, exists := r.tags[id]; exists {
		return tagSet
	}
	tagSet := NewORSet()
	r.tags[id] = tagSet
	return tagSet
}

func (r *Replica) getEntryWithTags(id uuid.UUID) core.Entry {
	entry, exists := r.entries.LookupWithDeleted(id)
	if !exists {
		return core.Entry{}
	}

	result := entry.Clone()
	if tagSet, exists := r.tags[id]; exists {
		result.Tags = tagSet.Elements()
	} else {
		result.Tags = []string{}
	}

	return result
}

func (r *Replica) exportTags() map[uuid.UUID]TagSetState {
	result := make(map[uuid.UUID]TagSetState)
	for id, tagSet := range r.tags {
		result[id] = TagSetState{
			Adds:    tagSet.AllAdds(),
			Removes: tagSet.AllRemoves(),
		}
	}
	return result
}

// ReplicaState represents the serializable state of a replica.
type ReplicaState struct {
	Entries   []LWWElement              `json:"entries"`
	Tags      map[uuid.UUID]TagSetState `json:"tags"`
	ClockTime uint64                    `json:"clock_time"`
}

// TagSetState represents the serializable state of an OR-Set.
type TagSetState struct {
	Adds    []TagToken `json:"adds"`
	Removes []TagToken `json:"removes"`
}

// Error types

type ErrEntryNotFound struct {
	ID uuid.UUID
}

func (e *ErrEntryNotFound) Error() string {
	return "entry not found: " + e.ID.String()
}

type ErrEntryDeleted struct {
	ID uuid.UUID
}

func (e *ErrEntryDeleted) Error() string {
	return "entry is deleted: " + e.ID.String()
}
