// Package sync provides peer-to-peer synchronization for acorde.
//
// It uses libp2p for networking and mDNS for local peer discovery.
// The protocol uses state-based sync with hash comparison for efficiency.
package sync

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"time"

	"github.com/amaydixit11/acorde/internal/crdt"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
)

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

	// EnableDHT enables Kademlia DHT for global peer discovery
	// Default: false (uses IPFS bootstrap nodes)
	EnableDHT bool

	// AllowlistPath is the path to the trusted peers file
	// Default: "" (no persistence)
	AllowlistPath string

	// StrictAllowlist rejects peers not in the allowlist
	// Default: false (accept all)
	StrictAllowlist bool

	// Logger for sync events (optional)
	Logger Logger

	// PrivateKey is the identity key for the host
	// Optional (generated if nil)
	PrivateKey crypto.PrivKey
}

// Logger interface for sync events
type Logger interface {
	Printf(format string, v ...interface{})
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

	// Metrics returns sync statistics
	Metrics() SyncMetrics

	// GetHost returns the underlying libp2p host
	GetHost() host.Host

	// ConnectPeer connects to a peer from an invite
	ConnectPeer(invite *PeerInvite) error
}

// SyncMetrics provides sync statistics
type SyncMetrics struct {
	SyncAttempts  int64
	SyncSuccesses int64
	SyncFailures  int64
}

// StateProvider provides CRDT state for sync
// This interface decouples the sync layer from the engine internals.
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
	SessionID string      `json:"session_id,omitempty"` // Prevents duplicate sync operations
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

// GenerateSessionID creates a unique session identifier
// Format: timestamp-random (e.g., "1706707200-a1b2c3d4")
func GenerateSessionID() string {
	ts := time.Now().UnixNano()
	randomBytes := make([]byte, 4)
	rand.Read(randomBytes)
	return fmt.Sprintf("%d-%s", ts, hex.EncodeToString(randomBytes))
}
