// Package version provides entry versioning and history tracking.
package version

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Version represents a historical version of an entry
type Version struct {
	ID        int64     `json:"id"`
	EntryID   uuid.UUID `json:"entry_id"`
	Content   []byte    `json:"content"`
	Tags      []string  `json:"tags"`
	Timestamp uint64    `json:"timestamp"`
	CreatedAt time.Time `json:"created_at"`
	Author    string    `json:"author,omitempty"` // PeerID who made the change
}

// Diff represents the difference between two versions
type Diff struct {
	OldVersion   int64  `json:"old_version"`
	NewVersion   int64  `json:"new_version"`
	ContentDiff  string `json:"content_diff,omitempty"`
	TagsAdded    []string `json:"tags_added,omitempty"`
	TagsRemoved  []string `json:"tags_removed,omitempty"`
}

// Store manages version history in SQLite
type Store struct {
	db         *sql.DB
	maxVersions int // Max versions to keep per entry (0 = unlimited)
}

// NewStore creates a new version store
func NewStore(db *sql.DB, maxVersions int) (*Store, error) {
	store := &Store{
		db:         db,
		maxVersions: maxVersions,
	}

	if err := store.initSchema(); err != nil {
		return nil, err
	}

	return store, nil
}

func (s *Store) initSchema() error {
	schema := `
		CREATE TABLE IF NOT EXISTS entry_versions (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			entry_id TEXT NOT NULL,
			content BLOB NOT NULL,
			tags TEXT NOT NULL,
			timestamp INTEGER NOT NULL,
			created_at INTEGER NOT NULL,
			author TEXT
		);

		CREATE INDEX IF NOT EXISTS idx_versions_entry_id ON entry_versions(entry_id);
		CREATE INDEX IF NOT EXISTS idx_versions_timestamp ON entry_versions(timestamp);
	`
	_, err := s.db.Exec(schema)
	return err
}

// SaveVersion saves a new version of an entry
func (s *Store) SaveVersion(entryID uuid.UUID, content []byte, tags []string, timestamp uint64, author string) error {
	tagsJSON, _ := json.Marshal(tags)

	_, err := s.db.Exec(`
		INSERT INTO entry_versions (entry_id, content, tags, timestamp, created_at, author)
		VALUES (?, ?, ?, ?, ?, ?)
	`, entryID.String(), content, tagsJSON, timestamp, time.Now().Unix(), author)

	if err != nil {
		return fmt.Errorf("failed to save version: %w", err)
	}

	// Prune old versions if limit set
	if s.maxVersions > 0 {
		return s.pruneVersions(entryID)
	}

	return nil
}

// GetHistory returns all versions of an entry, newest first
func (s *Store) GetHistory(entryID uuid.UUID) ([]Version, error) {
	rows, err := s.db.Query(`
		SELECT id, entry_id, content, tags, timestamp, created_at, author
		FROM entry_versions
		WHERE entry_id = ?
		ORDER BY timestamp DESC
	`, entryID.String())
	if err != nil {
		return nil, fmt.Errorf("failed to get history: %w", err)
	}
	defer rows.Close()

	var versions []Version
	for rows.Next() {
		var v Version
		var entryIDStr string
		var tagsJSON []byte
		var createdAtUnix int64
		var author sql.NullString

		if err := rows.Scan(&v.ID, &entryIDStr, &v.Content, &tagsJSON, &v.Timestamp, &createdAtUnix, &author); err != nil {
			return nil, err
		}

		v.EntryID, _ = uuid.Parse(entryIDStr)
		v.CreatedAt = time.Unix(createdAtUnix, 0)
		json.Unmarshal(tagsJSON, &v.Tags)
		if author.Valid {
			v.Author = author.String
		}

		versions = append(versions, v)
	}

	return versions, nil
}

// GetVersion retrieves a specific version
func (s *Store) GetVersion(entryID uuid.UUID, versionID int64) (*Version, error) {
	var v Version
	var entryIDStr string
	var tagsJSON []byte
	var createdAtUnix int64
	var author sql.NullString

	err := s.db.QueryRow(`
		SELECT id, entry_id, content, tags, timestamp, created_at, author
		FROM entry_versions
		WHERE entry_id = ? AND id = ?
	`, entryID.String(), versionID).Scan(&v.ID, &entryIDStr, &v.Content, &tagsJSON, &v.Timestamp, &createdAtUnix, &author)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("version not found")
	}
	if err != nil {
		return nil, err
	}

	v.EntryID, _ = uuid.Parse(entryIDStr)
	v.CreatedAt = time.Unix(createdAtUnix, 0)
	json.Unmarshal(tagsJSON, &v.Tags)
	if author.Valid {
		v.Author = author.String
	}

	return &v, nil
}

// GetVersionAt retrieves the version at a specific timestamp
func (s *Store) GetVersionAt(entryID uuid.UUID, timestamp uint64) (*Version, error) {
	var v Version
	var entryIDStr string
	var tagsJSON []byte
	var createdAtUnix int64
	var author sql.NullString

	err := s.db.QueryRow(`
		SELECT id, entry_id, content, tags, timestamp, created_at, author
		FROM entry_versions
		WHERE entry_id = ? AND timestamp <= ?
		ORDER BY timestamp DESC
		LIMIT 1
	`, entryID.String(), timestamp).Scan(&v.ID, &entryIDStr, &v.Content, &tagsJSON, &v.Timestamp, &createdAtUnix, &author)

	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("no version found at timestamp %d", timestamp)
	}
	if err != nil {
		return nil, err
	}

	v.EntryID, _ = uuid.Parse(entryIDStr)
	v.CreatedAt = time.Unix(createdAtUnix, 0)
	json.Unmarshal(tagsJSON, &v.Tags)
	if author.Valid {
		v.Author = author.String
	}

	return &v, nil
}

// GetVersionCount returns the number of versions for an entry
func (s *Store) GetVersionCount(entryID uuid.UUID) (int, error) {
	var count int
	err := s.db.QueryRow(`
		SELECT COUNT(*) FROM entry_versions WHERE entry_id = ?
	`, entryID.String()).Scan(&count)
	return count, err
}

// DeleteVersions removes all versions for an entry
func (s *Store) DeleteVersions(entryID uuid.UUID) error {
	_, err := s.db.Exec(`DELETE FROM entry_versions WHERE entry_id = ?`, entryID.String())
	return err
}

// pruneVersions removes old versions beyond the limit
func (s *Store) pruneVersions(entryID uuid.UUID) error {
	_, err := s.db.Exec(`
		DELETE FROM entry_versions 
		WHERE entry_id = ? AND id NOT IN (
			SELECT id FROM entry_versions 
			WHERE entry_id = ? 
			ORDER BY timestamp DESC 
			LIMIT ?
		)
	`, entryID.String(), entryID.String(), s.maxVersions)
	return err
}

// ComputeDiff computes the difference between two versions
func ComputeDiff(old, new *Version) *Diff {
	diff := &Diff{
		OldVersion: old.ID,
		NewVersion: new.ID,
	}

	// Simple content diff (just show if changed)
	if string(old.Content) != string(new.Content) {
		diff.ContentDiff = "content changed"
	}

	// Compute tag changes
	oldTags := make(map[string]bool)
	for _, t := range old.Tags {
		oldTags[t] = true
	}

	newTags := make(map[string]bool)
	for _, t := range new.Tags {
		newTags[t] = true
	}

	for t := range newTags {
		if !oldTags[t] {
			diff.TagsAdded = append(diff.TagsAdded, t)
		}
	}

	for t := range oldTags {
		if !newTags[t] {
			diff.TagsRemoved = append(diff.TagsRemoved, t)
		}
	}

	return diff
}
