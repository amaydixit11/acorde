package storage

import (
	"github.com/amaydixit11/acorde/internal/core"
	"github.com/google/uuid"
)

// ListFilter specifies criteria for filtering entries
type ListFilter struct {
	Type      *core.EntryType // Filter by entry type
	Tag       *string         // Filter by tag
	Since     *uint64         // Entries updated after this time
	Until     *uint64         // Entries updated before this time
	Deleted   bool            // Include deleted entries
	Limit     int             // Max number of results (0 = no limit)
	Offset    int             // Skip first N results
}

// OperationType represents the type of batch operation
type OperationType int

const (
	OpPut OperationType = iota
	OpDelete
)

// Operation represents a single batch operation
type Operation struct {
	Type  OperationType
	Entry core.Entry
}

// Store defines the storage interface for vaultd
// Storage is an optimization layer, not the source of truth - CRDTs are
type Store interface {
	// Put stores an entry with its tags
	// This operation must be idempotent
	Put(entry core.Entry) error
	
	// Get retrieves an entry by ID
	// Returns error if entry not found
	Get(id uuid.UUID) (core.Entry, error)
	
	// List returns entries matching the filter
	List(filter ListFilter) ([]core.Entry, error)
	
	// Delete marks an entry as deleted (tombstone)
	// This is a logical delete for CRDT purposes
	Delete(id uuid.UUID) error
	
	// ApplyBatch applies multiple operations atomically
	ApplyBatch(ops []Operation) error
	
	// GetMaxTimestamp returns the highest UpdatedAt timestamp in storage
	// Used for clock recovery after restart
	GetMaxTimestamp() (uint64, error)
	
	// Close releases all resources
	Close() error
}

// ErrNotFound is returned when an entry is not found
type ErrNotFound struct {
	ID uuid.UUID
}

func (e ErrNotFound) Error() string {
	return "entry not found: " + e.ID.String()
}
