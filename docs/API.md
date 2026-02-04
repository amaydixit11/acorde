# Acorde API Reference

Complete reference for the Acorde REST API and Go library.

## REST API

Start the server:
```bash
./acorde serve --port 7331
```

### Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/entries` | List entries with filtering |
| `POST` | `/entries` | Create new entry |
| `GET` | `/entries/:id` | Get entry by UUID |
| `PUT` | `/entries/:id` | Update entry content/tags |
| `DELETE` | `/entries/:id`| Soft delete entry |
| `GET` | `/status` | Server status |
| `GET` | `/events` | Real-time SSE stream |

#### List Entries
```http
GET /entries?type=note&tag=work
```

#### Create Entry
```http
POST /entries
Content-Type: application/json

{
  "type": "note",
  "content": "Hello World",
  "tags": ["work", "important"]
}
```

## Go Library

### Installation
```bash
go get github.com/amaydixit11/acorde/pkg/engine
```

### Basic Lifecycle
```go
import "github.com/amaydixit11/acorde/pkg/engine"

// Initialize
e, _ := engine.New(engine.Config{
    DataDir: "./data",
    MaxVersions: 50, // Enable version history
})

// Register Schema (Optional)
e.RegisterSchema("task", []byte(`{"type":"object", "required":["title"]}`))

// Add Entry (Version 1 saved)
entry, _ := e.AddEntry(engine.AddEntryInput{
    Type: "task", 
    Content: []byte(`{"title":"Work"}`),
})

// Update Entry (Version 2 saved)
e.UpdateEntry(entry.ID, engine.UpdateEntryInput{...})

// History Access
history, _ := e.Versions().GetHistory(entry.ID)
```

### Feature Accessors

#### Versioning
```go
// Get all versions
history, err := e.Versions().GetHistory(entryID)

// Restore version
version, err := e.Versions().GetVersion(entryID, versionID)
```

#### Access Control (ACL)
```go
// Check permissions
canWrite, _ := e.ACL().CheckWrite(entryID, myPeerID)

// Grant access
e.ACL().GrantRead(entryID, alicePeerID)

// Make public
e.ACL().MakePublic(entryID)
```

#### Webhooks
```go
hooks := engine.NewHookManager()
hooks.OnCreate(func(e engine.HookEvent) {
    fmt.Printf("New entry: %s", e.EntryID)
})
```

#### Multi-Vault
```go
mgr, _ := engine.NewVaultManager("~/.acorde")
workVault, _ := mgr.Create("work")
mgr.SetActive("work")
```

### Query Language

```go
// String DSL
results, err := e.Query(`type = "note" AND tags CONTAINS "work" LIMIT 10`)

// Fluent Builder
entries, err := e.NewQuery().
    Type(engine.Note).
    Tag("work").
    Limit(10).
    Execute()
```
