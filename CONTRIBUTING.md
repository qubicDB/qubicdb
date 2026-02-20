# Contributing to QubicDB

Thank you for your interest in contributing to QubicDB.

## Install

```bash
# Install server binary
go install github.com/qubicDB/qubicdb/cmd/qubicdb@latest

# Install CLI
go install github.com/qubicDB/qubicdb/cmd/qubicdb-cli@latest
```

## Getting Started (from source)

```bash
git clone https://github.com/qubicDB/qubicdb.git
cd qubicdb
go mod download
go build ./...
go test ./...
```

Requires Go 1.21+.

## Project Structure

```
cmd/
  qubicdb/        # Server entry point
  qubicdb-cli/    # CLI REPL entry point
pkg/
  api/            # HTTP server, routes, middleware
  core/           # Config, types, matrix bounds
  concurrency/    # Worker pool, per-index brain workers
  engine/         # Neuron engine, search, recall
  synapse/        # Hebbian learning engine
  lifecycle/      # Active/Idle/Sleeping/Dormant state machine
  persistence/    # On-disk storage
  registry/       # UUID registry
  sentiment/      # VADER sentiment layer
  vector/         # Vector search (optional, requires libllama_go)
  mcp/            # Model Context Protocol handler
  protocol/       # Wire format helpers
```

## How to Contribute

### Bug Reports

Open an issue with:
- QubicDB version / Docker tag
- Steps to reproduce
- Expected vs actual behavior
- Relevant logs

### Feature Requests

Open an issue describing the use case before submitting a PR. QubicDB has a deliberate design philosophy around organic memory mechanics — changes that conflict with the core model will not be accepted.

### Pull Requests

1. Fork the repository
2. Create a feature branch: `git checkout -b feat/your-feature`
3. Write tests for your changes
4. Ensure all tests pass: `go test ./...`
5. Keep commits focused and descriptive
6. Open a PR against `main`

### Code Style

- Follow standard Go conventions (`gofmt`, `go vet`)
- No external dependencies without prior discussion
- Keep the core memory model intact — Hebbian learning, lifecycle states, and fractal clustering are not optional

## Running Tests

```bash
# All tests
go test ./...

# Specific package
go test ./pkg/api/...

# With race detector
go test -race ./...
```

## Vector Layer (Optional)

The vector search layer requires `libllama_go`. Without it, QubicDB runs in lexical-only mode — all other functionality is unaffected. See `pkg/vector/` for build instructions.

## License

By contributing, you agree that your contributions will be licensed under the MIT License.
