# Contributing to vaultd

Thank you for your interest in contributing to **vaultd**!

## Development Setup

```bash
# Clone the repository
git clone https://github.com/amaydixit11/vaultd.git
cd vaultd

# Install dependencies
go mod download

# Run tests (ensure >80% pass)
go test ./...

# Build CLI
go build -o vaultd ./cmd/vaultd
```

## Code Organization

```
vaultd/
├── pkg/             # Public API
│   ├── engine/      # Main Engine Interface
│   └── crypto/      # Cryptography Primitives
├── internal/        # Private Implementation
│   ├── core/        # Domain Models
│   ├── crdt/        # Conflict-Free Data Types
│   ├── engine/      # Engine Implementation
│   ├── storage/     # SQLite Adapter
│   └── sync/        # P2P Networking (libp2p)
├── cmd/vaultd/      # Daemon CLI
├── docs/            # Architecture & Security Docs
└── tests/           # Integration Tests
```

## Guidelines

1.  **Public API**: Only packages in `pkg/` are importable by users. `internal/` is strict.
2.  **Dependencies**: Minimise external deps. Uses `libp2p` (networking), `sqlite` (storage), `crypto` (std/x).
3.  **Tests**:
    -   Unit tests for logic (CRDT, Crypto).
    -   Integration tests for flows.
    -   Run `go test ./... -race` to check for race conditions.
4.  **Commits**: Use [Conventional Commits](https://www.conventionalcommits.org/):
    -   `feat: add msgpack support`
    -   `fix: resolve deadlock in keystore`
    -   `docs: update architecture diagram`

## Testing

```bash
# Run all tests
go test ./...

# With Race Detector (Recommended)
go test -race ./...

# Run specific package
go test ./internal/crdt/... -v
```

## Pull Request Process

1.  Fork the repository.
2.  Create a feature branch (`git checkout -b feat/my-feature`).
3.  Implement changes.
4.  Ensure tests pass.
5.  Commit with conventional messages.
6.  Open a Pull Request.

## Encryption Implementation

If modifying `internal/crypto` or key handling:
-   **NEVER** log keys or passwords.
-   **ALWAYS** use AAD when encrypting structural data.
-   Ensure backward compatibility for KeyStore format.
