# vaultd

A local-first data synchronization engine providing durable storage, conflict-free replication, peer-to-peer synchronization, and end-to-end encryption for personal data applications.

## Overview

**vaultd** is an embedded engine, not a standalone service. It has zero UI concerns and zero product opinions.

## Features

- **State Management**: Entry lifecycle with logical clock tracking
- **Persistence**: SQLite-based durable storage with crash-safe writes
- **Replication**: CRDT-based conflict-free merging (Phase 2)
- **Sync**: Transport-agnostic peer-to-peer synchronization (Phase 3)
- **Security**: End-to-end encryption at rest and in transit (Phase 4)

## Installation

```bash
go get github.com/amaydixit11/vaultd
```

## Usage

### As a Library

```go
package main

import (
    "github.com/amaydixit11/vaultd/pkg/engine"
)

func main() {
    // Create engine
    e, err := engine.New(engine.Config{})
    if err != nil {
        panic(err)
    }
    defer e.Close()

    // Add an entry
    entry, err := e.AddEntry(engine.AddEntryInput{
        Type:    engine.Note,
        Content: []byte("Hello, vaultd!"),
        Tags:    []string{"demo"},
    })
    if err != nil {
        panic(err)
    }

    // List entries
    entries, _ := e.ListEntries(engine.ListFilter{})
    for _, e := range entries {
        fmt.Printf("%s: %s\n", e.ID, string(e.Content))
    }
}
```

### CLI (Development)

```bash
# Build CLI
go build -o vaultd ./cmd/vaultd

# Add entry
./vaultd add --type note --content "Hello World" --tags work,important

# List entries
./vaultd list --type note

# Get entry
./vaultd get <uuid>

# Update entry
./vaultd update <uuid> --content "Updated content"

# Delete entry
./vaultd delete <uuid>
```

## Data Model

### Entry Types

- `note` - General notes
- `log` - Log entries
- `file` - File references
- `event` - Calendar events

### Entry Fields

| Field | Type | Description |
|-------|------|-------------|
| ID | UUID | Unique identifier |
| Type | string | Entry type |
| Content | []byte | Opaque content |
| Tags | []string | Associated tags |
| CreatedAt | uint64 | Logical creation time |
| UpdatedAt | uint64 | Logical update time |
| Deleted | bool | Tombstone marker |

## Architecture

```
vaultd/
├── cmd/vaultd/          # Test CLI
├── pkg/engine/          # Public API
└── internal/
    ├── core/            # Domain model (Entry, Clock)
    ├── storage/         # Storage abstraction
    │   └── sqlite/      # SQLite implementation
    ├── engine/          # Engine implementation
    ├── crdt/            # Replication logic (Phase 2)
    ├── sync/            # Sync protocol (Phase 3)
    └── crypto/          # Encryption (Phase 4)
```

## Development

```bash
# Run tests
go test ./...

# Run with coverage
go test -cover ./...

# Build
go build ./...
```

## License

MIT
