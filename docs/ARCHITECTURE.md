# Architecture Design

**acorde** is designed as a modular, layered system emphasizing local-first availability and conflict-free synchronization.

## 1. System Layers

```
+----------------------------------------------------------+
|              Application / CLI / REST API                 |
+----------------------------------------------------------+
|                      pkg/engine                           |  <-- Public API
|    (Engine, Query, Search, BlobStore, SharingManager)     |
+----------------------------------------------------------+
|                    pkg/api                                |  <-- HTTP Server
+----------------------------------------------------------+
|                   internal/engine                         |  <-- Coordination
+-----------+-------------+-------------+-------------------+
|  storage  |    crdt     |   crypto    |      events       |
+-----------+-------------+-------------+-------------------+
|  sqlite   | lww-set     |  xchacha    |    event-bus      |
|           | or-set      |  argon2     |                   |
|           | delta-sync  |  x25519     |                   |
+-----------+-------------+-------------+-------------------+
|   search  |    blob     |   sharing   |                   |
+-----------+-------------+-------------+-------------------+
|   bleve   | content-id  |  per-entry  |                   |
+-----------+-------------+-------------+-------------------+
|                    internal/sync                          |  <-- Networking
+----------------------------------------------------------+
|                        libp2p                             |
+----------------------------------------------------------+
```

## 2. Data Model

### Entry (Canonical Unit)
The fundamental unit of data is the `Entry`.

| Field | Type | Description | Sync Behavior |
|-------|------|-------------|---------------|
| `ID` | UUID | Unique Identifier | Immutable |
| `Type` | Enum | `note`, `file`, etc. | Immutable |
| `Content` | []byte | Encrypted Payload | Last-Write-Wins (LWW) |
| `Tags` | []string | Metadata categories | Observed-Remove Set (OR-Set) |
| `CreatedAt` | uint64 | Lamport Timestamp | Immutable |
| `UpdatedAt` | uint64 | Lamport Timestamp | Updates locally |
| `Deleted` | bool | Tombstone flag | LWW |

### Storage Schema (SQLite)

Tables:
- `entries`: Stores the current state (Materialized View).
- `tags`: Entry-to-tag mappings.
- `metadata`: Stores local-only state (e.g., max logical clock).

### Blob Storage
Large files are stored separately using content-addressing:
- **CID**: SHA-256 hash of content
- **Storage**: `{dataDir}/blobs/{prefix}/{cid}`
- **Reference**: Entry content contains CID reference

## 3. Conflict Resolution (CRDTs)

### Entries: LWW-Set
For content updates, acorde uses a **Last-Write-Wins** strategy based on Logical Clocks (Lamport Timestamps).
- If `Remote.Time > Local.Time`: Apply update.
- If `Remote.Time == Local.Time`: Tie-break using `PeerID` (Deterministic).

### Tags: OR-Set (Observed-Remove Set)
Tags support concurrent additions and removals without lost updates.
- **Add(tag)**: Generates a unique token for the tag.
- **Remove(tag)**: Adds the token to a "Tombstone Set".
- **Merge**: A tag exists if it is in the `AddSet` and NOT in the `RemoveSet`.

### Delta Sync
For efficiency, acorde supports delta synchronization:
- `EntriesSince(timestamp)`: Get entries modified after timestamp.
- `DeltaState(since)`: Export only changed entries and tags.
- `ApplyDelta(state)`: Merge remote delta into local replica.

## 4. Synchronization Protocol

### Discovery
1. **mDNS**: Multicasts presence on local network. Service Tag: `_acorde._tcp`.
2. **DHT**: Advertises `PeerID` under the `/acorde/1.0.0` namespace.

### Handshake
1. **Transport Security**: Noise handshake (Curve25519, ChaCha20, Poly1305).
2. **Protocol ID**: `/acorde/sync/1.0.0`.

### Sync Flow
1. **Alice** connects to **Bob**.
2. **Alice** sends `StateHash` (SHA-256 of all local state).
3. **Bob** compares Hash.
    - If Match: Send `Ack`. End.
    - If Mismatch: Send `MsgStateRequest`.
4. **Alice** sends full `ReplicaState` (or `DeltaState` if available).
5. **Bob** merges state into local CRDT.
6. **Bob** calculates new Hash and sends back to **Alice**.

## 5. API Layer

### REST API
HTTP server on configurable port (default: 7331).

| Endpoint | Method | Description |
|----------|--------|-------------|
| `/entries` | GET | List entries (with filters) |
| `/entries` | POST | Create entry |
| `/entries/:id` | GET | Get entry |
| `/entries/:id` | PUT | Update entry |
| `/entries/:id` | DELETE | Delete entry |
| `/status` | GET | Vault status |
| `/events` | GET | SSE stream |

### Event System
Real-time notifications via event bus:
- **EventCreated**: New entry added
- **EventUpdated**: Entry modified
- **EventDeleted**: Entry deleted
- **EventSynced**: Remote sync applied

## 6. Encryption

### At Rest
- **Algorithm**: XChaCha20-Poly1305
- **Key Derivation**: Argon2id
- **Storage**: Encrypted key in `~/.acorde/keys.json`

### Per-Entry Encryption
For selective sharing:
- **Key Exchange**: X25519 (Curve25519)
- **Key Derivation**: HKDF with entry ID as salt
- **Sharing**: Entry key wrapped with shared secret

## 7. Directory Structure

```
acorde/
├── cmd/acorde/          # CLI application
├── pkg/
│   ├── engine/          # Public API
│   │   ├── engine.go    # Core interface
│   │   ├── query.go     # Query language
│   │   ├── search.go    # Full-text search
│   │   ├── blob.go      # Blob storage
│   │   └── sharing.go   # Per-entry encryption
│   ├── api/             # REST API server
│   └── crypto/          # Cryptographic primitives
├── internal/
│   ├── core/            # Domain models, clock
│   ├── crdt/            # LWW-Set, OR-Set, delta sync
│   ├── engine/          # Engine implementation
│   ├── storage/         # SQLite adapter
│   ├── sync/            # P2P networking (libp2p)
│   ├── search/          # Bleve full-text search
│   ├── sharing/         # Key exchange & sharing
│   └── blob/            # Content-addressed storage
├── examples/            # Example applications
└── docs/                # Documentation
```
