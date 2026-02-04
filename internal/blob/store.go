// Package blob provides content-addressed storage for large files.
package blob

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

// CID is a Content Identifier (hash of content)
type CID string

// Store provides content-addressed blob storage
type Store struct {
	dir string
}

// NewStore creates a new blob store at the given directory
func NewStore(dataDir string) (*Store, error) {
	blobDir := filepath.Join(dataDir, "blobs")
	if err := os.MkdirAll(blobDir, 0700); err != nil {
		return nil, fmt.Errorf("failed to create blob directory: %w", err)
	}

	return &Store{dir: blobDir}, nil
}

// Put stores a blob and returns its CID
func (s *Store) Put(data []byte) (CID, error) {
	cid := computeCID(data)
	path := s.blobPath(cid)

	// Check if already exists (content-addressed = idempotent)
	if _, err := os.Stat(path); err == nil {
		return cid, nil
	}

	// Write to temp file then rename (atomic)
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return "", fmt.Errorf("failed to write blob: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("failed to finalize blob: %w", err)
	}

	return cid, nil
}

// PutReader stores a blob from a reader
func (s *Store) PutReader(r io.Reader) (CID, error) {
	// Read all into memory (for now - could stream with multipass)
	data, err := io.ReadAll(r)
	if err != nil {
		return "", fmt.Errorf("failed to read blob: %w", err)
	}
	return s.Put(data)
}

// Get retrieves a blob by CID
func (s *Store) Get(cid CID) ([]byte, error) {
	path := s.blobPath(cid)
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return nil, fmt.Errorf("blob not found: %s", cid)
	}
	if err != nil {
		return nil, fmt.Errorf("failed to read blob: %w", err)
	}

	// Verify integrity
	actualCID := computeCID(data)
	if actualCID != cid {
		return nil, fmt.Errorf("blob integrity check failed: expected %s, got %s", cid, actualCID)
	}

	return data, nil
}

// Has checks if a blob exists
func (s *Store) Has(cid CID) bool {
	path := s.blobPath(cid)
	_, err := os.Stat(path)
	return err == nil
}

// Delete removes a blob
func (s *Store) Delete(cid CID) error {
	path := s.blobPath(cid)
	if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete blob: %w", err)
	}
	return nil
}

// Size returns the size of a blob
func (s *Store) Size(cid CID) (int64, error) {
	path := s.blobPath(cid)
	info, err := os.Stat(path)
	if os.IsNotExist(err) {
		return 0, fmt.Errorf("blob not found: %s", cid)
	}
	if err != nil {
		return 0, err
	}
	return info.Size(), nil
}

// List returns all CIDs in the store
func (s *Store) List() ([]CID, error) {
	entries, err := os.ReadDir(s.dir)
	if err != nil {
		return nil, fmt.Errorf("failed to list blobs: %w", err)
	}

	cids := make([]CID, 0, len(entries))
	for _, entry := range entries {
		if !entry.IsDir() && len(entry.Name()) == 64 { // SHA256 hex = 64 chars
			cids = append(cids, CID(entry.Name()))
		}
	}
	return cids, nil
}

// GarbageCollect removes unreferenced blobs
// referencedCIDs should contain all CIDs that are still in use
func (s *Store) GarbageCollect(referencedCIDs map[CID]bool) (int, error) {
	all, err := s.List()
	if err != nil {
		return 0, err
	}

	removed := 0
	for _, cid := range all {
		if !referencedCIDs[cid] {
			if err := s.Delete(cid); err == nil {
				removed++
			}
		}
	}
	return removed, nil
}

func (s *Store) blobPath(cid CID) string {
	// Use first 2 chars as subdirectory for better filesystem performance
	prefix := string(cid)[:2]
	return filepath.Join(s.dir, prefix, string(cid))
}

func computeCID(data []byte) CID {
	hash := sha256.Sum256(data)
	return CID(hex.EncodeToString(hash[:]))
}

// Ensure blob subdirectory exists
func (s *Store) ensureSubdir(cid CID) error {
	prefix := string(cid)[:2]
	subdir := filepath.Join(s.dir, prefix)
	return os.MkdirAll(subdir, 0700)
}

// Put with subdirectory creation
func (s *Store) PutWithSubdir(data []byte) (CID, error) {
	cid := computeCID(data)
	
	if err := s.ensureSubdir(cid); err != nil {
		return "", err
	}

	path := s.blobPath(cid)

	// Check if already exists
	if _, err := os.Stat(path); err == nil {
		return cid, nil
	}

	// Write atomically
	tmpPath := path + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0600); err != nil {
		return "", fmt.Errorf("failed to write blob: %w", err)
	}

	if err := os.Rename(tmpPath, path); err != nil {
		os.Remove(tmpPath)
		return "", fmt.Errorf("failed to finalize blob: %w", err)
	}

	return cid, nil
}
