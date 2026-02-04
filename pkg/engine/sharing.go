package engine

import (
	"github.com/amaydixit11/vaultd/internal/sharing"
	"github.com/amaydixit11/vaultd/pkg/crypto"
)

// PeerID is a public key identifying a peer for key exchange
type PeerID = sharing.PeerID

// SharingManager manages per-entry encryption and key sharing
type SharingManager = sharing.SharingManager

// NewSharingManager creates a new sharing manager with a master key
func NewSharingManager(masterKey crypto.Key) (*SharingManager, error) {
	return sharing.NewSharingManager(masterKey)
}

// ShareableKey represents an entry key encrypted for a specific peer
type ShareableKey = sharing.ShareableKey

// For future: AddEntryInputWithSharing would look like:
// type AddEntryInputWithSharing struct {
//     Type      EntryType
//     Content   []byte
//     Tags      []string
//     ShareWith []PeerID  // Peers who can decrypt this entry
// }
