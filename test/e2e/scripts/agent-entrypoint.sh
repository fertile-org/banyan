#!/bin/bash
set -e

echo "========================================="
echo "Banyan Agent Container"
echo "========================================="

# Get node name from environment or hostname
NODE_NAME=${NODE_NAME:-$(hostname)}
ENGINE_HOST=${ENGINE_HOST:-engine}
ENGINE_GRPC_PORT=${ENGINE_GRPC_PORT:-50051}

# 1. Write base config (engine host + port, so init sees it as "already configured")
echo "Writing config..."
mkdir -p /etc/banyan
cat > /etc/banyan/banyan.yaml <<EOF
agent:
    engine_host: ${ENGINE_HOST}
    engine_port: "${ENGINE_GRPC_PORT}"
    node_name: ${NODE_NAME}
EOF
chmod 600 /etc/banyan/banyan.yaml

# 2. Init agent (generates WireGuard keypair, no --password)
echo "Initializing agent..."
banyan-agent init

# 3. Export agent public key for engine to whitelist
AGENT_PUB_KEY=$(grep 'wg_public_key' /etc/banyan/banyan.yaml | head -1 | awk '{print $2}')
echo "$AGENT_PUB_KEY" > /tmp/keys-exchange/${NODE_NAME}.pub
echo "Agent public key ($NODE_NAME): $AGENT_PUB_KEY"

# 4. Wait for engine to be ready (engine writes this file after gRPC is listening)
echo "Waiting for engine to be ready..."
while [ ! -f /tmp/keys-exchange/engine-ready ]; do
    sleep 1
done

# 5. Wait for engine gRPC to be reachable from this container
echo "Waiting for engine gRPC at ${ENGINE_HOST}:${ENGINE_GRPC_PORT}..."
until nc -z "${ENGINE_HOST}" "${ENGINE_GRPC_PORT}" 2>/dev/null; do
    sleep 1
done
echo "Engine is ready!"

# 6. Start containerd in background
echo "Starting containerd..."
containerd &
sleep 2

# 7. Start agent
echo "Starting agent..."
exec banyan-agent start --node-name "$NODE_NAME"
