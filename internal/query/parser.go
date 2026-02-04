// Package query provides SQL-like query parsing and execution.
package query

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"
)

// Query represents a parsed query
type Query struct {
	Type        *string           // Entry type filter
	Tags        []string          // Tags to include (AND)
	TagsAny     []string          // Tags to include (OR)
	ContentLike *string           // Content LIKE pattern
	CreatedAfter  *time.Time
	CreatedBefore *time.Time
	UpdatedAfter  *time.Time
	UpdatedBefore *time.Time
	Deleted     *bool
	OrderBy     []OrderClause
	Limit       int
	Offset      int
}

// OrderClause specifies ordering
type OrderClause struct {
	Field string
	Desc  bool
}

// Parser parses SQL-like query strings
type Parser struct{}

// NewParser creates a new query parser
func NewParser() *Parser {
	return &Parser{}
}

// Parse parses a query string into a Query struct
func (p *Parser) Parse(queryStr string) (*Query, error) {
	q := &Query{}
	queryStr = strings.TrimSpace(queryStr)

	// Handle SELECT prefix (optional)
	if strings.HasPrefix(strings.ToUpper(queryStr), "SELECT") {
		re := regexp.MustCompile(`(?i)^SELECT\s+\*\s+FROM\s+entries\s*`)
		queryStr = re.ReplaceAllString(queryStr, "")
	}

	// Parse WHERE clause
	whereMatch := regexp.MustCompile(`(?i)^WHERE\s+(.+?)(?:\s+ORDER\s+BY|\s+LIMIT|\s+OFFSET|$)`).FindStringSubmatch(queryStr)
	if whereMatch != nil {
		if err := p.parseWhere(whereMatch[1], q); err != nil {
			return nil, err
		}
	} else {
		// Try without WHERE keyword
		if err := p.parseWhere(queryStr, q); err != nil {
			// Not an error, might just be ORDER BY or LIMIT
		}
	}

	// Parse ORDER BY
	orderMatch := regexp.MustCompile(`(?i)ORDER\s+BY\s+(.+?)(?:\s+LIMIT|\s+OFFSET|$)`).FindStringSubmatch(queryStr)
	if orderMatch != nil {
		q.OrderBy = p.parseOrderBy(orderMatch[1])
	}

	// Parse LIMIT
	limitMatch := regexp.MustCompile(`(?i)LIMIT\s+(\d+)`).FindStringSubmatch(queryStr)
	if limitMatch != nil {
		q.Limit, _ = strconv.Atoi(limitMatch[1])
	}

	// Parse OFFSET
	offsetMatch := regexp.MustCompile(`(?i)OFFSET\s+(\d+)`).FindStringSubmatch(queryStr)
	if offsetMatch != nil {
		q.Offset, _ = strconv.Atoi(offsetMatch[1])
	}

	return q, nil
}

func (p *Parser) parseWhere(whereClause string, q *Query) error {
	// Split by AND (case insensitive)
	conditions := regexp.MustCompile(`(?i)\s+AND\s+`).Split(whereClause, -1)

	for _, cond := range conditions {
		cond = strings.TrimSpace(cond)
		if cond == "" {
			continue
		}

		// Handle OR within parentheses
		if strings.Contains(strings.ToUpper(cond), " OR ") {
			orParts := regexp.MustCompile(`(?i)\s+OR\s+`).Split(cond, -1)
			for _, orPart := range orParts {
				if err := p.parseCondition(strings.TrimSpace(orPart), q, true); err != nil {
					return err
				}
			}
		} else {
			if err := p.parseCondition(cond, q, false); err != nil {
				return err
			}
		}
	}

	return nil
}

func (p *Parser) parseCondition(cond string, q *Query, isOr bool) error {
	cond = strings.TrimSpace(cond)
	cond = strings.Trim(cond, "()")

	// type = "note"
	if match := regexp.MustCompile(`(?i)^type\s*=\s*['""]?(\w+)['""]?$`).FindStringSubmatch(cond); match != nil {
		t := match[1]
		q.Type = &t
		return nil
	}

	// tag = "work" or tags CONTAINS "work"
	if match := regexp.MustCompile(`(?i)^tags?\s*(?:=|CONTAINS)\s*['""]?(\w+)['""]?$`).FindStringSubmatch(cond); match != nil {
		tag := match[1]
		if isOr {
			q.TagsAny = append(q.TagsAny, tag)
		} else {
			q.Tags = append(q.Tags, tag)
		}
		return nil
	}

	// content LIKE "%pattern%"
	if match := regexp.MustCompile(`(?i)^content\s+LIKE\s+['""](.+)['""]$`).FindStringSubmatch(cond); match != nil {
		pattern := match[1]
		q.ContentLike = &pattern
		return nil
	}

	// created_at > "2024-01-01" or created_at > 1700000000
	if match := regexp.MustCompile(`(?i)^created_at\s*([<>=!]+)\s*['""]?(.+?)['""]?$`).FindStringSubmatch(cond); match != nil {
		op := match[1]
		value := match[2]
		t, err := p.parseTime(value)
		if err != nil {
			return err
		}
		switch op {
		case ">", ">=":
			q.CreatedAfter = &t
		case "<", "<=":
			q.CreatedBefore = &t
		}
		return nil
	}

	// updated_at > "2024-01-01"
	if match := regexp.MustCompile(`(?i)^updated_at\s*([<>=!]+)\s*['""]?(.+?)['""]?$`).FindStringSubmatch(cond); match != nil {
		op := match[1]
		value := match[2]
		t, err := p.parseTime(value)
		if err != nil {
			return err
		}
		switch op {
		case ">", ">=":
			q.UpdatedAfter = &t
		case "<", "<=":
			q.UpdatedBefore = &t
		}
		return nil
	}

	// deleted = true/false
	if match := regexp.MustCompile(`(?i)^deleted\s*=\s*(true|false|1|0)$`).FindStringSubmatch(cond); match != nil {
		deleted := match[1] == "true" || match[1] == "1"
		q.Deleted = &deleted
		return nil
	}

	// Unknown condition - ignore for flexibility
	return nil
}

func (p *Parser) parseTime(value string) (time.Time, error) {
	// Try unix timestamp
	if ts, err := strconv.ParseInt(value, 10, 64); err == nil {
		return time.Unix(ts, 0), nil
	}

	// Try ISO date
	formats := []string{
		"2006-01-02",
		"2006-01-02T15:04:05",
		"2006-01-02T15:04:05Z",
		time.RFC3339,
	}

	for _, format := range formats {
		if t, err := time.Parse(format, value); err == nil {
			return t, nil
		}
	}

	return time.Time{}, fmt.Errorf("invalid time format: %s", value)
}

func (p *Parser) parseOrderBy(orderClause string) []OrderClause {
	var clauses []OrderClause

	parts := strings.Split(orderClause, ",")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		fields := strings.Fields(part)
		if len(fields) == 0 {
			continue
		}

		clause := OrderClause{Field: fields[0]}
		if len(fields) > 1 && strings.ToUpper(fields[1]) == "DESC" {
			clause.Desc = true
		}
		clauses = append(clauses, clause)
	}

	return clauses
}

// ToSQL generates a SQL WHERE clause (for SQLite)
func (q *Query) ToSQL() (string, []interface{}) {
	var conditions []string
	var args []interface{}

	if q.Type != nil {
		conditions = append(conditions, "type = ?")
		args = append(args, *q.Type)
	}

	if q.ContentLike != nil {
		conditions = append(conditions, "content LIKE ?")
		args = append(args, *q.ContentLike)
	}

	if q.CreatedAfter != nil {
		conditions = append(conditions, "created_at > ?")
		args = append(args, q.CreatedAfter.Unix())
	}

	if q.CreatedBefore != nil {
		conditions = append(conditions, "created_at < ?")
		args = append(args, q.CreatedBefore.Unix())
	}

	if q.UpdatedAfter != nil {
		conditions = append(conditions, "updated_at > ?")
		args = append(args, q.UpdatedAfter.Unix())
	}

	if q.UpdatedBefore != nil {
		conditions = append(conditions, "updated_at < ?")
		args = append(args, q.UpdatedBefore.Unix())
	}

	if q.Deleted != nil {
		conditions = append(conditions, "deleted = ?")
		if *q.Deleted {
			args = append(args, 1)
		} else {
			args = append(args, 0)
		}
	}

	sql := ""
	if len(conditions) > 0 {
		sql = "WHERE " + strings.Join(conditions, " AND ")
	}

	// ORDER BY
	if len(q.OrderBy) > 0 {
		var orderParts []string
		for _, o := range q.OrderBy {
			if o.Desc {
				orderParts = append(orderParts, o.Field+" DESC")
			} else {
				orderParts = append(orderParts, o.Field+" ASC")
			}
		}
		sql += " ORDER BY " + strings.Join(orderParts, ", ")
	}

	// LIMIT
	if q.Limit > 0 {
		sql += fmt.Sprintf(" LIMIT %d", q.Limit)
	}

	// OFFSET
	if q.Offset > 0 {
		sql += fmt.Sprintf(" OFFSET %d", q.Offset)
	}

	return sql, args
}
