#!/bin/bash
set -e

echo "========================================="
echo "Banyan Engine Container"
echo "========================================="

# Default E2E password (override with E2E_PASSWORD env var)
E2E_PASSWORD=${E2E_PASSWORD:-banyan-e2e-secret}

# Write config file with password before init (init prompts for stdin which doesn't work non-interactively)
echo "Writing config..."
mkdir -p /etc/banyan
cat > /etc/banyan/banyan.yaml <<EOF
security:
    auth_type: password
    password: ${E2E_PASSWORD}
engine:
    api_port: "8443"
cli:
    engine_host: localhost
    engine_port: "8443"
EOF
chmod 600 /etc/banyan/banyan.yaml

# Initialize engine
echo "Initializing engine..."
echo "" | banyan-engine init

# Start engine (this will also start etcd and the HTTP API)
echo "Starting engine..."
exec banyan-engine start --etcd-client-urls http://0.0.0.0:2379
