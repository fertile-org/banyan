# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

Banyan is a Docker Compose deployment orchestrator that allows deploying docker-compose.yaml files to various cloud and on-premises servers with production-ready CI/CD and monitoring. The project uses a multi-binary architecture with three main components:

- **CLI**: User interface for deployment commands (runs on user's machine/CI)
- **Engine**: Orchestrator server that manages deployments across multiple agents
- **Agent**: Lightweight deployment agent that runs on target servers

## Architecture

The project follows a modular Go workspace structure with clear separation of concerns:

```
banyan/
├── cmd/                    # Binary entry points
│   ├── cli/               # CLI binary (user's machine/CI)  
│   ├── engine/            # Engine binary (orchestrator server)
│   └── agent/             # Agent binary (target servers)
├── pkg/                   # Public APIs
│   ├── interfaces/        # Core interfaces (Engine, Agent)
│   └── plugin-sdk/        # Plugin SDK for community developers
├── internal/              # Private shared code
│   └── common/           # Shared utilities and types
└── test/                 # Test suites
    ├── unit/             # Unit tests
    └── integration/      # Integration tests (future)
```

### Key Interfaces

- **Engine Interface**: Defines deployment orchestration (Deploy, GetStatus, Cancel)
- **Agent Interface**: Defines remote execution (Execute, HealthCheck)
- **DeploymentConfig**: Core deployment configuration structure
- **Plugin SDK**: Extensibility framework for custom providers and strategies

## Development Commands

### Setup and Dependencies
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

### Running Components
```bash
# Run CLI (development)
go run cmd/cli/main.go

# Run Engine (development)
go run cmd/engine/main.go

# Run Agent (development)
go run cmd/agent/main.go
```

### Building Binaries
```bash
go build -o bin/banyan-cli cmd/cli/main.go
go build -o bin/banyan-engine cmd/engine/main.go
go build -o bin/banyan-agent cmd/agent/main.go
```

### Testing
```bash
# Run unit tests with verbose output
go test -v ./test/unit/...

# Run tests with coverage
go test -cover ./test/unit/...
```

## Code Patterns and Conventions

### Module Dependencies
- All modules depend on `internal/common` for shared utilities
- The workspace uses Go 1.21+ with local module replacements
- Dependencies are managed via go.work across all modules
- Uses testify for unit testing and logrus with JSON formatting

### Interface Design
- Clean interface separation between Engine and Agent components
- Context-aware operations with proper cancellation support
- Structured error handling with domain-specific error types
- JSON serialization for deployment configurations and status

### Plugin Architecture
- Plugin SDK provides validation utilities and common patterns
- Extensible design for cloud providers, deployment strategies, and monitoring
- Version-aware plugin system with name/version identification

## Current Implementation Status

The project is in early development (v0.1.0-dev) with basic structure in place:
- ✅ Module structure and workspace configuration
- ✅ Core interfaces defined
- ✅ Basic plugin SDK framework
- ✅ Unit test structure with testify
- 🚧 CLI, Engine, and Agent implementations (stubs only)
- 🚧 Docker Compose parsing
- 🚧 Cloud provider integrations
- 🚧 Monitoring and observability features