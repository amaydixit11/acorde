# Contributing to vaultd

Thank you for your interest in contributing to **vaultd**!

## Development Setup

```bash
# Clone the repository
git clone https://github.com/amaydixit11/vaultd.git
cd vaultd

# Install dependencies
go mod download

# Run tests
go test ./...

# Build CLI
cd cmd/vaultd
go build -o ../../vaultd .

# Test the CLI
../../vaultd help
../../vaultd serve --port 8080
```

## Code Organization

```
vaultd/
├── pkg/                  # Public API (importable by users)
│   ├── engine/           # Core Engine, Query, Search, Blob, Sharing
│   ├── api/              # REST API Server
│   └── crypto/           # Cryptography Primitives
├── internal/             # Private Implementation
│   ├── core/             # Domain Models, Clock
│   ├── crdt/             # LWW-Set, OR-Set, Delta Sync
│   ├── engine/           # Engine Implementation, Events
│   ├── storage/          # SQLite Adapter
│   ├── sync/             # P2P Networking (libp2p)
│   ├── search/           # Bleve Full-Text Search
│   ├── sharing/          # Per-Entry Encryption
│   └── blob/             # Content-Addressed Storage
├── cmd/vaultd/           # CLI Application
├── examples/             # Example Applications
│   ├── notes-cli/        # Go CLI Example
│   └── notes-web/        # HTML/JS Web Example
└── docs/                 # Documentation
```

## Quick Commands

```bash
# Build and run REST API
cd cmd/vaultd && go build -o ../../vaultd . && cd ../..
./vaultd serve --port 8080

# Test REST API
curl http://localhost:8080/status
curl http://localhost:8080/entries
curl -X POST http://localhost:8080/entries -d '{"type":"note","content":"Test"}'

# Run specific package tests
go test ./internal/crdt/... -v
go test ./pkg/engine/... -v
go test -race ./...  # Race detector

# Build examples
cd examples/notes-cli && go build
```

## Guidelines

### Code
1. **Public API**: Only packages in `pkg/` are importable by users. `internal/` is private.
2. **Dependencies**: Minimize external deps. Core deps: `libp2p`, `sqlite3`, `bleve`.
3. **Error Handling**: Wrap errors with context using `fmt.Errorf("...: %w", err)`.
4. **Concurrency**: Use mutexes appropriately. Run `go test -race ./...` regularly.

### Tests
- Unit tests for logic (CRDT, Crypto, Query).
- Integration tests for flows (API, Sync).
- Property-based tests for CRDT convergence.

```bash
# Run all tests
go test ./...

# With Race Detector
go test -race ./...

# Verbose output
go test ./internal/engine/... -v
```

### Commits
Use [Conventional Commits](https://www.conventionalcommits.org/):
- `feat(api): add pagination to /entries`
- `fix(sync): resolve deadlock in handshake`
- `docs(readme): update API examples`
- `refactor(storage): optimize batch operations`
- `test(crdt): add property-based tests`

## Pull Request Process

1. Fork the repository.
2. Create a feature branch (`git checkout -b feat/my-feature`).
3. Implement changes with tests.
4. Ensure all tests pass (`go test ./...`).
5. Commit with conventional messages.
6. Open a Pull Request.

## Feature Areas

### Adding a New Entry Type
1. Add constant in `internal/core/entry.go`
2. Add validation in `pkg/engine/engine.go`
3. Update tests

### Adding a New API Endpoint
1. Add handler in `pkg/api/api.go`
2. Update documentation

### Adding a New Query Operator
1. Extend parser in `pkg/engine/query.go`
2. Add tests for new operator

### Adding a New Event Type
1. Add constant in `internal/engine/events.go`
2. Publish from appropriate engine method

## Security

If modifying `internal/sharing` or key handling:
- **NEVER** log keys or passwords.
- **ALWAYS** use proper key derivation (HKDF, Argon2id).
- Ensure backward compatibility for KeyStore format.
- Run security review for crypto changes.

## Questions?

Open an issue or reach out to maintainers.
