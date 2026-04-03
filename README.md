# ACORDE

**Local-first sync engine for apps that work offline, store data on-device, and replicate directly between trusted peers — no central backend required.**

[![Go](https://img.shields.io/badge/Go-1.25+-00ADD8?style=flat&logo=go)](https://golang.org)
[![License](https://img.shields.io/github/license/amaydixit11/acorde)](LICENSE)
[![Docker](https://img.shields.io/badge/Docker-ready-2496ED?style=flat&logo=docker)](docker-compose.yml)

---

## What is ACORDE?

ACORDE is a Go library and CLI that gives your app conflict-tolerant, peer-to-peer sync without a server. It combines:

- **SQLite** — durable, on-device storage
- **CRDTs** — automatic conflict resolution via Last-Write-Wins registers and OR-Sets
- **libp2p** — peer-to-peer transport with mDNS discovery or explicit pairing
- **Encrypted vaults** — optional at-rest encryption per data directory
- **Per-entry ACLs** — ownership and writer permissions baked into the data model

The device is the source of truth. Sync is a replication problem, not a request/response dependency. Conflicts are merged, not rejected.

### Good fits

- Offline-capable note or log apps
- Local-network peer-to-peer tools
- Private, user-owned data stores
- Prototypes that need sync semantics without standing up a backend

---

## Install

```bash
go install github.com/amaydixit11/acorde/cmd/acorde@latest
```

Or build from source:

```bash
git clone https://github.com/amaydixit11/acorde.git
cd acorde
go build -o acorde ./cmd/acorde
```

Or run with Docker:

```bash
docker compose up
```

> **Requirements:** Go 1.25+, CGO enabled (for SQLite). See [docs/setup.md](docs/setup.md) for platform-specific notes.

---

## Quick Start

### Single node

```bash
# Start the daemon (local engine + p2p sync + REST API)
acorde daemon --data /tmp/acorde-a --port 4001 --api-port 7331

# In another terminal
acorde add  --data /tmp/acorde-a --type note --content "hello"
acorde list --data /tmp/acorde-a
curl -s http://localhost:7331/entries
```

### Two nodes syncing

```bash
# Terminal 1
acorde daemon --data /tmp/acorde-a --port 4001 --api-port 7331

# Terminal 2
acorde daemon --data /tmp/acorde-b --port 4002 --api-port 7332

# Terminal 3 — write on A, verify it reached B
acorde add --data /tmp/acorde-a --type note --content "hello from A"
sqlite3 /tmp/acorde-b/acorde.db \
  'select id,type,updated_at,deleted from entries order by id;'
```

Both nodes discover each other automatically via mDNS on the same LAN.

---

## Usage Modes

### 1. Local-only storage

No sync, no daemon — just a local SQLite-backed store.

```bash
acorde add  --data ./data --type note --content "draft"
acorde list --data ./data
acorde get  --data ./data <entry-id>
acorde update --data ./data <entry-id> --content "revised"
acorde delete --data ./data <entry-id>
```

### 2. Daemon with REST API

Run a single process that manages local state, sync, and an HTTP API together.

```bash
acorde daemon --data ./data --port 4001 --api-port 7331
```

This starts the local engine, the p2p sync service, and the REST API at `http://localhost:7331`.

### 3. Explicit peer pairing (no mDNS)

For cross-network pairing or when you want explicit trust without broadcast discovery.

```bash
# Device A — generate an invite link
acorde daemon  --data /tmp/a --port 4021 --api-port 7351 --mdns=false
acorde invite  --data /tmp/a
# prints: acorde://...

# Device B — pair using the link, then start daemon
acorde pair    --data /tmp/b 'acorde://...'
acorde daemon  --data /tmp/b --port 4022 --api-port 7352 --mdns=false
```

### 4. Encrypted vault

```bash
acorde init --data /tmp/enc-a          # initializes an encrypted vault
acorde add  --data /tmp/enc-a --type note --content "secret"
acorde get  --data /tmp/enc-a <entry-id>
```

---

## CLI Reference

| Command | Description |
|---|---|
| `acorde daemon` | Start the sync daemon and REST API |
| `acorde add` | Create a new entry |
| `acorde list` | List readable entries |
| `acorde get <id>` | Get a single entry |
| `acorde update <id>` | Update content or tags |
| `acorde delete <id>` | Tombstone an entry (replicates to peers) |
| `acorde authorize <id> <peer-id>` | Grant write access to a peer |
| `acorde invite` | Generate a pairing invite link |
| `acorde pair <link>` | Accept an invite and add a trusted peer |
| `acorde init` | Initialize an encrypted vault |

**Common flags:**

| Flag | Description |
|---|---|
| `--data <path>` | Data directory (required) |
| `--port <n>` | p2p listen port (default: 4001) |
| `--api-port <n>` | REST API port |
| `--mdns=false` | Disable mDNS peer discovery |
| `--name <name>` | Human-readable node name |

---

## REST API

Start the daemon with `--api-port` to enable the HTTP API.

| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/entries` | List readable entries |
| `POST` | `/entries` | Create an entry |
| `GET` | `/entries/:id` | Get a single entry |
| `PUT` | `/entries/:id` | Update an entry |
| `DELETE` | `/entries/:id` | Tombstone an entry |
| `POST` | `/entries/:id/authorize` | Grant write access to a peer |
| `GET` | `/status` | Node status |
| `GET` | `/events` | Server-sent event stream |

```bash
# Create
curl -s -X POST http://localhost:7331/entries \
  -H 'Content-Type: application/json' \
  -d '{"type":"note","content":"hello","tags":["demo"]}'

# List
curl -s http://localhost:7331/entries

# Stream live events
curl -s http://localhost:7331/events
```

---

## How Sync Works

ACORDE's CRDT layer ensures all peers converge to the same state regardless of the order operations arrive.

- **Entries** use a Last-Write-Wins (LWW) register — the update with the highest logical timestamp wins on merge.
- **Tags** use an Observed-Remove Set (OR-Set) — concurrent adds from different peers are preserved; removes only remove what a peer has seen.
- **Deletes** are tombstones that replicate and are never silently lost.
- **Private entries** sync to peers as opaque records; they won't appear in `list`, `get`, or `/entries` on a peer that hasn't been authorized.
- **ACLs** are part of the CRDT state and replicate along with entries.

For a deeper explanation, see [docs/WHY_CRDT.md](docs/WHY_CRDT.md) and [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md).

---

## Embedding the Engine

ACORDE exposes a Go package so you can embed the sync engine directly instead of shelling out to the CLI.

```go
import "github.com/amaydixit11/acorde/internal/engine"

e, err := engine.New(engine.Config{
    DataDir: "./data",
})
if err != nil {
    log.Fatal(err)
}
defer e.Close()

entry, err := e.AddEntry(engine.AddEntryInput{
    Type:    core.Note,
    Content: []byte("hello from Go"),
    Tags:    []string{"example"},
})
```

See [docs/API.md](docs/API.md) for the full engine interface and [examples/](examples/) for working CLI and web examples.

---

## Docker

```bash
# Build and run a single node
docker compose up

# Or build the image directly
docker build -t acorde .
docker run -p 4001:4001 -p 7331:7331 -v $(pwd)/data:/home/acorde/data acorde
```

The container exposes port `4001` for p2p (TCP/UDP) and `7331` for the REST API. mDNS is disabled by default in Docker; use explicit pairing for multi-container setups.

---

## Android (Termux)

ACORDE runs on Android via [Termux](https://termux.dev) (install from F-Droid, not the Play Store).

```bash
pkg update && pkg upgrade
pkg install golang git
git clone https://github.com/amaydixit11/acorde.git
cd acorde/cmd/acorde
go build -o acorde .
./acorde daemon --data ~/acorde-data --port 4001 --api-port 7331
```

See [docs/ANDROID.md](docs/ANDROID.md) for troubleshooting Go version issues on newer Android releases and tips for keeping the daemon running in the background.

---

## Behavior Notes

- New entries are **private by default**. Use `authorize` or `POST /entries/:id/authorize` to grant access to a peer.
- Writers can also read the entries they're authorized on.
- CLI writes made against a vault while its daemon is running are picked up and synced automatically.
- Tombstoned entries replicate to all peers; deletion is not silent.

---

## Documentation

| Doc | Description |
|---|---|
| [docs/setup.md](docs/setup.md) | Installation and platform setup |
| [docs/API.md](docs/API.md) | REST API and Go engine reference |
| [docs/ARCHITECTURE.md](docs/ARCHITECTURE.md) | System design overview |
| [docs/DEVELOPER_GUIDE.md](docs/DEVELOPER_GUIDE.md) | Contributing and dev workflow |
| [docs/WHY_CRDT.md](docs/WHY_CRDT.md) | Why CRDTs and which ones |
| [docs/WHY_NO_SERVER.md](docs/WHY_NO_SERVER.md) | Design rationale |
| [docs/FEATURES.md](docs/FEATURES.md) | Feature notes and status |
| [docs/SECURITY.md](docs/SECURITY.md) | Security model |
| [docs/ANDROID.md](docs/ANDROID.md) | Running on Android via Termux |
| [ROADMAP.md](ROADMAP.md) | Planned features |
| [CONTRIBUTING.md](CONTRIBUTING.md) | Contribution guide |

---

## License

[See LICENSE](LICENSE)
