# Banyan E2E Test Image
# Multi-stage build for smaller final image

FROM golang:1.24-bookworm AS builder

WORKDIR /src

# Copy go.work and module files first for better caching
COPY go.work go.work
COPY cmd/banyan-cli/go.mod cmd/banyan-cli/go.sum ./cmd/banyan-cli/
COPY pkg/vpc/go.mod pkg/vpc/go.sum ./pkg/vpc/
COPY pkg/engine/go.mod pkg/engine/go.sum ./pkg/engine/
COPY internal/common/go.mod internal/common/go.sum ./internal/common/

# Download dependencies
RUN cd cmd/banyan-cli && go mod download

# Copy source code
COPY . .

# Build banyan-cli
RUN go build -o /banyan-cli ./cmd/banyan-cli

# Final image
FROM debian:bookworm-slim

# Install runtime dependencies
RUN apt-get update && apt-get install -y \
    etcd-server \
    containerd \
    iptables \
    iproute2 \
    curl \
    procps \
    ca-certificates \
    && rm -rf /var/lib/apt/lists/*

# Create CNI directories
RUN mkdir -p /etc/cni/net.d /opt/cni/bin /var/lib/banyan /var/run

# Download CNI plugins
RUN curl -L https://github.com/containernetworking/plugins/releases/download/v1.4.0/cni-plugins-linux-amd64-v1.4.0.tgz \
    | tar -xz -C /opt/cni/bin

# Copy banyan-cli from builder
COPY --from=builder /banyan-cli /usr/local/bin/banyan-cli

# Copy entrypoint scripts
COPY test/e2e/scripts/engine-entrypoint.sh /entrypoint-engine.sh
COPY test/e2e/scripts/agent-entrypoint.sh /entrypoint-agent.sh
RUN chmod +x /entrypoint-engine.sh /entrypoint-agent.sh

# Default command
CMD ["/bin/bash"]
