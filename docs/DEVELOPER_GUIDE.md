# ACORDE Developer Guide

**ACORDE** (Always-Available Conflict-free Offline-first Replicated Distributed Engine) is a local-first, peer-to-peer data synchronization engine. This document is a complete reference for building applications on top of ACORDE.

---

## Table of Contents

1. [Overview](#overview)
2. [Quick Start](#quick-start)
3. [REST API Reference](#rest-api-reference)
4. [Go Library Reference](#go-library-reference)
5. [Data Model](#data-model)
6. [Features](#features)
7. [Sync Protocol](#sync-protocol)
8. [Security Model](#security-model)

---

## Overview

### What ACORDE Does

- **Stores data locally** in SQLite with full offline support.
- **Syncs data** between devices using peer-to-peer networking (libp2p).
- **Resolves conflicts** automatically using CRDTs (Conflict-free Replicated Data Types).
- **Encrypts everything** with XChaCha20-Poly1305.

### Architecture

```
┌─────────────────────────────────────────────────────────────┐
│                      Your Application                        │
├─────────────────────────────────────────────────────────────┤
│                     ACORDE REST API (:8080)                  │
├─────────────────────────────────────────────────────────────┤
│  Engine  │  CRDT  │  Search  │  Blobs  │  ACL  │  Hooks    │
├─────────────────────────────────────────────────────────────┤
│                       SQLite Storage                         │
├─────────────────────────────────────────────────────────────┤
│                    libp2p (mDNS + DHT)                       │
└─────────────────────────────────────────────────────────────┘
```

---

## Quick Start

### 1. Run the ACORDE Daemon

```bash
# Initialize a new vault (first time only)
acorde init

# Start the REST API server
acorde serve --port 8080

# Or start the P2P sync daemon
acorde daemon
```

### 2. Create an Entry via REST

```bash
curl -X POST http://localhost:8080/entries \
  -H "Content-Type: application/json" \
  -d '{
    "type": "note",
    "content": "Hello from ACORDE!",
    "tags": ["test", "demo"]
  }'
```

### 3. List Entries

```bash
curl http://localhost:8080/entries
curl http://localhost:8080/entries?type=note
curl http://localhost:8080/entries?tag=demo
```

---

## REST API Reference

Base URL: `http://localhost:8080`

### Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/entries` | List entries with optional filters |
| `POST` | `/entries` | Create a new entry |
| `GET` | `/entries/:id` | Get entry by UUID |
| `PUT` | `/entries/:id` | Update entry content/tags |
| `DELETE` | `/entries/:id` | Soft-delete entry |
| `GET` | `/status` | Get vault status |
| `GET` | `/events` | Server-Sent Events stream |

### Query Parameters for GET /entries

| Parameter | Type | Description |
|-----------|------|-------------|
| `type` | string | Filter by entry type (`note`, `log`, `file`, `event`) |
| `tag` | string | Filter by tag |
| `since` | int64 | Unix timestamp, entries created after |
| `until` | int64 | Unix timestamp, entries created before |
| `limit` | int | Max results (default: 100) |
| `offset` | int | Pagination offset |

### Request/Response Formats

#### Create Entry (POST /entries)

```json
// Request
{
  "type": "note",
  "content": "Base64 or plain text content",
  "tags": ["tag1", "tag2"]
}

// Response
{
  "ID": "550e8400-e29b-41d4-a716-446655440000",
  "Type": "note",
  "Content": "SGVsbG8=",
  "Tags": ["tag1", "tag2"],
  "CreatedAt": 1707000000,
  "UpdatedAt": 1707000000,
  "Deleted": false
}
```

#### Update Entry (PUT /entries/:id)

```json
// Request (all fields optional)
{
  "content": "New content",
  "tags": ["new-tag"]
}
```

### Server-Sent Events (GET /events)

Connect to receive real-time updates:

```javascript
const events = new EventSource('http://localhost:8080/events');
events.onmessage = (e) => {
  const event = JSON.parse(e.data);
  console.log(event.type, event.entry_id);
};
```

Event format:
```json
{
  "type": "created|updated|deleted|synced",
  "entry_id": "uuid",
  "entry_type": "note",
  "timestamp": "2024-01-01T00:00:00Z"
}
```

---

## Go Library Reference

### Installation

```bash
go get github.com/amaydixit11/acorde/pkg/engine
go get github.com/amaydixit11/acorde/pkg/crypto
```

### Basic Usage

```go
import (
    "github.com/amaydixit11/acorde/pkg/engine"
    "github.com/amaydixit11/acorde/pkg/crypto"
)

// Initialize engine
key, _ := crypto.GenerateKey()
e, _ := engine.New(engine.Config{
    DataDir:       "./data",      // Storage location
    EncryptionKey: &key,          // Optional encryption
    InMemory:      false,         // Use SQLite (true = RAM only)
})
defer e.Close()

// Create entry
entry, _ := e.AddEntry(engine.AddEntryInput{
    Type:    engine.Note,
    Content: []byte("Hello ACORDE"),
    Tags:    []string{"demo"},
})

// Read entry
entry, _ = e.GetEntry(entry.ID)

// Update entry
newContent := []byte("Updated content")
e.UpdateEntry(entry.ID, engine.UpdateEntryInput{
    Content: &newContent,
})

// Delete entry
e.DeleteEntry(entry.ID)

// List entries
entries, _ := e.ListEntries(engine.ListFilter{
    Type: &engine.Note,
    Tag:  strPtr("demo"),
})
```

### Entry Types

```go
const (
    Note       EntryType = "note"   // General notes
    Log        EntryType = "log"    // Log entries
    File       EntryType = "file"   // File references
    EventEntry EntryType = "event"  // Calendar events
)
```

### Query Language

```go
// SQL-like DSL
results, _ := e.Query(`
    type = "note" AND 
    tags CONTAINS "work" AND 
    created_at > 1700000000
    LIMIT 20
`)

// Fluent builder
entries, _ := e.NewQuery().
    Type(engine.Note).
    Tag("work").
    Since(timestamp).
    Limit(10).
    Execute()
```

### Full-Text Search

```go
results, _ := e.Search("machine learning", engine.SearchOptions{
    Type:  &engine.Note,
    Limit: 20,
})

for _, result := range results {
    fmt.Printf("Score: %.2f, ID: %s\n", result.Score, result.Entry.ID)
}
```

### Event Subscription

```go
sub := e.Subscribe()
defer sub.Close()

go func() {
    for event := range sub.Events() {
        switch event.Type {
        case engine.EventCreated:
            fmt.Println("New entry:", event.EntryID)
        case engine.EventUpdated:
            fmt.Println("Updated:", event.EntryID)
        case engine.EventDeleted:
            fmt.Println("Deleted:", event.EntryID)
        }
    }
}()
```

---

## Data Model

### Entry

The core data unit in ACORDE.

| Field | Type | Description |
|-------|------|-------------|
| `ID` | UUID | Unique identifier |
| `Type` | string | Entry type (note, log, file, event) |
| `Content` | []byte | Arbitrary content (encrypted if key set) |
| `Tags` | []string | OR-Set of tags |
| `CreatedAt` | int64 | Lamport timestamp |
| `UpdatedAt` | int64 | Lamport timestamp |
| `Deleted` | bool | Tombstone flag |

### Content Encoding

- Content is `[]byte` — ACORDE doesn't interpret it.
- For structured data, use JSON:
  ```go
  content, _ := json.Marshal(map[string]any{
      "title": "My Note",
      "body":  "Content here",
  })
  ```
- For files, store metadata in Content and file bytes in Blob Store.

---

## Features

### Schema Validation

Define JSON schemas to validate entry content:

```go
// Register schema
e.RegisterSchema("task", []byte(`{
    "type": "object",
    "required": ["title"],
    "properties": {
        "title": {"type": "string"},
        "done": {"type": "boolean"}
    }
}`))

// Entries of type "task" are now validated
entry, err := e.AddEntry(engine.AddEntryInput{
    Type:    "task",
    Content: []byte(`{"title": "Buy milk", "done": false}`),
})
// err != nil if content doesn't match schema
```

### Version History

Every entry change is tracked:

```go
// Get version history
history, _ := e.Versions().GetHistory(entryID)
for _, v := range history {
    fmt.Printf("Version %d at %v by %s\n", v.Number, v.CreatedAt, v.Author)
}

// Get specific version
version, _ := e.Versions().GetVersion(entryID, versionNumber)

// Restore old version
e.UpdateEntry(entryID, engine.UpdateEntryInput{
    Content: &version.Content,
})
```

### Access Control (ACL)

Per-entry permissions:

```go
acls := e.ACL()

// Check permissions
canRead, _ := acls.CheckRead(entryID, peerID)
canWrite, _ := acls.CheckWrite(entryID, peerID)

// Grant access
acls.GrantRead(entryID, peerID)
acls.GrantWrite(entryID, peerID)

// Make public
acls.MakePublic(entryID)
```

### Webhooks

Register HTTP callbacks for events:

```go
hooks := e.Hooks()

// In-process callback
hooks.OnCreate(func(event engine.HookEvent) {
    fmt.Println("Created:", event.EntryID)
})

// HTTP webhook
hooks.RegisterWebhook(engine.WebhookConfig{
    URL:    "https://your-server.com/webhook",
    Events: []engine.HookEventType{engine.HookEventCreate, engine.HookEventUpdate},
    Async:  true,
})
```

### Blob Storage

Content-addressed storage for large files:

```go
blobs, _ := engine.NewBlobStore("./data")

// Store file
cid, _ := blobs.StoreBlob(fileBytes)
// cid = "sha256-a1b2c3d4..."

// Reference in entry
entry, _ := e.AddEntry(engine.AddEntryInput{
    Type:    engine.File,
    Content: []byte(`{"name": "photo.jpg", "cid": "` + cid + `"}`),
})

// Retrieve later
data, _ := blobs.GetBlob(cid)
```

### Import/Export

Migrate data between vaults:

```go
exporter := engine.NewExporter()
data, _ := exporter.ExportAll(entries, engine.FormatJSON)

importer := engine.NewImporter()
result, _ := importer.ImportJSON(data)
fmt.Printf("Imported %d entries\n", result.Imported)
```

---

## Sync Protocol

### How Sync Works

1. **Discovery**: Peers find each other via mDNS (LAN) or DHT (Internet).
2. **Handshake**: Peers exchange state hashes.
3. **Delta Sync**: Only changed entries are transferred.
4. **Merge**: CRDTs automatically resolve conflicts.

### CRDT Details

- **Entries**: LWW-Set (Last-Write-Wins based on Lamport clock)
- **Tags**: OR-Set (Observed-Remove Set)
- **Guarantee**: All replicas eventually converge to identical state.

### Pairing Devices

```bash
# Device A: Generate invite
acorde invite --share-key
# Output: acorde://QmPeerID@192.168.1.5:4001?key=...

# Device B: Accept invite
acorde pair "acorde://..."
```

---

## Security Model

### Encryption

| Layer | Algorithm |
|-------|-----------|
| Content encryption | XChaCha20-Poly1305 |
| Key derivation | Argon2id |
| Key exchange | X25519 |

### Data at Rest

- All entry content is encrypted before writing to SQLite.
- The master key is derived from a password using Argon2id.
- AAD (Additional Authenticated Data) binds content to entry ID.

### Data in Transit

- libp2p uses TLS 1.3 for transport encryption.
- Peer authentication via Ed25519 public keys.

### Per-Entry Sharing

Share individual entries with specific peers:

```go
mgr, _ := engine.NewSharingManager(masterKey)

// Share entry with peer
shares, _ := mgr.ShareEntry(entryID, []engine.PeerID{alicePeerID})

// Alice recovers key
key, _ := sharing.RecoverSharedKey(share, entryID, alicePrivate, senderPublic)
```

---

## Configuration Reference

### Engine Config

```go
engine.Config{
    DataDir:       string       // Storage path (default: ~/.acorde)
    EncryptionKey: *crypto.Key  // Optional encryption key
    InMemory:      bool         // RAM-only mode
    MaxVersions:   int          // Version history limit
}
```

### Environment Variables

| Variable | Description |
|----------|-------------|
| `ACORDE_DATA_DIR` | Override default data directory |
| `ACORDE_PORT` | REST API port (default: 8080) |

---

## Error Handling

```go
import "github.com/amaydixit11/acorde/pkg/engine"

entry, err := e.GetEntry(id)
if errors.Is(err, engine.ErrNotFound) {
    // Entry doesn't exist
}

err = e.UpdateEntry(id, input)
if errors.Is(err, engine.ErrAccessDenied) {
    // No write permission
}
```

---

## Best Practices

1. **Use JSON for Content**: Easier to query, validate, and evolve.
2. **Register Schemas**: Catch data bugs early.
3. **Subscribe to Events**: React to changes in real-time.
4. **Use Blobs for Files**: Don't store binary data in Content.
5. **Handle Offline**: ACORDE handles sync automatically, just read/write.

---

## Example: Note-Taking App

```go
package main

import (
    "encoding/json"
    "github.com/amaydixit11/acorde/pkg/engine"
)

type Note struct {
    Title string `json:"title"`
    Body  string `json:"body"`
}

func main() {
    e, _ := engine.New(engine.Config{DataDir: "./notes"})
    defer e.Close()

    // Create note
    note := Note{Title: "Meeting Notes", Body: "Discussed roadmap..."}
    content, _ := json.Marshal(note)
    entry, _ := e.AddEntry(engine.AddEntryInput{
        Type:    engine.Note,
        Content: content,
        Tags:    []string{"work", "meeting"},
    })

    // Search notes
    results, _ := e.Search("roadmap", engine.SearchOptions{Limit: 10})
    for _, r := range results {
        var n Note
        json.Unmarshal(r.Entry.Content, &n)
        fmt.Println(n.Title)
    }
}
```

---

## Version

This documentation is for **ACORDE v0.8** (Phase 8 Complete).

Repository: https://github.com/amaydixit11/acorde
