# ACORDE

ACORDE is a local-first data engine with SQLite storage, CRDT-based replication, optional vault encryption, and peer-to-peer sync over libp2p.

## What It Does

- Stores entries locally in SQLite and works offline.
- Syncs entries between peers over LAN discovery or direct pairing.
- Uses CRDTs to merge replicated state.
- Encrypts vault contents at rest when initialized with `acorde init`.
- Enforces per-entry ownership and writer permissions.
- Exposes both a Go API and a REST API.

## Install

```bash
go install github.com/amaydixit11/acorde/cmd/acorde@latest
```

Or from source:

```bash
git clone https://github.com/amaydixit11/acorde.git
cd acorde
go build -o acorde ./cmd/acorde
```

## Core Commands

```bash
acorde daemon --data /tmp/acorde-a --port 4001 --api-port 7331
acorde add --data /tmp/acorde-a --type note --content "hello"
acorde list --data /tmp/acorde-a
acorde get --data /tmp/acorde-a <entry-id>
acorde update --data /tmp/acorde-a <entry-id> --content "updated"
acorde delete --data /tmp/acorde-a <entry-id>
acorde authorize --data /tmp/acorde-a <entry-id> <peer-id>
```

## Recommended Run Mode

Run `daemon` when you want sync and the REST API in the same long-lived process:

```bash
acorde daemon --data /tmp/acorde-a --port 4001 --api-port 7331
```

This starts:

- the local engine
- the libp2p sync service
- the REST API on `http://localhost:7331`

## Quick Start

### Single Node

```bash
acorde daemon --data /tmp/acorde-main --port 4101 --api-port 7401
```

In another terminal:

```bash
acorde add --data /tmp/acorde-main --type note --content "hello"
acorde list --data /tmp/acorde-main
acorde get --data /tmp/acorde-main <entry-id>
```

### Two-Node Sync

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

Grant B write access to A's entry:

```bash
cat /tmp/acorde-b/node_id
acorde authorize --data /tmp/acorde-a <entry-id> <peer-id-from-b>
```

Then update from B:

```bash
acorde update --data /tmp/acorde-b <entry-id> --content "updated from B"
```

### Direct Pairing

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

Notes:

- `pair` stores the peer in the allowlist.
- If the peer is not reachable immediately, pairing can still succeed locally and syncing starts once the daemon runs.
- When a daemon is already running, `invite` prefers the daemon's live addresses.

### Encrypted Vault

```bash
acorde init --data /tmp/acorde-enc-a
acorde add --data /tmp/acorde-enc-a --type note --content "secret"
acorde list --data /tmp/acorde-enc-a
acorde get --data /tmp/acorde-enc-a <entry-id>
```

## REST API

When `daemon` is started with `--api-port`, these endpoints are available:

| Method | Endpoint | Description |
| --- | --- | --- |
| `GET` | `/entries` | List readable entries |
| `POST` | `/entries` | Create an entry |
| `GET` | `/entries/:id` | Get one entry |
| `PUT` | `/entries/:id` | Update an entry |
| `DELETE` | `/entries/:id` | Tombstone an entry |
| `POST` | `/entries/:id/authorize` | Grant write access to a peer |
| `GET` | `/status` | Basic server status |
| `GET` | `/events` | Server-sent event stream |

Example:

```bash
curl -s http://localhost:7331/entries
curl -s -X POST http://localhost:7331/entries \
  -H 'Content-Type: application/json' \
  -d '{"type":"note","content":"hello","tags":["demo"]}'
```

## Behavior Notes

- Synced private entries may exist in a peer's SQLite DB but remain hidden from `list`, `get`, and `/entries` until that peer has read access.
- `authorize` grants write access. Writers can also read the entry.
- CLI writes made against the same vault while a daemon is already running are picked up by the daemon and synced.
- Deletions are tombstones and replicate across peers.

## Project Docs

- [Setup Guide](docs/setup.md)
- [API Reference](docs/API.md)
- [Feature Notes](docs/FEATURES.md)
- [Developer Guide](docs/DEVELOPER_GUIDE.md)
