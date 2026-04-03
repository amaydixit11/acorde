package engine

import (
	"github.com/amaydixit11/acorde/internal/acl"
	"github.com/amaydixit11/acorde/internal/storage"
	"github.com/google/uuid"
)

// ErrNotFound is returned when an entry is not found
type ErrNotFound struct {
	ID uuid.UUID
}

func (e ErrNotFound) Error() string {
	return "entry not found: " + e.ID.String()
}

// ErrInvalidType is returned when an invalid entry type is provided
type ErrInvalidType struct {
	Type string
}

func (e ErrInvalidType) Error() string {
	return "invalid entry type: " + e.Type
}

// ErrDeleted is returned when attempting to update a deleted entry
type ErrDeleted struct {
	ID uuid.UUID
}

func (e ErrDeleted) Error() string {
	return "cannot update deleted entry: " + e.ID.String()
}

// ErrAccessDenied is returned when access is denied.
type ErrAccessDenied struct {
	EntryID uuid.UUID
	PeerID  string
	Action  string
}

func (e ErrAccessDenied) Error() string {
	return "access denied: peer " + e.PeerID + " cannot " + e.Action + " entry " + e.EntryID.String()
}

// convertError converts internal errors to public error types
func convertError(err error) error {
	if err == nil {
		return nil
	}

	// Convert storage.ErrNotFound to public ErrNotFound
	if notFound, ok := err.(storage.ErrNotFound); ok {
		return ErrNotFound{ID: notFound.ID}
	}
	if denied, ok := err.(acl.ErrAccessDenied); ok {
		return ErrAccessDenied{
			EntryID: denied.EntryID,
			PeerID:  denied.PeerID,
			Action:  denied.Action,
		}
	}

	return err
}
