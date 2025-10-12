.PHONY: setup run lint test build clean

# Development setup (run once)
setup: install-dependencies setup-hooks
	@echo "Development environment ready!"

# Install git hooks
setup-hooks:
	git config core.hooksPath .githooks
	@echo "Git hooks configured!"

# Install development dependencies
install-dependencies:
	go install github.com/golangci/golangci-lint/cmd/golangci-lint@latest
	go work sync

# Run applications
run-cli:
	go run cmd/cli/main.go

run-engine:
	go run cmd/engine/main.go

run-agent:
	go run cmd/agent/main.go

# Lint and format code
lint:
	cd cmd/cli && $(shell go env GOPATH)/bin/golangci-lint run
	cd cmd/engine && $(shell go env GOPATH)/bin/golangci-lint run
	cd cmd/agent && $(shell go env GOPATH)/bin/golangci-lint run
	cd pkg/interfaces && $(shell go env GOPATH)/bin/golangci-lint run
	cd pkg/plugin-sdk && $(shell go env GOPATH)/bin/golangci-lint run
	cd internal/common && $(shell go env GOPATH)/bin/golangci-lint run

# Auto-fix linting issues
lint-fix:
	cd cmd/cli && $(shell go env GOPATH)/bin/golangci-lint run --fix
	cd cmd/engine && $(shell go env GOPATH)/bin/golangci-lint run --fix
	cd cmd/agent && $(shell go env GOPATH)/bin/golangci-lint run --fix
	cd pkg/interfaces && $(shell go env GOPATH)/bin/golangci-lint run --fix
	cd pkg/plugin-sdk && $(shell go env GOPATH)/bin/golangci-lint run --fix
	cd internal/common && $(shell go env GOPATH)/bin/golangci-lint run --fix

# Run tests
test:
	go test ./test/unit/...

# Run tests without logs
test-verbose:
	go test -v ./test/unit/...

# Run tests with coverage
test-coverage:
	go test -cover ./test/unit/...

# Test specific module (usage: make test-module MODULE=pkg/vpc/network)
# Examples:
#   make test-module MODULE=pkg/vpc/network
#   make test-module MODULE=pkg/vpc/network VERBOSE=1
#   make test-module MODULE=pkg/vpc/ipam
test-module:
ifndef MODULE
	@echo "Error: MODULE parameter is required"
	@echo "Usage: make test-module MODULE=pkg/vpc/network"
	@echo "       make test-module MODULE=pkg/vpc/network VERBOSE=1"
	@exit 1
endif
ifdef VERBOSE
	cd $(MODULE) && go test -v ./...
else
	cd $(MODULE) && go test ./...
endif

# VPC-specific test shortcuts
test-vpc-network:
	cd pkg/vpc && go test -v ./network/

test-vpc-all:
	cd pkg/vpc && go test ./...

# Build all binaries
build:
	go build -o bin/banyan-cli cmd/cli/main.go
	go build -o bin/banyan-engine cmd/engine/main.go
	go build -o bin/banyan-agent cmd/agent/main.go

# Clean build artifacts
clean:
	rm -rf bin/