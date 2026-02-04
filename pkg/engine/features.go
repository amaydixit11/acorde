package engine

// Re-export internal packages for public use

import (
	"github.com/amaydixit11/acorde/internal/acl"
	"github.com/amaydixit11/acorde/internal/core"
	"github.com/amaydixit11/acorde/internal/hooks"
	"github.com/amaydixit11/acorde/internal/importer"
	"github.com/amaydixit11/acorde/internal/query"
	"github.com/amaydixit11/acorde/internal/schema"
	"github.com/amaydixit11/acorde/internal/vault"
	"github.com/amaydixit11/acorde/internal/version"
)

// ========== Schema Validation ==========

// SchemaRegistry manages JSON schemas for entry validation
type SchemaRegistry = schema.Registry

// NewSchemaRegistry creates a new schema registry
func NewSchemaRegistry() *SchemaRegistry {
	return schema.NewRegistry()
}

// Schema represents a JSON Schema
type Schema = schema.Schema

// ValidationResult contains validation results
type ValidationResult = schema.ValidationResult

// ValidationError represents a validation error
type ValidationError = schema.ValidationError

// Predefined schemas
var (
	TaskSchema       = schema.TaskSchema
	ContactSchema    = schema.ContactSchema
	BookmarkSchema   = schema.BookmarkSchema
	CredentialSchema = schema.CredentialSchema
)

// ========== Versioning & History ==========

// VersionStore manages entry version history
type VersionStore = version.Store

// NewVersionStore creates a version store (requires *sql.DB)
var NewVersionStore = version.NewStore

// Version represents a historical entry version
type Version = version.Version

// VersionDiff represents differences between versions
type VersionDiff = version.Diff

// ComputeVersionDiff computes diff between two versions
var ComputeVersionDiff = version.ComputeDiff

// ========== Access Control ==========

// ACLStore manages access control lists
type ACLStore = acl.Store

// NewACLStore creates an ACL store (requires *sql.DB)
var NewACLStore = acl.NewStore

// ACL represents access control for an entry
type ACL = core.ACL

// Permission levels
type Permission = acl.Permission

const (
	PermNone  = acl.PermNone
	PermRead  = acl.PermRead
	PermWrite = acl.PermWrite
	PermAdmin = acl.PermAdmin
)

// ErrAccessDenied is returned when access is denied
type ErrAccessDenied = acl.ErrAccessDenied

// ========== Webhooks & Callbacks ==========

// HookManager manages webhooks and callbacks
type HookManager = hooks.Manager

// NewHookManager creates a new hook manager
func NewHookManager() *HookManager {
	return hooks.NewManager()
}

// HookEvent represents an event passed to callbacks
type HookEvent = hooks.HookEvent

// HookEventType represents hook event types
type HookEventType = hooks.EventType

const (
	HookEventCreate = hooks.EventCreate
	HookEventUpdate = hooks.EventUpdate
	HookEventDelete = hooks.EventDelete
	HookEventSync   = hooks.EventSync
)

// HookCallback is a function called on events
type HookCallback = hooks.Callback

// WebhookConfig configures an HTTP webhook
type WebhookConfig = hooks.WebhookConfig

// ========== Import/Export ==========

// Exporter handles exporting entries
type Exporter = importer.Exporter

// NewExporter creates a new exporter
func NewExporter() *Exporter {
	return importer.NewExporter()
}

// Importer handles importing entries
type Importer = importer.Importer

// NewImporter creates a new importer
func NewImporter() *Importer {
	return importer.NewImporter()
}

// ExportEntry represents an entry for import/export
type ExportEntry = importer.ExportEntry

// ExportData represents a full vault export
type ExportData = importer.ExportData

// ExportFormat specifies export format
type ExportFormat = importer.ExportFormat

const (
	FormatJSON     = importer.FormatJSON
	FormatMarkdown = importer.FormatMarkdown
	FormatCSV      = importer.FormatCSV
)

// ImportResult contains import statistics
type ImportResult = importer.ImportResult

// ========== Multi-Vault ==========

// VaultManager manages multiple vaults
type VaultManager = vault.Manager

// NewVaultManager creates a new vault manager
func NewVaultManager(baseDir string) (*VaultManager, error) {
	return vault.NewManager(baseDir)
}

// VaultInfo contains vault metadata
type VaultInfo = vault.VaultInfo

// ========== Rich Query ==========

// QueryParser parses SQL-like queries
type QueryParser = query.Parser

// NewQueryParser creates a new query parser
func NewQueryParser() *QueryParser {
	return query.NewParser()
}

// ParsedQuery represents a parsed query
type ParsedQuery = query.Query

// QueryOrderClause specifies ordering
type QueryOrderClause = query.OrderClause
