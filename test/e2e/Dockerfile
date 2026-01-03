# Banyan E2E Test Image
# Multi-stage build for smaller final image

FROM golang:1.24-bookworm AS builder

WORKDIR /src

# Copy all source code (go.work will be ignored via GOWORK=off)
COPY . .

# Build banyan-cli without go.work (it has its own go.mod with replace directives)
RUN cd cmd/banyan-cli && GOWORK=off go build -o /banyan-cli .

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
