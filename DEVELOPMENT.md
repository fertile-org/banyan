# Development Guide

## Structure

```
banyan/
├── cmd/
│   ├── cli/           # CLI binary (user's machine/CI)
│   ├── engine/        # Engine binary (orchestrator server)
│   └── agent/         # Agent binary (target servers)
├── pkg/
│   ├── interfaces/    # Public interfaces (Engine, Agent)
│   └── plugin-sdk/    # Plugin SDK for community developers
├── internal/
│   └── common/        # Private shared utilities/types
└── test/
    ├── unit/          # Unit tests
    └── integration/   # Integration tests (future)
```

## Requirements

- Go 1.21+

## Install Dependencies

```bash
# Install all dependencies across workspace
go work sync

# Or install per module
go mod tidy -C cmd/cli
go mod tidy -C cmd/engine  
go mod tidy -C cmd/agent
go mod tidy -C internal/common
go mod tidy -C pkg/interfaces
go mod tidy -C pkg/plugin-sdk
go mod tidy -C test
```

## Run Commands

```bash
# Run CLI
go run cmd/cli/main.go

# Run Engine
go run cmd/engine/main.go

# Run Agent
go run cmd/agent/main.go
```

## Build Commands

```bash
go build -o bin/banyan-cli cmd/cli/main.go
go build -o bin/banyan-engine cmd/engine/main.go
go build -o bin/banyan-agent cmd/agent/main.go
```

## Test Commands

```bash
# Run tests with verbose output
go test -v ./test/unit/...

# Run tests with coverage
go test -cover ./test/unit/...
```

## Workspace

This project uses Go workspaces. Dependencies are managed via `go.work`.