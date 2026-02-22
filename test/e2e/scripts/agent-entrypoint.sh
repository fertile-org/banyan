#!/bin/bash
set -e

echo "========================================="
echo "Banyan Agent Container"
echo "========================================="

# Get node name from environment or hostname
NODE_NAME=${NODE_NAME:-$(hostname)}
ENGINE_HOST=${ENGINE_HOST:-engine}
ENGINE_GRPC_PORT=${ENGINE_GRPC_PORT:-50051}

# Default E2E password (must match engine password)
E2E_PASSWORD=${E2E_PASSWORD:-banyan-e2e-secret}

# Write config file (everything except password — that goes via --password flag)
echo "Writing config..."
mkdir -p /etc/banyan
cat > /etc/banyan/banyan.yaml <<EOF
agent:
    engine_host: ${ENGINE_HOST}
    engine_port: "${ENGINE_GRPC_PORT}"
EOF
chmod 600 /etc/banyan/banyan.yaml

# Wait for engine gRPC to be ready (TCP check on gRPC port)
echo "Waiting for engine gRPC at ${ENGINE_HOST}:${ENGINE_GRPC_PORT}..."
until nc -z "${ENGINE_HOST}" "${ENGINE_GRPC_PORT}" 2>/dev/null; do
    echo "Engine not ready, waiting..."
    sleep 2
done
echo "Engine is ready!"

# Initialize agent (password via flag to avoid interactive prompt)
echo "Initializing agent..."
banyan-agent init --password "${E2E_PASSWORD}"

# Start containerd in background
echo "Starting containerd..."
containerd &
sleep 2

# Start agent
echo "Starting agent..."
exec banyan-agent start --node-name "$NODE_NAME"
