package sync

import (
	"crypto/sha256"
	"encoding/json"

	"github.com/amaydixit11/acorde/internal/crdt"
)

// Syncable defines the interface an engine must implement for sync
// This decouples the sync layer from engine internals.
type Syncable interface {
	// GetSyncState returns the current CRDT state for sync
	GetSyncState() crdt.ReplicaState

	// ApplySyncState applies remote CRDT state and merges
	ApplySyncState(state crdt.ReplicaState) error
}

// EngineAdapter adapts a Syncable engine for the sync service
type EngineAdapter struct {
	engine Syncable
}

// NewEngineAdapter creates a StateProvider from a Syncable engine
func NewEngineAdapter(engine Syncable) *EngineAdapter {
	return &EngineAdapter{engine: engine}
}

// GetState returns the current replica state
func (a *EngineAdapter) GetState() crdt.ReplicaState {
	return a.engine.GetSyncState()
}

// ApplyState merges remote state into local
func (a *EngineAdapter) ApplyState(state crdt.ReplicaState) error {
	return a.engine.ApplySyncState(state)
}

// StateHash returns a hash of current state for quick comparison
func (a *EngineAdapter) StateHash() []byte {
	state := a.engine.GetSyncState()
	data, _ := json.Marshal(state)
	hash := sha256.Sum256(data)
	return hash[:]
}
