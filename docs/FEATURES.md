# Feature Notes

This document describes the current feature surface in the repository without treating roadmap items as already complete.

## Core Data Model

- Entries have an ID, type, content, tags, created timestamp, updated timestamp, and deleted tombstone flag.
- Supported entry types in the public API are `note`, `log`, `file`, and `event`.
- Data is stored locally in SQLite.

## CRDT Replication

- Entries replicate as an LWW set.
- Tags replicate as OR-sets.
- ACLs replicate with timestamp-based last-write-wins semantics.
- Sync state is exchanged over libp2p and merged on receipt.
- Deletions are tombstones and are replicated.

## Sync Modes

### LAN Discovery

- mDNS-based peer discovery on the local network.

### Direct Pairing

- `invite` generates a signed pairing payload.
- `pair` verifies and saves the peer to the allowlist.
- Daemons proactively reconnect to allowlisted peers.

### Allowlist

- Known peers are stored locally and can be used even when mDNS is disabled.

## Access Control

- New entries are private by default and owned by the creating peer.
- Owners can grant writer access with `acorde authorize <entry-id> <peer-id>`.
- Writers can read and update the entry.
- Unauthorized remote writes are rejected during sync.
- Private entries can sync into another peer's SQLite store without being visible in `list` or `/entries` until that peer has read access.

## Encryption

- `acorde init` creates an encrypted vault.
- Entry content is encrypted at rest.
- CLI commands prompt for the vault password when needed.

## REST API

When `daemon` is started with `--api-port`, the API exposes:

- `GET /entries`
- `POST /entries`
- `GET /entries/:id`
- `PUT /entries/:id`
- `DELETE /entries/:id`
- `POST /entries/:id/authorize`
- `GET /status`
- `GET /events`

## Public Go Surface

The public engine exposes:

- entry CRUD
- `GrantWrite`
- entry listing with filters
- event subscriptions
- sync payload helpers
- `PeerID`

Other helper packages exist in the repo, including blob storage, search, sharing, query helpers, version storage, and vault management, but they should be treated as library-level components rather than part of the basic CLI workflow.

## Verified Behavior

The current tree has been manually verified for:

- single-node CRUD
- two-node mDNS sync
- unauthorized write rejection
- authorize then remote update
- delete propagation
- direct pairing without mDNS
- encrypted local vault usage
- three-node convergence
