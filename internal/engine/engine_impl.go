package engine

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/amaydixit11/acorde/internal/acl"
	"github.com/amaydixit11/acorde/internal/core"
	"github.com/amaydixit11/acorde/internal/crdt"
	"github.com/amaydixit11/acorde/internal/hooks"
	"github.com/amaydixit11/acorde/internal/schema"
	"github.com/amaydixit11/acorde/internal/storage"
	"github.com/amaydixit11/acorde/internal/storage/sqlite"
	"github.com/amaydixit11/acorde/internal/version"
	"github.com/amaydixit11/acorde/pkg/crypto"
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
	Public  bool
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
	Owner     string // PeerID of creator/owner
}

// Engine is the main interface for acorde
type Engine interface {
	// Entry lifecycle
	AddEntry(input AddEntryInput) (Entry, error)
	GetEntry(id uuid.UUID) (Entry, error)
	UpdateEntry(id uuid.UUID, input UpdateEntryInput) error
	DeleteEntry(id uuid.UUID) error
	GrantWrite(id uuid.UUID, peerID string) error

	// Querying
	ListEntries(filter ListFilter) ([]Entry, error)

	// Sync hooks (called by transport layer)
	GetSyncPayload() ([]byte, error)
	ApplyRemotePayload(payload []byte) error
	ApplyRemotePayloadFromPeer(payload []byte, senderPeerID string) error
	ApplySyncState(state crdt.ReplicaState, senderPeerID string) error

	// Events
	Subscribe() Subscription

	// Features
	RegisterSchema(entryType string, schemaJSON []byte) error

	// Accessors for new features
	Versions() *version.Store
	ACL() *acl.Store
	Hooks() *hooks.Manager
	PeerID() string

	// Lifecycle
	Close() error
}

// engineImpl is the concrete implementation of the Engine interface
// Replica is the source of truth, Storage is a materialized view

type engineImpl struct {
	mu       sync.Mutex
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

func (e *engineImpl) reconcileLocalState() error {
	entries, err := e.store.List(storage.ListFilter{Deleted: true})
	if err != nil {
		return fmt.Errorf("failed to load entries from storage: %w", err)
	}
	for _, entry := range entries {
		e.replica.HydrateEntry(entry)
	}

	acls, err := e.acls.List()
	if err != nil {
		return fmt.Errorf("failed to load ACLs from storage: %w", err)
	}
	for _, acl := range acls {
		e.replica.SetACL(acl)
	}

	return nil
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

	// Initialize ACL Store
	var localPeerID string
	if cfg.InMemory {
		// In-memory engines should not share process cwd state via ./node_id.
		localPeerID = uuid.New().String()
	} else {
		nodeIDPath := filepath.Join(filepath.Dir(dbPath), "node_id")
		if idBytes, err := os.ReadFile(nodeIDPath); err == nil {
			localPeerID = string(idBytes)
		} else {
			localPeerID = uuid.New().String()
			if err := os.WriteFile(nodeIDPath, []byte(localPeerID), 0644); err != nil {
				store.Close()
				return nil, fmt.Errorf("failed to persist node id: %w", err)
			}
		}
	}

	aclStore, err := acl.NewStore(store.GetDB(), localPeerID)
	if err != nil {
		store.Close()
		return nil, fmt.Errorf("failed to create acl store: %w", err)
	}

	// Hydrate ACLs
	acls, err := aclStore.List()
	if err != nil {
		store.Close()
		return nil, fmt.Errorf("failed to load ACLs: %w", err)
	}
	for _, a := range acls {
		replica.SetACL(a)
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
	e.mu.Lock()
	defer e.mu.Unlock()

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
	defaultACL := core.ACL{
		EntryID:   result2.ID,
		Owner:     e.localID,
		Public:    input.Public,
		Timestamp: result2.CreatedAt,
	}
	e.acls.SetACL(defaultACL)
	e.replica.SetACL(defaultACL) // Update Sync Replica

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
	e.mu.Lock()
	defer e.mu.Unlock()

	if err := e.reconcileLocalState(); err != nil {
		return Entry{}, err
	}

	// Check read permission
	if allowed, _ := e.acls.CheckRead(id, e.localID); !allowed {
		return Entry{}, acl.ErrAccessDenied{EntryID: id, PeerID: e.localID, Action: "read"}
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
	e.mu.Lock()
	defer e.mu.Unlock()

	if err := e.reconcileLocalState(); err != nil {
		return err
	}

	// Check write permission
	if allowed, _ := e.acls.CheckWrite(id, e.localID); !allowed {
		return acl.ErrAccessDenied{EntryID: id, PeerID: e.localID, Action: "write"}
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
	e.mu.Lock()
	defer e.mu.Unlock()

	if err := e.reconcileLocalState(); err != nil {
		return err
	}

	if allowed, _ := e.acls.CheckWrite(id, e.localID); !allowed {
		return acl.ErrAccessDenied{EntryID: id, PeerID: e.localID, Action: "delete"}
	}

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

// GrantWrite authorizes a peer to write to an entry
func (e *engineImpl) GrantWrite(id uuid.UUID, peerID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if err := e.reconcileLocalState(); err != nil {
		return err
	}

	// 1. Check admin permission (only owner can grant)
	if allowed, _ := e.acls.CheckAdmin(id, e.localID); !allowed {
		return acl.ErrAccessDenied{EntryID: id, PeerID: e.localID, Action: "grant write access"}
	}

	// 2. Update ACL store with a fresh timestamp
	aclState, err := e.acls.GetACL(id)
	if err != nil {
		return err
	}
	for _, writer := range aclState.Writers {
		if writer == peerID {
			return nil
		}
	}
	aclState.Writers = append(aclState.Writers, peerID)
	aclState.Timestamp = e.replica.Clock().Tick()
	if err := e.acls.SetACL(*aclState); err != nil {
		return err
	}

	// 3. Update Sync Replica to ensure propagation
	e.replica.SetACL(*aclState)

	return nil
}

// ListEntries returns entries matching the filter
func (e *engineImpl) ListEntries(filter ListFilter) ([]Entry, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if err := e.reconcileLocalState(); err != nil {
		return nil, err
	}

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

	result := make([]Entry, 0, len(entries))
	for _, entry := range entries {
		if allowed, _ := e.acls.CheckRead(entry.ID, e.localID); !allowed {
			continue
		}

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

		result = append(result, internal)
	}
	return result, nil
}

// GetSyncPayload returns the current CRDT state for synchronization
func (e *engineImpl) GetSyncPayload() ([]byte, error) {
	e.mu.Lock()
	defer e.mu.Unlock()

	if err := e.reconcileLocalState(); err != nil {
		return nil, err
	}

	state := e.replica.State()
	return json.Marshal(state)
}

// ApplyRemotePayload applies remote CRDT state and merges
func (e *engineImpl) ApplyRemotePayload(payload []byte) error {
	return e.ApplyRemotePayloadFromPeer(payload, "")
}

func (e *engineImpl) ApplyRemotePayloadFromPeer(payload []byte, senderPeerID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	var state crdt.ReplicaState
	if err := json.Unmarshal(payload, &state); err != nil {
		return fmt.Errorf("failed to unmarshal payload: %w", err)
	}
	return e.applyReplicaState(state, senderPeerID, senderPeerID != "")
}

// GetSyncState returns the current CRDT state (implements sync.Syncable)
func (e *engineImpl) GetSyncState() crdt.ReplicaState {
	e.mu.Lock()
	defer e.mu.Unlock()

	_ = e.reconcileLocalState()
	return e.replica.State()
}

// ApplySyncState applies remote CRDT state and merges (implements sync.Syncable)
func (e *engineImpl) ApplySyncState(state crdt.ReplicaState, senderPeerID string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	return e.applyReplicaState(state, senderPeerID, true)
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

func (e *engineImpl) PeerID() string {
	return e.localID
}

// Hooks returns the hooks manager
func (e *engineImpl) Hooks() *hooks.Manager {
	return e.hooks
}

func (e *engineImpl) applyReplicaState(state crdt.ReplicaState, senderPeerID string, enforceAuth bool) error {
	filtered := crdt.ReplicaState{
		Entries:   make([]crdt.LWWElement, 0, len(state.Entries)),
		Tags:      make(map[uuid.UUID]crdt.TagSetState),
		ACLs:      make(map[uuid.UUID]core.ACL),
		ClockTime: state.ClockTime,
	}

	acceptedEntries := make(map[uuid.UUID]struct{})

	for _, elem := range state.Entries {
		if e.allowRemoteEntry(elem, state.ACLs[elem.Entry.ID], senderPeerID, enforceAuth) {
			filtered.Entries = append(filtered.Entries, elem)
			if tags, ok := state.Tags[elem.Entry.ID]; ok {
				filtered.Tags[elem.Entry.ID] = tags
			}
			acceptedEntries[elem.Entry.ID] = struct{}{}
		}
	}

	for entryID, remoteACL := range state.ACLs {
		if _, ok := acceptedEntries[entryID]; ok && e.allowRemoteACL(remoteACL, senderPeerID, enforceAuth) {
			filtered.ACLs[entryID] = remoteACL
			continue
		}
		if e.allowRemoteACL(remoteACL, senderPeerID, enforceAuth) {
			filtered.ACLs[entryID] = remoteACL
		}
	}

	tempClock := core.NewClockWithTime(filtered.ClockTime)
	tempReplica := crdt.NewReplica(tempClock)
	tempReplica.LoadState(filtered)
	e.replica.Merge(tempReplica)

	mergedState := e.replica.State()
	for _, elem := range mergedState.Entries {
		if err := e.store.Put(elem.Entry); err != nil {
			return fmt.Errorf("failed to persist merged entry: %w", err)
		}
	}

	for _, remoteACL := range filtered.ACLs {
		if err := e.acls.SetACL(remoteACL); err != nil {
			return fmt.Errorf("failed to persist merged ACL: %w", err)
		}
	}

	return nil
}

func (e *engineImpl) allowRemoteEntry(elem crdt.LWWElement, remoteACL core.ACL, senderPeerID string, enforceAuth bool) bool {
	if !enforceAuth {
		return true
	}
	if senderPeerID == "" {
		return false
	}

	localACL, err := e.acls.GetACL(elem.Entry.ID)
	if err == nil && localACL.Owner != "" {
		return e.canApplyForLocalACL(localACL, senderPeerID, elem.Deleted)
	}

	return remoteACL.EntryID == elem.Entry.ID && remoteACL.Owner == senderPeerID
}

func (e *engineImpl) allowRemoteACL(remoteACL core.ACL, senderPeerID string, enforceAuth bool) bool {
	if !enforceAuth {
		return true
	}
	if senderPeerID == "" || remoteACL.EntryID == uuid.Nil {
		return false
	}

	localACL, err := e.acls.GetACL(remoteACL.EntryID)
	if err != nil || localACL.Owner == "" {
		return remoteACL.Owner == senderPeerID
	}

	return localACL.Owner == senderPeerID && remoteACL.Owner == localACL.Owner
}

func (e *engineImpl) canApplyForLocalACL(localACL *core.ACL, senderPeerID string, deleting bool) bool {
	if localACL == nil {
		return false
	}
	if deleting {
		return e.peerHasWrite(localACL, senderPeerID)
	}
	return e.peerHasWrite(localACL, senderPeerID)
}

func (e *engineImpl) peerHasWrite(localACL *core.ACL, senderPeerID string) bool {
	if localACL.Owner == senderPeerID || localACL.Owner == "" {
		return true
	}
	for _, writer := range localACL.Writers {
		if writer == senderPeerID {
			return true
		}
	}
	return false
}
