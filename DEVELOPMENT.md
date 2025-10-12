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
# One-time setup (includes pre-commit hooks)
make setup

# Development workflow
make lint-fix    # Format and fix issues
make test        # Run tests
make run-cli     # Test your changes
```

**Note**: `make setup` configures Git pre-commit hooks that automatically run `make lint` and `make test` before each commit.

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
make test                                    # Run unit tests
make test-verbose                            # Run unit tests with verbose logging
make test-coverage                           # Run tests with coverage
make test-module MODULE=pkg/vpc/network      # Test specific module
make test-module MODULE=pkg/vpc/network VERBOSE=1  # Test module with verbose output

# Build
make build       # Build all binaries
make clean       # Clean build artifacts
```

## Workspace

This project uses Go workspaces. Dependencies are managed via `go.work`.

## Troubleshooting

### Go PATH and Tools Issues

If you encounter "Go is not installed" or "gopls is not installed" errors when using MCP tools (like Serena), even though Go is installed, it's likely a PATH issue. To resolve this:

```bash
# Install gopls (Go language server)
go install golang.org/x/tools/gopls@latest

# Add Go bin directory to PATH permanently
echo 'export PATH=$PATH:$(go env GOPATH)/bin' >> ~/.bashrc
source ~/.bashrc

# Optional: Create symlinks to make Go available system-wide
sudo ln -sf /usr/local/go/bin/* /usr/bin/
```

This ensures gopls is installed and the Go bin directory is in your PATH for all sessions. The symlink step creates symbolic links from the Go installation (typically `/usr/local/go/bin/`) to the system binaries directory (`/usr/bin/`), making Go accessible to all processes including MCP servers.

**Note**: This assumes Go is installed in the standard location `/usr/local/go/`. Adjust the path if your Go installation is elsewhere.