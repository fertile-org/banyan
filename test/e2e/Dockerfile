# Banyan E2E Test Image
# Expects banyan-cli to be pre-built at test/e2e/bin/banyan-cli
# Run: cd cmd/banyan-cli && GOWORK=off CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -o ../../test/e2e/bin/banyan-cli .

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

# Download nerdctl (containerd CLI, used by agent for container operations)
RUN curl -L https://github.com/containerd/nerdctl/releases/download/v2.0.3/nerdctl-2.0.3-linux-amd64.tar.gz \
    | tar -xz -C /usr/local/bin nerdctl

# Copy pre-built banyan-cli
COPY bin/banyan-cli /usr/local/bin/banyan-cli
RUN chmod +x /usr/local/bin/banyan-cli

# Copy entrypoint scripts
COPY scripts/engine-entrypoint.sh /entrypoint-engine.sh
COPY scripts/agent-entrypoint.sh /entrypoint-agent.sh
RUN chmod +x /entrypoint-engine.sh /entrypoint-agent.sh

# Default command
CMD ["/bin/bash"]
