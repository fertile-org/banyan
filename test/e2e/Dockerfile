# Banyan E2E Test Image
# Expects three binaries to be pre-built at test/e2e/bin/
# Run: see run-e2e.sh for build commands

FROM debian:bookworm-slim

# Install runtime dependencies
RUN apt-get update && apt-get install -y \
    etcd-server \
    containerd \
    iptables \
    iproute2 \
    ethtool \
    tcpdump \
    curl \
    netcat-openbsd \
    procps \
    ca-certificates \
    wireguard-tools \
    && rm -rf /var/lib/apt/lists/*

# Create CNI directories
RUN mkdir -p /etc/cni/net.d /opt/cni/bin /var/lib/banyan /var/lib/banyan/store /var/run

# Download CNI plugins
RUN curl -L https://github.com/containernetworking/plugins/releases/download/v1.6.2/cni-plugins-linux-amd64-v1.6.2.tgz \
    | tar -xz -C /opt/cni/bin

# Download nerdctl (containerd CLI, used by agent for container operations)
# v2.1.3+ required for --health-cmd support
RUN curl -L https://github.com/containerd/nerdctl/releases/download/v2.1.3/nerdctl-2.1.3-linux-amd64.tar.gz \
    | tar -xz -C /usr/local/bin nerdctl

# Download Distribution registry (OCI image storage for engine)
RUN curl -L https://github.com/distribution/distribution/releases/download/v2.8.3/registry_2.8.3_linux_amd64.tar.gz \
    | tar -xz -C /usr/local/bin registry && chmod +x /usr/local/bin/registry

# Copy pre-built binaries (cache-bust file forces rebuild when binaries change)
COPY bin/.cache-bust /tmp/.cache-bust
COPY bin/banyan-engine /usr/local/bin/banyan-engine
COPY bin/banyan-agent /usr/local/bin/banyan-agent
COPY bin/banyan-cli /usr/local/bin/banyan-cli
RUN chmod +x /usr/local/bin/banyan-engine /usr/local/bin/banyan-agent /usr/local/bin/banyan-cli

# Copy entrypoint scripts
COPY scripts/engine-entrypoint.sh /entrypoint-engine.sh
COPY scripts/agent-entrypoint.sh /entrypoint-agent.sh
COPY scripts/engine-ha-entrypoint.sh /entrypoint-engine-ha.sh
COPY scripts/agent-ha-entrypoint.sh /entrypoint-agent-ha.sh
RUN chmod +x /entrypoint-engine.sh /entrypoint-agent.sh /entrypoint-engine-ha.sh /entrypoint-agent-ha.sh

# Default command
CMD ["/bin/bash"]
