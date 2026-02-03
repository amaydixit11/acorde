package crypto

import (
	"bytes"
	"testing"
)

func TestEncryption(t *testing.T) {
	key, err := GenerateKey()
	if err != nil {
		t.Fatalf("failed to generate key: %v", err)
	}

	plaintext := []byte("Hello, World!")
	aad := []byte("metadata")

	// Encrypt
	ciphertext, err := Encrypt(key, plaintext, aad)
	if err != nil {
		t.Fatalf("encrypt failed: %v", err)
	}

	if len(ciphertext) <= len(plaintext) {
		t.Error("ciphertext too short")
	}

	// Decrypt
	decrypted, err := Decrypt(key, ciphertext, aad)
	if err != nil {
		t.Fatalf("decrypt failed: %v", err)
	}

	if !bytes.Equal(plaintext, decrypted) {
		t.Error("decrypted content mismatch")
	}

	// Tamper ciphertext
	ciphertext[0] ^= 0xFF
	_, err = Decrypt(key, ciphertext, aad)
	if err == nil {
		t.Error("decryption should fail for tampered ciphertext")
	}

	// Wrong context (AAD)
	// Repair ciphertext first
	ciphertext[0] ^= 0xFF 
	_, err = Decrypt(key, ciphertext, []byte("wrong_aad"))
	if err == nil {
		t.Error("decryption should fail using wrong AAD")
	}
}

func TestKeyDerivation(t *testing.T) {
	password := []byte("password123")
	salt, _ := GenerateSalt()

	key1 := DeriveKey(password, salt)
	key2 := DeriveKey(password, salt)

	if key1 != key2 {
		t.Error("key derivation should be deterministic")
	}

	salt2, _ := GenerateSalt()
	key3 := DeriveKey(password, salt2)

	if key1 == key3 {
		t.Error("different salts should produce different keys")
	}
}

func TestKeyStore(t *testing.T) {
	tmpDir := t.TempDir()
	store := NewFileKeyStore(tmpDir)

	if store.IsInitialized() {
		t.Error("should not be initialized")
	}

	password := []byte("secret")

	// Initialize
	if err := store.Initialize(password); err != nil {
		t.Fatalf("initialize failed: %v", err)
	}

	if !store.IsInitialized() {
		t.Error("should be initialized")
	}

	// Unlock
	key, err := store.Unlock(password)
	if err != nil {
		t.Fatalf("unlock failed: %v", err)
	}

	// Unlock with wrong password
	_, err = store.Unlock([]byte("wrong"))
	if err == nil {
		t.Error("unlock should fail with wrong password")
	}

	// Verify key consistency by doing round trip
	// Since we can't see the master key without unlocking,
	// we assume if Unlock succeeds and gives 32 bytes, it's correct.
	// But let's verify persistence:
	
	store2 := NewFileKeyStore(tmpDir)
	key2, err := store2.Unlock(password)
	if err != nil {
		t.Fatalf("re-unlock failed: %v", err)
	}

	if key != key2 {
		t.Error("keys should match across instances")
	}
}
