# Acorde API Reference

This document covers the REST API exposed by `acorde daemon --api-port ...` and the main public Go engine surface.

## Start The Server

```bash
acorde daemon --data /tmp/acorde-a --port 4001 --api-port 7331
```

## REST API

### Endpoints

| Method | Endpoint | Description |
| --- | --- | --- |
| `GET` | `/entries` | List readable entries |
| `POST` | `/entries` | Create an entry |
| `GET` | `/entries/:id` | Get a readable entry |
| `PUT` | `/entries/:id` | Update an entry |
| `DELETE` | `/entries/:id` | Tombstone an entry |
| `POST` | `/entries/:id/authorize` | Grant write access to a peer |
| `GET` | `/status` | Basic status and peer count |
| `GET` | `/events` | SSE change stream |

### List Entries

```http
GET /entries?type=note&tag=work
```

Response:

```json
[
  {
    "id": "b786ab30-823e-4da8-931f-0ee82c5e2f7e",
    "type": "note",
    "content": "aGVsbG8=",
    "tags": [],
    "created_at": 1,
    "updated_at": 1,
    "deleted": false,
    "owner": "12D3KooW..."
  }
]
```

Notes:

- `content` is JSON base64 because the field is `[]byte`.
- Private entries are filtered out for peers without read access.

### Create Entry

```http
POST /entries
Content-Type: application/json

{
  "type": "note",
  "content": "hello",
  "tags": ["demo"]
}
```

### Get Entry

```http
GET /entries/ENTRY_ID
```

Returns:

- `200` when readable
- `403` when access is denied
- `404` when missing

### Update Entry

```http
PUT /entries/ENTRY_ID
Content-Type: application/json

{
  "content": "updated hello",
  "tags": ["demo", "updated"]
}
```

Returns:

- `204` on success
- `403` when the caller lacks write permission
- `404` when missing

### Delete Entry

```http
DELETE /entries/ENTRY_ID
```

This is a tombstone delete, not a hard physical delete.

### Authorize A Writer

```http
POST /entries/ENTRY_ID/authorize
Content-Type: application/json

{
  "peer_id": "12D3KooW..."
}
```

Returns:

- `204` on success
- `403` when the caller is not the owner/admin
- `404` when the entry is missing

### Status

```http
GET /status
```

Example:

```json
{
  "status": "ok",
  "entry_count": 3,
  "peer_count": 1
}
```

### Events

```http
GET /events
```

This is a server-sent event stream. Each event is emitted as JSON after `data:`.

## Go Library

### Create An Engine

```go
e, err := engine.New(engine.Config{
    DataDir: "./data",
})
if err != nil {
    panic(err)
}
defer e.Close()
```

### Core Operations

```go
entry, err := e.AddEntry(engine.AddEntryInput{
    Type:    engine.Note,
    Content: []byte("hello"),
    Tags:    []string{"demo"},
})

err = e.UpdateEntry(entry.ID, engine.UpdateEntryInput{
    Content: ptr([]byte("updated")),
})

entries, err := e.ListEntries(engine.ListFilter{})
got, err := e.GetEntry(entry.ID)
err = e.DeleteEntry(entry.ID)
```

### Access Control

```go
err = e.GrantWrite(entry.ID, peerID)
peerID := e.PeerID()
```

The public error surface includes:

- `engine.ErrNotFound`
- `engine.ErrDeleted`
- `engine.ErrAccessDenied`

### Sync Hooks

The engine exposes low-level sync methods used by the transport layer:

```go
payload, err := e.GetSyncPayload()
err = e.ApplyRemotePayload(payload)
err = e.ApplyRemotePayloadFromPeer(payload, peerID)
```

These are mainly useful when embedding a custom transport.
