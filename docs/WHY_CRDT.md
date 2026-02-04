# Why CRDTs?

acorde uses **Conflict-free Replicated Data Types (CRDTs)** as the foundation of its synchronization engine. This document explains the rationale, the specific types used, and the guarantees they provide.

## The Problem: Data Consistency in Distributed Systems

In a local-first, peer-to-peer system like acorde:
1.  **No Central Authority**: There is no server to act as the "source of truth".
2.  **Concurrent Edits**: Users might edit the same note on their Phone and Laptop simultaneously while offline.
3.  **Network Partitions**: Devices might sync out of order (Phone -> Tablet -> Laptop).

Traditional approaches like "Last Writer Wins" (based on wall clock) or "Git-style merging" (manual resolution) are either prone to data loss or user friction.

## The Solution: Strong Eventual Consistency

CRDTs provide a mathematical guarantee: **If two replicas have seen the same set of updates, they will be in the same state.**

This property holds regardless of:
-   The order updates are received.
-   The network topology (Star, Mesh, Line).
-   How many times an update is received (Idempotence).

## Implemented Types

### 1. Entries: LWW-Set (Last-Write-Wins Set)
For the content of entries (Notes, Logs), we use a variation of the LWW-Set.

-   **Structure**: Every entry has a Logical Timestamp (Lamport Clock).
-   **Merge Rule**:
    -   If `Timestamp(A) > Timestamp(B)`, A wins.
    -   If `Timestamp(A) == Timestamp(B)`, comparing the deterministic `PeerID` breaks the tie.

*Why?* For unstructured text/content, simple LWW is usually "good enough" for personal data and much cheaper to store than full text CRDTs (like RGA/Yjs).

### 2. Tags: OR-Set (Observed-Remove Set)
For tags, we need better semantics than LWW. If you add tag `#work` on Laptop, and remove tag `#todo` on Phone, a simple LWW merge might lose one of those changes.

The OR-Set preserves concurrent operations:
-   **Add**: Generates a unique token for that specific addition.
-   **Remove**: Adds that token to a "Tombstone Set".
-   **State**: An element exists if it is in the `AddSet` and NOT in the `RemoveSet`.

*Result*: If you concurrently add `#work` and remove `#todo`, the merged state safely reflects both intentions (has `#work`, lacks `#todo`).

## Mathematical Properties

To ensure robustness, our CRDT implementation is tested against three key invariants:

1.  **Commutativity**: `Merge(A, B) == Merge(B, A)`
    -   *Order doesn't matter.*
2.  **Associativity**: `Merge(Merge(A, B), C) == Merge(A, Merge(B, C))`
    -   *Grouping doesn't matter.*
3.  **Idempotence**: `Merge(A, A) == A`
    -   *Duplicate syncs don't corrupt state.*

These properties are verified via automated randomized property testing (Fuzzing) in our CI pipeline.
