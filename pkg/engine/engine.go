// Package engine provides the public API for vaultd.
//
// This is the only package external applications should import.
// All internal implementation details are hidden behind this interface.
//
// Example usage:
//
//	e, err := engine.New(engine.Config{})
//	if err != nil {
//	    log.Fatal(err)
//	}
//	defer e.Close()
//
//	entry, err := e.AddEntry(engine.AddEntryInput{
//	    Type:    engine.Note,
//	    Content: []byte("Hello, vaultd!"),
//	    Tags:    []string{"demo"},
//	})
package engine

import (
	"time"

	impl "github.com/amaydixit11/vaultd/internal/engine"
	"github.com/amaydixit11/vaultd/pkg/crypto"
	"github.com/google/uuid"
)

// EntryType represents the category of an entry
type EntryType string

// Entry type constants
const (
	Note       EntryType = "note"
	Log        EntryType = "log"
	File       EntryType = "file"
	EventEntry EntryType = "event"
)

// IsValid checks if the entry type is valid
func (t EntryType) IsValid() bool {
	switch t {
	case Note, Log, File, EventEntry:
		return true
	default:
		return false
	}
}

// Entry is the canonical state unit in vaultd.
// Content is opaque to vaultd - it doesn't parse or interpret it.
// Tags is always a non-nil slice (may be empty).
type Entry struct {
	ID        uuid.UUID
	Type      EntryType
	Content   []byte   // Opaque to vaultd
	Tags      []string // Never nil, use []string{} for no tags
	CreatedAt uint64   // Logical time (Lamport)
	UpdatedAt uint64   // Logical time (Lamport)
	Deleted   bool     // Tombstone for CRDT
}

// AddEntryInput contains parameters for adding a new entry
type AddEntryInput struct {
	Type    EntryType
	Content []byte
	Tags    []string
}

// UpdateEntryInput contains parameters for updating an entry.
// Nil fields mean no change.
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
	Deleted bool // Include deleted entries
	Limit   int  // Max results (0 = no limit)
	Offset  int  // Skip first N results
}

// Engine is the main interface for vaultd.
// Products embed this interface to interact with vaultd.
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

	// Events - Subscribe to change notifications
	Subscribe() Subscription

	// Lifecycle
	Close() error
}

// Config contains configuration options for the engine
type Config struct {
	// DataDir is the directory for storing vault data.
	// If empty, defaults to ~/.vaultd
	DataDir string

	// InMemory creates a temporary in-memory database.
	// If true, DataDir is ignored.
	InMemory bool

	// EncryptionKey is the key for encrypting entry content.
	EncryptionKey *crypto.Key
}

// New creates a new vaultd Engine with the given configuration.
func New(cfg Config) (Engine, error) {
	internalEngine, err := impl.New(impl.Config{
		DataDir:       cfg.DataDir,
		InMemory:      cfg.InMemory,
		EncryptionKey: cfg.EncryptionKey,
	})
	if err != nil {
		return nil, err
	}
	return &engineWrapper{impl: internalEngine}, nil
}

// engineWrapper wraps internal engine and converts types
type engineWrapper struct {
	impl impl.Engine
}

func (w *engineWrapper) AddEntry(input AddEntryInput) (Entry, error) {
	entry, err := w.impl.AddEntry(impl.AddEntryInput{
		Type:    toInternalEntryType(input.Type),
		Content: input.Content,
		Tags:    input.Tags,
	})
	if err != nil {
		return Entry{}, err
	}
	return fromInternalEntry(entry), nil
}

func (w *engineWrapper) GetEntry(id uuid.UUID) (Entry, error) {
	entry, err := w.impl.GetEntry(id)
	if err != nil {
		return Entry{}, convertError(err)
	}
	return fromInternalEntry(entry), nil
}

func (w *engineWrapper) UpdateEntry(id uuid.UUID, input UpdateEntryInput) error {
	return convertError(w.impl.UpdateEntry(id, impl.UpdateEntryInput{
		Content: input.Content,
		Tags:    input.Tags,
	}))
}

func (w *engineWrapper) DeleteEntry(id uuid.UUID) error {
	return convertError(w.impl.DeleteEntry(id))
}

func (w *engineWrapper) ListEntries(filter ListFilter) ([]Entry, error) {
	var internalType *impl.EntryType
	if filter.Type != nil {
		t := toInternalEntryType(*filter.Type)
		internalType = &t
	}

	entries, err := w.impl.ListEntries(impl.ListFilter{
		Type:    internalType,
		Tag:     filter.Tag,
		Since:   filter.Since,
		Until:   filter.Until,
		Deleted: filter.Deleted,
		Limit:   filter.Limit,
		Offset:  filter.Offset,
	})
	if err != nil {
		return nil, err
	}

	result := make([]Entry, len(entries))
	for i, e := range entries {
		result[i] = fromInternalEntry(e)
	}
	return result, nil
}

func (w *engineWrapper) GetSyncPayload() ([]byte, error) {
	return w.impl.GetSyncPayload()
}

func (w *engineWrapper) ApplyRemotePayload(payload []byte) error {
	return w.impl.ApplyRemotePayload(payload)
}

func (w *engineWrapper) Close() error {
	return w.impl.Close()
}

// Subscribe returns a subscription for change events
func (w *engineWrapper) Subscribe() Subscription {
	internalSub := w.impl.Subscribe()
	return &subscriptionWrapper{impl: internalSub}
}

// Subscription wraps internal subscription
type Subscription interface {
	Events() <-chan Event
	Close()
}

type subscriptionWrapper struct {
	impl impl.Subscription
}

func (s *subscriptionWrapper) Events() <-chan Event {
	// We need to convert internal events to public events
	ch := make(chan Event, 100)
	go func() {
		for e := range s.impl.Events() {
			ch <- Event{
				Type:      EventType(e.Type),
				EntryID:   e.EntryID,
				EntryType: e.EntryType,
				Timestamp: e.Timestamp,
			}
		}
		close(ch)
	}()
	return ch
}

func (s *subscriptionWrapper) Close() {
	s.impl.Close()
}

// Event types
type EventType string

const (
	EventCreated EventType = "created"
	EventUpdated EventType = "updated"
	EventDeleted EventType = "deleted"
	EventSynced  EventType = "synced"
)

// Event represents a change notification
type Event struct {
	Type      EventType `json:"type"`
	EntryID   uuid.UUID `json:"entry_id"`
	EntryType string    `json:"entry_type,omitempty"`
	Timestamp time.Time `json:"timestamp"`
}

// Type conversion helpers
func toInternalEntryType(t EntryType) impl.EntryType {
	return impl.EntryType(t)
}

func fromInternalEntry(e impl.Entry) Entry {
	tags := e.Tags
	if tags == nil {
		tags = []string{}
	}
	return Entry{
		ID:        e.ID,
		Type:      EntryType(e.Type),
		Content:   e.Content,
		Tags:      tags,
		CreatedAt: e.CreatedAt,
		UpdatedAt: e.UpdatedAt,
		Deleted:   e.Deleted,
	}
}
