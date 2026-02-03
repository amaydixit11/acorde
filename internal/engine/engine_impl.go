package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"

	"github.com/amaydixit11/vaultd/internal/core"
	"github.com/amaydixit11/vaultd/internal/crdt"
	"github.com/amaydixit11/vaultd/internal/storage"
	"github.com/amaydixit11/vaultd/internal/storage/sqlite"
	"github.com/google/uuid"
)

// Config contains configuration options for the engine
type Config struct {
	DataDir  string
	InMemory bool
}

// EntryType is re-exported from core for use by pkg/engine wrapper
type EntryType = core.EntryType

// AddEntryInput contains parameters for adding a new entry
type AddEntryInput struct {
	Type    EntryType
	Content []byte
	Tags    []string
}

// UpdateEntryInput contains parameters for updating an entry
type UpdateEntryInput struct {
	Content *[]byte   // nil means no change
	Tags    *[]string // nil means no change
}

// ListFilter specifies criteria for filtering entries
type ListFilter struct {
	Type    *EntryType
	Tag     *string
	Since   *uint64
	Until   *uint64
	Deleted bool
	Limit   int
	Offset  int
}

// Entry is the internal entry type
type Entry struct {
	ID        uuid.UUID
	Type      EntryType
	Content   []byte
	Tags      []string
	CreatedAt uint64
	UpdatedAt uint64
	Deleted   bool
}

// Engine is the main interface for vaultd
type Engine interface {
	// Entry lifecycle
	AddEntry(input AddEntryInput) (Entry, error)
	GetEntry(id uuid.UUID) (Entry, error)
	UpdateEntry(id uuid.UUID, input UpdateEntryInput) error
	DeleteEntry(id uuid.UUID) error

	// Querying
	ListEntries(filter ListFilter) ([]Entry, error)

	// Sync hooks (called by transport layer)
	GetSyncPayload() ([]byte, error)
	ApplyRemotePayload(payload []byte) error

	// Lifecycle
	Close() error
}

// engineImpl is the concrete implementation of the Engine interface
// Replica is the source of truth, Storage is a materialized view
type engineImpl struct {
	replica *crdt.Replica // CRDT state (source of truth)
	store   storage.Store // Persistent storage (materialized view)
}

// New creates a new engine instance
func New(cfg Config) (Engine, error) {
	var dbPath string

	if cfg.InMemory {
		dbPath = ":memory:"
	} else {
		dataDir := cfg.DataDir
		if dataDir == "" {
			home, err := os.UserHomeDir()
			if err != nil {
				return nil, fmt.Errorf("failed to get home directory: %w", err)
			}
			dataDir = filepath.Join(home, ".vaultd")
		}

		// Create data directory if it doesn't exist
		if err := os.MkdirAll(dataDir, 0700); err != nil {
			return nil, fmt.Errorf("failed to create data directory: %w", err)
		}

		dbPath = filepath.Join(dataDir, "vault.db")
	}

	store, err := sqlite.New(dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to create storage: %w", err)
	}

	// Get max timestamp from storage for clock recovery
	maxTime, err := store.GetMaxTimestamp()
	if err != nil {
		store.Close()
		return nil, fmt.Errorf("failed to get max timestamp: %w", err)
	}

	// Create CRDT Replica with recovered clock
	clock := core.NewClockWithTime(maxTime)
	replica := crdt.NewReplica(clock)

	// Hydrate replica from storage (load existing entries into CRDT)
	entries, err := store.List(storage.ListFilter{Deleted: true})
	if err != nil {
		store.Close()
		return nil, fmt.Errorf("failed to load entries: %w", err)
	}
	for _, entry := range entries {
		replica.HydrateEntry(entry)
	}

	return &engineImpl{
		replica: replica,
		store:   store,
	}, nil
}

// AddEntry creates a new entry
func (e *engineImpl) AddEntry(input AddEntryInput) (Entry, error) {
	if !input.Type.IsValid() {
		return Entry{}, fmt.Errorf("invalid entry type: %s", input.Type)
	}

	// Add to CRDT Replica (source of truth)
	coreEntry := e.replica.AddEntry(input.Type, input.Content, input.Tags)

	// Persist to storage (materialized view)
	if err := e.store.Put(coreEntry); err != nil {
		return Entry{}, fmt.Errorf("failed to store entry: %w", err)
	}

	return toInternalEntry(coreEntry), nil
}

// GetEntry retrieves an entry by ID
func (e *engineImpl) GetEntry(id uuid.UUID) (Entry, error) {
	coreEntry, err := e.replica.GetEntry(id)
	if err != nil {
		return Entry{}, convertCRDTError(err)
	}
	return toInternalEntry(coreEntry), nil
}

// UpdateEntry updates an existing entry
func (e *engineImpl) UpdateEntry(id uuid.UUID, input UpdateEntryInput) error {
	// Update in CRDT Replica
	if err := e.replica.UpdateEntry(id, input.Content, input.Tags); err != nil {
		return convertCRDTError(err)
	}

	// Get updated entry and persist
	coreEntry, _ := e.replica.GetEntry(id)
	return e.store.Put(coreEntry)
}

// DeleteEntry marks an entry as deleted
func (e *engineImpl) DeleteEntry(id uuid.UUID) error {
	// Delete in CRDT Replica (creates tombstone)
	if err := e.replica.DeleteEntry(id); err != nil {
		return convertCRDTError(err)
	}

	// Persist tombstone
	return e.store.Delete(id)
}

// ListEntries returns entries matching the filter
func (e *engineImpl) ListEntries(filter ListFilter) ([]Entry, error) {
	// List from storage (it's the indexed/filtered view)
	storeFilter := storage.ListFilter{
		Type:    filter.Type,
		Tag:     filter.Tag,
		Since:   filter.Since,
		Until:   filter.Until,
		Deleted: filter.Deleted,
		Limit:   filter.Limit,
		Offset:  filter.Offset,
	}

	entries, err := e.store.List(storeFilter)
	if err != nil {
		return nil, err
	}

	result := make([]Entry, len(entries))
	for i, entry := range entries {
		result[i] = toInternalEntry(entry)
	}
	return result, nil
}

// GetSyncPayload returns the current CRDT state for synchronization
func (e *engineImpl) GetSyncPayload() ([]byte, error) {
	state := e.replica.State()
	return json.Marshal(state)
}

// ApplyRemotePayload applies remote CRDT state and merges
func (e *engineImpl) ApplyRemotePayload(payload []byte) error {
	var state crdt.ReplicaState
	if err := json.Unmarshal(payload, &state); err != nil {
		return fmt.Errorf("failed to unmarshal payload: %w", err)
	}

	// Create temporary replica with received state
	tempClock := core.NewClockWithTime(state.ClockTime)
	tempReplica := crdt.NewReplica(tempClock)
	tempReplica.LoadState(state)

	// Merge into our replica
	e.replica.Merge(tempReplica)

	// Persist merged state to storage
	for _, entry := range e.replica.ListEntries() {
		if err := e.store.Put(entry); err != nil {
			return fmt.Errorf("failed to persist merged entry: %w", err)
		}
	}

	return nil
}

// GetSyncState returns the current CRDT state (implements sync.Syncable)
func (e *engineImpl) GetSyncState() crdt.ReplicaState {
	return e.replica.State()
}

// ApplySyncState applies remote CRDT state and merges (implements sync.Syncable)
func (e *engineImpl) ApplySyncState(state crdt.ReplicaState) error {
	// Create temporary replica with received state
	tempClock := core.NewClockWithTime(state.ClockTime)
	tempReplica := crdt.NewReplica(tempClock)
	tempReplica.LoadState(state)

	// Merge into our replica
	e.replica.Merge(tempReplica)

	// Persist merged state to storage
	for _, entry := range e.replica.ListEntries() {
		if err := e.store.Put(entry); err != nil {
			return fmt.Errorf("failed to persist merged entry: %w", err)
		}
	}

	return nil
}

// Close releases all resources
func (e *engineImpl) Close() error {
	return e.store.Close()
}

// toInternalEntry converts a core.Entry to internal Entry
func toInternalEntry(e core.Entry) Entry {
	tags := e.Tags
	if tags == nil {
		tags = []string{}
	}
	return Entry{
		ID:        e.ID,
		Type:      e.Type,
		Content:   e.Content,
		Tags:      tags,
		CreatedAt: e.CreatedAt,
		UpdatedAt: e.UpdatedAt,
		Deleted:   e.Deleted,
	}
}

// convertCRDTError converts crdt errors to storage errors for consistency
func convertCRDTError(err error) error {
	if err == nil {
		return nil
	}
	if notFound, ok := err.(*crdt.ErrEntryNotFound); ok {
		return storage.ErrNotFound{ID: notFound.ID}
	}
	return err
}
