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
	return r.AddEntryWithID(uuid.New(), entryType, content, tags)
}

// AddEntryWithID adds a new entry with a specific ID.
func (r *Replica) AddEntryWithID(id uuid.UUID, entryType core.EntryType, content []byte, tags []string) core.Entry {
	timestamp := r.clock.Tick()

	entry := core.Entry{
		ID:        id,
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
		
		// CRDT Set Semantics for "Update":
		// Is it "Set these tags" (replace) or "Add these tags"?
		// User requirement says current implementation "removes all existing tags... even from other replicas".
		// This happens because we iterate tagSet.Elements() and Remove() them.
		// If we want "Replace" semantics in LWW/OR-Set, we indeed remove local view.
		// But Concurrent Adds should survive? 
		// In OR-Set, Remove(tag) adds a "tombstone" for the *specific tokens* we see.
		// If another replica added a tag with a different token *before* we merge, and we haven't seen it, we can't remove it.
		// If we HAVE seen it, we remove it.
		// So actually, clearing local view IS the correct way to implement "Replace" in OR-Set.
		// But maybe the user wants "Merge" semantics (Add/Remove specific tags)?
		// "Fix: Use Add-Wins semantics or make tags a full LWW-Map"
		
		// If the user wants to keep OTHER tags, they should probably do GET -> MODIFY -> UPDATE.
		// But if they say it's a bug, let's assume they expect "Add/Remove" delta or "Merge".
		// However, the input is just `[]string`.
		// Let's implement this as: Remove tags that are NOT in the new list, Add tags that ARE in the new list.
		// This preserves tags that are in both.
		// AND it respects concurrent adds?
		// If we blindly remove everything, we generate tombstones for everything we see.
		// If we only remove what's missing, we are safer.
		
		currentTags := make(map[string]struct{})
		for _, t := range tagSet.Elements() {
			currentTags[t] = struct{}{}
		}
		
		newTags := make(map[string]struct{})
		for _, t := range *updateTags {
			newTags[t] = struct{}{}
		}
		
		// Remove tags not in new list
		for t := range currentTags {
			if _, keep := newTags[t]; !keep {
				tagSet.Remove(t)
			}
		}
		
		// Add new tags
		for t := range newTags {
			if _, exists := currentTags[t]; !exists {
				tagSet.Add(t)
			}
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

// EntriesSince returns entries updated after the given timestamp.
// Used for delta sync.
func (r *Replica) EntriesSince(since uint64) []LWWElement {
	var result []LWWElement
	for _, elem := range r.entries.AllElements() {
		if elem.Timestamp > since {
			result = append(result, elem)
		}
	}
	return result
}

// DeltaState returns only changes since the given timestamp.
func (r *Replica) DeltaState(since uint64) DeltaReplicaState {
	entries := r.EntriesSince(since)
	
	// Collect tags for changed entries
	tags := make(map[uuid.UUID]TagSetState)
	for _, elem := range entries {
		if tagSet, ok := r.tags[elem.Entry.ID]; ok {
			tags[elem.Entry.ID] = TagSetState{
				Adds:    tagSet.AllAdds(),
				Removes: tagSet.AllRemoves(),
			}
		}
	}
	
	return DeltaReplicaState{
		Entries:   entries,
		Tags:      tags,
		ClockTime: r.clock.Now(),
		Since:     since,
	}
}

// DeltaReplicaState contains only entries changed since a given time.
type DeltaReplicaState struct {
	Entries   []LWWElement              `json:"entries"`
	Tags      map[uuid.UUID]TagSetState `json:"tags"`
	ClockTime uint64                    `json:"clock_time"`
	Since     uint64                    `json:"since"`
}

// ApplyDelta merges a delta state into this replica.
func (r *Replica) ApplyDelta(delta DeltaReplicaState) {
	// Apply entries
	for _, elem := range delta.Entries {
		r.entries.Add(elem.Entry)
	}
	
	// Apply tags
	for id, tagState := range delta.Tags {
		tagSet := r.getOrCreateTagSet(id)
		for _, token := range tagState.Adds {
			tagSet.AddWithToken(token.Tag, token.Token)
		}
		for _, token := range tagState.Removes {
			tagSet.RemoveToken(token.Token)
		}
	}
	
	// Update clock
	r.clock.Update(delta.ClockTime)
}

