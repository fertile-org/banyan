#!/bin/bash
set -e

echo "========================================="
echo "Banyan Agent Container"
echo "========================================="

# Get node name from environment or hostname
NODE_NAME=${NODE_NAME:-$(hostname)}
ENGINE_ENDPOINT=${ENGINE_ENDPOINT:-http://engine:2379}

# Default E2E password (must match engine password)
E2E_PASSWORD=${E2E_PASSWORD:-banyan-e2e-secret}

# Parse host and port from ENGINE_ENDPOINT (e.g. http://engine:2379 → engine / 2379)
ENGINE_HOST=$(echo "$ENGINE_ENDPOINT" | sed -E 's|https?://([^:]+):.*|\1|')
ENGINE_PORT=$(echo "$ENGINE_ENDPOINT" | sed -E 's|https?://[^:]+:([0-9]+).*|\1|')

# Write config file with password and engine connection
echo "Writing config..."
mkdir -p /etc/banyan
cat > /etc/banyan/banyan.yaml <<EOF
security:
    auth_type: password
    password: ${E2E_PASSWORD}
agent:
    engine_host: ${ENGINE_HOST}
    engine_port: "${ENGINE_PORT}"
EOF
chmod 600 /etc/banyan/banyan.yaml

# Wait for engine to be ready
echo "Waiting for engine at $ENGINE_ENDPOINT..."
until curl -sf "$ENGINE_ENDPOINT/health" > /dev/null 2>&1 || etcdctl --endpoints="$ENGINE_ENDPOINT" endpoint health > /dev/null 2>&1; do
    echo "Engine not ready, waiting..."
    sleep 2
done
echo "Engine is ready!"

# Initialize agent
echo "Initializing agent..."
echo "" | banyan-cli agent init

# Start containerd in background
echo "Starting containerd..."
containerd &
sleep 2

# Start agent
echo "Starting agent..."
exec banyan-cli agent start --node-name "$NODE_NAME"
