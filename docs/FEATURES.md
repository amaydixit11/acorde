# Complete Feature List for ACORDE

Based on the codebase, here are **all** the features you need to test:

---

## **1. Core Entry Management**

### Create Entries
- Add entries with types: `note`, `log`, `file`, `event`
- Attach content (arbitrary bytes)
- Add multiple tags
- Auto-generated UUID
- Lamport timestamp tracking

### Read Entries
- Get single entry by ID
- List all entries
- Filter by type
- Filter by tag
- Filter by date range (Since/Until)
- Include/exclude deleted entries
- Pagination (Limit/Offset)

### Update Entries
- Update content
- Update tags
- Tags use OR-Set semantics (concurrent add/remove merges correctly)
- Timestamps auto-increment

### Delete Entries
- Soft delete (tombstone)
- Entry marked as deleted but preserved for CRDT
- Doesn't appear in default lists

---

## **2. Encryption**

### At-Rest Encryption
- XChaCha20-Poly1305 content encryption
- Argon2id key derivation from password
- AAD binding (entry ID tied to ciphertext)
- Master key storage in `keys.json`

### Per-Entry Encryption (Sharing)
- X25519 key exchange
- ECDH shared secret derivation
- Share specific entries with specific peers
- Recipients can decrypt without master key

---

## **3. CRDT Synchronization**

### Conflict-Free Merging
- **Entries**: LWW-Set (Last-Write-Wins)
  - Higher timestamp wins
  - Tie-breaker: lexicographic comparison
- **Tags**: OR-Set (Observed-Remove)
  - Concurrent add/remove operations merge correctly
  - Each add gets unique token
- **ACLs**: LWW (timestamp-based)

### Delta Sync
- `EntriesSince(timestamp)` - only changed entries
- 10x faster than full state transfer

---

## **4. P2P Sync**

### Discovery Methods
- **mDNS**: Local LAN discovery (zero-config)
  - Service name: `_acorde._tcp`
  - Automatic peer finding on same network
- **DHT**: Global discovery via Kademlia
  - Uses IPFS bootstrap nodes
  - Namespace: `/acorde/1.0.0`
- **Direct Pairing**: QR code / invite URL

### Sync Protocol
- State hash comparison (SHA-256)
- Only sync if hashes differ
- Bidirectional merge
- Session IDs prevent duplicate syncs
- Periodic sync every 5 seconds (configurable)

### Allowlist
- Trusted peer management
- Strict mode (reject unknown peers)
- Stored in `peers.json`

---

## **5. Device Pairing**

### Invite Generation
- Creates signed invite with:
  - Peer ID
  - Network addresses
  - Public key
  - Expiration (24h default)
  - Signature (ed25519)
- Optionally includes encryption key

### Invite Formats
- Full: `acorde://BASE64_JSON`
- Minimal: `acorde://PEERID@ADDRESS`
- QR Code: PNG or ASCII art

### Pairing
- Parse invite
- Verify signature
- Add to allowlist (if enabled)
- Connect and sync

---

## **6. Schema Validation**

### JSON Schema
- Register schema per entry type
- Validate content on create/update
- Enforced automatically
- Built-in schemas:
  - Task (title, completed, due_date, priority)
  - Contact (name, email, phone)
  - Bookmark (url, title)
  - Credential (service, username, password)

---

## **7. Version History**

### Tracking
- Every entry change saved as version
- Includes: content, tags, timestamp, author (peer ID)
- Configurable max versions per entry

### Operations
- `GetHistory(entryID)` - all versions
- `GetVersion(entryID, versionID)` - specific version
- `GetVersionAt(entryID, timestamp)` - point-in-time
- Restore by updating entry with old version content

### Diff
- Compare two versions
- Shows content changes
- Shows tags added/removed

---

## **8. Access Control (ACL)**

### Permissions
- **Owner**: Full control
- **Writer**: Read + Write
- **Reader**: Read only
- **Public**: Anyone can read

### Operations
- `CheckRead(entryID, peerID)`
- `CheckWrite(entryID, peerID)`
- `GrantRead/Write/Admin`
- `RevokeRead/Write`
- `MakePublic/Private`
- Default ACL: Private, owned by creator

---

## **9. Webhooks**

### Event Types
- `create` - Entry created
- `update` - Entry updated
- `delete` - Entry deleted
- `sync` - Sync completed with peer

### Configuration
- URL endpoint
- Event filters
- Custom headers
- HMAC signing secret
- Max retries (default: 3)
- Timeout (default: 10s)
- Async/sync mode

### In-Process Callbacks
- `OnCreate(callback)`
- `OnUpdate(callback)`
- `OnDelete(callback)`
- `OnSync(callback)`

---

## **10. Full-Text Search**

### Bleve Integration
- Pure Go search engine
- Indexes entry content
- Standard analyzer for text
- Keyword analyzer for tags/types

### Search Options
- Filter by type
- Filter by tags
- Result limit
- Returns entries sorted by relevance score

---

## **11. Blob Storage**

### Content-Addressed Storage
- SHA-256 CID (Content Identifier)
- Stores in `{dataDir}/blobs/{prefix}/{cid}`
- Idempotent (same content = same CID)

### Operations
- `Put(data)` - returns CID
- `Get(cid)` - retrieves data
- `Has(cid)` - check existence
- `Delete(cid)`
- `List()` - all CIDs
- `GarbageCollect(referencedCIDs)` - remove unreferenced

### Usage Pattern
Store file reference in entry:
```json
{
  "name": "photo.jpg",
  "cid": "a1b2c3d4..."
}
```

---

## **12. Query Language**

### SQL-Like DSL
```
type = "note" AND 
tags CONTAINS "work" AND 
created_at > 1700000000 
ORDER BY updated_at DESC 
LIMIT 20
```

### Fluent Builder
```go
e.NewQuery().
  Type(engine.Note).
  Tag("work").
  Since(timestamp).
  Limit(10).
  Execute()
```

---

## **13. Multi-Vault**

### Vault Manager
- Separate vaults (Work/Personal)
- Stored in `{baseDir}/vaults.json`
- Each vault has own data directory

### Operations
- `Create(name)` - new vault
- `List()` - all vaults
- `Get(idOrName)` - retrieve vault
- `Delete(idOrName, removeData)` - remove vault
- `SetActive(idOrName)` - switch active vault
- `GetActive()` - current vault
- `Rename(idOrName, newName)`

---

## **14. Import/Export**

### Formats
- **JSON**: Full structured export with metadata
- **CSV**: Tabular data (id, type, content, tags, timestamps)
- **Markdown**: Notes with frontmatter

### Export
- `ExportToJSON(entries, writer)`
- `ExportToMarkdown(entries, directory)` - one file per note
- `ExportToCSV(entries, writer)`

### Import
- `ImportFromJSON(reader)` - returns entries
- `ImportFromCSV(reader)` - returns entries
- `ImportFromMarkdown(reader)` - single note
- Parse frontmatter (id, type, tags)

---

## **15. REST API**

### Endpoints

| Method | Endpoint | Description |
|--------|----------|-------------|
| `GET` | `/entries` | List entries (filters: type, tag, since, until) |
| `POST` | `/entries` | Create entry |
| `GET` | `/entries/:id` | Get entry |
| `PUT` | `/entries/:id` | Update entry |
| `DELETE` | `/entries/:id` | Delete entry |
| `GET` | `/status` | Server status (peer count, sync stats) |
| `GET` | `/events` | SSE stream (real-time events) |

### Server-Sent Events
- Real-time change notifications
- Event types: created, updated, deleted, synced
- JSON payload with entry ID, type, timestamp

---

## **16. CLI Commands**

### Daemon
```bash
acorde daemon --port 4001 --api-port 7331
```
Runs P2P sync + REST API in unified mode

### Initialization
```bash
acorde init              # Create vault
acorde init --encrypt    # With encryption
```

### Entry Operations
```bash
acorde add --type note --content "Hello" --tags work,urgent
acorde list
acorde get <ID>
acorde update <ID> --content "New"
acorde delete <ID>
```

### Pairing
```bash
acorde invite --share-key    # Generate invite + QR
acorde pair "acorde://..."   # Accept invite
```

### Search
```bash
acorde search "keyword"
```

### Sync Status
```bash
acorde status    # Show peers, sync stats
```

---

## **17. Events & Subscriptions**

### Event Types
- `created` - Entry added
- `updated` - Entry modified
- `deleted` - Entry removed
- `synced` - Remote sync applied

### Subscription Options
- Filter by event types
- Filter by entry type
- Buffered channel (100 events)
- Close to unsubscribe

---

## **18. Docker Support**

### Docker Compose
```bash
docker-compose up -d
```
- Runs daemon mode
- Ports: 4001 (P2P), 7331 (API)
- Volume: `./data` persists to host
- Healthcheck via `/status`

### Dockerfile
- Multi-stage build
- Alpine-based (~20MB)
- Non-root user
- SQLite + CA certs included

---

## **19. Clock Recovery**

### Lamport Clock
- Monotonically increasing logical timestamps
- Survives restarts
- `GetMaxTimestamp()` from storage on startup
- Clock initialized to max(stored timestamps) + 1

---

## **20. Testing Features**

### Property Tests (Fuzzing)
- Commutativity: A ⊔ B = B ⊔ A
- Associativity: (A ⊔ B) ⊔ C = A ⊔ (B ⊔ C)
- Idempotence: A ⊔ A = A
- Convergence: All replicas reach identical state

### Unit Tests
- CRDT operations (LWW, OR-Set)
- Storage (SQLite)
- Sync protocol
- Encryption/decryption
- Clock operations
- Invite creation/parsing

---

## **21. Configuration**

### Sync Config
```go
Config{
    ListenAddrs: []string{"/ip4/0.0.0.0/tcp/4001"},
    SyncInterval: 5 * time.Second,
    EnableMDNS: true,
    EnableDHT: false,
    AllowlistPath: "",
    StrictAllowlist: false,
}
```

### Engine Config
```go
Config{
    DataDir: "./data",
    InMemory: false,
    EncryptionKey: &key,
    MaxVersions: 50,
}
```

---

## **Testing Checklist**

Start with these test scenarios:

### Basic Operations
- [ ] Create entry → verify stored
- [ ] Read entry → verify content
- [ ] Update entry → verify changes
- [ ] Delete entry → verify tombstone
- [ ] List entries with filters

### Sync
- [ ] Two nodes on same network (mDNS)
- [ ] Create entry on Node A → appears on Node B
- [ ] Concurrent updates → both merge correctly
- [ ] Offline edits → sync when reconnected

### Encryption
- [ ] Create vault with password
- [ ] Content encrypted at rest
- [ ] Decrypt on retrieval

### Pairing
- [ ] Generate invite with QR
- [ ] Pair second device
- [ ] Sync after pairing

### Advanced
- [ ] Schema validation (reject invalid)
- [ ] Version history (restore old)
- [ ] ACL (deny unauthorized peer)
- [ ] Webhooks (receive HTTP callback)
- [ ] Search (find by keyword)
- [ ] Import/Export (JSON/CSV/Markdown)

---

This is **everything** in ACORDE. Start testing from the top down!