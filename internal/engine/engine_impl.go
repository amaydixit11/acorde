package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/amaydixit11/acorde/internal/acl"
	"github.com/amaydixit11/acorde/internal/core"
	"github.com/amaydixit11/acorde/internal/hooks"
	"github.com/amaydixit11/acorde/internal/crdt"
	"github.com/amaydixit11/acorde/internal/schema"
	"github.com/amaydixit11/acorde/internal/version"
	"github.com/amaydixit11/acorde/pkg/crypto"
	"github.com/amaydixit11/acorde/internal/storage"
	"github.com/amaydixit11/acorde/internal/storage/sqlite"
	"github.com/google/uuid"
)


// Config contains configuration options for the engine
type Config struct {
	DataDir       string
	InMemory      bool
	EncryptionKey *crypto.Key // *crypto.Key or nil
	MaxVersions   int         // 0 = unlimited
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
	Owner     string    // PeerID of creator/owner
}

// Engine is the main interface for acorde
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

	// Events
	Subscribe() Subscription

	// Features
	RegisterSchema(entryType string, schemaJSON []byte) error
	
	// Accessors for new features
	Versions() *version.Store
	ACL() *acl.Store
	Hooks() *hooks.Manager

	// Lifecycle
	Close() error
}

// engineImpl is the concrete implementation of the Engine interface
// Replica is the source of truth, Storage is a materialized view

type engineImpl struct {
	replica  *crdt.Replica    // CRDT state (source of truth)
	store    storage.Store    // Persistent storage (materialized view)
	key      *crypto.Key      // Encryption key (nil = disabled)
	events   *EventBus        // Event subscriptions
	schemas  *schema.Registry // Schema validation
	versions *version.Store   // Version history
	acls     *acl.Store       // Access control
	hooks    *hooks.Manager   // Webhooks
	localID  string           // Local Peer ID
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
			dataDir = filepath.Join(home, ".acorde")
		}

		// Create data directory if it doesn't exist
		if err := os.MkdirAll(dataDir, 0700); err != nil {
			return nil, fmt.Errorf("failed to create data directory: %w", err)
		}

		dbPath = filepath.Join(dataDir, "acorde.db")
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

	var key *crypto.Key
	if cfg.EncryptionKey != nil {
		key = cfg.EncryptionKey
	}

	// Initialize Version Store
	versionStore, err := version.NewStore(store.GetDB(), cfg.MaxVersions)
	if err != nil {
		store.Close()
		return nil, fmt.Errorf("failed to create version store: %w", err)
	}

	// Initialize ACL Store
	// For now using random UUID as peerID if not provided (should be consistent in real app)
	localPeerID := uuid.New().String() 
	aclStore, err := acl.NewStore(store.GetDB(), localPeerID)
	if err != nil {
		store.Close()
		return nil, fmt.Errorf("failed to create acl store: %w", err)
	}

	return &engineImpl{
		replica:  replica,
		store:    store,
		key:      key,
		events:   NewEventBus(),
		schemas:  schema.NewRegistry(),
		versions: versionStore,
		acls:     aclStore,
		hooks:    hooks.NewManager(),
		localID:  localPeerID,
	}, nil
}

// AddEntry creates a new entry
func (e *engineImpl) AddEntry(input AddEntryInput) (Entry, error) {
	if !input.Type.IsValid() {
		return Entry{}, fmt.Errorf("invalid entry type: %s", input.Type)
	}

	// Validate against schema if registered
	result := e.schemas.Validate(string(input.Type), input.Content)
	if !result.Valid {
		return Entry{}, fmt.Errorf("schema validation failed: %v", result.Errors)
	}

	// Generate ID for AAD binding
	id := uuid.New()

	// Encrypt content if key is present
	content := input.Content
	if e.key != nil {
		aad := []byte(id.String()) // Bind ID to content
		encrypted, err := crypto.Encrypt(*e.key, content, aad)
		if err != nil {
			return Entry{}, fmt.Errorf("encryption failed: %w", err)
		}
		content = encrypted
	}

	// Add to CRDT Replica (source of truth)
	coreEntry := e.replica.AddEntryWithID(id, input.Type, content, input.Tags)

	// Persist to storage (materialized view)
	if err := e.store.Put(coreEntry); err != nil {
		return Entry{}, fmt.Errorf("failed to store entry: %w", err)
	}

	result2 := toInternalEntry(coreEntry)
	result2.Content = input.Content // Return plaintext to caller
	result2.Owner = e.localID       // Set owner

	// Set default ACL (Private, Owned by creator)
	e.acls.SetACL(acl.ACL{
		EntryID: result2.ID,
		Owner:   e.localID,
		Public:  false,
	})

	// Save initial version
	e.versions.SaveVersion(result2.ID, content, input.Tags, result2.CreatedAt, e.localID)

	// Emit event
	e.events.Publish(Event{
		Type:      EventCreated,
		EntryID:   result2.ID,
		EntryType: string(result2.Type),
		Timestamp: time.Now(),
	})

	// Trigger Webhooks
	e.hooks.TriggerAsync(hooks.NewCreateEvent(result2.ID, string(result2.Type), input.Content, input.Tags))

	return result2, nil
}

// GetEntry retrieves an entry by ID
func (e *engineImpl) GetEntry(id uuid.UUID) (Entry, error) {
	// Check read permission
	if allowed, _ := e.acls.CheckRead(id, e.localID); !allowed {
		return Entry{}, fmt.Errorf("permission denied")
	}

	coreEntry, err := e.replica.GetEntry(id)
	if err != nil {
		return Entry{}, convertCRDTError(err)
	}
	
	entry := toInternalEntry(coreEntry)
	if e.key != nil && len(entry.Content) > 0 {
		aad := []byte(id.String())
		plaintext, err := crypto.Decrypt(*e.key, entry.Content, aad)
		if err != nil {
			return Entry{}, fmt.Errorf("decryption failed: %w", err)
		}
		entry.Content = plaintext
	}

	// Populate Owner
	if acl, err := e.acls.GetACL(id); err == nil {
		entry.Owner = acl.Owner
	}

	return entry, nil
}

// UpdateEntry updates an existing entry
func (e *engineImpl) UpdateEntry(id uuid.UUID, input UpdateEntryInput) error {
	// Check write permission
	if allowed, _ := e.acls.CheckWrite(id, e.localID); !allowed {
		return fmt.Errorf("permission denied")
	}

	var content []byte
	var tags []string

	// Check if update is needed
	current, err := e.replica.GetEntry(id)
	if err != nil {
		return convertCRDTError(err)
	}

	if input.Content != nil {
		// Validate against schema if registered
		typeStr := string(toInternalEntry(current).Type)
		result := e.schemas.Validate(typeStr, *input.Content)
		if !result.Valid {
			return fmt.Errorf("schema validation failed: %v", result.Errors)
		}

		content = *input.Content
		if e.key != nil {
			aad := []byte(id.String())
			encrypted, err := crypto.Encrypt(*e.key, content, aad)
			if err != nil {
				return fmt.Errorf("encryption failed: %w", err)
			}
			content = encrypted
		}
	} else {
		content = current.Content
	}

	if input.Tags != nil {
		tags = *input.Tags
	} else {
		tags = current.Tags
	}

	// Update in CRDT Replica
	if err := e.replica.UpdateEntry(id, &content, &tags); err != nil {
		return convertCRDTError(err)
	}

	// Get updated entry and persist
	coreEntry, _ := e.replica.GetEntry(id)
	if err := e.store.Put(coreEntry); err != nil {
		return fmt.Errorf("failed to store updated entry: %w", err)
	}

	// Save new version
	e.versions.SaveVersion(id, content, tags, coreEntry.UpdatedAt, e.localID)

	// Emit event
	e.events.Publish(Event{
		Type:      EventUpdated,
		EntryID:   id,
		EntryType: string(toInternalEntry(coreEntry).Type),
		Timestamp: time.Now(),
	})

	// Trigger Webhooks
	e.hooks.TriggerAsync(hooks.NewUpdateEvent(id, string(toInternalEntry(coreEntry).Type), content, tags))

	return nil
}

// DeleteEntry marks an entry as deleted
func (e *engineImpl) DeleteEntry(id uuid.UUID) error {
	// Delete in CRDT Replica (creates tombstone)
	if err := e.replica.DeleteEntry(id); err != nil {
		return convertCRDTError(err)
	}

	// Persist tombstone
	if err := e.store.Delete(id); err != nil {
		return err
	}

	// Emit event
	e.events.Publish(Event{
		Type:      EventDeleted,
		EntryID:   id,
		Timestamp: time.Now(),
	})

	// Trigger Webhooks
	e.hooks.TriggerAsync(hooks.NewDeleteEvent(id))

	return nil
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
		internal := toInternalEntry(entry)
		if e.key != nil && len(internal.Content) > 0 {
			aad := []byte(internal.ID.String())
			plaintext, err := crypto.Decrypt(*e.key, internal.Content, aad)
			if err != nil {
				return nil, fmt.Errorf("decryption failed for entry %s: %w", internal.ID, err)
			}
			internal.Content = plaintext
		}
		
		// Populate Owner
		if acl, err := e.acls.GetACL(internal.ID); err == nil {
			internal.Owner = acl.Owner
		}
		
		result[i] = internal
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

// Subscribe returns a subscription for receiving change events
func (e *engineImpl) Subscribe() Subscription {
	return e.events.Subscribe()
}

// RegisterSchema registers a JSON schema for an entry type
func (e *engineImpl) RegisterSchema(entryType string, schemaJSON []byte) error {
	return e.schemas.RegisterFromJSON(entryType, entryType+"-schema", schemaJSON)
}

// Versions returns the version store
func (e *engineImpl) Versions() *version.Store {
	return e.versions
}

// ACL returns the ACL store
func (e *engineImpl) ACL() *acl.Store {
	return e.acls
}

// Hooks returns the hooks manager
func (e *engineImpl) Hooks() *hooks.Manager {
	return e.hooks
}
