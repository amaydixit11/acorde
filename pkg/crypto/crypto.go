package crypto

import (
	"crypto/rand"
	"errors"
	"fmt"
	"io"

	"golang.org/x/crypto/argon2"
	"golang.org/x/crypto/chacha20poly1305"
)

const (
	KeySize   = 32
	NonceSize = 24 // XChaCha20 nonce size
	SaltSize  = 16
)

var (
	ErrInvalidKey = errors.New("invalid key size")
	ErrDecrypt    = errors.New("decryption failed")
)

// Key represents a 32-byte encryption key
type Key [KeySize]byte

// GenerateKey creates a new random key
func GenerateKey() (Key, error) {
	var k Key
	if _, err := io.ReadFull(rand.Reader, k[:]); err != nil {
		return k, err
	}
	return k, nil
}

// DeriveKey derives a key from a password and salt using Argon2id
func DeriveKey(password, salt []byte) Key {
	var k Key
	// Argon2id parameters (OWASP recommendations)
	// Time: 3 passes
	// Memory: 64 MB (64 * 1024)
	// Threads: 2
	// KeyLen: 32 bytes
	dk := argon2.IDKey(password, salt, 3, 64*1024, 2, KeySize)
	copy(k[:], dk)
	return k
}

// Encrypt encrypts plaintext using XChaCha20-Poly1305
// Format: [Nonce 24][Ciphertext ...][Tag 16] (Tag is appended by Seal)
func Encrypt(key Key, plaintext, aad []byte) ([]byte, error) {
	aead, err := chacha20poly1305.NewX(key[:])
	if err != nil {
		return nil, fmt.Errorf("failed to create AEAD: %w", err)
	}

	// Generate random nonce
	nonce := make([]byte, NonceSize, NonceSize+len(plaintext)+aead.Overhead())
	if _, err := io.ReadFull(rand.Reader, nonce); err != nil {
		return nil, fmt.Errorf("failed to generate nonce: %w", err)
	}

	// Encrypt and append to nonce
	return aead.Seal(nonce, nonce, plaintext, aad), nil
}

// Decrypt decrypts ciphertext using XChaCha20-Poly1305
func Decrypt(key Key, ciphertext, aad []byte) ([]byte, error) {
	if len(ciphertext) < NonceSize {
		return nil, ErrDecrypt
	}

	aead, err := chacha20poly1305.NewX(key[:])
	if err != nil {
		return nil, fmt.Errorf("failed to create AEAD: %w", err)
	}

	nonce := ciphertext[:NonceSize]
	encryptedMsg := ciphertext[NonceSize:]

	plaintext, err := aead.Open(nil, nonce, encryptedMsg, aad)
	if err != nil {
		return nil, ErrDecrypt
	}

	return plaintext, nil
}

// GenerateSalt creates a random salt
func GenerateSalt() ([]byte, error) {
	salt := make([]byte, SaltSize)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return nil, err
	}
	return salt, nil
}
