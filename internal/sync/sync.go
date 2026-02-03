// Package sync provides peer-to-peer synchronization for vaultd.
//
// It uses libp2p for networking and mDNS for local peer discovery.
package sync

import (
	"context"
	"encoding/json"
	"time"

	"github.com/amaydixit11/vaultd/internal/crdt"
	"github.com/libp2p/go-libp2p/core/peer"
)

// ProtocolID is the libp2p protocol identifier for vaultd sync
const ProtocolID = "/vaultd/sync/1.0.0"

// ServiceName is the mDNS service name for discovery
const ServiceName = "vaultd"

// Config contains configuration for the SyncService
type Config struct {
	// ListenAddrs are the multiaddrs to listen on
	// Default: /ip4/0.0.0.0/tcp/0 (random port)
	ListenAddrs []string

	// SyncInterval is how often to sync with peers
	// Default: 5 seconds
	SyncInterval time.Duration

	// EnableMDNS enables mDNS for LAN peer discovery
	// Default: true
	EnableMDNS bool
}

// DefaultConfig returns the default sync configuration
func DefaultConfig() Config {
	return Config{
		ListenAddrs:  []string{"/ip4/0.0.0.0/tcp/0"},
		SyncInterval: 5 * time.Second,
		EnableMDNS:   true,
	}
}

// SyncService manages peer-to-peer synchronization
type SyncService interface {
	// Start begins listening and discovering peers
	Start(ctx context.Context) error

	// Stop gracefully shuts down the service
	Stop() error

	// Peers returns the list of connected peers
	Peers() []peer.ID

	// SyncWith triggers a sync with a specific peer
	SyncWith(ctx context.Context, peerID peer.ID) error
}

// StateProvider provides CRDT state for sync
type StateProvider interface {
	// GetState returns the current replica state
	GetState() crdt.ReplicaState

	// ApplyState merges remote state into local
	ApplyState(state crdt.ReplicaState) error

	// StateHash returns a hash of current state for quick comparison
	StateHash() []byte
}

// MessageType identifies the type of sync message
type MessageType uint8

const (
	MsgStateHash    MessageType = 1 // Exchange state hashes
	MsgStateRequest MessageType = 2 // Request full state
	MsgState        MessageType = 3 // Full state payload
)

// Message is a sync protocol message
type Message struct {
	Type      MessageType `json:"type"`
	StateHash []byte      `json:"state_hash,omitempty"`
	State     []byte      `json:"state,omitempty"` // JSON-encoded ReplicaState
}

// Encode serializes the message to bytes
func (m *Message) Encode() ([]byte, error) {
	return json.Marshal(m)
}

// DecodeMessage deserializes a message from bytes
func DecodeMessage(data []byte) (*Message, error) {
	var m Message
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	return &m, nil
}
