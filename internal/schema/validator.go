// Package schema provides JSON Schema validation for entry content.
package schema

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/xeipuuv/gojsonschema"
)

// Schema represents a JSON Schema for validating entry content
type Schema struct {
	ID          string          `json:"id"`
	Name        string          `json:"name"`
	Version     int             `json:"version"`
	Definition  json.RawMessage `json:"definition"`
	compiled    *gojsonschema.Schema
}

// ValidationError represents a schema validation error
type ValidationError struct {
	Field       string `json:"field"`
	Description string `json:"description"`
}

func (e ValidationError) Error() string {
	return fmt.Sprintf("%s: %s", e.Field, e.Description)
}

// ValidationResult contains the result of validation
type ValidationResult struct {
	Valid  bool              `json:"valid"`
	Errors []ValidationError `json:"errors,omitempty"`
}

// Registry manages schemas for different entry types
type Registry struct {
	schemas map[string]*Schema // entryType -> schema
	mu      sync.RWMutex
}

// NewRegistry creates a new schema registry
func NewRegistry() *Registry {
	return &Registry{
		schemas: make(map[string]*Schema),
	}
}

// Register adds a schema for an entry type
func (r *Registry) Register(entryType string, schema *Schema) error {
	r.mu.Lock()
	defer r.mu.Unlock()

	// Compile the schema
	loader := gojsonschema.NewBytesLoader(schema.Definition)
	compiled, err := gojsonschema.NewSchema(loader)
	if err != nil {
		return fmt.Errorf("invalid schema: %w", err)
	}
	schema.compiled = compiled

	r.schemas[entryType] = schema
	return nil
}

// RegisterFromJSON registers a schema from JSON definition
func (r *Registry) RegisterFromJSON(entryType, name string, definition []byte) error {
	schema := &Schema{
		ID:         entryType + "-schema",
		Name:       name,
		Version:    1,
		Definition: definition,
	}
	return r.Register(entryType, schema)
}

// Get retrieves a schema for an entry type
func (r *Registry) Get(entryType string) (*Schema, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	schema, ok := r.schemas[entryType]
	return schema, ok
}

// Unregister removes a schema
func (r *Registry) Unregister(entryType string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	delete(r.schemas, entryType)
}

// Validate validates content against the schema for an entry type
func (r *Registry) Validate(entryType string, content []byte) ValidationResult {
	r.mu.RLock()
	schema, ok := r.schemas[entryType]
	r.mu.RUnlock()

	if !ok {
		// No schema registered - validation passes
		return ValidationResult{Valid: true}
	}

	return schema.Validate(content)
}

// Validate validates content against this schema
func (s *Schema) Validate(content []byte) ValidationResult {
	if s.compiled == nil {
		return ValidationResult{Valid: true}
	}

	documentLoader := gojsonschema.NewBytesLoader(content)
	result, err := s.compiled.Validate(documentLoader)
	if err != nil {
		return ValidationResult{
			Valid: false,
			Errors: []ValidationError{{
				Field:       "content",
				Description: fmt.Sprintf("validation error: %v", err),
			}},
		}
	}

	if result.Valid() {
		return ValidationResult{Valid: true}
	}

	errors := make([]ValidationError, len(result.Errors()))
	for i, err := range result.Errors() {
		errors[i] = ValidationError{
			Field:       err.Field(),
			Description: err.Description(),
		}
	}

	return ValidationResult{
		Valid:  false,
		Errors: errors,
	}
}

// HasSchema checks if a schema is registered for an entry type
func (r *Registry) HasSchema(entryType string) bool {
	r.mu.RLock()
	defer r.mu.RUnlock()
	_, ok := r.schemas[entryType]
	return ok
}

// ListSchemas returns all registered entry types
func (r *Registry) ListSchemas() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()

	types := make([]string, 0, len(r.schemas))
	for t := range r.schemas {
		types = append(types, t)
	}
	return types
}

// Common schema definitions

// TaskSchema is a schema for task/todo entries
var TaskSchema = []byte(`{
	"$schema": "http://json-schema.org/draft-07/schema#",
	"type": "object",
	"required": ["title"],
	"properties": {
		"title": {"type": "string", "minLength": 1},
		"completed": {"type": "boolean", "default": false},
		"due_date": {"type": "string", "format": "date-time"},
		"priority": {"type": "integer", "minimum": 1, "maximum": 5},
		"description": {"type": "string"}
	}
}`)

// ContactSchema is a schema for contact entries
var ContactSchema = []byte(`{
	"$schema": "http://json-schema.org/draft-07/schema#",
	"type": "object",
	"required": ["name"],
	"properties": {
		"name": {"type": "string", "minLength": 1},
		"email": {"type": "string", "format": "email"},
		"phone": {"type": "string"},
		"address": {"type": "string"},
		"notes": {"type": "string"}
	}
}`)

// BookmarkSchema is a schema for bookmark entries
var BookmarkSchema = []byte(`{
	"$schema": "http://json-schema.org/draft-07/schema#",
	"type": "object",
	"required": ["url"],
	"properties": {
		"url": {"type": "string", "format": "uri"},
		"title": {"type": "string"},
		"description": {"type": "string"},
		"favicon": {"type": "string"}
	}
}`)

// CredentialSchema is a schema for credential/password entries
var CredentialSchema = []byte(`{
	"$schema": "http://json-schema.org/draft-07/schema#",
	"type": "object",
	"required": ["service", "username"],
	"properties": {
		"service": {"type": "string", "minLength": 1},
		"username": {"type": "string", "minLength": 1},
		"password": {"type": "string"},
		"url": {"type": "string", "format": "uri"},
		"notes": {"type": "string"},
		"totp_secret": {"type": "string"}
	}
}`)
