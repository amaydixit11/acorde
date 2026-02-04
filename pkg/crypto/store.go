package crypto

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"

	"golang.org/x/crypto/argon2"
)

const KeyFileName = "keys.json"

// KeyStore manages the master encryption key for the vault.
// It handles secure storage, retrieval, and lifecycle of the key material.
type KeyStore interface {
	// Initialize creates a new master key, encrypts it with the provided password,
	// and persists it to storage. It returns an error if the store is already initialized.
	Initialize(password []byte) error

	// InitializeWithKey creates a master key file with an existing key
	InitializeWithKey(password []byte, key Key) error

	// Unlock loads the master key using the password
	Unlock(password []byte) (Key, error)

	// IsInitialized checks if a key file exists
	IsInitialized() bool
}

// FileKeyStore implements KeyStore using a file
type FileKeyStore struct {
	dir string
	mu  sync.RWMutex
}

// keyFileStruct is the JSON structure for the key file
type keyFileStruct struct {
	Salt      string `json:"salt"`
	Ciphertext string `json:"data"` // Encrypted master key
	Params    params `json:"params"`
}

type params struct {
	Memory      uint32 `json:"mem"`
	Iterations  uint32 `json:"time"`
	Parallelism uint8  `json:"threads"`
}

// NewFileKeyStore creates a new filesystem-backed KeyStore.
// The key file will be stored at <dir>/keys.json.
func NewFileKeyStore(dir string) *FileKeyStore {
	return &FileKeyStore{dir: dir}
}

func (s *FileKeyStore) Initialize(password []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isInitialized() {
		return fmt.Errorf("keystore already initialized")
	}

	// 1. Generate master key
	masterKey, err := GenerateKey()
	if err != nil {
		return err
	}

	// 2. Generate salt for password wrapper
	salt, err := GenerateSalt()
	if err != nil {
		return err
	}

	// 3. Derive wrapper key from password
	wrapperKey := DeriveKey(password, salt)

	// 4. Encrypt master key with wrapper key
	// Use path as AAD to prevent file substitution
	aad := []byte(filepath.Base(s.dir)) 
	encryptedKey, err := Encrypt(wrapperKey, masterKey[:], aad)
	if err != nil {
		return err
	}

	// 5. Save to file
	kf := keyFileStruct{
		Salt:       base64.StdEncoding.EncodeToString(salt),
		Ciphertext: base64.StdEncoding.EncodeToString(encryptedKey),
		Params: params{
			Memory:      64 * 1024,
			Iterations:  3,
			Parallelism: 2,
		},
	}

	data, err := json.MarshalIndent(kf, "", "  ")
	if err != nil {
		return err
	}

	if err := os.MkdirAll(s.dir, 0700); err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(s.dir, KeyFileName), data, 0600)
}

func (s *FileKeyStore) InitializeWithKey(password []byte, masterKey Key) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.isInitialized() {
		return fmt.Errorf("keystore already initialized")
	}

	// 1. Generate salt for password wrapper
	salt, err := GenerateSalt()
	if err != nil {
		return err
	}

	// 2. Derive wrapper key from password
	// Use standard params for new keys
	params := params{
		Memory:      64 * 1024,
		Iterations:  3,
		Parallelism: 2,
	}
	dk := argon2.IDKey(password, salt, params.Iterations, params.Memory, params.Parallelism, KeySize)
	wrapperKey := Key{}
	copy(wrapperKey[:], dk)

	// 3. Encrypt master key with wrapper key
	aad := []byte(filepath.Base(s.dir))
	encryptedKey, err := Encrypt(wrapperKey, masterKey[:], aad)
	if err != nil {
		return err
	}

	// 4. Save to file
	kf := keyFileStruct{
		Salt:       base64.StdEncoding.EncodeToString(salt),
		Ciphertext: base64.StdEncoding.EncodeToString(encryptedKey),
		Params:     params,
	}

	data, err := json.MarshalIndent(kf, "", "  ")
	if err != nil {
		return err
	}

	if err := os.MkdirAll(s.dir, 0700); err != nil {
		return err
	}

	return os.WriteFile(filepath.Join(s.dir, KeyFileName), data, 0600)
}

func (s *FileKeyStore) Unlock(password []byte) (Key, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var k Key

	// 1. Read file
	data, err := os.ReadFile(filepath.Join(s.dir, KeyFileName))
	if err != nil {
		return k, err
	}

	var kf keyFileStruct
	if err := json.Unmarshal(data, &kf); err != nil {
		return k, err
	}

	// 2. Decode fields
	salt, err := base64.StdEncoding.DecodeString(kf.Salt)
	if err != nil {
		return k, err
	}
	ciphertext, err := base64.StdEncoding.DecodeString(kf.Ciphertext)
	if err != nil {
		return k, err
	}

	// 3. Derive wrapper key
	// Ensure params from file are used if possible (currently hardcoded in DeriveKey, 
	// so we actually need to update DeriveKey to accept params OR just validate they match defaults for now.
	// Since DeriveKey API is fixed to constants, we should probably update DeriveKey or validate params.
	// For now, let's validate parallelism/memory/iterations match what we expect.
	// If we want to support changing params, we need to pass them to Argon2.
	
	// Better fix: Update DeriveKey to take params, or just use the values from json.
	// Let's use the values from JSON.
	dk := argon2.IDKey(password, salt, kf.Params.Iterations, kf.Params.Memory, kf.Params.Parallelism, KeySize)
	wrapperKey := Key{}
	copy(wrapperKey[:], dk)

	// 4. Decrypt master key
	aad := []byte(filepath.Base(s.dir))
	plaintext, err := Decrypt(wrapperKey, ciphertext, aad)
	if err != nil {
		return k, errors.New("incorrect password or corrupted key file")
	}

	if len(plaintext) != KeySize {
		return k, errors.New("invalid key size")
	}

	copy(k[:], plaintext)
	return k, nil
}

func (s *FileKeyStore) IsInitialized() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.isInitialized()
}

// isInitialized is the internal lock-less check
func (s *FileKeyStore) isInitialized() bool {
	_, err := os.Stat(filepath.Join(s.dir, KeyFileName))
	return err == nil
}
