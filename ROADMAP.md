# vaultd Roadmap

## Current Status: Phase 7 Complete ✅

**vaultd** is a full-featured local-first data engine with P2P sync, encryption, REST API, and advanced querying.

## Completed Phases

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

### Phase 5: REST API & Events ✅
- [x] HTTP REST API (`vaultd serve`)
- [x] CRUD endpoints for entries
- [x] Server-Sent Events for real-time updates
- [x] Event subscription system with filtering
- [x] CLI commands: `status`, `export`, `serve`

### Phase 6: Query & Search ✅
- [x] Query language with DSL parser
- [x] Fluent query builder API
- [x] Full-text search with Bleve
- [x] Example applications (notes-cli, notes-web)

### Phase 7: Advanced Features ✅
- [x] Delta Sync (only send changed entries)
- [x] Per-Entry Encryption (share with specific peers)
- [x] Content-Addressed Blob Storage
- [x] X25519 key exchange for sharing

## Upcoming Phases

### Phase 8: Performance & Optimization 🔜
- [ ] MsgPack/Protobuf for sync payloads
- [ ] Garbage collection for tombstones
- [ ] Delta sync integration into P2P layer
- [ ] Bloom filter for entry negotiation
- [ ] Connection pooling and rate limiting

### Phase 9: Mobile & Web 🔜
- [ ] WebAssembly build
- [ ] React Native bindings
- [ ] iOS Swift bindings
- [ ] Android Kotlin bindings
- [ ] Cross-platform sync testing

### Phase 10: Enterprise Features 📅
- [ ] Multi-user permissions (ACLs)
- [ ] Audit logging
- [ ] Backup/restore functionality
- [ ] Admin dashboard
- [ ] Prometheus metrics

## Non-Goals (v1)

- User accounts/central authentication
- Cloud backup (unless self-hosted peer)
- Plugin system

## Definition of Done (v1.0)

Two vaultd instances can:
1. ✅ Modify data offline
2. ✅ Sync later via LAN or Internet
3. ✅ Converge to identical state
4. ✅ All data encrypted at rest and in transit
5. ✅ Query and search entries efficiently
6. ✅ Access via REST API from any language
7. ✅ Subscribe to real-time change events
8. ✅ Store large files in blob storage

**Status: v1.0 Core Complete! 🎉**
