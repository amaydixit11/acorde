package sync

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	gosync "sync"

	"github.com/libp2p/go-libp2p/core/peer"
)

// Allowlist manages trusted peers
type Allowlist struct {
	peers   map[peer.ID]AllowedPeer
	mu      gosync.RWMutex
	path    string
	strict  bool // If true, reject unknown peers
}

// AllowedPeer contains info about a trusted peer
type AllowedPeer struct {
	PeerID    string `json:"peer_id"`
	Name      string `json:"name,omitempty"`
	AddedAt   int64  `json:"added_at"`
	Addresses []string `json:"addresses,omitempty"`
}

// allowlistFile is the storage format
type allowlistFile struct {
	Peers []AllowedPeer `json:"peers"`
}

// NewAllowlist creates a new allowlist, loading from disk if exists
func NewAllowlist(dataDir string, strict bool) (*Allowlist, error) {
	path := filepath.Join(dataDir, "peers.json")
	
	al := &Allowlist{
		peers:  make(map[peer.ID]AllowedPeer),
		path:   path,
		strict: strict,
	}

	// Load existing allowlist
	if err := al.load(); err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	return al, nil
}

// Add adds a peer to the allowlist
func (al *Allowlist) Add(peerID peer.ID, name string, addresses []string) error {
	al.mu.Lock()
	defer al.mu.Unlock()

	al.peers[peerID] = AllowedPeer{
		PeerID:    peerID.String(),
		Name:      name,
		AddedAt:   0, // Will be set on save
		Addresses: addresses,
	}

	return al.save()
}

// Remove removes a peer from the allowlist
func (al *Allowlist) Remove(peerID peer.ID) error {
	al.mu.Lock()
	defer al.mu.Unlock()

	delete(al.peers, peerID)
	return al.save()
}

// IsAllowed checks if a peer is in the allowlist
func (al *Allowlist) IsAllowed(peerID peer.ID) bool {
	al.mu.RLock()
	defer al.mu.RUnlock()

	if !al.strict {
		return true // Accept all peers if not strict
	}

	_, ok := al.peers[peerID]
	return ok
}

// List returns all allowed peers
func (al *Allowlist) List() []AllowedPeer {
	al.mu.RLock()
	defer al.mu.RUnlock()

	result := make([]AllowedPeer, 0, len(al.peers))
	for _, p := range al.peers {
		result = append(result, p)
	}
	return result
}

// load reads the allowlist from disk
func (al *Allowlist) load() error {
	data, err := os.ReadFile(al.path)
	if err != nil {
		return err
	}

	var file allowlistFile
	if err := json.Unmarshal(data, &file); err != nil {
		return err
	}

	for _, p := range file.Peers {
		peerID, err := peer.Decode(p.PeerID)
		if err != nil {
			continue // Skip invalid entries
		}
		al.peers[peerID] = p
	}

	return nil
}

// save writes the allowlist to disk
func (al *Allowlist) save() error {
	// Ensure directory exists
	if err := os.MkdirAll(filepath.Dir(al.path), 0700); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	file := allowlistFile{
		Peers: make([]AllowedPeer, 0, len(al.peers)),
	}
	for _, p := range al.peers {
		file.Peers = append(file.Peers, p)
	}

	data, err := json.MarshalIndent(file, "", "  ")
	if err != nil {
		return err
	}

	return os.WriteFile(al.path, data, 0600)
}

// Count returns the number of allowed peers
func (al *Allowlist) Count() int {
	al.mu.RLock()
	defer al.mu.RUnlock()
	return len(al.peers)
}
