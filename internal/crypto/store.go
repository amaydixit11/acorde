package crypto

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

const KeyFileName = "keys.json"

// KeyStore manages the master key
type KeyStore interface {
	// Initialize creates a new master key, encrypts it with password, and saves it
	Initialize(password []byte) error

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

// NewFileKeyStore creates a new FileKeyStore
func NewFileKeyStore(dir string) *FileKeyStore {
	return &FileKeyStore{dir: dir}
}

func (s *FileKeyStore) Initialize(password []byte) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.IsInitialized() {
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
	encryptedKey, err := Encrypt(wrapperKey, masterKey[:], nil)
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
	wrapperKey := DeriveKey(password, salt)

	// 4. Decrypt master key
	plaintext, err := Decrypt(wrapperKey, ciphertext, nil)
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
	_, err := os.Stat(filepath.Join(s.dir, KeyFileName))
	return err == nil
}
