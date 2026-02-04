# Security Model & Encryption Specification

## Threat Model

**vaultd** assumes the following:
1.  **Device Compromise**: If an attacker gains full access to a running, unlocked device, they can read the data (Memory is not encrypted).
2.  **Network Compromise**: An attacker on the network (MITM) cannot read sync traffic or inject invalid data.
3.  **Storage Theft**: An attacker stealing the physical disk (`~/.vaultd`) cannot read data without the password.
4.  **Malicious Peer**: A malicious peer *without* the key can store/relay data but cannot read it. A malicious peer *with* the key can corrupt data (detected by AEAD authentication failure).

## Encryption Specifications

### 1. Key Derivation (KDF)
The Master Key is encrypted using a "Wrapper Key" derived from the user's password.

-   **Algorithm**: Argon2id
-   **Memory**: 64 MB (`64 * 1024` KB)
-   **Iterations**: 3
-   **Parallelism**: 2 threads
-   **Salt**: 16 bytes (Random)

### 2. Content Encryption
Entry content is encrypted using **AEAD** (Authenticated Encryption with Associated Data).

-   **Algorithm**: XChaCha20-Poly1305
    -   *Why?* Native Go support, resistant to nonce reuse (192-bit nonce), high performance.
-   **Key**: 32-byte Master Key.
-   **Nonce**: 24 bytes (Randomly generated per encryption).
-   **AAD (Associated Data)**: `Entry.ID`
    -   *Why?* Binds the ciphertext to a specific Entry ID. Prevents "replay" or "swapping" content between entries.

### 3. Key Storage
Keys are stored in `~/.vaultd/keys.json`.

```json
{
  "salt": "<base64_salt>",
  "data": "<base64_encrypted_master_key>",
  "params": {
    "mem": 65536,
    "time": 3,
    "threads": 2
  }
}
```

## Workflows

### Initialization (`vaultd init`)
1.  User inputs Password.
2.  Generate random `MasterKey` (32 bytes).
3.  Generate random `Salt` (16 bytes).
4.  Derive `WrapperKey` = `Argon2id(Password, Salt)`.
5.  Encrypt `MasterKey` with `WrapperKey` (AAD = directory path).
6.  Save to disk.

### Unlocking (`vaultd daemon`)
1.  User inputs Password.
2.  Read `Salt` and `EncryptedMasterKey` from disk.
3.  Derive `WrapperKey`.
4.  Decrypt `MasterKey`.
    -   If integrity check fails: Incorrect password.
5.  Keep `MasterKey` in memory for duration of process.

### Pairing (`vaultd pair`)
1.  **Inviter** encrypts the `MasterKey` using a temporary ephemeral key (PeerID + Time).
2.  **Inviter** encodes result into Invite Link.
3.  **Receiver** imports Invite.
4.  **Receiver** prompts user for *new* local password.
5.  **Receiver** re-encrypts the `MasterKey` with their new password and saves to their disk.
    -   *Result*: Both devices share the same `MasterKey`, but typically protect it with different local passwords.
