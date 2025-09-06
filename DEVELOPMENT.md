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

## Quick Start

```bash
# One-time setup
make setup

# Development workflow
make lint-fix    # Format and fix issues
make test        # Run tests
make run-cli     # Test your changes
```

## Development Commands

```bash
# Setup (run once)
make setup

# Run applications
make run-cli
make run-engine  
make run-agent

# Run linter
make lint        # Check code quality
make lint-fix    # Auto-fix formatting and issues

# Testing
make test        # Run unit tests
make test-coverage # Run tests with coverage

# Build
make build       # Build all binaries
make clean       # Clean build artifacts
```

## Workspace

This project uses Go workspaces. Dependencies are managed via `go.work`.