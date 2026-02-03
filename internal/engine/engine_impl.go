package engine

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/amaydixit11/vaultd/internal/core"
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
type engineImpl struct {
	clock *core.Clock
	store storage.Store
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

	// Restore clock from storage
	maxTime, err := store.GetMaxTimestamp()
	if err != nil {
		store.Close()
		return nil, fmt.Errorf("failed to get max timestamp: %w", err)
	}

	clock := core.NewClockWithTime(maxTime)

	return &engineImpl{
		clock: clock,
		store: store,
	}, nil
}

// AddEntry creates a new entry
func (e *engineImpl) AddEntry(input AddEntryInput) (Entry, error) {
	if !input.Type.IsValid() {
		return Entry{}, fmt.Errorf("invalid entry type: %s", input.Type)
	}

	clockTime := e.clock.Tick()
	entry := core.NewEntry(input.Type, input.Content, input.Tags, clockTime)

	if err := e.store.Put(entry); err != nil {
		return Entry{}, fmt.Errorf("failed to store entry: %w", err)
	}

	return toInternalEntry(entry), nil
}

// GetEntry retrieves an entry by ID
func (e *engineImpl) GetEntry(id uuid.UUID) (Entry, error) {
	entry, err := e.store.Get(id)
	if err != nil {
		return Entry{}, err
	}
	return toInternalEntry(entry), nil
}

// UpdateEntry updates an existing entry
func (e *engineImpl) UpdateEntry(id uuid.UUID, input UpdateEntryInput) error {
	entry, err := e.store.Get(id)
	if err != nil {
		return err
	}

	if entry.Deleted {
		return fmt.Errorf("cannot update deleted entry")
	}

	if input.Content != nil {
		entry.Content = *input.Content
	}
	if input.Tags != nil {
		entry.Tags = *input.Tags
	}

	entry.UpdatedAt = e.clock.Tick()

	return e.store.Put(entry)
}

// DeleteEntry marks an entry as deleted
func (e *engineImpl) DeleteEntry(id uuid.UUID) error {
	entry, err := e.store.Get(id)
	if err != nil {
		return err
	}

	entry.Deleted = true
	entry.UpdatedAt = e.clock.Tick()

	return e.store.Put(entry)
}

// ListEntries returns entries matching the filter
func (e *engineImpl) ListEntries(filter ListFilter) ([]Entry, error) {
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

// GetSyncPayload returns the current state for synchronization
// This is a placeholder for Phase 3 implementation
func (e *engineImpl) GetSyncPayload() ([]byte, error) {
	return nil, fmt.Errorf("sync not implemented yet")
}

// ApplyRemotePayload applies remote state and merges
// This is a placeholder for Phase 3 implementation
func (e *engineImpl) ApplyRemotePayload(payload []byte) error {
	return fmt.Errorf("sync not implemented yet")
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
