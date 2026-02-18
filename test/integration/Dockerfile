# Banyan Integration Test Container
# Based on Kubernetes "kind" approach: run containerd INSIDE the container
# This allows testing network utilities, CNI plugins, and containerd without host dependencies

FROM golang:1.24-alpine

# Install system dependencies
RUN apk add --no-cache \
    # Container runtime
    containerd \
    containerd-ctr \
    # Container management CLI (nerdctl)
    nerdctl \
    # CNI plugins
    cni-plugins \
    # Network utilities
    iptables \
    iptables-legacy \
    iproute2 \
    bridge-utils \
    net-tools \
    # DNS tools
    bind-tools \
    # Process management
    supervisor \
    # etcd client
    etcd-ctl \
    # General utilities
    bash \
    curl \
    wget \
    jq \
    ca-certificates \
    sudo \
    # Build dependencies
    git \
    make \
    gcc \
    musl-dev

# Configure sudo to work without password for root
RUN echo "root ALL=(ALL) NOPASSWD: ALL" >> /etc/sudoers

# Install Flannel binaries
RUN mkdir -p /opt/cni/bin /etc/cni/net.d /var/lib/cni/networks /var/run/netns && \
    # Flannel daemon
    wget -q -O /usr/local/bin/flanneld https://github.com/flannel-io/flannel/releases/download/v0.22.0/flanneld-amd64 && \
    chmod +x /usr/local/bin/flanneld && \
    # Flannel CNI plugin
    wget -q -O /opt/cni/bin/flannel https://github.com/flannel-io/cni-plugin/releases/download/v1.2.0/flannel-amd64 && \
    chmod +x /opt/cni/bin/flannel

# Copy CNI plugins from system location to standard path (if different)
RUN if [ -d /usr/libexec/cni ]; then cp -a /usr/libexec/cni/* /opt/cni/bin/; fi

# Create etcd data directory
RUN mkdir -p /var/lib/etcd

# Install etcd server (for multi-host tests)
RUN wget -q -O /tmp/etcd.tar.gz https://github.com/etcd-io/etcd/releases/download/v3.5.9/etcd-v3.5.9-linux-amd64.tar.gz && \
    tar -xzf /tmp/etcd.tar.gz -C /tmp && \
    mv /tmp/etcd-v3.5.9-linux-amd64/etcd /usr/local/bin/ && \
    mv /tmp/etcd-v3.5.9-linux-amd64/etcdctl /usr/local/bin/ && \
    rm -rf /tmp/etcd*

# Configure containerd
RUN mkdir -p /etc/containerd
COPY test/integration/containerd-config.toml /etc/containerd/config.toml

# Configure supervisord
COPY test/integration/supervisord.conf /etc/supervisord.conf

# Set up working directory
WORKDIR /app

# Copy the entire project (multi-module workspace)
COPY . .

# Download dependencies and build banyan-cli
RUN cd /app && go work sync && go build -o /tmp/banyan-cli ./cmd/banyan-cli/

# Copy entrypoint script
COPY test/integration/entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh

# Expose etcd port
EXPOSE 2379

ENTRYPOINT ["/entrypoint.sh"]
CMD ["all"]
