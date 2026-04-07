# Acorde UML Documentation

This document provides a visual representation of the Acorde architecture using Mermaid UML diagrams.

## 1. Class Diagram (System Architecture)

The following diagram shows the main components of the Acorde system and their relationships.

```mermaid
classDiagram
    class Engine {
        <<interface>>
        +AddEntry(input) Entry
        +GetEntry(id) Entry
        +UpdateEntry(id, input)
        +DeleteEntry(id)
        +GrantWrite(id, peerID)
        +ListEntries(filter) []Entry
        +Search(query, opts) SearchResult
        +GetSyncPayload() []byte
        +ApplyRemotePayload(payload)
        +Subscribe() Subscription
        +PeerID() string
        +Close()
    }
    class engineWrapper {
        -impl internal.Engine
    }

    class InternalEngineInterface {
        <<interface>>
        +AddEntry(input) Entry
        +GetEntry(id) Entry
        +UpdateEntry(id, input)
        +DeleteEntry(id)
        +ListEntries(filter) []Entry
        +ApplySyncState(state, peerID)
        +Subscribe() Subscription
        +ACL() *acl.Store
        +Versions() *version.Store
    }
    class engineImpl {
        -mu sync.Mutex
        -replica *crdt.Replica
        -store storage.Store
        -events *EventBus
        -schemas *schema.Registry
        -versions *version.Store
        -acls *acl.Store
        -hooks *hooks.Manager
        -localID string
    }

    class Replica {
        -mu sync.RWMutex
        -clock *core.Clock
        -entries map[uuid.UUID]LWWElement
        -tags map[uuid.UUID]*TagSet
        -acls map[uuid.UUID]core.ACL
        +AddEntry(type, content, tags) core.Entry
        +UpdateEntry(id, content, tags)
        +DeleteEntry(id)
        +Merge(other *Replica)
        +State() ReplicaState
    }

    class Store {
        <<interface>>
        +Put(entry)
        +Get(id) Entry
        +List(filter) []Entry
        +Delete(id)
        +Close()
    }
    class SQLiteStore {
        -db *sql.DB
    }

    class SyncService {
        <<interface>>
        +Start(ctx)
        +Stop()
        +SyncWith(ctx, peerID)
        +ConnectPeer(invite)
    }
    class p2pService {
        -host libp2p.Host
        -provider StateProvider
        -allowlist *Allowlist
        -mdnsService mdns.Service
        -dhtDiscovery *DHTDiscovery
        +handleStream(stream)
        +syncLoop()
    }
    class StateProvider {
        <<interface>>
        +StateHash() []byte
        +GetState() ReplicaState
        +ApplyState(state, peerID)
    }

    Engine <|.. engineWrapper
    InternalEngineInterface <|.. engineImpl
    engineWrapper --> InternalEngineInterface : wraps
    engineImpl --> Replica : manages state
    engineImpl --> Store : persists views
    engineImpl --> StateProvider : implements
    p2pService --> StateProvider : uses to get/apply state
    Store <|.. SQLiteStore
    SyncService <|.. p2pService
```

## 2. Sequence Diagram: Data Write Flow

This diagram illustrates what happens when a user adds a new entry via the Engine API.

```mermaid
sequenceDiagram
    participant App as Application
    participant Wrapper as engineWrapper
    participant Impl as engineImpl
    participant Replica as CRDT Replica
    participant Store as SQLite Store
    participant Bus as EventBus

    App->>Wrapper: AddEntry(content, tags)
    Wrapper->>Impl: AddEntry(input)
    
    Note over Impl: Encrypt content (if enabled)
    
    Impl->>Replica: AddEntry(type, content, tags)
    Replica-->>Impl: core.Entry (with Lamport Timestamp)
    
    Impl->>Store: Put(core.Entry)
    Note over Store: Write to entries table
    
    Impl->>Bus: Publish(EventCreated)
    
    Impl-->>App: engine.Entry
```

## 3. Sequence Diagram: P2P Synchronization Flow

This diagram shows the "Pull-Push" synchronization protocol between two peers (Alice and Bob).

```mermaid
sequenceDiagram
    participant Alice as Alice (Peer A)
    participant Bob as Bob (Peer B)
    
    Note over Alice, Bob: Discovery via mDNS/DHT
    
    Alice->>Bob: Connect (libp2p Stream)
    Alice->>Bob: MsgStateHash(AliceHash, SessionID)
    
    Note over Bob: Compare AliceHash with BobHash
    
    alt Hashes Match
        Bob-->>Alice: MsgStateHash(BobHash)
        Note over Alice, Bob: Sync Complete (No changes)
    else Hashes Mismatch
        Bob-->>Alice: MsgState(BobFullState)
        Note over Alice: Merge BobState into local CRDT
        Note over Alice: Update local SQLite View
        
        Alice->>Bob: MsgState(AliceMergedState)
        Note over Bob: Merge AliceState into local CRDT
        Note over Bob: Update local SQLite View
        
        Bob-->>Alice: MsgStateHash(NewBobHash) (Ack)
        Note over Alice, Bob: Converged!
    end
```

## 4. Component Diagram

```mermaid
graph TD
    subgraph "Application Layer"
        CLI[acorde CLI]
        API[REST API]
    end

    subgraph "Public API (pkg/engine)"
        PE[Engine Interface]
    end

    subgraph "Core Orchestration (internal/engine)"
        IE[engineImpl]
        EB[EventBus]
        SM[Schema Registry]
    end

    subgraph "Data & Logic"
        REP[CRDT Replica]
        ACL[ACL Store]
        VER[Version Store]
    end

    subgraph "Persistence (internal/storage)"
        SQL[SQLite]
        BLOB[Blob Store]
    end

    subgraph "Networking (internal/sync)"
        P2P[libp2p Service]
        MDNS[mDNS Discovery]
        DHT[DHT Discovery]
    end

    CLI --> PE
    API --> PE
    PE --> IE
    IE --> REP
    IE --> SQL
    IE --> ACL
    IE --> EB
    IE --> P2P
    P2P --> MDNS
    P2P --> DHT
    P2P <--> |State Exchange| REP
```
