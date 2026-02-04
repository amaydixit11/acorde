# Acorde Roadmap

## Current Status: Phase 8 Complete ✅

**Acorde** is a production-ready engine with P2P sync, encryption, history tracking, and access control.

## Completed Phases

### Phase 1: Local Engine ✅
- [x] Entry model types (note, log, file, event)
- [x] SQLite storage & Lamport clocks

### Phase 2: CRDT Core ✅
- [x] LWW-Set & OR-Set merge logic

### Phase 3: Sync Protocol ✅
- [x] P2P Sync (libp2p), mDNS, DHT, Pairing

### Phase 4: Encryption ✅
- [x] XChaCha20-Poly1305 & Argon2id

### Phase 5-7: Advanced Features ✅
- [x] REST API & Events
- [x] Query Language & Search (Bleve)
- [x] Blob Storage

### Phase 8: Production Readiness ✅
- [x] **Schema Validation**: JSON Schema support
- [x] **Versioning**: History tracking & Restore
- [x] **Access Control**: Owner/Reader/Writer ACLs
- [x] **Multi-Vault**: Work/Personal separation
- [x] **Webhooks**: Sync & Async callbacks
- [x] **Import/Export**: JSON/CSV/Markdown

## Upcoming Phases

### Phase 9: Mobile & Web 🔜
- [ ] WebAssembly build
- [ ] React Native / iOS / Android bindings

### Phase 10: Enterprise Features 📅
- [ ] Audit logging
- [ ] Backup/restore automation
- [ ] Prometheus metrics

## Definition of Done (v1.0)
1. ✅ Modify data offline & Sync via P2P
2. ✅ Converge to identical state (CRDT)
3. ✅ End-to-End Encryption
4. ✅ History & Access Control
5. ✅ Rich Query & Search
