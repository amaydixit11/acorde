// Package acl provides access control for entries.
package acl

import (
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/google/uuid"
)

// Permission represents access level
type Permission int

const (
	PermNone Permission = iota
	PermRead
	PermWrite
	PermAdmin
)

// ACL represents access control for an entry
type ACL struct {
	EntryID uuid.UUID `json:"entry_id"`
	Owner   string    `json:"owner"`              // PeerID of owner
	Readers []string  `json:"readers,omitempty"`  // PeerIDs with read access
	Writers []string  `json:"writers,omitempty"`  // PeerIDs with write access
	Public  bool      `json:"public"`              // Anyone can read
}

// Store manages ACLs in SQLite
type Store struct {
	db      *sql.DB
	localID string // This peer's ID
}

// NewStore creates a new ACL store
func NewStore(db *sql.DB, localPeerID string) (*Store, error) {
	store := &Store{
		db:      db,
		localID: localPeerID,
	}

	if err := store.initSchema(); err != nil {
		return nil, err
	}

	return store, nil
}

func (s *Store) initSchema() error {
	schema := `
		CREATE TABLE IF NOT EXISTS entry_acl (
			entry_id TEXT PRIMARY KEY,
			owner TEXT NOT NULL,
			readers TEXT NOT NULL,
			writers TEXT NOT NULL,
			public INTEGER NOT NULL DEFAULT 0
		);
	`
	_, err := s.db.Exec(schema)
	return err
}

// SetACL sets the ACL for an entry
func (s *Store) SetACL(acl ACL) error {
	readersJSON, _ := json.Marshal(acl.Readers)
	writersJSON, _ := json.Marshal(acl.Writers)
	public := 0
	if acl.Public {
		public = 1
	}

	_, err := s.db.Exec(`
		INSERT OR REPLACE INTO entry_acl (entry_id, owner, readers, writers, public)
		VALUES (?, ?, ?, ?, ?)
	`, acl.EntryID.String(), acl.Owner, readersJSON, writersJSON, public)

	return err
}

// GetACL retrieves the ACL for an entry
func (s *Store) GetACL(entryID uuid.UUID) (*ACL, error) {
	var acl ACL
	var entryIDStr string
	var readersJSON, writersJSON []byte
	var public int

	err := s.db.QueryRow(`
		SELECT entry_id, owner, readers, writers, public
		FROM entry_acl
		WHERE entry_id = ?
	`, entryID.String()).Scan(&entryIDStr, &acl.Owner, &readersJSON, &writersJSON, &public)

	if err == sql.ErrNoRows {
		// No ACL = public access
		return &ACL{
			EntryID: entryID,
			Owner:   "",
			Public:  true,
		}, nil
	}
	if err != nil {
		return nil, err
	}

	acl.EntryID, _ = uuid.Parse(entryIDStr)
	json.Unmarshal(readersJSON, &acl.Readers)
	json.Unmarshal(writersJSON, &acl.Writers)
	acl.Public = public == 1

	return &acl, nil
}

// DeleteACL removes the ACL for an entry
func (s *Store) DeleteACL(entryID uuid.UUID) error {
	_, err := s.db.Exec(`DELETE FROM entry_acl WHERE entry_id = ?`, entryID.String())
	return err
}

// CheckRead checks if a peer can read an entry
func (s *Store) CheckRead(entryID uuid.UUID, peerID string) (bool, error) {
	acl, err := s.GetACL(entryID)
	if err != nil {
		return false, err
	}

	return s.canRead(acl, peerID), nil
}

// CheckWrite checks if a peer can write an entry
func (s *Store) CheckWrite(entryID uuid.UUID, peerID string) (bool, error) {
	acl, err := s.GetACL(entryID)
	if err != nil {
		return false, err
	}

	return s.canWrite(acl, peerID), nil
}

// CheckAdmin checks if a peer is the owner
func (s *Store) CheckAdmin(entryID uuid.UUID, peerID string) (bool, error) {
	acl, err := s.GetACL(entryID)
	if err != nil {
		return false, err
	}

	return acl.Owner == peerID || acl.Owner == "", nil
}

func (s *Store) canRead(acl *ACL, peerID string) bool {
	// Public entries are readable by all
	if acl.Public {
		return true
	}

	// Owner can always read
	if acl.Owner == peerID || acl.Owner == "" {
		return true
	}

	// Check writers (writers can also read)
	for _, w := range acl.Writers {
		if w == peerID {
			return true
		}
	}

	// Check readers
	for _, r := range acl.Readers {
		if r == peerID {
			return true
		}
	}

	return false
}

func (s *Store) canWrite(acl *ACL, peerID string) bool {
	// Owner can always write
	if acl.Owner == peerID || acl.Owner == "" {
		return true
	}

	// Check writers
	for _, w := range acl.Writers {
		if w == peerID {
			return true
		}
	}

	return false
}

// GrantRead adds a peer to the readers list
func (s *Store) GrantRead(entryID uuid.UUID, peerID string) error {
	acl, err := s.GetACL(entryID)
	if err != nil {
		return err
	}

	// Check if already has access
	for _, r := range acl.Readers {
		if r == peerID {
			return nil
		}
	}

	acl.Readers = append(acl.Readers, peerID)
	return s.SetACL(*acl)
}

// GrantWrite adds a peer to the writers list
func (s *Store) GrantWrite(entryID uuid.UUID, peerID string) error {
	acl, err := s.GetACL(entryID)
	if err != nil {
		return err
	}

	// Check if already has access
	for _, w := range acl.Writers {
		if w == peerID {
			return nil
		}
	}

	acl.Writers = append(acl.Writers, peerID)
	return s.SetACL(*acl)
}

// RevokeRead removes a peer from the readers list
func (s *Store) RevokeRead(entryID uuid.UUID, peerID string) error {
	acl, err := s.GetACL(entryID)
	if err != nil {
		return err
	}

	newReaders := make([]string, 0, len(acl.Readers))
	for _, r := range acl.Readers {
		if r != peerID {
			newReaders = append(newReaders, r)
		}
	}
	acl.Readers = newReaders

	return s.SetACL(*acl)
}

// RevokeWrite removes a peer from the writers list
func (s *Store) RevokeWrite(entryID uuid.UUID, peerID string) error {
	acl, err := s.GetACL(entryID)
	if err != nil {
		return err
	}

	newWriters := make([]string, 0, len(acl.Writers))
	for _, w := range acl.Writers {
		if w != peerID {
			newWriters = append(newWriters, w)
		}
	}
	acl.Writers = newWriters

	return s.SetACL(*acl)
}

// SetOwner sets the owner of an entry
func (s *Store) SetOwner(entryID uuid.UUID, ownerID string) error {
	acl, err := s.GetACL(entryID)
	if err != nil {
		return err
	}

	acl.Owner = ownerID
	return s.SetACL(*acl)
}

// MakePublic makes an entry publicly readable
func (s *Store) MakePublic(entryID uuid.UUID) error {
	acl, err := s.GetACL(entryID)
	if err != nil {
		return err
	}

	acl.Public = true
	return s.SetACL(*acl)
}

// MakePrivate makes an entry private
func (s *Store) MakePrivate(entryID uuid.UUID) error {
	acl, err := s.GetACL(entryID)
	if err != nil {
		return err
	}

	acl.Public = false
	return s.SetACL(*acl)
}

// ErrAccessDenied is returned when access is denied
type ErrAccessDenied struct {
	EntryID uuid.UUID
	PeerID  string
	Action  string
}

func (e ErrAccessDenied) Error() string {
	return fmt.Sprintf("access denied: peer %s cannot %s entry %s", e.PeerID, e.Action, e.EntryID)
}
