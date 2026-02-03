# Contributing to vaultd

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
go build -o vaultd ./cmd/vaultd
```

## Code Organization

```
vaultd/
├── pkg/engine/      # Public API (import this)
├── internal/
│   ├── core/        # Domain model
│   ├── engine/      # Engine implementation
│   └── storage/     # Storage layer
└── cmd/vaultd/      # Test CLI
```

## Guidelines

1. **Public API**: Only `pkg/engine/` is public. Don't import `internal/`.
2. **Tests**: Add tests for new functionality. Aim for >70% coverage.
3. **Commits**: Use conventional commits (`feat:`, `fix:`, `test:`, `docs:`).

## Testing

```bash
# Run all tests
go test ./...

# With coverage
go test ./... -cover

# Verbose
go test ./... -v
```

## Pull Request Process

1. Fork the repository
2. Create a feature branch (`git checkout -b feat/my-feature`)
3. Make your changes
4. Run tests (`go test ./...`)
5. Commit with conventional commit messages
6. Open a pull request
