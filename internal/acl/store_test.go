package acl

import (
	"database/sql"
	"testing"

	"github.com/amaydixit11/acorde/internal/core"
	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

func newTestStore(t *testing.T, localID string) *Store {
	db, err := sql.Open("sqlite3", ":memory:")
	if err != nil {
		t.Fatalf("failed to open in-memory db: %v", err)
	}

	s, err := NewStore(db, localID)
	if err != nil {
		t.Fatalf("failed to create store: %v", err)
	}
	return s
}

func TestStore_CheckWrite(t *testing.T) {
	node1 := "node1-peer-id"
	node2 := "node2-peer-id"
	s := newTestStore(t, node1)

	id := uuid.New()

	// Initial add (Node 1 is owner)
	s.SetACL(core.ACL{
		EntryID: id,
		Owner:   node1,
		Public:  false,
	})

	// Node 1 should be allowed
	if ok, _ := s.CheckWrite(id, node1); !ok {
		t.Error("Owner should have write access")
	}

	// Node 2 should be denied
	if ok, _ := s.CheckWrite(id, node2); ok {
		t.Error("Other nodes should NOT have write access by default")
	}

	// Grant Node 2 access
	if err := s.GrantWrite(id, node2); err != nil {
		t.Fatalf("Failed to grant write: %v", err)
	}

	// Node 2 should now be allowed
	if ok, _ := s.CheckWrite(id, node2); !ok {
		t.Error("Authorized node should have write access")
	}
}
