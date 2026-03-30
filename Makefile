.PHONY: setup run lint test build clean test-integration test-integration-build test-integration-shell proto demo

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
run-engine:
	go run ./cmd/banyan-engine

run-agent:
	go run ./cmd/banyan-agent

run-cli:
	go run ./cmd/banyan-cli

# Generate protobuf code
proto:
	protoc --proto_path=pkg/rpc/proto/banyan/v1 \
		--go_out=pkg/rpc/banyanpb --go_opt=paths=source_relative \
		--go-grpc_out=pkg/rpc/banyanpb --go-grpc_opt=paths=source_relative \
		--connect-go_out=pkg/rpc/banyanpb --connect-go_opt=paths=source_relative \
		pkg/rpc/proto/banyan/v1/*.proto

# Lint and format code
lint:
	cd pkg/engine && $(shell go env GOPATH)/bin/golangci-lint run
	cd pkg/agent && $(shell go env GOPATH)/bin/golangci-lint run
	cd cmd/banyan-engine && $(shell go env GOPATH)/bin/golangci-lint run
	cd cmd/banyan-agent && $(shell go env GOPATH)/bin/golangci-lint run
	cd cmd/banyan-cli && $(shell go env GOPATH)/bin/golangci-lint run
	cd pkg/types && $(shell go env GOPATH)/bin/golangci-lint run
	cd pkg/rpc && $(shell go env GOPATH)/bin/golangci-lint run
	cd internal/common && $(shell go env GOPATH)/bin/golangci-lint run

# Auto-fix linting issues
lint-fix:
	cd pkg/engine && $(shell go env GOPATH)/bin/golangci-lint run --fix
	cd pkg/agent && $(shell go env GOPATH)/bin/golangci-lint run --fix
	cd cmd/banyan-engine && $(shell go env GOPATH)/bin/golangci-lint run --fix
	cd cmd/banyan-agent && $(shell go env GOPATH)/bin/golangci-lint run --fix
	cd cmd/banyan-cli && $(shell go env GOPATH)/bin/golangci-lint run --fix
	cd pkg/types && $(shell go env GOPATH)/bin/golangci-lint run --fix
	cd pkg/rpc && $(shell go env GOPATH)/bin/golangci-lint run --fix
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

# Build all binaries
build:
	go build -o bin/banyan-engine ./cmd/banyan-engine
	go build -o bin/banyan-agent ./cmd/banyan-agent
	go build -o bin/banyan-cli ./cmd/banyan-cli

# Clean build artifacts
clean:
	rm -rf bin/

# ============================================================================
# Integration Tests (DinD-based)
# ============================================================================

# Build (or rebuild) the integration test Docker image
# Usage: make test-integration-build
test-integration-build:
	./test/integration/run-integration-tests.sh --build

# Run integration test(s) - builds image if needed
# Usage: make test-integration                                                    # Run all tests
#        make test-integration FILE=./test/integration/vpc/run_dns_integration.go # Run specific test
test-integration:
ifdef FILE
	./test/integration/run-integration-tests.sh $(FILE)
else
	./test/integration/run-integration-tests.sh all
endif

# Start a debug shell inside the integration test container
# Usage: make test-integration-shell
test-integration-shell:
	./test/integration/run-integration-tests.sh shell

# List available integration tests
# Usage: make test-integration-list
test-integration-list:
	@echo "Available integration tests:"
	@find ./test/integration -name "run_*.go" -type f | sort

# Record terminal demo (requires vhs: https://github.com/charmbracelet/vhs)
demo:
	vhs demo.tape
