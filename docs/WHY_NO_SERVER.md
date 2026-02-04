# Why No Server?

acorde is architected as a **Local-First, Peer-to-Peer** system. This was a deliberate design choice prioritizing privacy, ownership, and longevity.

## The "Cloud" Problem

In a traditional Client-Server architecture (Google Drive, Notion, iCloud):
1.  **You verify nothing**: You trust the server to not read your data.
2.  **You own nothing**: If the company shuts down or bans your account, you lose everything.
3.  **No Internet = No Data**: Functionality is often crippled offline.

## The acorde Solution

### 1. You Own The Data
Your data lives on **your disk** (SQLite endpoint). It is not "cached" there; it is *stored* there.
-   If you delete the acorde binary, your data remains in standard formats (`~/.acorde/vault.db`).
-   Encryption keys never leave your devices (except when you explicitly share them via Inter-Device Invite).

### 2. Privacy by Architecture
Since there is no central server:
-   There is no "admin" who can reset your password or see your files.
-   Sync happens directly between your devices (Laptop <-> Phone).
-   If devices are on different networks, they use a DHT (Distributed Hash Table) to find each other, but the traffic is end-to-end encrypted. Even the DHT relays cannot read it.

### 3. Longevity
acorde is software, not a service. As long as you have the binary, it will work—forever. It does not depend on a subscription or an API that might be deprecated.

## Trade-offs
-   **Responsibility**: You are responsible for your backups. If you lose all your devices and your password, the data is gone.
-   **Availability**: Sync requires devices to be online at the same time (or use a dedicated "Always-On" peer like a Raspberry Pi).
