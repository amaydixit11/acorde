// Package search provides full-text search functionality using Bleve.
package search

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/blevesearch/bleve/v2"
	"github.com/google/uuid"
)

// Index wraps Bleve for full-text search
type Index struct {
	index bleve.Index
	path  string
}

// Document represents a searchable document
type Document struct {
	ID      string   `json:"id"`
	Type    string   `json:"type"`
	Content string   `json:"content"`
	Tags    []string `json:"tags"`
}

// NewIndex creates or opens a Bleve index at the given path
func NewIndex(dataDir string) (*Index, error) {
	indexPath := filepath.Join(dataDir, "search.bleve")

	var idx bleve.Index
	var err error

	// Try to open existing index
	idx, err = bleve.Open(indexPath)
	if err == bleve.ErrorIndexPathDoesNotExist {
		// Create new index
		mapping := bleve.NewIndexMapping()
		
		// Custom document mapping for better search
		docMapping := bleve.NewDocumentMapping()
		
		// Content field - full text searchable
		contentField := bleve.NewTextFieldMapping()
		contentField.Analyzer = "standard"
		docMapping.AddFieldMappingsAt("content", contentField)
		
		// Tags field - keyword searchable
		tagsField := bleve.NewTextFieldMapping()
		tagsField.Analyzer = "keyword"
		docMapping.AddFieldMappingsAt("tags", tagsField)
		
		// Type field - keyword
		typeField := bleve.NewTextFieldMapping()
		typeField.Analyzer = "keyword"
		docMapping.AddFieldMappingsAt("type", typeField)
		
		mapping.AddDocumentMapping("entry", docMapping)
		
		idx, err = bleve.New(indexPath, mapping)
		if err != nil {
			return nil, fmt.Errorf("failed to create index: %w", err)
		}
	} else if err != nil {
		return nil, fmt.Errorf("failed to open index: %w", err)
	}

	return &Index{
		index: idx,
		path:  indexPath,
	}, nil
}

// NewMemoryIndex creates an in-memory index for testing
func NewMemoryIndex() (*Index, error) {
	mapping := bleve.NewIndexMapping()
	idx, err := bleve.NewMemOnly(mapping)
	if err != nil {
		return nil, err
	}
	return &Index{index: idx}, nil
}

// IndexDocument adds or updates a document in the index
func (i *Index) IndexDocument(id uuid.UUID, entryType string, content []byte, tags []string) error {
	doc := Document{
		ID:      id.String(),
		Type:    entryType,
		Content: string(content),
		Tags:    tags,
	}
	return i.index.Index(id.String(), doc)
}

// DeleteDocument removes a document from the index
func (i *Index) DeleteDocument(id uuid.UUID) error {
	return i.index.Delete(id.String())
}

// SearchOptions configures a search query
type SearchOptions struct {
	Type  string   // Filter by entry type
	Tags  []string // Filter by tags
	Limit int      // Max results (default 50)
}

// SearchResult represents a search hit
type SearchResult struct {
	ID    uuid.UUID
	Score float64
}

// Search performs a full-text search
func (i *Index) Search(query string, opts SearchOptions) ([]SearchResult, error) {
	// Build query
	q := bleve.NewMatchQuery(query)
	q.SetField("content")

	searchReq := bleve.NewSearchRequest(q)
	searchReq.Size = opts.Limit
	if searchReq.Size <= 0 {
		searchReq.Size = 50
	}

	// Execute search
	searchRes, err := i.index.Search(searchReq)
	if err != nil {
		return nil, fmt.Errorf("search failed: %w", err)
	}

	// Convert results
	results := make([]SearchResult, 0, len(searchRes.Hits))
	for _, hit := range searchRes.Hits {
		id, err := uuid.Parse(hit.ID)
		if err != nil {
			continue
		}
		results = append(results, SearchResult{
			ID:    id,
			Score: hit.Score,
		})
	}

	return results, nil
}

// Close closes the index
func (i *Index) Close() error {
	return i.index.Close()
}

// Delete removes the index from disk
func (i *Index) Delete() error {
	i.index.Close()
	if i.path != "" {
		return os.RemoveAll(i.path)
	}
	return nil
}
