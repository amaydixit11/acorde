# ACORDE

ACORDE is a local-first sync engine for apps that should keep working offline, store data on-device, and replicate directly between trusted peers without depending on a central backend.

It combines:

- SQLite for durable local storage
- CRDT replication for conflict-tolerant merges
- libp2p for peer-to-peer sync
- optional encrypted vaults for at-rest protection
- per-entry ownership and writer permissions

## Why It Exists

Most apps are still built around a server being the source of truth. ACORDE takes the opposite approach:

- the local device is authoritative
- sync is a replication problem, not a request/response dependency
- peers can discover each other on a LAN or connect through explicit pairing
- conflicts are merged instead of treated as fatal errors

This makes it a good fit for:

- offline-capable note or log apps
- peer-to-peer tools on a local network
- private, user-owned data stores
- prototypes that need sync semantics without building a full backend first

## Current Product Shape

Today ACORDE gives you:

- a CLI for local CRUD, pairing, encryption, and permissions
- a long-running `daemon` mode that runs sync and the REST API together
- direct peer syncing over mDNS or allowlisted pairing
- private entries with explicit writer authorization
- encrypted vault support through `acorde init`

The easiest way to treat it as a product is to run the daemon and use either the CLI or the HTTP API against the same vault.

## Quick Start

### Install

```bash
go install github.com/amaydixit11/acorde/cmd/acorde@latest
```

Or build locally:

```bash
git clone https://github.com/amaydixit11/acorde.git
cd acorde
go build -o acorde ./cmd/acorde
```

### Start One Node

```bash
acorde daemon --data /tmp/acorde-main --port 4101 --api-port 7401
```

In another terminal:

```bash
acorde add --data /tmp/acorde-main --type note --content "hello"
acorde list --data /tmp/acorde-main
curl -s http://localhost:7401/entries
```

### Start Two Nodes

Terminal 1:

```bash
acorde daemon --data /tmp/acorde-a --port 4001 --api-port 7331
```

Terminal 2:

```bash
acorde daemon --data /tmp/acorde-b --port 4002 --api-port 7332
```

Terminal 3:

```bash
acorde add --data /tmp/acorde-a --type note --content "hello from A"
sqlite3 /tmp/acorde-b/acorde.db 'select id,type,updated_at,deleted from entries order by id;'
```

That verifies replication reached B's local store.

## The Main CLI

```bash
acorde daemon --data /tmp/acorde-a --port 4001 --api-port 7331
acorde add --data /tmp/acorde-a --type note --content "hello"
acorde list --data /tmp/acorde-a
acorde get --data /tmp/acorde-a <entry-id>
acorde update --data /tmp/acorde-a <entry-id> --content "updated"
acorde delete --data /tmp/acorde-a <entry-id>
acorde authorize --data /tmp/acorde-a <entry-id> <peer-id>
acorde invite --data /tmp/acorde-a
acorde pair --data /tmp/acorde-b 'acorde://...'
acorde init --data /tmp/acorde-enc-a
```

## Product Flows

### 1. Local-Only App

If you only need offline local storage:

```bash
acorde add --data ./data --type note --content "draft"
acorde list --data ./data
```

### 2. Daemon + REST API

If you want one process that keeps local state, sync, and HTTP together:

```bash
acorde daemon --data ./data --port 4001 --api-port 7331
```

This starts:

- the local engine
- the p2p sync service
- the REST API on `http://localhost:7331`

### 3. Direct Pairing

For explicit peer trust without relying on mDNS:

Device A:

```bash
acorde daemon --data /tmp/acorde-pair-a --port 4021 --api-port 7351 --mdns=false
acorde invite --data /tmp/acorde-pair-a
```

Device B:

```bash
acorde pair --data /tmp/acorde-pair-b 'acorde://...'
acorde daemon --data /tmp/acorde-pair-b --port 4022 --api-port 7352 --mdns=false
```

### 4. Encrypted Vault

```bash
acorde init --data /tmp/acorde-enc-a
acorde add --data /tmp/acorde-enc-a --type note --content "secret"
acorde get --data /tmp/acorde-enc-a <entry-id>
```

## REST API

When `daemon` is started with `--api-port`, these endpoints are available:

| Method | Endpoint | Description |
| --- | --- | --- |
| `GET` | `/entries` | List readable entries |
| `POST` | `/entries` | Create an entry |
| `GET` | `/entries/:id` | Get an entry |
| `PUT` | `/entries/:id` | Update an entry |
| `DELETE` | `/entries/:id` | Tombstone an entry |
| `POST` | `/entries/:id/authorize` | Grant write access to a peer |
| `GET` | `/status` | Basic node status |
| `GET` | `/events` | Server-sent event stream |

Example:

```bash
curl -s http://localhost:7331/entries
curl -s -X POST http://localhost:7331/entries \
  -H 'Content-Type: application/json' \
  -d '{"type":"note","content":"hello","tags":["demo"]}'
```

## Important Behavior Notes

- New entries are private by default.
- `authorize` grants writer access; writers can also read the entry.
- Private synced entries can exist in another peer's SQLite DB without appearing in `list`, `get`, or `/entries` until that peer has access.
- CLI writes made against a vault while its daemon is already running are picked up and synced by that daemon.
- Deletes are tombstones and replicate across peers.

## For Builders

ACORDE also exposes a Go engine package if you want to embed it directly instead of shelling out to the CLI.

Start here:

- [Setup Guide](docs/setup.md)
- [API Reference](docs/API.md)
- [Feature Notes](docs/FEATURES.md)
- [Developer Guide](docs/DEVELOPER_GUIDE.md)
