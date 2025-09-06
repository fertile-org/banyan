# Development Guide

## Structure

```
banyan/
├── cmd/
│   ├── cli/           # CLI binary (user's machine/CI)
│   ├── engine/        # Engine binary (orchestrator server)
│   └── agent/         # Agent binary (target servers)
├── pkg/
│   ├── common/        # Shared utilities/types
│   └── plugins/       # Plugin system
└── internal/
    └── version/
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
go mod tidy -C pkg/common
go mod tidy -C pkg/plugins
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

## Workspace

This project uses Go workspaces. Dependencies are managed via `go.work`.