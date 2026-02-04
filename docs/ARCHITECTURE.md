# Architecture Design

**vaultd** is designed as a modular, layered system emphasizing local-first availability and conflict-free synchronization.

## 1. System Layers

```
+-------------------------------------------------------+
|                 Application / CLI                     |
+-------------------------------------------------------+
|                    pkg/engine                         |  <-- Public API
+-------------------------------------------------------+
|                  internal/engine                      |  <-- Coordination
+-------------+-----------------------+-----------------+
|   storage   |         crdt          |     crypto      |  <-- Core Logic
+-------------+-----------------------+-----------------+
|   sqlite    |      merkle-dag       |    argon2/poly  |
+-------------+-----------------------+-----------------+
|                    internal/sync                      |  <-- Networking
+-------------------------------------------------------+
|                       libp2p                          |
+-------------------------------------------------------+
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
- `tombstones`: Tracks deleted items for sync.
- `metadata`: Stores local-only state (e.g., max logical clock).

## 3. Conflict Resolution (CRDTs)

### Entries: LWW-Set
For content updates, vaultd uses a **Last-Write-Wins** strategy based on Logical Clocks (Lamport Timestamps).
- If `Remote.Time > Local.Time`: Apply update.
- If `Remote.Time == Local.Time`: Tie-break using `PeerID` (Deterministic).

### Tags: OR-Set (Observed-Remove Set)
Tags support concurrent additions and removals without lost updates.
- **Add(tag)**: Generates a unique token for the tag.
- **Remove(tag)**: Adds the token to a "Tombstone Set".
- **Merge**: A tag exists if it is in the `AddSet` and NOT in the `RemoveSet`.
- *Note*: Current implementation simplifies this to an optimized Map-based merge for efficiency.

## 4. Synchronization Protocol

The sync protocol is **state-based** (delta-state optimization planned).

### Discovery
1.  **mDNS**: Multicasts presence on generic port (default). Service Tag: `_vaultd._tcp`.
2.  **DHT**: Advertises `PeerID` under the `/vaultd/1.0.0` namespace.

### Handshake
1.  **Transport Security**: Noise handshake (Curve25519, ChaCha20, Poly1305).
2.  **Protocol ID**: `/vaultd/sync/1.0.0`.

### Sync Flow
1.  **Alice** connects to **Bob**.
2.  **Alice** sends `StateHash` (SHA-256 of all local state).
3.  **Bob** compares Hash.
    -   If Match: Send `Ack`. End.
    -   If Mismatch: Send `MsgStateRequest`.
4.  **Alice** sends full `ReplicaState` (JSON).
5.  **Bob** merges `ReplicaState` into local CRDT.
6.  **Bob** calculates new Hash and sends back to **Alice** (Optimization: Bi-directional sync).

## 5. Directory Structure

- `cmd/vaultd`: The CLI entry point.
- `pkg/engine`: The high-level library entry point.
- `pkg/crypto`: Cryptographic primitives and KeyStore.
- `internal/core`: Domain models.
- `internal/crdt`: Conflict resolution logic.
- `internal/storage`: Database adapter (SQLite).
- `internal/sync`: P2P networking (libp2p).
