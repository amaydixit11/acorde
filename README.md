# vaultd

**vaultd** is a local-first, peer-to-peer data synchronization engine. It provides durable storage, conflict-free replication (CRDTs), and decentralized sync for personal data applications.

Designed to be embedded in apps or run as a personal data daemon.

## Features

- **Local-First**: Works offline, stores data locally (SQLite).
- **Conflict-Free**: Merges data from multiple devices automatically using CRDTs.
- **Peer-to-Peer Sync**:
  - **mDNS**: Zero-config LAN discovery (default).
  - **DHT**: Global internet-wide discovery (optional).
  - **Manual Pairing**: Privacy-first QR code invites.
- **Embedded**: Go library + CLI daemon.

## Installation

```bash
go get github.com/amaydixit11/vaultd
go install github.com/amaydixit11/vaultd/cmd/vaultd@latest
```

## Quick Start (CLI)

### 1. Start the Daemon
Run this in a terminal to start the sync node. It will auto-discover peers.
```bash
vaultd daemon
```

### 2. Add Entries
In another terminal, add data. It will sync to other peers automatically.
```bash
vaultd add --content "Hello World" --tags note,test
```

### 3. Global Discovery (DHT)
To sync across the internet (different networks), enable DHT:
```bash
vaultd daemon --dht
```

---

## Manual Pairing (Privacy Mode)

For explicit, secure pairing between devices (e.g., Phone ↔ Laptop):

**Device A (Generate Invite):**
```bash
vaultd invite
# Displays QR code and invite string
```

**Device B (Connect):**
```bash
vaultd pair "vaultd://..."
```
This adds the peer to `~/.vaultd/peers.json` and establishes a direct connection.

---

## Library Usage

```go
package main

import (
    "context"
    "github.com/amaydixit11/vaultd/pkg/engine"
    "github.com/amaydixit11/vaultd/internal/sync"
)

func main() {
    // 1. Create Engine (Storage + CRDT)
    e, _ := engine.New(engine.Config{DataDir: "./data"})
    defer e.Close()

    // 2. Add Data
    e.AddEntry(engine.AddEntryInput{
        Type:    engine.Note,
        Content: []byte("Local-first data"),
    })

    // 3. Start Sync (Optional)
    // Adapts engine to sync protocol
    adapter := sync.NewEngineAdapter(e)
    svc, _ := sync.NewP2PService(adapter, sync.DefaultConfig())
    svc.Start(context.Background())
}
```

## Architecture

- **Engine**: Handles storage (SQLite) and logical clocks.
- **CRDT**: Merkle-DAG based causal trees for conflict resolution.
- **Sync**: Protocol agnostic (currently libp2p) state synchronization.

## Status

- ✅ Phase 1: Local Engine
- ✅ Phase 2: CRDT Replication
- ✅ Phase 3: P2P Sync (mDNS + DHT + Pairing)
- 🚧 Phase 4: End-to-End Encryption (Next)
