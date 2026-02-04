# vaultd Roadmap

## Current Status: Phase 4.5 Complete (Encryption & Stability Fixes) ✅

**vaultd** is feature-complete for v1.0 core functionality. It supports persistent storage, multi-peer discovery, CRDT-based merging, and end-to-end encryption.

## Phases

### Phase 1: Local Engine ✅
- [x] Entry model with types (note, log, file, event)
- [x] Lamport clock for causal ordering
- [x] SQLite storage with idempotent operations
- [x] Public Engine API
- [x] Test CLI

### Phase 2: CRDT Core ✅
- [x] LWW-Set for entries (Last-Write-Wins)
- [x] OR-Set for tags (Observed-Remove Set)
- [x] Deterministic merge function
- [x] Merge order independence tests

### Phase 3: Sync Protocol ✅
- [x] JSON payload serialization
- [x] State hash comparison (Sync Check)
- [x] libp2p integration
- [x] LAN peer discovery (mDNS)
- [x] Global peer discovery (DHT)
- [x] Manual Pairing (QR/Invite)

### Phase 4: Encryption ✅
- [x] XChaCha20-Poly1305 encryption for entries
- [x] Argon2id Key Derivation
- [x] Encrypted Master Key storage
- [x] Secure Key Sharing (Invite Flow)
- [x] Verified Zero-Knowledge Sync

### Phase 5: Hardening & Optimization (Upcoming) 🔜
- [ ] **Performance**: Switch to MsgPack or Protobuf for sync payloads
- [ ] **Conflict Resolution**: UI for manual conflict handling on collisions
- [ ] **Garbage Collection**: Pruning deleted tombstones
- [ ] **Partial Sync**: Vector clocks / Delta syncing (currently sends full state)
- [ ] **Fault Injection**: Jepsen-style network partition testing
- [ ] **Observability**: Metrics and structured logging

## Non-Goals (v1)

- User accounts/central authentication
- Cloud backup (unless self-hosted peer)
- Multi-user permissions (ACLs)
- Plugin system

## Definition of Done (v1)

Two vaultd instances can:
1. Modify data offline
2. Sync later via LAN or Internet
3. Converge to identical state
4. All data encrypted at rest and in transit
5. **(Achieved)**
