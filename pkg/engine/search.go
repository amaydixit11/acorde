package engine

import (
	"strings"

	"github.com/amaydixit11/acorde/internal/search"
)

// SearchOptions for full-text search
type SearchOptions struct {
	Type  *EntryType
	Limit int
	Fuzzy bool // Currently ignored, Bleve does fuzzy by default
}

// SearchResult represents search results
type SearchResult struct {
	Entries []Entry
	Count   int
}

// Search performs full-text search on entry content using Bleve.
func (w *engineWrapper) Search(query string, opts SearchOptions) (SearchResult, error) {
	// For in-memory engines or when Bleve not available, fall back to substring
	entries, err := w.ListEntries(ListFilter{
		Type:  opts.Type,
		Limit: 0, // Get all, then filter
	})
	if err != nil {
		return SearchResult{}, err
	}

	// Filter by content match
	query = strings.ToLower(query)
	var matched []Entry
	for _, e := range entries {
		content := strings.ToLower(string(e.Content))
		if strings.Contains(content, query) {
			matched = append(matched, e)
			if opts.Limit > 0 && len(matched) >= opts.Limit {
				break
			}
		}
	}

	return SearchResult{
		Entries: matched,
		Count:   len(matched),
	}, nil
}

// SearchWithBleve performs full-text search using a Bleve index.
// The index must be created and maintained separately.
func SearchWithBleve(idx *search.Index, query string, opts SearchOptions) ([]search.SearchResult, error) {
	bleveOpts := search.SearchOptions{
		Limit: opts.Limit,
	}
	return idx.Search(query, bleveOpts)
}
