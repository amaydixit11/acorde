package sqlite

import (
	"database/sql"
	"fmt"
	"strings"

	"github.com/amaydixit11/acorde/internal/core"
	"github.com/amaydixit11/acorde/internal/storage"
	"github.com/google/uuid"
	_ "github.com/mattn/go-sqlite3"
)

// SQLiteStore implements the Store interface using SQLite
type SQLiteStore struct {
	db *sql.DB
}

// New creates a new SQLite store at the given path
// If path is ":memory:", creates an in-memory database
func New(path string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite3", path+"?_foreign_keys=on")
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	store := &SQLiteStore{db: db}
	if err := store.initSchema(); err != nil {
		db.Close()
		return nil, fmt.Errorf("failed to initialize schema: %w", err)
	}

	return store, nil
}

// GetDB returns the underlying SQL database
func (s *SQLiteStore) GetDB() *sql.DB {
	return s.db
}

// initSchema creates the database tables if they don't exist
func (s *SQLiteStore) initSchema() error {
	schema := `
		CREATE TABLE IF NOT EXISTS entries (
			id TEXT PRIMARY KEY,
			type TEXT NOT NULL,
			content BLOB NOT NULL,
			created_at INTEGER NOT NULL,
			updated_at INTEGER NOT NULL,
			deleted INTEGER NOT NULL DEFAULT 0
		);

		CREATE TABLE IF NOT EXISTS tags (
			entry_id TEXT NOT NULL,
			tag TEXT NOT NULL,
			PRIMARY KEY (entry_id, tag),
			FOREIGN KEY (entry_id) REFERENCES entries(id) ON DELETE CASCADE
		);

		CREATE INDEX IF NOT EXISTS idx_entries_type ON entries(type);
		CREATE INDEX IF NOT EXISTS idx_entries_updated ON entries(updated_at);
		CREATE INDEX IF NOT EXISTS idx_entries_deleted ON entries(deleted);
		CREATE INDEX IF NOT EXISTS idx_tags_tag ON tags(tag);
	`
	_, err := s.db.Exec(schema)
	return err
}

// Put stores an entry with its tags (idempotent - upsert)
func (s *SQLiteStore) Put(entry core.Entry) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	// Upsert entry
	_, err = tx.Exec(`
		INSERT INTO entries (id, type, content, created_at, updated_at, deleted)
		VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT(id) DO UPDATE SET
			type = excluded.type,
			content = excluded.content,
			updated_at = excluded.updated_at,
			deleted = excluded.deleted
	`, entry.ID.String(), string(entry.Type), entry.Content,
		entry.CreatedAt, entry.UpdatedAt, boolToInt(entry.Deleted))
	if err != nil {
		return fmt.Errorf("failed to upsert entry: %w", err)
	}

	// Delete existing tags and insert new ones
	_, err = tx.Exec("DELETE FROM tags WHERE entry_id = ?", entry.ID.String())
	if err != nil {
		return fmt.Errorf("failed to delete existing tags: %w", err)
	}

	for _, tag := range entry.Tags {
		_, err = tx.Exec("INSERT INTO tags (entry_id, tag) VALUES (?, ?)",
			entry.ID.String(), tag)
		if err != nil {
			return fmt.Errorf("failed to insert tag: %w", err)
		}
	}

	return tx.Commit()
}

// Get retrieves an entry by ID
func (s *SQLiteStore) Get(id uuid.UUID) (core.Entry, error) {
	var entry core.Entry
	var idStr, typeStr string
	var deleted int

	err := s.db.QueryRow(`
		SELECT id, type, content, created_at, updated_at, deleted
		FROM entries
		WHERE id = ?
	`, id.String()).Scan(&idStr, &typeStr, &entry.Content,
		&entry.CreatedAt, &entry.UpdatedAt, &deleted)

	if err == sql.ErrNoRows {
		return core.Entry{}, storage.ErrNotFound{ID: id}
	}
	if err != nil {
		return core.Entry{}, fmt.Errorf("failed to get entry: %w", err)
	}

	entry.ID = id
	entry.Type = core.EntryType(typeStr)
	entry.Deleted = deleted != 0

	// Get tags
	rows, err := s.db.Query("SELECT tag FROM tags WHERE entry_id = ?", id.String())
	if err != nil {
		return core.Entry{}, fmt.Errorf("failed to get tags: %w", err)
	}
	defer rows.Close()

	entry.Tags = []string{}
	for rows.Next() {
		var tag string
		if err := rows.Scan(&tag); err != nil {
			return core.Entry{}, fmt.Errorf("failed to scan tag: %w", err)
		}
		entry.Tags = append(entry.Tags, tag)
	}

	return entry, nil
}

// List returns entries matching the filter
func (s *SQLiteStore) List(filter storage.ListFilter) ([]core.Entry, error) {
	query := "SELECT id, type, content, created_at, updated_at, deleted FROM entries WHERE 1=1"
	args := []interface{}{}

	if filter.Type != nil {
		query += " AND type = ?"
		args = append(args, string(*filter.Type))
	}
	if !filter.Deleted {
		query += " AND deleted = 0"
	}
	if filter.Since != nil {
		query += " AND updated_at >= ?"
		args = append(args, *filter.Since)
	}
	if filter.Until != nil {
		query += " AND updated_at <= ?"
		args = append(args, *filter.Until)
	}
	if filter.Tag != nil {
		query += " AND id IN (SELECT entry_id FROM tags WHERE tag = ?)"
		args = append(args, *filter.Tag)
	}

	query += " ORDER BY updated_at DESC"

	if filter.Limit > 0 {
		query += " LIMIT ?"
		args = append(args, filter.Limit)
	}
	if filter.Offset > 0 {
		query += " OFFSET ?"
		args = append(args, filter.Offset)
	}

	rows, err := s.db.Query(query, args...)
	if err != nil {
		return nil, fmt.Errorf("failed to list entries: %w", err)
	}
	defer rows.Close()

	entries := []core.Entry{}
	for rows.Next() {
		var entry core.Entry
		var idStr, typeStr string
		var deleted int

		if err := rows.Scan(&idStr, &typeStr, &entry.Content,
			&entry.CreatedAt, &entry.UpdatedAt, &deleted); err != nil {
			return nil, fmt.Errorf("failed to scan entry: %w", err)
		}

		entry.ID, _ = uuid.Parse(idStr)
		entry.Type = core.EntryType(typeStr)
		entry.Deleted = deleted != 0
		entries = append(entries, entry)
	}

	// Batch load tags for all entries
	if len(entries) > 0 {
		ids := make([]string, len(entries))
		for i, e := range entries {
			ids[i] = e.ID.String()
		}

		tagQuery := fmt.Sprintf(
			"SELECT entry_id, tag FROM tags WHERE entry_id IN (%s)",
			strings.Repeat("?,", len(ids)-1)+"?",
		)
		tagArgs := make([]interface{}, len(ids))
		for i, id := range ids {
			tagArgs[i] = id
		}

		tagRows, err := s.db.Query(tagQuery, tagArgs...)
		if err != nil {
			return nil, fmt.Errorf("failed to load tags: %w", err)
		}
		defer tagRows.Close()

		tagMap := make(map[string][]string)
		for tagRows.Next() {
			var entryID, tag string
			if err := tagRows.Scan(&entryID, &tag); err != nil {
				return nil, fmt.Errorf("failed to scan tag: %w", err)
			}
			tagMap[entryID] = append(tagMap[entryID], tag)
		}

		for i := range entries {
			entries[i].Tags = tagMap[entries[i].ID.String()]
			if entries[i].Tags == nil {
				entries[i].Tags = []string{}
			}
		}
	}

	return entries, nil
}

// Delete marks an entry as deleted (tombstone)
func (s *SQLiteStore) Delete(id uuid.UUID) error {
	result, err := s.db.Exec(
		"UPDATE entries SET deleted = 1 WHERE id = ?",
		id.String(),
	)
	if err != nil {
		return fmt.Errorf("failed to delete entry: %w", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("failed to get rows affected: %w", err)
	}
	if rows == 0 {
		return storage.ErrNotFound{ID: id}
	}

	return nil
}

// ApplyBatch applies multiple operations atomically
func (s *SQLiteStore) ApplyBatch(ops []storage.Operation) error {
	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	for _, op := range ops {
		switch op.Type {
		case storage.OpPut:
			// Upsert entry
			_, err = tx.Exec(`
				INSERT INTO entries (id, type, content, created_at, updated_at, deleted)
				VALUES (?, ?, ?, ?, ?, ?)
				ON CONFLICT(id) DO UPDATE SET
					type = excluded.type,
					content = excluded.content,
					updated_at = excluded.updated_at,
					deleted = excluded.deleted
			`, op.Entry.ID.String(), string(op.Entry.Type), op.Entry.Content,
				op.Entry.CreatedAt, op.Entry.UpdatedAt, boolToInt(op.Entry.Deleted))
			if err != nil {
				return fmt.Errorf("failed to put entry in batch: %w", err)
			}

			// Update tags
			_, err = tx.Exec("DELETE FROM tags WHERE entry_id = ?", op.Entry.ID.String())
			if err != nil {
				return fmt.Errorf("failed to delete tags in batch: %w", err)
			}
			for _, tag := range op.Entry.Tags {
				_, err = tx.Exec("INSERT INTO tags (entry_id, tag) VALUES (?, ?)",
					op.Entry.ID.String(), tag)
				if err != nil {
					return fmt.Errorf("failed to insert tag in batch: %w", err)
				}
			}

		case storage.OpDelete:
			_, err = tx.Exec("UPDATE entries SET deleted = 1 WHERE id = ?",
				op.Entry.ID.String())
			if err != nil {
				return fmt.Errorf("failed to delete entry in batch: %w", err)
			}
		}
	}

	return tx.Commit()
}

// GetMaxTimestamp returns the highest UpdatedAt timestamp in storage
func (s *SQLiteStore) GetMaxTimestamp() (uint64, error) {
	var maxTime sql.NullInt64
	err := s.db.QueryRow("SELECT MAX(updated_at) FROM entries").Scan(&maxTime)
	if err != nil {
		return 0, fmt.Errorf("failed to get max timestamp: %w", err)
	}
	if !maxTime.Valid {
		return 0, nil
	}
	return uint64(maxTime.Int64), nil
}

// Close closes the database connection
func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

func boolToInt(b bool) int {
	if b {
		return 1
	}
	return 0
}

// SearchOptions for full-text search
type SearchOptions struct {
	Type  *core.EntryType
	Limit int
}

// Search performs full-text search on entry content
func (s *SQLiteStore) Search(query string, opts SearchOptions) ([]core.Entry, error) {
	// Use FTS5 MATCH query
	sqlQuery := `
		SELECT e.id, e.type, e.content, e.created_at, e.updated_at, e.deleted
		FROM entries e
		JOIN entries_fts fts ON e.rowid = fts.rowid
		WHERE entries_fts MATCH ? AND e.deleted = 0
	`
	args := []interface{}{query}

	if opts.Type != nil {
		sqlQuery += " AND e.type = ?"
		args = append(args, string(*opts.Type))
	}

	sqlQuery += " ORDER BY rank LIMIT ?"
	limit := opts.Limit
	if limit <= 0 {
		limit = 50
	}
	args = append(args, limit)

	rows, err := s.db.Query(sqlQuery, args...)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}
	defer rows.Close()

	entries := []core.Entry{}
	for rows.Next() {
		var entry core.Entry
		var idStr, typeStr string
		var deleted int

		if err := rows.Scan(&idStr, &typeStr, &entry.Content,
			&entry.CreatedAt, &entry.UpdatedAt, &deleted); err != nil {
			return nil, fmt.Errorf("failed to scan entry: %w", err)
		}

		entry.ID, _ = uuid.Parse(idStr)
		entry.Type = core.EntryType(typeStr)
		entry.Deleted = deleted != 0
		entry.Tags = []string{} // Tags loaded separately if needed
		entries = append(entries, entry)
	}

	return entries, nil
}

