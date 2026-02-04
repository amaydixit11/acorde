package engine

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
)

// QueryResult contains the result of a query
type QueryResult struct {
	Entries []Entry
	Count   int
}

// Query executes a query string and returns matching entries.
// Query syntax:
//   type = "note"
//   tags CONTAINS "work"
//   created_at > 1700000000
//   ORDER BY updated_at DESC
//   LIMIT 20
func (w *engineWrapper) Query(q string) (QueryResult, error) {
	filter, err := parseQuery(q)
	if err != nil {
		return QueryResult{}, err
	}

	entries, err := w.ListEntries(filter)
	if err != nil {
		return QueryResult{}, err
	}

	return QueryResult{
		Entries: entries,
		Count:   len(entries),
	}, nil
}

// parseQuery parses a simple query string into a ListFilter
func parseQuery(q string) (ListFilter, error) {
	filter := ListFilter{}

	// Normalize whitespace
	q = strings.TrimSpace(q)
	if q == "" {
		return filter, nil
	}

	// Parse LIMIT
	limitRe := regexp.MustCompile(`(?i)LIMIT\s+(\d+)`)
	if m := limitRe.FindStringSubmatch(q); len(m) > 1 {
		limit, _ := strconv.Atoi(m[1])
		filter.Limit = limit
		q = limitRe.ReplaceAllString(q, "")
	}

	// Parse ORDER BY (ignored for now, just remove it)
	orderRe := regexp.MustCompile(`(?i)ORDER\s+BY\s+\w+(\s+(ASC|DESC))?`)
	q = orderRe.ReplaceAllString(q, "")

	// Parse conditions
	conditions := strings.Split(q, "AND")
	for _, cond := range conditions {
		cond = strings.TrimSpace(cond)
		if cond == "" {
			continue
		}

		// type = "note"
		if m := regexp.MustCompile(`(?i)type\s*=\s*["']?(\w+)["']?`).FindStringSubmatch(cond); len(m) > 1 {
			entryType := EntryType(m[1])
			filter.Type = &entryType
			continue
		}

		// tags CONTAINS "work"
		if m := regexp.MustCompile(`(?i)tags\s+CONTAINS\s+["']?(\w+)["']?`).FindStringSubmatch(cond); len(m) > 1 {
			tag := m[1]
			filter.Tag = &tag
			continue
		}

		// created_at > 1700000000
		if m := regexp.MustCompile(`(?i)created_at\s*>\s*(\d+)`).FindStringSubmatch(cond); len(m) > 1 {
			ts, _ := strconv.ParseUint(m[1], 10, 64)
			filter.Since = &ts
			continue
		}

		// updated_at < 1700000000
		if m := regexp.MustCompile(`(?i)updated_at\s*<\s*(\d+)`).FindStringSubmatch(cond); len(m) > 1 {
			ts, _ := strconv.ParseUint(m[1], 10, 64)
			filter.Until = &ts
			continue
		}
	}

	return filter, nil
}

// QueryBuilder provides a fluent API for building queries
type QueryBuilder struct {
	filter ListFilter
	engine *engineWrapper
}

// NewQuery creates a new query builder
func (w *engineWrapper) NewQuery() *QueryBuilder {
	return &QueryBuilder{
		filter: ListFilter{},
		engine: w,
	}
}

// Type filters by entry type
func (qb *QueryBuilder) Type(t EntryType) *QueryBuilder {
	qb.filter.Type = &t
	return qb
}

// Tag filters by tag
func (qb *QueryBuilder) Tag(tag string) *QueryBuilder {
	qb.filter.Tag = &tag
	return qb
}

// Since filters entries created after timestamp
func (qb *QueryBuilder) Since(ts uint64) *QueryBuilder {
	qb.filter.Since = &ts
	return qb
}

// Until filters entries created before timestamp
func (qb *QueryBuilder) Until(ts uint64) *QueryBuilder {
	qb.filter.Until = &ts
	return qb
}

// Limit sets max results
func (qb *QueryBuilder) Limit(n int) *QueryBuilder {
	qb.filter.Limit = n
	return qb
}

// Offset sets skip count
func (qb *QueryBuilder) Offset(n int) *QueryBuilder {
	qb.filter.Offset = n
	return qb
}

// Execute runs the query
func (qb *QueryBuilder) Execute() ([]Entry, error) {
	return qb.engine.ListEntries(qb.filter)
}

// String returns the query as a string (for debugging)
func (qb *QueryBuilder) String() string {
	parts := []string{}

	if qb.filter.Type != nil {
		parts = append(parts, fmt.Sprintf("type = \"%s\"", *qb.filter.Type))
	}
	if qb.filter.Tag != nil {
		parts = append(parts, fmt.Sprintf("tags CONTAINS \"%s\"", *qb.filter.Tag))
	}
	if qb.filter.Since != nil {
		parts = append(parts, fmt.Sprintf("created_at > %d", *qb.filter.Since))
	}
	if qb.filter.Until != nil {
		parts = append(parts, fmt.Sprintf("updated_at < %d", *qb.filter.Until))
	}

	q := strings.Join(parts, " AND ")

	if qb.filter.Limit > 0 {
		q += fmt.Sprintf(" LIMIT %d", qb.filter.Limit)
	}

	return q
}
