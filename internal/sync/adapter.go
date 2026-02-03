package sync

import (
	"crypto/sha256"
	"encoding/json"
	"fmt"

	"github.com/amaydixit11/vaultd/internal/core"
	"github.com/amaydixit11/vaultd/internal/crdt"
	"github.com/amaydixit11/vaultd/internal/storage"
)

// EngineAdapter adapts an engine's replica for sync
type EngineAdapter struct {
	replica *crdt.Replica
	store   storage.Store
}

// NewEngineAdapter creates a StateProvider from engine components
func NewEngineAdapter(replica *crdt.Replica, store storage.Store) *EngineAdapter {
	return &EngineAdapter{
		replica: replica,
		store:   store,
	}
}

// GetState returns the current replica state
func (a *EngineAdapter) GetState() crdt.ReplicaState {
	return a.replica.State()
}

// ApplyState merges remote state into local
func (a *EngineAdapter) ApplyState(state crdt.ReplicaState) error {
	// Create temporary replica with received state
	tempClock := core.NewClockWithTime(state.ClockTime)
	tempReplica := crdt.NewReplica(tempClock)
	tempReplica.LoadState(state)

	// Merge into our replica
	a.replica.Merge(tempReplica)

	// Persist merged state to storage
	for _, entry := range a.replica.ListEntries() {
		if err := a.store.Put(entry); err != nil {
			return fmt.Errorf("failed to persist entry: %w", err)
		}
	}

	return nil
}

// StateHash returns a hash of current state for quick comparison
func (a *EngineAdapter) StateHash() []byte {
	state := a.replica.State()
	data, _ := json.Marshal(state)
	hash := sha256.Sum256(data)
	return hash[:]
}
