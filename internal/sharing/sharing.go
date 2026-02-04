// Package sharing provides per-entry encryption for selective sharing.
package sharing

import (
	"crypto/rand"
	"crypto/sha256"
	"fmt"

	"github.com/amaydixit11/acorde/pkg/crypto"
	"github.com/google/uuid"
	"golang.org/x/crypto/curve25519"
	"golang.org/x/crypto/hkdf"
)

// PeerID uniquely identifies a peer for sharing
type PeerID [32]byte

// KeyPair represents a peer's public/private key pair for key exchange
type KeyPair struct {
	Private [32]byte
	Public  [32]byte
}

// GenerateKeyPair creates a new X25519 key pair
func GenerateKeyPair() (*KeyPair, error) {
	var private, public [32]byte

	if _, err := rand.Read(private[:]); err != nil {
		return nil, fmt.Errorf("failed to generate private key: %w", err)
	}

	// Clamp private key for X25519
	private[0] &= 248
	private[31] &= 127
	private[31] |= 64

	curve25519.ScalarBaseMult(&public, &private)

	return &KeyPair{Private: private, Public: public}, nil
}

// EntryKey represents a per-entry encryption key
type EntryKey struct {
	Key       crypto.Key
	EntryID   uuid.UUID
	SharedWith []PeerID
}

// DeriveEntryKey derives a unique key for an entry from the master key
func DeriveEntryKey(masterKey crypto.Key, entryID uuid.UUID) (*EntryKey, error) {
	// Use HKDF to derive entry-specific key
	h := hkdf.New(sha256.New, masterKey[:], entryID[:], []byte("vaultd-entry-key"))

	var key crypto.Key
	if _, err := h.Read(key[:]); err != nil {
		return nil, fmt.Errorf("failed to derive key: %w", err)
	}

	return &EntryKey{
		Key:     key,
		EntryID: entryID,
	}, nil
}

// ShareableKey creates a key that can be shared with specific peers
type ShareableKey struct {
	EncryptedKey []byte   // Entry key encrypted for sharing
	ForPeerID    PeerID   // Recipient's public key
}

// ShareKeyWith encrypts an entry key for a specific peer using ECDH
func ShareKeyWith(entryKey *EntryKey, myPrivate [32]byte, peerPublic PeerID) (*ShareableKey, error) {
	// Compute shared secret using X25519
	var sharedSecret [32]byte
	curve25519.ScalarMult(&sharedSecret, &myPrivate, (*[32]byte)(&peerPublic))

	// Derive encryption key from shared secret
	h := hkdf.New(sha256.New, sharedSecret[:], entryKey.EntryID[:], []byte("vaultd-share-key"))
	
	var wrapKey crypto.Key
	if _, err := h.Read(wrapKey[:]); err != nil {
		return nil, fmt.Errorf("failed to derive wrap key: %w", err)
	}

	// Encrypt the entry key with the derived wrap key
	encrypted, err := crypto.Encrypt(wrapKey, entryKey.Key[:], entryKey.EntryID[:])
	if err != nil {
		return nil, fmt.Errorf("failed to encrypt key: %w", err)
	}

	return &ShareableKey{
		EncryptedKey: encrypted,
		ForPeerID:    peerPublic,
	}, nil
}

// RecoverSharedKey decrypts a shared key using our private key
func RecoverSharedKey(shared *ShareableKey, entryID uuid.UUID, myPrivate [32]byte, senderPublic PeerID) (*crypto.Key, error) {
	// Compute shared secret
	var sharedSecret [32]byte
	curve25519.ScalarMult(&sharedSecret, &myPrivate, (*[32]byte)(&senderPublic))

	// Derive unwrap key
	h := hkdf.New(sha256.New, sharedSecret[:], entryID[:], []byte("vaultd-share-key"))
	
	var unwrapKey crypto.Key
	if _, err := h.Read(unwrapKey[:]); err != nil {
		return nil, fmt.Errorf("failed to derive unwrap key: %w", err)
	}

	// Decrypt the entry key
	keyBytes, err := crypto.Decrypt(unwrapKey, shared.EncryptedKey, entryID[:])
	if err != nil {
		return nil, fmt.Errorf("failed to decrypt key: %w", err)
	}

	var key crypto.Key
	copy(key[:], keyBytes)
	return &key, nil
}

// SharingManager manages per-entry encryption keys
type SharingManager struct {
	keyPair    *KeyPair
	masterKey  crypto.Key
	entryKeys  map[uuid.UUID]*EntryKey
}

// NewSharingManager creates a new sharing manager
func NewSharingManager(masterKey crypto.Key) (*SharingManager, error) {
	kp, err := GenerateKeyPair()
	if err != nil {
		return nil, err
	}

	return &SharingManager{
		keyPair:   kp,
		masterKey: masterKey,
		entryKeys: make(map[uuid.UUID]*EntryKey),
	}, nil
}

// MyPeerID returns this peer's public identity
func (m *SharingManager) MyPeerID() PeerID {
	return PeerID(m.keyPair.Public)
}

// GetOrCreateEntryKey gets or creates a key for an entry
func (m *SharingManager) GetOrCreateEntryKey(entryID uuid.UUID) (*EntryKey, error) {
	if key, ok := m.entryKeys[entryID]; ok {
		return key, nil
	}

	key, err := DeriveEntryKey(m.masterKey, entryID)
	if err != nil {
		return nil, err
	}

	m.entryKeys[entryID] = key
	return key, nil
}

// ShareEntry creates shareable keys for specific peers
func (m *SharingManager) ShareEntry(entryID uuid.UUID, peers []PeerID) ([]*ShareableKey, error) {
	entryKey, err := m.GetOrCreateEntryKey(entryID)
	if err != nil {
		return nil, err
	}

	shares := make([]*ShareableKey, len(peers))
	for i, peerID := range peers {
		share, err := ShareKeyWith(entryKey, m.keyPair.Private, peerID)
		if err != nil {
			return nil, fmt.Errorf("failed to share with peer %d: %w", i, err)
		}
		shares[i] = share
	}

	entryKey.SharedWith = append(entryKey.SharedWith, peers...)
	return shares, nil
}
