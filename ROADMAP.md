# vaultd Roadmap

## Current Status: Phase 1 Complete ✅

## Phases

### Phase 1: Local Engine ✅
- [x] Entry model with types (note, log, file, event)
- [x] Lamport clock for causal ordering
- [x] SQLite storage with idempotent operations
- [x] Public Engine API
- [x] Test CLI

### Phase 2: CRDT Core 🔜
- [ ] LWW-Element-Set for entries
- [ ] OR-Set for tags
- [ ] Deterministic merge function
- [ ] Merge order independence tests

### Phase 3: Sync Protocol
- [ ] Payload serialization (protobuf/msgpack)
- [ ] State hash/diff computation
- [ ] libp2p integration
- [ ] LAN peer discovery (mDNS)

### Phase 4: Encryption
- [ ] AES-256-GCM encryption at rest
- [ ] Key derivation (HKDF)
- [ ] Encrypted sync payloads
- [ ] Key management API

### Phase 5: Hardening
- [ ] Comprehensive test suite (>80% coverage)
- [ ] Fault injection tests
- [ ] Performance benchmarks
- [ ] Full documentation

## Non-Goals (v1)

- User accounts/authentication
- Cloud backup
- Multi-user permissions
- Plugin system
- Undo/redo history

## Definition of Done (v1)

Two vaultd instances can:
1. Modify data offline
2. Sync later
3. Converge to identical state
4. All data encrypted at rest and in transit
