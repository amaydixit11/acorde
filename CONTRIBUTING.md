# Contributing to Acorde

Thank you for your interest in contributing to **Acorde**!

## Development Setup

```bash
# Clone
git clone https://github.com/amaydixit11/acorde.git
cd acorde

# Install dependencies
go mod download

# Run tests
go test ./...
```

## Code Organization

```
acorde/
├── pkg/                  # Public API
│   ├── engine/           # Engine, Query, Search, Features
│   ├── api/              # REST API
│   └── crypto/           # Crypto utils
├── internal/             # Private Implementation
│   ├── core/             # Domain Models
│   ├── crdt/             # Conflict Resolution
│   ├── engine/           # Engine Implementation
│   ├── storage/          # SQLite Adapter
│   ├── sync/             # P2P Network
│   ├── schema/           # Schema Validation
│   ├── version/          # History Tracking
│   ├── acl/              # Access Control
│   └── hooks/            # Webhooks
├── cmd/acorde/           # Daemon CLI
└── docs/                 # Documentation
```

## Guidelines

1. **Public API**: Only packages in `pkg/` are importable by users.
2. **Feature Integration**: New features (like ACLs) must be wired into `internal/engine/engine_impl.go`.
3. **Tests**: Run `go test ./...` before pushing.

## Feature Areas

### Adding a New Entry Type
1. Add constant in `internal/core/entry.go`
2. Add validation in `internal/engine`

### Adding a New Feature
1. Create package in `internal/` (e.g. `internal/audit`)
2. Expose safe methods via `pkg/engine` wrapper
3. Integrate into `engineImpl` if it affects core lifecycle
