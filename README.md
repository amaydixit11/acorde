# vaultd

**vaultd** is a **local-first**, **peer-to-peer** data synchronization engine constructed with Go. It enables applications to store data durably offline and sync it securely across devices without a central server.

![License](https://img.shields.io/badge/license-MIT-blue.svg)
![Go Version](https://img.shields.io/badge/go-1.21+-00ADD8.svg?logo=go)
![Status](https://img.shields.io/badge/status-beta-orange.svg)

## ✨ Features

- **Local-First**: Built on SQLite. Works completely offline.
- **Conflict-Free**: Uses State-based CRDTs (LWW-Set for entries, OR-Set for tags) to merge data automatically.
- **Peer-to-Peer Sync**:
  - **mDNS**: Automatic discovery on Release LAN.
  - **DHT**: Global discovery via Kademlia DHT (optional).
  - **Direct Pairing**: QR-code based pairing for trusted devices.
- **End-to-End Encryption**:
  - **XChaCha20-Poly1305** for content encryption.
  - **Argon2id** for key derivation.
  - Zero-knowledge sync (peers store encrypted blobs).
- **Developer Ready**:
  - Embeddable Go library `pkg/engine`.
  - Standalone daemon CLI `cmd/vaultd`.

## 📦 Installation

```bash
# Install CLI
go install github.com/amaydixit11/vaultd/cmd/vaultd@latest
```

## 🚀 Quick Start (CLI)

### 1. Initialize & Secure
Initialize a new vault. You will be prompted to set a password.

```bash
vaultd init
# Output:
# Enter new password: ...
# ✅ Vault initialized at ~/.vaultd
```

### 2. Start Daemon
Start the sync daemon. It will unlock your vault and begin discovering peers.

```bash
vaultd daemon
# Output:
# 🔒 Vault is encrypted. Enter password: ...
# 🚀 Starting vaultd daemon...
# ✅ Daemon started! Discovering peers on LAN...
```

### 3. Add Data
In another terminal, add some data.

```bash
# Add a note
vaultd add --type note --content "Meeting at 10am" --tags work,urgent

# List notes
vaultd list --type note

# Update a note
vaultd update <UUID> --content "Meeting rescheduled to 11am"
```

## 🔐 Encryption & Security

**vaultd** uses a "trust no one" architecture. 

1.  **At Rest**: All content is encrypted using **XChaCha20-Poly1305**. The encryption key is protected by a master key, which is encrypted with **Argon2id** (derived from your password) and stored in `~/.vaultd/keys.json`.
2.  **In Transit**: Sync streams are encrypted via libp2p's Noise protocol.
3.  **Syncing**: When syncing, peers exchange *encrypted* CRDT payloads. A peer without the key can store and relay the data but cannot read it (Zero-Knowledge Sync).

## 🤝 Pairing Devices

To sync 2 devices (e.g., A and B), they must share the same **Encryption Key**.

**Device A (Source):**
```bash
vaultd invite --share-key
# Output:
# 🔒 Vault is encrypted. Enter password: ...
# <QR Code>
# Invite code: vaultd://<PEER_ID>?key=<ENCRYPTED_KEY>
```

**Device B (Target):**
```bash
# Pairs with Device A and imports the key
vaultd pair "vaultd://..."
# Output:
# 🔑 Invite contains encryption key. Set a password to protect it: ...
# ✅ Vault initialized with imported key.
```

## 🛠️ Library Usage

Embed **vaultd** into your own Go application:

```go
package main

import (
	"log"
	
	"github.com/amaydixit11/vaultd/pkg/engine"
	"github.com/amaydixit11/vaultd/pkg/crypto"
)

func main() {
	// 1. Initialize Key (or load from storage)
	key, _ := crypto.GenerateKey()
	
	// 2. Configure Engine
	cfg := engine.Config{
		DataDir:       "./my-app-data",
		EncryptionKey: &key,
	}

	// 3. Start Engine
	e, err := engine.New(cfg)
	if err != nil {
		log.Fatal(err)
	}
	defer e.Close()

	// 4. Use Engine
	entry, _ := e.AddEntry(engine.AddEntryInput{
		Type:    engine.Note,
		Content: []byte("Secret Data"),
		Tags:    []string{"embedded"},
	})
	
	log.Printf("Added entry: %s", entry.ID)
}
```

## 🏗️ Architecture

```mermaid
graph TD
    User[User / App] --> API[pkg/engine (Public API)]
    API --> Engine[Internal Engine]
    
    subgraph Core Logic
        Engine --> Replica[CRDT Replica (Logic)]
        Engine --> Store[SQLite Store (Persistence)]
        Engine --> Crypto[Crypto Service]
    end
    
    subgraph Sync Layer
        Replica <--> Adapter[Sync Adapter]
        Adapter <--> P2P[P2P Service (libp2p)]
        P2P <--> Network((Internet))
    end
```

### Components

-   **CRDT**: Hybrid LWW-Set (Entries) and OR-Set (Tags). Ensures strong eventual consistency.
-   **Storage**: SQLite with a schema optimized for CRDT history and efficient querying.
-   **Sync**:
    -   **Transport**: TCP/QUIC via libp2p.
    -   **Discovery**: mDNS (Local), Kademlia DHT (Global).
    -   **Protocol**: State-based sync (Merkle-DAG optimization planned).

## 🗺️ Roadmap

- [x] Phase 1: Local Engine (SQLite + CRDT)
- [x] Phase 2: Core Replication Logic
- [x] Phase 3: P2P Sync & Discovery
- [x] Phase 4: End-to-End Encryption
- [ ] Phase 5: Hardening (Performance, Conflict UI, Partial Sync)

## 📄 License

MIT
