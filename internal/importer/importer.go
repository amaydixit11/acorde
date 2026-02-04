// Package importer provides import/export functionality for acorde.
package importer

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/google/uuid"
)

// ExportEntry represents an entry for import/export
type ExportEntry struct {
	ID        string    `json:"id"`
	Type      string    `json:"type"`
	Content   string    `json:"content"`
	Tags      []string  `json:"tags"`
	CreatedAt uint64    `json:"created_at"`
	UpdatedAt uint64    `json:"updated_at"`
	Metadata  map[string]interface{} `json:"metadata,omitempty"`
}

// ExportData represents a full vault export
type ExportData struct {
	Version   string        `json:"version"`
	ExportedAt time.Time    `json:"exported_at"`
	EntryCount int          `json:"entry_count"`
	Entries   []ExportEntry `json:"entries"`
}

// ExportFormat specifies the export format
type ExportFormat string

const (
	FormatJSON     ExportFormat = "json"
	FormatMarkdown ExportFormat = "markdown"
	FormatCSV      ExportFormat = "csv"
)

// Exporter handles exporting entries
type Exporter struct{}

// NewExporter creates a new exporter
func NewExporter() *Exporter {
	return &Exporter{}
}

// ExportToJSON exports entries to JSON format
func (e *Exporter) ExportToJSON(entries []ExportEntry, w io.Writer) error {
	export := ExportData{
		Version:    "1.0",
		ExportedAt: time.Now(),
		EntryCount: len(entries),
		Entries:    entries,
	}

	encoder := json.NewEncoder(w)
	encoder.SetIndent("", "  ")
	return encoder.Encode(export)
}

// ExportToMarkdown exports note entries to Markdown files
func (e *Exporter) ExportToMarkdown(entries []ExportEntry, dir string) error {
	if err := os.MkdirAll(dir, 0755); err != nil {
		return fmt.Errorf("failed to create directory: %w", err)
	}

	for _, entry := range entries {
		if entry.Type != "note" {
			continue
		}

		filename := sanitizeFilename(entry.ID) + ".md"
		path := dir + "/" + filename

		var content strings.Builder
		
		// Add frontmatter
		content.WriteString("---\n")
		content.WriteString(fmt.Sprintf("id: %s\n", entry.ID))
		content.WriteString(fmt.Sprintf("type: %s\n", entry.Type))
		if len(entry.Tags) > 0 {
			content.WriteString(fmt.Sprintf("tags: [%s]\n", strings.Join(entry.Tags, ", ")))
		}
		content.WriteString(fmt.Sprintf("created: %d\n", entry.CreatedAt))
		content.WriteString(fmt.Sprintf("updated: %d\n", entry.UpdatedAt))
		content.WriteString("---\n\n")
		
		// Add content
		content.WriteString(entry.Content)
		content.WriteString("\n")

		if err := os.WriteFile(path, []byte(content.String()), 0644); err != nil {
			return fmt.Errorf("failed to write %s: %w", filename, err)
		}
	}

	return nil
}

// ExportToCSV exports structured entries to CSV
func (e *Exporter) ExportToCSV(entries []ExportEntry, w io.Writer) error {
	writer := csv.NewWriter(w)
	defer writer.Flush()

	// Header
	header := []string{"id", "type", "content", "tags", "created_at", "updated_at"}
	if err := writer.Write(header); err != nil {
		return err
	}

	// Rows
	for _, entry := range entries {
		row := []string{
			entry.ID,
			entry.Type,
			entry.Content,
			strings.Join(entry.Tags, ";"),
			fmt.Sprintf("%d", entry.CreatedAt),
			fmt.Sprintf("%d", entry.UpdatedAt),
		}
		if err := writer.Write(row); err != nil {
			return err
		}
	}

	return nil
}

// Importer handles importing entries
type Importer struct{}

// NewImporter creates a new importer
func NewImporter() *Importer {
	return &Importer{}
}

// ImportResult contains import statistics
type ImportResult struct {
	TotalRead    int      `json:"total_read"`
	Imported     int      `json:"imported"`
	Skipped      int      `json:"skipped"`
	Failed       int      `json:"failed"`
	Errors       []string `json:"errors,omitempty"`
}

// ImportFromJSON imports entries from JSON
func (i *Importer) ImportFromJSON(r io.Reader) ([]ExportEntry, error) {
	var data ExportData
	decoder := json.NewDecoder(r)
	if err := decoder.Decode(&data); err != nil {
		// Try parsing as array directly
		var entries []ExportEntry
		r2 := io.MultiReader(r)
		decoder2 := json.NewDecoder(r2)
		if err2 := decoder2.Decode(&entries); err2 != nil {
			return nil, fmt.Errorf("invalid JSON format: %w", err)
		}
		return entries, nil
	}
	return data.Entries, nil
}

// ImportFromCSV imports entries from CSV
func (i *Importer) ImportFromCSV(r io.Reader) ([]ExportEntry, error) {
	reader := csv.NewReader(r)

	// Read header
	header, err := reader.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read header: %w", err)
	}

	// Find column indices
	indices := make(map[string]int)
	for i, col := range header {
		indices[strings.ToLower(col)] = i
	}

	var entries []ExportEntry
	for {
		record, err := reader.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}

		entry := ExportEntry{}
		
		if idx, ok := indices["id"]; ok && idx < len(record) {
			entry.ID = record[idx]
		} else {
			entry.ID = uuid.New().String()
		}
		
		if idx, ok := indices["type"]; ok && idx < len(record) {
			entry.Type = record[idx]
		} else {
			entry.Type = "note"
		}
		
		if idx, ok := indices["content"]; ok && idx < len(record) {
			entry.Content = record[idx]
		}
		
		if idx, ok := indices["tags"]; ok && idx < len(record) {
			if record[idx] != "" {
				entry.Tags = strings.Split(record[idx], ";")
			}
		}

		entries = append(entries, entry)
	}

	return entries, nil
}

// ImportFromMarkdown imports entries from a Markdown file
func (i *Importer) ImportFromMarkdown(r io.Reader) (*ExportEntry, error) {
	content, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	entry := &ExportEntry{
		ID:      uuid.New().String(),
		Type:    "note",
		Content: string(content),
	}

	// Try to parse frontmatter
	text := string(content)
	if strings.HasPrefix(text, "---") {
		parts := strings.SplitN(text, "---", 3)
		if len(parts) >= 3 {
			// Parse frontmatter
			frontmatter := parts[1]
			entry.Content = strings.TrimSpace(parts[2])

			for _, line := range strings.Split(frontmatter, "\n") {
				line = strings.TrimSpace(line)
				if strings.HasPrefix(line, "id:") {
					entry.ID = strings.TrimSpace(strings.TrimPrefix(line, "id:"))
				}
				if strings.HasPrefix(line, "type:") {
					entry.Type = strings.TrimSpace(strings.TrimPrefix(line, "type:"))
				}
				if strings.HasPrefix(line, "tags:") {
					tagsStr := strings.TrimSpace(strings.TrimPrefix(line, "tags:"))
					tagsStr = strings.Trim(tagsStr, "[]")
					for _, tag := range strings.Split(tagsStr, ",") {
						tag = strings.TrimSpace(tag)
						if tag != "" {
							entry.Tags = append(entry.Tags, tag)
						}
					}
				}
			}
		}
	}

	return entry, nil
}

func sanitizeFilename(s string) string {
	// Replace invalid characters
	replacer := strings.NewReplacer(
		"/", "_",
		"\\", "_",
		":", "_",
		"*", "_",
		"?", "_",
		"\"", "_",
		"<", "_",
		">", "_",
		"|", "_",
	)
	return replacer.Replace(s)
}
