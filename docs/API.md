# vaultd API Reference

Complete reference for the vaultd REST API and Go library.

## REST API

Start the server:
```bash
./vaultd serve --port 8080
```

### Endpoints

#### List Entries
```http
GET /entries
GET /entries?type=note
GET /entries?tag=work
GET /entries?type=note&tag=work
```

Response:
```json
[
  {
    "ID": "550e8400-e29b-41d4-a716-446655440000",
    "Type": "note",
    "Content": "SGVsbG8gV29ybGQ=",
    "Tags": ["work", "important"],
    "CreatedAt": 1,
    "UpdatedAt": 2,
    "Deleted": false
  }
]
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

Response:
```json
{
  "ID": "550e8400-e29b-41d4-a716-446655440000",
  "Type": "note",
  "Content": "SGVsbG8gV29ybGQ=",
  "Tags": ["work", "important"],
  "CreatedAt": 1,
  "UpdatedAt": 1,
  "Deleted": false
}
```

#### Get Entry
```http
GET /entries/:id
```

#### Update Entry
```http
PUT /entries/:id
Content-Type: application/json

{
  "content": "Updated content",
  "tags": ["new-tag"]
}
```

#### Delete Entry
```http
DELETE /entries/:id
```

#### Vault Status
```http
GET /status
```

Response:
```json
{
  "entry_count": 42,
  "status": "ok"
}
```

#### Real-Time Events (SSE)
```http
GET /events
```

Server-Sent Events stream:
```
data: {"type":"created","entry_id":"550e8400-...","entry_type":"note","timestamp":"2024-01-01T00:00:00Z"}

data: {"type":"updated","entry_id":"550e8400-...","entry_type":"note","timestamp":"2024-01-01T00:01:00Z"}
```

---

## Go Library

### Installation
```bash
go get github.com/amaydixit11/vaultd/pkg/engine
```

### Basic Usage

```go
import "github.com/amaydixit11/vaultd/pkg/engine"

// Create engine
e, err := engine.New(engine.Config{
    DataDir: "./data",
})
defer e.Close()

// Add entry
entry, err := e.AddEntry(engine.AddEntryInput{
    Type:    engine.Note,
    Content: []byte("Hello World"),
    Tags:    []string{"example"},
})

// Get entry
entry, err = e.GetEntry(entry.ID)

// Update entry
err = e.UpdateEntry(entry.ID, engine.UpdateEntryInput{
    Content: &[]byte("Updated"),
})

// Delete entry
err = e.DeleteEntry(entry.ID)

// List entries
entries, err := e.ListEntries(engine.ListFilter{
    Type: &engine.Note,
})
```

### Query Language

```go
// String DSL
results, err := e.Query(`type = "note" AND tags CONTAINS "work" LIMIT 10`)

// Fluent Builder
entries, err := e.NewQuery().
    Type(engine.Note).
    Tag("work").
    Since(timestamp).
    Limit(10).
    Execute()
```

### Full-Text Search

```go
results, err := e.Search("machine learning", engine.SearchOptions{
    Type:  &engine.Note,
    Limit: 20,
})
```

### Event Subscription

```go
sub := e.Subscribe()
defer sub.Close()

for event := range sub.Events() {
    fmt.Printf("[%s] Entry %s\n", event.Type, event.EntryID)
}
```

### Blob Storage

```go
blobs, err := engine.NewBlobStore("./data")

// Store
cid, err := blobs.StoreBlob(imageData)

// Retrieve
data, err := blobs.GetBlob(cid)

// Check
exists := blobs.HasBlob(cid)

// Delete
err = blobs.DeleteBlob(cid)
```

### Per-Entry Encryption

```go
import "github.com/amaydixit11/vaultd/pkg/crypto"

key, _ := crypto.GenerateKey()
mgr, _ := engine.NewSharingManager(key)

// Get my peer ID
myID := mgr.MyPeerID()

// Share entry with peers
shares, _ := mgr.ShareEntry(entryID, []engine.PeerID{aliceID, bobID})
```

---

## Entry Types

| Type | Constant | Description |
|------|----------|-------------|
| note | `engine.Note` | General notes |
| log | `engine.Log` | Log entries |
| file | `engine.File` | File references |
| event | `engine.EventEntry` | Calendar events |

---

## Event Types

| Type | Description |
|------|-------------|
| `created` | New entry added |
| `updated` | Entry modified |
| `deleted` | Entry deleted |
| `synced` | Remote sync applied |

---

## Error Types

```go
var ErrNotFound = errors.New("entry not found")
var ErrInvalidType = errors.New("invalid entry type")
```

Check error types:
```go
if errors.Is(err, engine.ErrNotFound) {
    // Handle not found
}
```
